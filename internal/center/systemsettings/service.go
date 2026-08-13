package systemsettings

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/ipchronicle/ipchronicle/internal/center/database/configdb"
)

const maximumExternalOriginLength = 2048

var ErrInvalidExternalOrigin = errors.New("external origin is invalid")

type Settings struct {
	ExternalOrigin string
}

type Service struct {
	queries *configdb.Queries
}

func NewService(queries *configdb.Queries) *Service {
	if queries == nil {
		panic("system settings queries must not be nil")
	}
	return &Service{queries: queries}
}

func (s *Service) Get(ctx context.Context) (Settings, error) {
	externalOrigin, err := s.queries.GetExternalOrigin(ctx)
	if err != nil {
		return Settings{}, err
	}
	return Settings{ExternalOrigin: externalOrigin}, nil
}

func (s *Service) Update(ctx context.Context, externalOrigin string) (Settings, error) {
	normalized, err := normalizeExternalOrigin(externalOrigin)
	if err != nil {
		return Settings{}, err
	}
	if _, err := s.queries.SetExternalOrigin(ctx, configdb.SetExternalOriginParams{
		ExternalOrigin:   normalized,
		ExternalOrigin_2: normalized,
	}); err != nil {
		return Settings{}, err
	}
	return Settings{ExternalOrigin: normalized}, nil
}

func (s *Service) EffectiveOrigin(ctx context.Context, requestOrigin string) (string, error) {
	settings, err := s.Get(ctx)
	if err != nil {
		return "", err
	}
	if settings.ExternalOrigin != "" {
		return settings.ExternalOrigin, nil
	}
	return requestOrigin, nil
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
