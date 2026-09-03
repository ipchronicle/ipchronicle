package center

import (
	"path/filepath"
	"testing"
)

func TestLoadRuntimeConfigDefaults(t *testing.T) {
	clearRuntimeEnvironment(t)

	configuration, err := LoadRuntimeConfig()
	if err != nil {
		t.Fatal(err)
	}
	if configuration.ListenAddress != defaultListenAddress || configuration.AdminUsername != "admin" || configuration.AdminPassword != "admin" {
		t.Fatalf("unexpected defaults: %#v", configuration)
	}
	if configuration.DatabasePaths.ConfigDatabase != "/var/lib/ipchronicle/config/config.db" ||
		configuration.DatabasePaths.HistoryDatabase != "/var/lib/ipchronicle/history/history.db" ||
		configuration.DatabasePaths.MasterKey != "/var/lib/ipchronicle/config/master.key" {
		t.Fatalf("unexpected default paths: %#v", configuration.DatabasePaths)
	}
}

func TestLoadRuntimeConfigOverrides(t *testing.T) {
	clearRuntimeEnvironment(t)
	dataDirectory := t.TempDir()
	configDatabase := filepath.Join(t.TempDir(), "configuration.db")
	t.Setenv("IPCHRONICLE_DATA_DIR", dataDirectory)
	t.Setenv("IPCHRONICLE_CONFIG_DATABASE_PATH", configDatabase)
	t.Setenv("IPCHRONICLE_LISTEN_ADDRESS", "127.0.0.1:9090")
	t.Setenv("IPCHRONICLE_ADMIN_USERNAME", "owner")
	t.Setenv("IPCHRONICLE_ADMIN_PASSWORD", "bootstrap-password")

	configuration, err := LoadRuntimeConfig()
	if err != nil {
		t.Fatal(err)
	}
	if configuration.ListenAddress != "127.0.0.1:9090" || configuration.AdminUsername != "owner" || configuration.AdminPassword != "bootstrap-password" {
		t.Fatalf("unexpected overrides: %#v", configuration)
	}
	if configuration.DatabasePaths.ConfigDatabase != configDatabase ||
		configuration.DatabasePaths.HistoryDatabase != filepath.Join(dataDirectory, "history", "history.db") ||
		configuration.DatabasePaths.MasterKey != filepath.Join(dataDirectory, "config", "master.key") {
		t.Fatalf("unexpected overridden paths: %#v", configuration.DatabasePaths)
	}
}

func TestLoadRuntimeConfigRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{name: "relative persistent path", key: "IPCHRONICLE_MASTER_KEY_PATH", value: "master.key"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clearRuntimeEnvironment(t)
			t.Setenv(test.key, test.value)
			if _, err := LoadRuntimeConfig(); err == nil {
				t.Fatalf("LoadRuntimeConfig accepted %s=%q", test.key, test.value)
			}
		})
	}
}

func clearRuntimeEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"IPCHRONICLE_DATA_DIR",
		"IPCHRONICLE_CONFIG_DATABASE_PATH",
		"IPCHRONICLE_HISTORY_DATABASE_PATH",
		"IPCHRONICLE_MASTER_KEY_PATH",
		"IPCHRONICLE_LISTEN_ADDRESS",
		"IPCHRONICLE_ADMIN_USERNAME",
		"IPCHRONICLE_ADMIN_PASSWORD",
	} {
		t.Setenv(name, "")
	}
}
