package systemsettings

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/ipchronicle/ipchronicle/internal/center/database/configdb"
)

const (
	maximumExternalOriginLength = 2048
	maximumIPAPIAPIKeyLength    = 256
)

var (
	ErrInvalidExternalOrigin = errors.New("external origin is invalid")
	ErrInvalidIPAPIAPIKey    = errors.New("ipapi API key update is invalid")
)

type Settings struct {
	ExternalOrigin        string
	IPAPIAPIKeyConfigured bool
}

type Update struct {
	ExternalOrigin    *string
	IPAPIAPIKeyAction string
	IPAPIAPIKey       *string
}

type ConfigurationWaker interface {
	Wake(nodeID string)
}

type Service struct {
	database  *sql.DB
	queries   *configdb.Queries
	masterKey [32]byte
	waker     ConfigurationWaker
}

func NewService(database *sql.DB, queries *configdb.Queries, masterKey [32]byte, waker ConfigurationWaker) *Service {
	if database == nil || queries == nil || waker == nil {
		panic("system settings dependencies must not be nil")
	}
	return &Service{database: database, queries: queries, masterKey: masterKey, waker: waker}
}

func (s *Service) Get(ctx context.Context) (Settings, error) {
	record, err := s.queries.GetSystemSettings(ctx)
	if err != nil {
		return Settings{}, err
	}
	return Settings{
		ExternalOrigin: record.ExternalOrigin, IPAPIAPIKeyConfigured: len(record.IpapiApiKeyEncrypted) != 0,
	}, nil
}

func (s *Service) IPAPIAPIKey(ctx context.Context) (string, error) {
	record, err := s.queries.GetSystemSettings(ctx)
	if err != nil || len(record.IpapiApiKeyEncrypted) == 0 {
		return "", err
	}
	return decryptIPAPIAPIKey(s.masterKey, record.IpapiApiKeyEncrypted)
}

func (s *Service) Update(ctx context.Context, input Update) (Settings, error) {
	var requestedExternalOrigin *string
	if input.ExternalOrigin != nil {
		externalOrigin, err := normalizeExternalOrigin(*input.ExternalOrigin)
		if err != nil {
			return Settings{}, err
		}
		requestedExternalOrigin = &externalOrigin
	}
	apiKey, err := normalizeIPAPIAPIKeyUpdate(input.IPAPIAPIKeyAction, input.IPAPIAPIKey)
	if err != nil {
		return Settings{}, err
	}

	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return Settings{}, err
	}
	defer transaction.Rollback()
	queries := s.queries.WithTx(transaction)
	record, err := queries.GetSystemSettings(ctx)
	if err != nil {
		return Settings{}, err
	}
	externalOrigin := record.ExternalOrigin
	if requestedExternalOrigin != nil {
		externalOrigin = *requestedExternalOrigin
		if _, err := queries.SetExternalOrigin(ctx, configdb.SetExternalOriginParams{
			ExternalOrigin: externalOrigin, ExternalOrigin_2: externalOrigin,
		}); err != nil {
			return Settings{}, err
		}
	}

	encrypted := record.IpapiApiKeyEncrypted
	keyChanged := false
	switch input.IPAPIAPIKeyAction {
	case "keep":
	case "clear":
		keyChanged = len(encrypted) != 0
		encrypted = nil
	case "replace":
		current := ""
		if len(encrypted) != 0 {
			current, err = decryptIPAPIAPIKey(s.masterKey, encrypted)
			if err != nil {
				return Settings{}, err
			}
		}
		keyChanged = current != apiKey
		if keyChanged {
			encrypted, err = encryptIPAPIAPIKey(s.masterKey, apiKey)
			if err != nil {
				return Settings{}, err
			}
		}
	}

	var affectedNodeIDs []string
	if keyChanged {
		if err := queries.SetIPAPIAPIKey(ctx, encrypted); err != nil {
			return Settings{}, err
		}
		affectedNodeIDs, err = queries.AdvanceAllNodeConfigurationRevisions(ctx)
		if err != nil {
			return Settings{}, err
		}
	}
	if err := transaction.Commit(); err != nil {
		return Settings{}, err
	}
	for _, nodeID := range affectedNodeIDs {
		s.waker.Wake(nodeID)
	}
	return Settings{ExternalOrigin: externalOrigin, IPAPIAPIKeyConfigured: len(encrypted) != 0}, nil
}

func (s *Service) EffectiveOrigin(ctx context.Context, requestOrigin string) (string, error) {
	externalOrigin, err := s.queries.GetExternalOrigin(ctx)
	if err != nil {
		return "", err
	}
	if externalOrigin != "" {
		return externalOrigin, nil
	}
	return requestOrigin, nil
}

func normalizeIPAPIAPIKeyUpdate(action string, value *string) (string, error) {
	switch action {
	case "keep", "clear":
		if value != nil {
			return "", ErrInvalidIPAPIAPIKey
		}
		return "", nil
	case "replace":
		if value == nil {
			return "", ErrInvalidIPAPIAPIKey
		}
		normalized := strings.TrimSpace(*value)
		if !utf8.ValidString(normalized) || len(normalized) < 1 || len(normalized) > maximumIPAPIAPIKeyLength ||
			strings.ContainsAny(normalized, "\x00\r\n\t") {
			return "", ErrInvalidIPAPIAPIKey
		}
		return normalized, nil
	default:
		return "", ErrInvalidIPAPIAPIKey
	}
}

func normalizeExternalOrigin(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if len(value) > maximumExternalOriginLength {
		return "", ErrInvalidExternalOrigin
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Host == "" || parsed.Hostname() == "" || parsed.User != nil ||
		(parsed.Path != "" && parsed.Path != "/") || parsed.RawPath != "" ||
		parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" {
		return "", ErrInvalidExternalOrigin
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Path = ""
	normalized := parsed.String()
	if len(normalized) > maximumExternalOriginLength {
		return "", ErrInvalidExternalOrigin
	}
	return normalized, nil
}
