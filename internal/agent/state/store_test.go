package state

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
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

func TestBroadStateDirectoryPermissionsFail(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "agent")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(directory); err == nil {
		t.Fatal("opening a group-readable Agent state directory unexpectedly succeeded")
	}
}
