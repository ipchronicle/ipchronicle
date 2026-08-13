package center

import (
	"net/netip"
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
	if len(configuration.TrustedProxies) != 0 {
		t.Fatalf("unexpected default network configuration: %#v", configuration)
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
	t.Setenv("IPCHRONICLE_TRUSTED_PROXIES", "10.1.2.3/8, 2001:db8::1/48")

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
	wantProxies := []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("2001:db8::/48"),
	}
	if len(configuration.TrustedProxies) != len(wantProxies) {
		t.Fatalf("trusted proxies = %v", configuration.TrustedProxies)
	}
	for index := range wantProxies {
		if configuration.TrustedProxies[index] != wantProxies[index] {
			t.Fatalf("trusted proxies = %v, want %v", configuration.TrustedProxies, wantProxies)
		}
	}
}

func TestLoadRuntimeConfigRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{name: "relative persistent path", key: "IPCHRONICLE_MASTER_KEY_PATH", value: "master.key"},
		{name: "invalid trusted proxy", key: "IPCHRONICLE_TRUSTED_PROXIES", value: "192.0.2.1"},
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
		"IPCHRONICLE_TRUSTED_PROXIES",
	} {
		t.Setenv(name, "")
	}
}
