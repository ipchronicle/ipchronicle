package database

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	if store.ConfigSchemaVersion != configSchemaVersion || store.HistorySchemaVersion != historySchemaVersion ||
		store.LogsSchemaVersion != logsSchemaVersion {
		t.Fatalf("unexpected schema versions: %d/%d/%d", store.ConfigSchemaVersion, store.HistorySchemaVersion, store.LogsSchemaVersion)
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

func TestDeletedLogsDatabaseRebuildsWithoutChangingConfiguration(t *testing.T) {
	ctx := context.Background()
	paths := PathsFromDataDirectory(t.TempDir())
	store, err := Open(ctx, paths)
	if err != nil {
		t.Fatal(err)
	}
	nodeID := "7289cfa3-a75d-4a3f-ac06-8f1074446a85"
	if _, err := store.Config.ExecContext(ctx, `
		INSERT INTO nodes (
			id, name, hostname, credential_digest, agent_version,
			operating_system, architecture, desired_configuration_revision, registered_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?)
	`, nodeID, "retained-edge", "retained-edge", make([]byte, 32), "0.1.1", "linux", "amd64", 1); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	removeSQLiteFiles(t, paths.LogsDatabase)

	restarted, err := Open(ctx, paths)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restarted.Close() })
	node, err := restarted.ConfigQueries.GetNodeByID(ctx, nodeID)
	if err != nil || node.Name != "retained-edge" || node.LogLevel != "info" {
		t.Fatalf("configuration after log reset = %#v, %v", node, err)
	}
	count, err := restarted.LogsQueries.CountLogEvents(ctx)
	if err != nil || count != 0 || restarted.LogsSchemaVersion != logsSchemaVersion {
		t.Fatalf("rebuilt logs database = count %d, version %d, %v", count, restarted.LogsSchemaVersion, err)
	}
}

func TestV011ConfigurationMigratesToAgentLogSettings(t *testing.T) {
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
	if err := goose.SetDialect("sqlite3"); err == nil {
		err = goose.UpToContext(ctx, database, "migrations/config", 1)
	}
	migrationMu.Unlock()
	if err != nil {
		_ = database.Close()
		t.Fatal(err)
	}
	nodeID := "7289cfa3-a75d-4a3f-ac06-8f1074446a85"
	registeredAt := int64(1_725_440_000)
	if _, err := database.ExecContext(ctx, `
		INSERT INTO nodes (
			id, name, hostname, credential_digest, agent_version,
			operating_system, architecture, desired_configuration_revision,
			applied_configuration_revision, registered_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, 7, 7, ?)
	`, nodeID, "stable-edge", "stable-edge", make([]byte, 32), "0.1.1", "linux", "amd64", registeredAt); err != nil {
		_ = database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	upgraded, err := OpenConfigurationForRecovery(ctx, paths)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = upgraded.Close() })
	if upgraded.SchemaVersion != 3 {
		t.Fatalf("configuration schema version = %d", upgraded.SchemaVersion)
	}
	node, err := upgraded.Queries.GetNodeByID(ctx, nodeID)
	if err != nil || node.Name != "stable-edge" || node.DesiredConfigurationRevision != 7 ||
		node.LogLevel != "info" || node.DesiredConfigurationUpdatedAt != registeredAt {
		t.Fatalf("migrated v0.1.1 node = %#v, %v", node, err)
	}
	retention, err := upgraded.Queries.GetLogRetentionSettings(ctx)
	if err != nil || retention.Mode != "age" || retention.MaxAgeDays == nil || *retention.MaxAgeDays != 7 {
		t.Fatalf("migrated log retention = %#v, %v", retention, err)
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
