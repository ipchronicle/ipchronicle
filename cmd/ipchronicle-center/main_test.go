package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/ipchronicle/ipchronicle/internal/center/admin"
	"github.com/ipchronicle/ipchronicle/internal/center/database"
	"github.com/ipchronicle/ipchronicle/internal/releaseinfo"
	"github.com/pquerna/otp/totp"
)

func TestVersionJSONCommand(t *testing.T) {
	output := captureStandardOutput(t, func() error {
		return run([]string{"version", "--json"})
	})
	var info releaseinfo.BinaryInfo
	if err := json.Unmarshal(output, &info); err != nil {
		t.Fatal(err)
	}
	if info.Version == "" || info.Revision == "" || info.Component != "center" ||
		info.OS != runtime.GOOS || info.Arch != runtime.GOARCH {
		t.Fatalf("unexpected Center metadata: %#v", info)
	}
}

func TestAdministratorResetPasswordCommand(t *testing.T) {
	paths := prepareAdministratorInstallation(t)
	historyContents := []byte("history database must remain untouched")
	if err := os.WriteFile(paths.HistoryDatabase, historyContents, 0o600); err != nil {
		t.Fatal(err)
	}
	withStandardInput(t, "recovered-password\n", func() {
		if err := run([]string{"admin", "reset-password", "--password-stdin"}); err != nil {
			t.Fatal(err)
		}
	})
	afterRecovery, err := os.ReadFile(paths.HistoryDatabase)
	if err != nil {
		t.Fatal(err)
	}
	if string(afterRecovery) != string(historyContents) {
		t.Fatalf("administrator recovery modified history database: %q", afterRecovery)
	}

	store, err := database.OpenConfigurationForRecovery(context.Background(), paths)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := admin.NewService(store.Database, store.Queries, store.MasterKey)
	if _, _, err := service.Login(context.Background(), "admin", "recovered-password", "", "192.0.2.1", "test"); err != nil {
		t.Fatalf("recovered password does not authenticate: %v", err)
	}
}

func TestAdministratorDisableTOTPCommand(t *testing.T) {
	paths := prepareAdministratorInstallation(t)
	store := openTestStore(t, paths)
	service := admin.NewService(store.Config, store.ConfigQueries, store.MasterKey)
	enrollment, err := service.StartTOTPEnrollment(context.Background(), "admin")
	if err != nil {
		t.Fatal(err)
	}
	code, err := totp.GenerateCode(enrollment.Secret, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmTOTPEnrollment(context.Background(), code); err != nil {
		t.Fatal(err)
	}
	session, _, err := service.Login(context.Background(), "admin", "admin", nextTOTPCode(t, enrollment.Secret), "192.0.2.1", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	if err := run([]string{"admin", "disable-totp"}); err != nil {
		t.Fatal(err)
	}

	store = openTestStore(t, paths)
	service = admin.NewService(store.Config, store.ConfigQueries, store.MasterKey)
	account, err := service.Account(context.Background())
	if err != nil || account.TOTPEnabled {
		t.Fatalf("TOTP remains enabled: %#v, %v", account, err)
	}
	if _, err := service.Authenticate(context.Background(), session.Token); !errors.Is(err, admin.ErrUnauthenticated) {
		t.Fatalf("recovery did not revoke existing session: %v", err)
	}
}

func prepareAdministratorInstallation(t *testing.T) database.Paths {
	t.Helper()
	dataDirectory := t.TempDir()
	paths := database.PathsFromDataDirectory(dataDirectory)
	setDatabaseEnvironment(t, paths)
	store := openTestStore(t, paths)
	service := admin.NewService(store.Config, store.ConfigQueries, store.MasterKey)
	if err := service.Bootstrap(context.Background(), "admin", "admin"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	return paths
}

func setDatabaseEnvironment(t *testing.T, paths database.Paths) {
	t.Helper()
	t.Setenv("IPCHRONICLE_CONFIG_DATABASE_PATH", paths.ConfigDatabase)
	t.Setenv("IPCHRONICLE_HISTORY_DATABASE_PATH", paths.HistoryDatabase)
	t.Setenv("IPCHRONICLE_MASTER_KEY_PATH", paths.MasterKey)
}

func openTestStore(t *testing.T, paths database.Paths) *database.Store {
	t.Helper()
	store, err := database.Open(context.Background(), paths)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func withStandardInput(t *testing.T, input string, action func()) {
	t.Helper()
	file, err := os.Create(filepath.Join(t.TempDir(), "stdin"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(input); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	previous := os.Stdin
	os.Stdin = file
	t.Cleanup(func() {
		os.Stdin = previous
		_ = file.Close()
	})
	action()
}

func captureStandardOutput(t *testing.T, action func() error) []byte {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stdout
	os.Stdout = writer
	defer func() {
		os.Stdout = previous
		_ = reader.Close()
		_ = writer.Close()
	}()
	if err := action(); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return output
}

func nextTOTPCode(t *testing.T, secret string) string {
	t.Helper()
	code, err := totp.GenerateCode(secret, time.Now().UTC().Add(30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return code
}
