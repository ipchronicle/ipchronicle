package database

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ipchronicle/ipchronicle/internal/center/database/configdb"
	_ "github.com/mattn/go-sqlite3"
	"github.com/pressly/goose/v3"
)

func TestFreshOpenAndRestart(t *testing.T) {
	paths := PathsFromDataDirectory(t.TempDir())
	store, err := Open(context.Background(), paths)
	if err != nil {
		t.Fatal(err)
	}
	firstGeneration := store.HistoryGeneration
	if store.ConfigSchemaVersion != configSchemaVersion || store.HistorySchemaVersion != historySchemaVersion {
		t.Fatalf("unexpected schema versions: %d/%d", store.ConfigSchemaVersion, store.HistorySchemaVersion)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	keyInfo, err := os.Stat(paths.MasterKey)
	if err != nil {
		t.Fatal(err)
	}
	if keyInfo.Mode().Perm() != 0o600 {
		t.Fatalf("master key mode = %o, want 600", keyInfo.Mode().Perm())
	}

	restarted, err := Open(context.Background(), paths)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if restarted.HistoryGeneration != firstGeneration {
		t.Fatalf("history generation changed across restart: %s != %s", restarted.HistoryGeneration, firstGeneration)
	}
}

func TestMissingMasterKeyFailsWhenDatabaseExists(t *testing.T) {
	paths := PathsFromDataDirectory(t.TempDir())
	store, err := Open(context.Background(), paths)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(paths.MasterKey); err != nil {
		t.Fatal(err)
	}
	_, err = Open(context.Background(), paths)
	if err == nil || !strings.Contains(err.Error(), "master key is missing") {
		t.Fatalf("error = %v, want explicit missing master key failure", err)
	}
}

func TestVersionFiveNetworkEgressesMigrateToCurrentVersion(t *testing.T) {
	ctx := context.Background()
	paths := PathsFromDataDirectory(t.TempDir())
	if err := prepareDirectories(paths); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.MasterKey, make([]byte, MasterKeySize), 0o600); err != nil {
		t.Fatal(err)
	}
	configDatabase, err := openSQLite(ctx, paths.ConfigDatabase)
	if err != nil {
		t.Fatal(err)
	}
	migrationMu.Lock()
	goose.SetBaseFS(migrationFiles)
	err = goose.SetDialect("sqlite3")
	if err == nil {
		err = goose.UpToContext(ctx, configDatabase, "migrations/config", 5)
	}
	migrationMu.Unlock()
	if err != nil {
		_ = configDatabase.Close()
		t.Fatal(err)
	}
	nodeID := "7289cfa3-a75d-4a3f-ac06-8f1074446a85"
	egressID := "6fc6d7e8-bc63-49e2-91fc-d4c58b43ac16"
	if _, err := configDatabase.ExecContext(ctx, `
		INSERT INTO nodes (
			id, name, hostname, credential_digest, agent_version,
			operating_system, architecture, desired_configuration_revision, registered_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?)
	`, nodeID, "edge-1", "edge-1", make([]byte, 32), "test", "linux", "amd64", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := configDatabase.ExecContext(ctx, `
		INSERT INTO network_egresses (
			id, node_id, name, kind, family, interface_name, source_address,
			enabled, available, automatic, lightweight_interval_seconds,
			probe_on_address_change, created_at, updated_at
		) VALUES (?, ?, ?, 'source', 'ipv4', 'eth0', '10.0.0.5', 1, 1, 0, 600, 1, 1, 1)
	`, egressID, nodeID, "source:10.0.0.5"); err != nil {
		t.Fatal(err)
	}
	if err := configDatabase.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(ctx, paths)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if store.ConfigSchemaVersion != configSchemaVersion {
		t.Fatalf("configuration schema = %d, want %d", store.ConfigSchemaVersion, configSchemaVersion)
	}
	egress, err := store.ConfigQueries.GetNodeEgress(ctx, configdb.GetNodeEgressParams{NodeID: nodeID, ID: egressID})
	if err != nil {
		t.Fatal(err)
	}
	if egress.Kind != "source" || egress.InterfaceName == nil || *egress.InterfaceName != "eth0" ||
		egress.SourceAddress == nil || *egress.SourceAddress != "10.0.0.5" || egress.ProxyID != nil {
		t.Fatalf("migrated egress = %#v", egress)
	}
}

func TestVersionElevenProbeTasksMigrateToSharedAgentTaskSlot(t *testing.T) {
	ctx := context.Background()
	paths := PathsFromDataDirectory(t.TempDir())
	if err := prepareDirectories(paths); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.MasterKey, make([]byte, MasterKeySize), 0o600); err != nil {
		t.Fatal(err)
	}
	database, err := openSQLite(ctx, paths.ConfigDatabase)
	if err != nil {
		t.Fatal(err)
	}
	migrationMu.Lock()
	goose.SetBaseFS(migrationFiles)
	err = goose.SetDialect("sqlite3")
	if err == nil {
		err = goose.UpToContext(ctx, database, "migrations/config", 11)
	}
	migrationMu.Unlock()
	if err != nil {
		_ = database.Close()
		t.Fatal(err)
	}
	nodeID := "7289cfa3-a75d-4a3f-ac06-8f1074446a85"
	activeTaskID := "6fc6d7e8-bc63-49e2-91fc-d4c58b43ac16"
	terminalTaskID := "e03f9d0a-5303-4f0f-91cc-ebf531436fc1"
	if _, err := database.ExecContext(ctx, `
		INSERT INTO nodes (
			id, name, hostname, credential_digest, agent_version,
			operating_system, architecture, desired_configuration_revision, registered_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?)
	`, nodeID, "edge-1", "edge-1", make([]byte, 32), "0.1.0", "linux", "amd64", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO probe_tasks (
			id, node_id, kind, status, created_at, expires_at, acknowledged_at
		) VALUES (?, ?, 'complete-probe', 'acknowledged', 10, 130, 20)
	`, activeTaskID, nodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO probe_tasks (
			id, node_id, kind, status, created_at, expires_at,
			acknowledged_at, started_at, completed_at, run_id, terminal_confirmed_at
		) VALUES (?, ?, 'complete-probe', 'succeeded', 1, 9, 2, 3, 8, ?, 8)
	`, terminalTaskID, nodeID, terminalTaskID); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(ctx, paths)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if store.ConfigSchemaVersion != configSchemaVersion {
		t.Fatalf("configuration schema = %d, want %d", store.ConfigSchemaVersion, configSchemaVersion)
	}
	active, err := store.ConfigQueries.GetActiveNodeTask(ctx, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if active.ID != activeTaskID || active.Kind != "complete-probe" || active.Status != "acknowledged" || active.TargetVersion != nil {
		t.Fatalf("migrated active task = %#v", active)
	}
	terminal, err := store.ConfigQueries.GetProbeTask(ctx, configdb.GetProbeTaskParams{ID: terminalTaskID, NodeID: nodeID})
	if err != nil {
		t.Fatal(err)
	}
	if terminal.Status != "succeeded" || terminal.RunID == nil || *terminal.RunID != terminalTaskID || terminal.TargetVersion != nil {
		t.Fatalf("migrated terminal task = %#v", terminal)
	}
}

func TestMasterKeyWithBroadPermissionsFails(t *testing.T) {
	paths := PathsFromDataDirectory(t.TempDir())
	store, err := Open(context.Background(), paths)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(paths.MasterKey, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = Open(context.Background(), paths)
	if err == nil || !strings.Contains(err.Error(), "allow group or other access") {
		t.Fatalf("error = %v, want explicit master key permissions failure", err)
	}
}

func TestDeletedHistoryAdvancesGeneration(t *testing.T) {
	paths := PathsFromDataDirectory(t.TempDir())
	store, err := Open(context.Background(), paths)
	if err != nil {
		t.Fatal(err)
	}
	firstGeneration := store.HistoryGeneration
	if _, err := store.Config.ExecContext(context.Background(), `
		INSERT INTO nodes (
			id, name, hostname, credential_digest, agent_version,
			operating_system, architecture, desired_configuration_revision, registered_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?)
	`, "7289cfa3-a75d-4a3f-ac06-8f1074446a85", "edge-1", "edge-1", make([]byte, 32), "test", "linux", "amd64", 1); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	removeSQLiteFiles(t, paths.HistoryDatabase)

	restarted, err := Open(context.Background(), paths)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if restarted.HistoryGeneration == firstGeneration {
		t.Fatal("history generation did not advance after deliberate history removal")
	}
	state, err := restarted.ConfigQueries.GetSystemState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.PendingHistoryGeneration != nil || state.HistoryGeneration != restarted.HistoryGeneration || state.HistoryResetAt == nil {
		t.Fatalf("history reset was not fully reconciled: %#v", state)
	}
	node, err := restarted.ConfigQueries.GetNodeByID(context.Background(), "7289cfa3-a75d-4a3f-ac06-8f1074446a85")
	if err != nil || node.DesiredConfigurationRevision != 2 {
		t.Fatalf("history reset did not advance node configuration: %#v, %v", node, err)
	}
}

func TestCorruptHistoryFailsExplicitly(t *testing.T) {
	paths := PathsFromDataDirectory(t.TempDir())
	store, err := Open(context.Background(), paths)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.HistoryDatabase, []byte("not a SQLite database"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = Open(context.Background(), paths)
	if err == nil || !strings.Contains(err.Error(), "history database") {
		t.Fatalf("error = %v, want explicit corrupt history failure", err)
	}
}

func TestStartupContinuesAfterConfigMigrationOnly(t *testing.T) {
	paths := PathsFromDataDirectory(t.TempDir())
	if err := prepareDirectories(paths); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, MasterKeySize)
	if err := os.WriteFile(paths.MasterKey, key, 0o600); err != nil {
		t.Fatal(err)
	}
	configDatabase, err := openSQLite(context.Background(), paths.ConfigDatabase)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := migrate(context.Background(), configDatabase, "migrations/config", configSchemaVersion); err != nil {
		t.Fatal(err)
	}
	if err := configDatabase.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(context.Background(), paths)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if store.HistoryGeneration == "" {
		t.Fatal("history generation was not initialized after interrupted startup")
	}
}

func TestNewerSchemaIsRejected(t *testing.T) {
	paths := PathsFromDataDirectory(t.TempDir())
	store, err := Open(context.Background(), paths)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	database, err := sql.Open("sqlite3", paths.ConfigDatabase)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec("INSERT INTO goose_db_version (version_id, is_applied, tstamp) VALUES (99, 1, CURRENT_TIMESTAMP)"); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = Open(context.Background(), paths)
	if err == nil || !strings.Contains(err.Error(), "newer than supported") {
		t.Fatalf("error = %v, want newer schema rejection", err)
	}
}

func removeSQLiteFiles(t *testing.T, databasePath string) {
	t.Helper()
	for _, suffix := range []string{"", "-wal", "-shm"} {
		err := os.Remove(databasePath + suffix)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(filepath.Dir(databasePath))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), filepath.Base(databasePath)+"-") {
			t.Fatalf("unexpected SQLite sidecar remains: %s", entry.Name())
		}
	}
}
