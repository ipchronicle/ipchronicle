package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ipchronicle/ipchronicle/internal/center/database/configdb"
	"github.com/ipchronicle/ipchronicle/internal/center/database/historydb"
	_ "github.com/mattn/go-sqlite3"
	"github.com/pressly/goose/v3"
)

const (
	MasterKeySize        = 32
	configSchemaVersion  = 11
	historySchemaVersion = 5
)

//go:embed migrations/config/*.sql migrations/history/*.sql
var migrationFiles embed.FS

var migrationMu sync.Mutex

type Paths struct {
	ConfigDatabase  string
	HistoryDatabase string
	MasterKey       string
}

func PathsFromDataDirectory(dataDirectory string) Paths {
	return Paths{
		ConfigDatabase:  filepath.Join(dataDirectory, "config", "config.db"),
		HistoryDatabase: filepath.Join(dataDirectory, "history", "history.db"),
		MasterKey:       filepath.Join(dataDirectory, "config", "master.key"),
	}
}

type Store struct {
	Config               *sql.DB
	History              *sql.DB
	ConfigQueries        *configdb.Queries
	HistoryQueries       *historydb.Queries
	MasterKey            [MasterKeySize]byte
	HistoryGeneration    string
	ConfigSchemaVersion  int64
	HistorySchemaVersion int64
}

type ConfigurationStore struct {
	Database      *sql.DB
	Queries       *configdb.Queries
	MasterKey     [MasterKeySize]byte
	SchemaVersion int64
}

// OpenConfigurationForRecovery opens only durable administrator state. It
// deliberately leaves history ownership untouched so account recovery still
// works when the history database is absent or damaged.
func OpenConfigurationForRecovery(ctx context.Context, paths Paths) (*ConfigurationStore, error) {
	if err := validatePaths(paths); err != nil {
		return nil, err
	}
	configExists, err := regularFileExists(paths.ConfigDatabase)
	if err != nil {
		return nil, fmt.Errorf("inspect configuration database: %w", err)
	}
	if !configExists {
		return nil, errors.New("configuration database does not exist")
	}
	masterKey, err := loadOrCreateMasterKey(paths.MasterKey, true)
	if err != nil {
		return nil, err
	}
	database, err := openSQLite(ctx, paths.ConfigDatabase)
	if err != nil {
		return nil, fmt.Errorf("open configuration database: %w", err)
	}
	version, err := migrate(ctx, database, "migrations/config", configSchemaVersion)
	if err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("migrate configuration database: %w", err)
	}
	return &ConfigurationStore{
		Database: database, Queries: configdb.New(database),
		MasterKey: masterKey, SchemaVersion: version,
	}, nil
}

func (s *ConfigurationStore) Close() error {
	if s == nil {
		return nil
	}
	return s.Database.Close()
}

func Open(ctx context.Context, paths Paths) (*Store, error) {
	if err := validatePaths(paths); err != nil {
		return nil, err
	}
	if err := prepareDirectories(paths); err != nil {
		return nil, err
	}

	configExisted, err := regularFileExists(paths.ConfigDatabase)
	if err != nil {
		return nil, fmt.Errorf("inspect configuration database: %w", err)
	}
	historyExisted, err := regularFileExists(paths.HistoryDatabase)
	if err != nil {
		return nil, fmt.Errorf("inspect history database: %w", err)
	}
	masterKey, err := loadOrCreateMasterKey(paths.MasterKey, configExisted || historyExisted)
	if err != nil {
		return nil, err
	}

	configDatabase, err := openSQLite(ctx, paths.ConfigDatabase)
	if err != nil {
		return nil, fmt.Errorf("open configuration database: %w", err)
	}
	historyDatabase, err := openSQLite(ctx, paths.HistoryDatabase)
	if err != nil {
		_ = configDatabase.Close()
		return nil, fmt.Errorf("open history database: %w", err)
	}

	store := &Store{
		Config:         configDatabase,
		History:        historyDatabase,
		ConfigQueries:  configdb.New(configDatabase),
		HistoryQueries: historydb.New(historyDatabase),
		MasterKey:      masterKey,
	}
	closeOnError := func(cause error) (*Store, error) {
		return nil, errors.Join(cause, store.Close())
	}

	configVersion, err := migrate(ctx, configDatabase, "migrations/config", configSchemaVersion)
	if err != nil {
		return closeOnError(fmt.Errorf("migrate configuration database: %w", err))
	}
	historyVersion, err := migrate(ctx, historyDatabase, "migrations/history", historySchemaVersion)
	if err != nil {
		return closeOnError(fmt.Errorf("migrate history database: %w", err))
	}
	generation, err := reconcileHistoryGeneration(ctx, store, historyExisted)
	if err != nil {
		return closeOnError(fmt.Errorf("reconcile history database: %w", err))
	}

	store.ConfigSchemaVersion = configVersion
	store.HistorySchemaVersion = historyVersion
	store.HistoryGeneration = generation
	return store, nil
}

