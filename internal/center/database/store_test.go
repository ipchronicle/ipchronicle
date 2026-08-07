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
)

func TestFreshOpenAndRestart(t *testing.T) {
	paths := PathsFromDataDirectory(t.TempDir())
	store, err := Open(context.Background(), paths)
	if err != nil {
		t.Fatal(err)
	}
	firstGeneration := store.HistoryGeneration
	if store.ConfigSchemaVersion != 1 || store.HistorySchemaVersion != 1 {
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
