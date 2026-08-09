package nodes

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/center/database/configdb"
)

const maxNetworkProxies = 64

var (
	ErrInvalidNetworkProxy       = errors.New("network proxy is invalid")
	ErrNetworkProxyNotFound      = errors.New("network proxy does not exist")
	ErrNetworkProxyAlreadyExists = errors.New("network proxy name already exists")
	ErrNetworkProxyLimitReached  = errors.New("network proxy limit reached")
	ErrNetworkProxyInUse         = errors.New("network proxy is referenced by a network egress")
)

type NetworkProxy struct {
	ID                 uuid.UUID
	Name               string
	Scheme             string
	Host               string
	Port               int64
	Username           *string
	PasswordConfigured bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type NetworkProxyCreate struct {
	Name     string
	Scheme   string
	Host     string
	Port     int64
	Username *string
	Password *string
}

type NetworkProxyUpdate struct {
	Name           string
	Scheme         string
	Host           string
	Port           int64
	Username       *string
	PasswordAction string
	Password       *string
}

type AgentProxyConfiguration struct {
	ID       uuid.UUID
	Scheme   string
	Host     string
	Port     int64
	Username *string
	Password *string
}

func (s *Service) ListNetworkProxies(ctx context.Context) ([]NetworkProxy, error) {
	records, err := s.queries.ListNetworkProxies(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]NetworkProxy, 0, len(records))
	for _, record := range records {
		proxy, err := networkProxyFromRecord(record)
		if err != nil {
			return nil, err
		}
		result = append(result, proxy)
	}
	return result, nil
}

func (s *Service) CreateNetworkProxy(ctx context.Context, input NetworkProxyCreate) (NetworkProxy, error) {
	normalized, err := normalizeNetworkProxy(input.Name, input.Scheme, input.Host, input.Port, input.Username)
	if err != nil || !validProxyPassword(input.Password) {
		return NetworkProxy{}, ErrInvalidNetworkProxy
	}
	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return NetworkProxy{}, err
	}
	defer transaction.Rollback()
	queries := s.queries.WithTx(transaction)
	if _, err := queries.GetNetworkProxyByName(ctx, normalized.Name); err == nil {
		return NetworkProxy{}, ErrNetworkProxyAlreadyExists
	} else if !errors.Is(err, sql.ErrNoRows) {
		return NetworkProxy{}, err
	}
	count, err := queries.CountNetworkProxies(ctx)
	if err != nil {
		return NetworkProxy{}, err
	}
	if count >= maxNetworkProxies {
		return NetworkProxy{}, ErrNetworkProxyLimitReached
	}
	id := uuid.New()
	var encrypted []byte
	if input.Password != nil {
		encrypted, err = encryptProxyPassword(s.masterKey, id.String(), *input.Password)
		if err != nil {
			return NetworkProxy{}, err
		}
	}
	now := s.now().UTC().Truncate(time.Second).Unix()
	if err := queries.CreateNetworkProxy(ctx, configdb.CreateNetworkProxyParams{
		ID: id.String(), Name: normalized.Name, Scheme: normalized.Scheme,
		Host: normalized.Host, Port: normalized.Port, Username: normalized.Username,
		PasswordEncrypted: encrypted, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		return NetworkProxy{}, err
	}
	record, err := queries.GetNetworkProxy(ctx, id.String())
	if err != nil {
		return NetworkProxy{}, err
	}
	if err := transaction.Commit(); err != nil {
		return NetworkProxy{}, err
	}
	return networkProxyFromRecord(record)
}

func (s *Service) UpdateNetworkProxy(ctx context.Context, id uuid.UUID, input NetworkProxyUpdate) (NetworkProxy, error) {
	normalized, err := normalizeNetworkProxy(input.Name, input.Scheme, input.Host, input.Port, input.Username)
	if err != nil || (input.PasswordAction != "keep" && input.PasswordAction != "replace" && input.PasswordAction != "clear") ||
		(input.PasswordAction == "replace") != (input.Password != nil) || !validProxyPassword(input.Password) {
		return NetworkProxy{}, ErrInvalidNetworkProxy
	}
	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return NetworkProxy{}, err
	}
	defer transaction.Rollback()
	queries := s.queries.WithTx(transaction)
	record, err := queries.GetNetworkProxy(ctx, id.String())
	if errors.Is(err, sql.ErrNoRows) {
		return NetworkProxy{}, ErrNetworkProxyNotFound
	}
	if err != nil {
		return NetworkProxy{}, err
	}
	if sameName, lookupErr := queries.GetNetworkProxyByName(ctx, normalized.Name); lookupErr == nil && sameName.ID != id.String() {
		return NetworkProxy{}, ErrNetworkProxyAlreadyExists
	} else if lookupErr != nil && !errors.Is(lookupErr, sql.ErrNoRows) {
		return NetworkProxy{}, lookupErr
	}
	encrypted := record.PasswordEncrypted
	switch input.PasswordAction {
	case "replace":
		encrypted, err = encryptProxyPassword(s.masterKey, id.String(), *input.Password)
		if err != nil {
			return NetworkProxy{}, err
		}
	case "clear":
		encrypted = nil
	}
	passwordChanged := input.PasswordAction == "replace" || (input.PasswordAction == "clear" && len(record.PasswordEncrypted) != 0)
	changed := record.Name != normalized.Name || record.Scheme != normalized.Scheme ||
		record.Host != normalized.Host || record.Port != normalized.Port ||
		!equalOptionalString(record.Username, normalized.Username) || passwordChanged
	var affectedNodeIDs []string
	if changed {
		now := s.now().UTC().Truncate(time.Second).Unix()
		updatedRows, err := queries.UpdateNetworkProxy(ctx, configdb.UpdateNetworkProxyParams{
			Name: normalized.Name, Scheme: normalized.Scheme, Host: normalized.Host,
			Port: normalized.Port, Username: normalized.Username, PasswordEncrypted: encrypted,
			UpdatedAt: now, ID: id.String(),
		})
		if err != nil {
			return NetworkProxy{}, err
		}
		if updatedRows != 1 {
			return NetworkProxy{}, ErrNetworkProxyNotFound
		}
		affectedNodeIDs, err = queries.ListNodeIDsReferencingNetworkProxy(ctx, &record.ID)
		if err != nil {
			return NetworkProxy{}, err
		}
		for _, nodeID := range affectedNodeIDs {
			if err := incrementNodeConfiguration(ctx, queries, nodeID); err != nil {
				return NetworkProxy{}, err
			}
		}
	}
	updated, err := queries.GetNetworkProxy(ctx, id.String())
	if err != nil {
		return NetworkProxy{}, err
	}
	if err := transaction.Commit(); err != nil {
		return NetworkProxy{}, err
	}
	for _, nodeID := range affectedNodeIDs {
		s.sync.Wake(nodeID)
	}
	return networkProxyFromRecord(updated)
}

