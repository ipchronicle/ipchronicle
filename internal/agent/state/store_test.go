package state

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestIdentityPersistsEncryptedAcrossRestart(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "agent")
	store, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Identity(); !errors.Is(err, ErrNotEnrolled) {
		t.Fatalf("empty identity error = %v", err)
	}
	identity := Identity{
		CenterURL: "https://center.example", NodeID: "7289cfa3-a75d-4a3f-ac06-8f1074446a85",
		Credential: "ipc_agent_secret-credential", AppliedConfigurationRevision: 0,
	}
	if err := store.SaveIdentity(identity); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	databaseBody, err := os.ReadFile(filepath.Join(directory, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(databaseBody, []byte(identity.Credential)) {
		t.Fatal("Agent credential is stored as plaintext in bbolt")
	}
	for _, name := range []string{"state.db", "master.key"} {
		info, err := os.Stat(filepath.Join(directory, name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode = %o, want 600", name, info.Mode().Perm())
		}
	}

	restarted, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	actual, err := restarted.Identity()
	if err != nil || actual != identity {
		t.Fatalf("restarted identity = %#v, %v", actual, err)
	}
	if err := restarted.SaveIdentity(Identity{
		CenterURL: identity.CenterURL, NodeID: "other-node", Credential: "other", AppliedConfigurationRevision: 0,
	}); err == nil {
		t.Fatal("overwriting an existing identity unexpectedly succeeded")
	}
}

func TestMissingMasterKeyFailsWithExistingState(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "agent")
	store, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(directory, "master.key")); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(directory); err == nil {
		t.Fatal("opening Agent state without its master key unexpectedly succeeded")
	}
}

func TestProxyCredentialsPersistEncryptedAcrossRestart(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "agent")
	store, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveIdentity(Identity{
		CenterURL: "https://center.example", NodeID: "7289cfa3-a75d-4a3f-ac06-8f1074446a85",
		Credential: "ipc_agent_secret-credential",
	}); err != nil {
		t.Fatal(err)
	}
	proxyID := "6fc6d7e8-bc63-49e2-91fc-d4c58b43ac16"
	password := "retained-proxy-password"
	configuration := Configuration{
		SchemaVersion: 5, Revision: 1, Enabled: true, DiscoveryServices: testDiscoveryServices(),
		ProbeSchedule:     ProbeSchedule{Enabled: true, Cron: "0 0 0 * * *", Timezone: "agent-local"},
		HistoryGeneration: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Egresses: []Egress{{
			ID: "c7b5eeac-903d-4b99-961d-190a8a4e5d2e", Kind: "proxy", Family: "ipv4",
			ProxyID: &proxyID, Enabled: true, LightweightIntervalSeconds: 600, ProbeOnAddressChange: true,
		}},
		Proxies: []Proxy{{
			ID: proxyID, Scheme: "socks5", Host: "proxy.example.test", Port: 1080, Password: &password,
		}},
	}
	if err := store.ApplyConfiguration(configuration); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	databaseBody, err := os.ReadFile(filepath.Join(directory, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(databaseBody, []byte(password)) {
		t.Fatal("retained proxy password is stored as plaintext in bbolt")
	}
	restarted, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	actual, err := restarted.Configuration()
	if err != nil || !reflect.DeepEqual(actual, configuration) {
		t.Fatalf("restarted proxy configuration = %#v, %v", actual, err)
	}
}

func TestBroadStateDirectoryPermissionsFail(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "agent")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(directory); err == nil {
		t.Fatal("opening a group-readable Agent state directory unexpectedly succeeded")
	}
}

func TestConfigurationReplacementFailureAndRevocationPersist(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "agent")
	store, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	identity := Identity{
		CenterURL: "https://center.example", NodeID: "7289cfa3-a75d-4a3f-ac06-8f1074446a85",
		Credential: "ipc_agent_secret-credential", AppliedConfigurationRevision: 0,
	}
	if err := store.SaveIdentity(identity); err != nil {
		t.Fatal(err)
	}
	first := Configuration{
		SchemaVersion: 5, Revision: 1, Enabled: true, DiscoveryServices: testDiscoveryServices(),
		ProbeSchedule:     ProbeSchedule{Enabled: true, Cron: "0 0 0 * * *", Timezone: "agent-local"},
		HistoryGeneration: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	}
	if err := store.ApplyConfiguration(first); err != nil {
		t.Fatal(err)
	}
	if err := store.ApplyConfiguration(first); err == nil {
		t.Fatal("same configuration revision unexpectedly replaced the current snapshot")
	}
	if err := store.RecordConfigurationFailure(2, errors.New("invalid revision two")); err != nil {
		t.Fatal(err)
	}
	control, err := store.ControlState()
	if err != nil || control.AppliedConfigurationRevision != 1 || control.ConfigurationErrorRevision == nil || *control.ConfigurationErrorRevision != 2 {
		t.Fatalf("control state = %#v, %v", control, err)
	}
	current, err := store.Configuration()
	if err != nil || !reflect.DeepEqual(current, first) {
		t.Fatalf("configuration after failure = %#v, %v", current, err)
	}
	invalid := first
	invalid.Revision = 2
	invalid.HistoryGeneration = "invalid"
	if err := store.ApplyConfiguration(invalid); err == nil {
		t.Fatal("invalid configuration unexpectedly replaced the current snapshot")
	}
	second := first
	second.Revision = 2
	second.Enabled = false
	if err := store.ApplyConfiguration(second); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkRevoked(); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	restarted, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	control, err = restarted.ControlState()
	if err != nil || control.AppliedConfigurationRevision != 2 || control.ConfigurationError != nil || !control.Revoked {
		t.Fatalf("restarted control state = %#v, %v", control, err)
	}
	current, err = restarted.Configuration()
	if err != nil || !reflect.DeepEqual(current, second) {
		t.Fatalf("restarted configuration = %#v, %v", current, err)
	}
}

func testDiscoveryServices() DiscoveryServices {
	return DiscoveryServices{
		IPv4: []string{"https://one.example/ip", "https://two.example/ip"},
		IPv6: []string{"https://six-one.example/ip", "https://six-two.example/ip"},
	}
}
