package center

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/ipchronicle/ipchronicle/internal/center/database"
)

const (
	defaultListenAddress = ":8080"
	defaultDataDirectory = "/var/lib/ipchronicle"
)

type RuntimeConfig struct {
	ListenAddress  string
	DatabasePaths  database.Paths
	AdminUsername  string
	AdminPassword  string
	ExternalOrigin *url.URL
	TrustedProxies []netip.Prefix
}

func LoadRuntimeConfig() (RuntimeConfig, error) {
	paths, err := LoadDatabasePaths()
	if err != nil {
		return RuntimeConfig{}, err
	}

	externalOrigin, err := parseExternalOrigin(os.Getenv("IPCHRONICLE_EXTERNAL_URL"))
	if err != nil {
		return RuntimeConfig{}, err
	}
	trustedProxies, err := parseTrustedProxies(os.Getenv("IPCHRONICLE_TRUSTED_PROXIES"))
	if err != nil {
		return RuntimeConfig{}, err
	}
	return RuntimeConfig{
		ListenAddress:  environmentOrDefault("IPCHRONICLE_LISTEN_ADDRESS", defaultListenAddress),
		DatabasePaths:  paths,
		AdminUsername:  environmentOrDefault("IPCHRONICLE_ADMIN_USERNAME", "admin"),
		AdminPassword:  environmentOrDefault("IPCHRONICLE_ADMIN_PASSWORD", "admin"),
		ExternalOrigin: externalOrigin,
		TrustedProxies: trustedProxies,
	}, nil
}

func LoadDatabasePaths() (database.Paths, error) {
	dataDirectory := environmentOrDefault("IPCHRONICLE_DATA_DIR", defaultDataDirectory)
	paths := database.PathsFromDataDirectory(dataDirectory)
	paths.ConfigDatabase = environmentOrDefault("IPCHRONICLE_CONFIG_DATABASE_PATH", paths.ConfigDatabase)
	paths.HistoryDatabase = environmentOrDefault("IPCHRONICLE_HISTORY_DATABASE_PATH", paths.HistoryDatabase)
	paths.MasterKey = environmentOrDefault("IPCHRONICLE_MASTER_KEY_PATH", paths.MasterKey)
	for _, path := range []string{paths.ConfigDatabase, paths.HistoryDatabase, paths.MasterKey} {
		if !filepath.IsAbs(path) {
			return database.Paths{}, fmt.Errorf("persistent path must be absolute: %s", path)
		}
	}
	return paths, nil
}

func parseExternalOrigin(value string) (*url.URL, error) {
	if value == "" {
		return nil, nil
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return nil, fmt.Errorf("parse IPCHRONICLE_EXTERNAL_URL: %w", err)
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil ||
		(parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("IPCHRONICLE_EXTERNAL_URL must be an HTTP or HTTPS origin without credentials, path, query, or fragment")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Path = ""
	return parsed, nil
}

func parseTrustedProxies(value string) ([]netip.Prefix, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parts := strings.Split(value, ",")
	prefixes := make([]netip.Prefix, 0, len(parts))
	for _, part := range parts {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(part))
		if err != nil {
			return nil, fmt.Errorf("parse IPCHRONICLE_TRUSTED_PROXIES entry %q: %w", part, err)
		}
		prefixes = append(prefixes, prefix.Masked())
	}
	return prefixes, nil
}

func environmentOrDefault(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok && value != "" {
		return value
	}
	return fallback
}