func (s *Service) DeleteNetworkProxy(ctx context.Context, id uuid.UUID) error {
	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	queries := s.queries.WithTx(transaction)
	if _, err := queries.GetNetworkProxy(ctx, id.String()); errors.Is(err, sql.ErrNoRows) {
		return ErrNetworkProxyNotFound
	} else if err != nil {
		return err
	}
	count, err := queries.CountNetworkProxyReferences(ctx, stringPointer(id.String()))
	if err != nil {
		return err
	}
	if count != 0 {
		return ErrNetworkProxyInUse
	}
	changed, err := queries.DeleteNetworkProxy(ctx, id.String())
	if err != nil {
		return err
	}
	if changed != 1 {
		return ErrNetworkProxyNotFound
	}
	return transaction.Commit()
}

func agentProxyFromRecord(masterKey [32]byte, record configdb.NetworkProxy) (AgentProxyConfiguration, error) {
	id, err := uuid.Parse(record.ID)
	if err != nil {
		return AgentProxyConfiguration{}, fmt.Errorf("parse stored network proxy ID %q: %w", record.ID, err)
	}
	result := AgentProxyConfiguration{
		ID: id, Scheme: record.Scheme, Host: record.Host, Port: record.Port, Username: record.Username,
	}
	if len(record.PasswordEncrypted) != 0 {
		password, err := decryptProxyPassword(masterKey, record.ID, record.PasswordEncrypted)
		if err != nil {
			return AgentProxyConfiguration{}, fmt.Errorf("read stored proxy credential for %s: %w", record.ID, err)
		}
		result.Password = &password
	}
	return result, nil
}

func networkProxyFromRecord(record configdb.NetworkProxy) (NetworkProxy, error) {
	id, err := uuid.Parse(record.ID)
	if err != nil {
		return NetworkProxy{}, fmt.Errorf("parse stored network proxy ID %q: %w", record.ID, err)
	}
	return NetworkProxy{
		ID: id, Name: record.Name, Scheme: record.Scheme, Host: record.Host, Port: record.Port,
		Username: record.Username, PasswordConfigured: len(record.PasswordEncrypted) != 0,
		CreatedAt: time.Unix(record.CreatedAt, 0).UTC(), UpdatedAt: time.Unix(record.UpdatedAt, 0).UTC(),
	}, nil
}

func normalizeNetworkProxy(name, scheme, host string, port int64, username *string) (NetworkProxyCreate, error) {
	name = strings.TrimSpace(name)
	scheme = strings.ToLower(strings.TrimSpace(scheme))
	host = strings.TrimSpace(host)
	if !validBoundedText(name, 128) || (scheme != "http" && scheme != "https" && scheme != "socks5") ||
		!validProxyHost(host) || port < 1 || port > 65535 {
		return NetworkProxyCreate{}, ErrInvalidNetworkProxy
	}
	if username != nil {
		value := *username
		if value == "" {
			username = nil
		} else if !utf8.ValidString(value) || utf8.RuneCountInString(value) > 512 || strings.ContainsRune(value, '\x00') {
			return NetworkProxyCreate{}, ErrInvalidNetworkProxy
		} else {
			username = &value
		}
	}
	return NetworkProxyCreate{Name: name, Scheme: scheme, Host: host, Port: port, Username: username}, nil
}

func validProxyHost(host string) bool {
	if address, err := netip.ParseAddr(host); err == nil {
		return address == address.Unmap() && !address.IsUnspecified() && !address.IsMulticast()
	}
	if len(host) > 253 || strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") || strings.Contains(host, "..") {
		return false
	}
	for _, character := range host {
		if !((character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '-' || character == '_' || character == '.') {
			return false
		}
	}
	return host != ""
}

func validProxyPassword(password *string) bool {
	return password == nil || (utf8.ValidString(*password) && len(*password) >= 1 && len(*password) <= 4096 && !strings.ContainsRune(*password, '\x00'))
}

func equalOptionalString(left, right *string) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && *left == *right)
}
