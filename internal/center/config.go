package center

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ipchronicle/ipchronicle/internal/center/database"
)

const (
	defaultListenAddress = ":8080"
	defaultDataDirectory = "/var/lib/ipchronicle"
)

type RuntimeConfig struct {
	ListenAddress string
	DatabasePaths database.Paths
	AdminUsername string
	AdminPassword string
}

func LoadRuntimeConfig() (RuntimeConfig, error) {
	paths, err := LoadDatabasePaths()
	if err != nil {
		return RuntimeConfig{}, err
	}

	return RuntimeConfig{
		ListenAddress: environmentOrDefault("IPCHRONICLE_LISTEN_ADDRESS", defaultListenAddress),
		DatabasePaths: paths,
		AdminUsername: environmentOrDefault("IPCHRONICLE_ADMIN_USERNAME", "admin"),
		AdminPassword: environmentOrDefault("IPCHRONICLE_ADMIN_PASSWORD", "admin"),
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

func environmentOrDefault(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok && value != "" {
		return value
	}
	return fallback
}