func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	return errors.Join(s.Config.Close(), s.History.Close())
}

func validatePaths(paths Paths) error {
	for name, value := range map[string]string{
		"configuration database": paths.ConfigDatabase,
		"history database":       paths.HistoryDatabase,
		"master key":             paths.MasterKey,
	} {
		if value == "" {
			return fmt.Errorf("%s path must not be empty", name)
		}
	}
	if paths.ConfigDatabase == paths.HistoryDatabase {
		return errors.New("configuration and history databases must use different paths")
	}
	return nil
}

func prepareDirectories(paths Paths) error {
	for _, path := range []string{paths.ConfigDatabase, paths.HistoryDatabase, paths.MasterKey} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return fmt.Errorf("create data directory for %s: %w", path, err)
		}
	}
	return nil
}

func regularFileExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("%s is not a regular file", path)
	}
	return true, nil
}

func loadOrCreateMasterKey(path string, persistentDataExists bool) ([MasterKeySize]byte, error) {
	var key [MasterKeySize]byte
	file, err := os.Open(path)
	if err == nil {
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			return key, fmt.Errorf("inspect master key: %w", err)
		}
		if !info.Mode().IsRegular() {
			return key, errors.New("master key must be a regular file")
		}
		if info.Mode().Perm()&0o077 != 0 {
			return key, fmt.Errorf("master key permissions %o allow group or other access", info.Mode().Perm())
		}
		if _, err := io.ReadFull(file, key[:]); err != nil {
			return key, fmt.Errorf("read master key: %w", err)
		}
		var extra [1]byte
		if count, err := file.Read(extra[:]); err != io.EOF || count != 0 {
			return key, errors.New("master key must contain exactly 32 bytes")
		}
		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return key, fmt.Errorf("open master key: %w", err)
	}
	if persistentDataExists {
		return key, errors.New("master key is missing while persistent databases exist")
	}
	if _, err := rand.Read(key[:]); err != nil {
		return key, fmt.Errorf("generate master key: %w", err)
	}
	file, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return key, fmt.Errorf("create master key: %w", err)
	}
	if _, err := file.Write(key[:]); err != nil {
		_ = file.Close()
		return key, fmt.Errorf("write master key: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return key, fmt.Errorf("sync master key: %w", err)
	}
	if err := file.Close(); err != nil {
		return key, fmt.Errorf("close master key: %w", err)
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return key, fmt.Errorf("open master key directory: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return key, fmt.Errorf("sync master key directory: %w", err)
	}
	return key, nil
}

func openSQLite(ctx context.Context, path string) (*sql.DB, error) {
	dsn := url.URL{Scheme: "file", Path: path}
	query := dsn.Query()
	query.Set("_busy_timeout", "5000")
	query.Set("_foreign_keys", "on")
	query.Set("_journal_mode", "WAL")
	query.Set("_synchronous", "NORMAL")
	query.Set("_txlock", "immediate")
	dsn.RawQuery = query.Encode()

	database, err := sql.Open("sqlite3", dsn.String())
	if err != nil {
		return nil, err
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	database.SetConnMaxLifetime(0)
	if err := database.PingContext(ctx); err != nil {
		_ = database.Close()
		return nil, err
	}
	var integrity string
	if err := database.QueryRowContext(ctx, "PRAGMA quick_check(1)").Scan(&integrity); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("run SQLite quick check: %w", err)
	}
	if integrity != "ok" {
		_ = database.Close()
		return nil, fmt.Errorf("SQLite quick check failed: %s", integrity)
	}
	return database, nil
}

func migrate(ctx context.Context, database *sql.DB, directory string, maximumVersion int64) (int64, error) {
	migrationMu.Lock()
	defer migrationMu.Unlock()
	goose.SetBaseFS(migrationFiles)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return 0, err
	}
	currentVersion, err := goose.GetDBVersionContext(ctx, database)
	if err != nil {
		return 0, err
	}
	if currentVersion > maximumVersion {
		return 0, fmt.Errorf("schema version %d is newer than supported version %d", currentVersion, maximumVersion)
	}
	if err := goose.UpContext(ctx, database, directory); err != nil {
		return 0, err
	}
	currentVersion, err = goose.GetDBVersionContext(ctx, database)
	if err != nil {
		return 0, err
	}
	if currentVersion != maximumVersion {
		return 0, fmt.Errorf("schema version is %d after migration, expected %d", currentVersion, maximumVersion)
	}
	return currentVersion, nil
}

func reconcileHistoryGeneration(ctx context.Context, store *Store, historyExisted bool) (string, error) {
	state, stateErr := store.ConfigQueries.GetSystemState(ctx)
	metadata, metadataErr := store.HistoryQueries.GetHistoryMetadata(ctx)
	stateMissing := errors.Is(stateErr, sql.ErrNoRows)
	metadataMissing := errors.Is(metadataErr, sql.ErrNoRows)
	if stateErr != nil && !stateMissing {
		return "", stateErr
	}
	if metadataErr != nil && !metadataMissing {
		return "", metadataErr
	}

	if stateMissing {
		if !metadataMissing {
			return "", errors.New("history metadata exists without configuration system state")
		}
		if historyExisted {
			return "", errors.New("existing history database has no generation metadata")
		}
		generation, err := randomGeneration()
		if err != nil {
			return "", err
		}
		if err := store.ConfigQueries.CreateSystemState(ctx, generation); err != nil {
			return "", err
		}
		if err := store.HistoryQueries.CreateHistoryMetadata(ctx, historydb.CreateHistoryMetadataParams{
			Generation: generation,
			CreatedAt:  time.Now().UTC().Unix(),
		}); err != nil {
			return "", err
		}
		return generation, nil
	}

	if metadataMissing {
		pending := state.PendingHistoryGeneration
		if pending == nil {
			if historyExisted {
				return "", errors.New("existing history database has no generation metadata")
			}
			generation, err := randomGeneration()
			if err != nil {
				return "", err
			}
			if err := store.ConfigQueries.SetPendingHistoryGeneration(ctx, &generation); err != nil {
				return "", err
			}
			pending = &generation
			state.PendingHistoryGeneration = pending
		}
		if err := store.HistoryQueries.CreateHistoryMetadata(ctx, historydb.CreateHistoryMetadataParams{
			Generation: *pending,
			CreatedAt:  time.Now().UTC().Unix(),
		}); err != nil {
			return "", err
		}
		metadata.Generation = *pending
	}

	if state.PendingHistoryGeneration != nil {
		if metadata.Generation != *state.PendingHistoryGeneration {
			return "", errors.New("history generation does not match the pending reset generation")
		}
		transaction, err := store.Config.BeginTx(ctx, nil)
		if err != nil {
			return "", err
		}
		defer transaction.Rollback()
		queries := store.ConfigQueries.WithTx(transaction)
		updated, err := queries.PromotePendingHistoryGeneration(ctx, configdb.PromotePendingHistoryGenerationParams{
			HistoryResetAt:           pointerTo(time.Now().UTC().Unix()),
			PendingHistoryGeneration: state.PendingHistoryGeneration,
		})
		if err != nil {
			return "", err
		}
		if updated != 1 {
			return "", errors.New("pending history generation was not promoted")
		}
		if err := queries.IncrementAllNodeDesiredConfigurationRevisions(ctx); err != nil {
			return "", err
		}
		if err := transaction.Commit(); err != nil {
			return "", err
		}
		return metadata.Generation, nil
	}

	if metadata.Generation != state.HistoryGeneration {
		return "", errors.New("configuration and history generations do not match")
	}
	return state.HistoryGeneration, nil
}

func randomGeneration() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func pointerTo[T any](value T) *T {
	return &value
}
