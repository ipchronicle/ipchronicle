package systemsettings

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/ipchronicle/ipchronicle/internal/center/database"
)

type recordingWaker struct {
	nodeIDs []string
}

func (waker *recordingWaker) Wake(nodeID string) {
	waker.nodeIDs = append(waker.nodeIDs, nodeID)
}

func TestExternalOriginDefaultsToRequestAndCanBeCustomized(t *testing.T) {
	store, err := database.Open(context.Background(), database.PathsFromDataDirectory(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := NewService(store.Config, store.ConfigQueries, store.MasterKey, &recordingWaker{})

	effective, err := service.EffectiveOrigin(context.Background(), "https://current.example")
	if err != nil || effective != "https://current.example" {
		t.Fatalf("automatic origin = %q, %v", effective, err)
	}
	settings, err := service.Update(context.Background(), Update{
		ExternalOrigin: stringPointer(" HTTPS://EXAMPLE.COM:8443/ "), IPAPIAPIKeyAction: "keep",
	})
	if err != nil || settings.ExternalOrigin != "https://example.com:8443" {
		t.Fatalf("custom settings = %#v, %v", settings, err)
	}
	effective, err = service.EffectiveOrigin(context.Background(), "https://current.example")
	if err != nil || effective != "https://example.com:8443" {
		t.Fatalf("custom origin = %q, %v", effective, err)
	}
	settings, err = service.Update(context.Background(), Update{
		ExternalOrigin: stringPointer(""), IPAPIAPIKeyAction: "keep",
	})
	if err != nil || settings.ExternalOrigin != "" {
		t.Fatalf("automatic settings = %#v, %v", settings, err)
	}
}

func TestExternalOriginRejectsNonOrigins(t *testing.T) {
	store, err := database.Open(context.Background(), database.PathsFromDataDirectory(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := NewService(store.Config, store.ConfigQueries, store.MasterKey, &recordingWaker{})

	for _, value := range []string{
		"ftp://example.com",
		"https://owner@example.com",
		"https://example.com/path",
		"https://example.com?query=yes",
		"https://example.com/#fragment",
		"https://",
		stringsOfLength(maximumExternalOriginLength + 1),
	} {
		if _, err := service.Update(context.Background(), Update{
			ExternalOrigin: &value, IPAPIAPIKeyAction: "keep",
		}); !errors.Is(err, ErrInvalidExternalOrigin) {
			t.Errorf("Update(%q) error = %v", value, err)
		}
	}
}

func TestIPAPIAPIKeyIsEncryptedReplaceOnlyAndAdvancesAgentConfiguration(t *testing.T) {
	ctx := context.Background()
	store, err := database.Open(ctx, database.PathsFromDataDirectory(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	nodeID := "7289cfa3-a75d-4a3f-ac06-8f1074446a85"
	if _, err := store.Config.ExecContext(ctx, `
		INSERT INTO nodes (
			id, name, hostname, credential_digest, agent_version, operating_system,
			architecture, desired_configuration_revision,
			applied_configuration_revision, registered_at
		) VALUES (?, 'edge', 'edge.example', zeroblob(32), 'dev', 'linux', 'amd64', 1, 1, 1)
	`, nodeID); err != nil {
		t.Fatal(err)
	}
	waker := &recordingWaker{}
	service := NewService(store.Config, store.ConfigQueries, store.MasterKey, waker)
	if _, err := service.Update(ctx, Update{
		ExternalOrigin: stringPointer("https://center.example"), IPAPIAPIKeyAction: "keep",
	}); err != nil {
		t.Fatal(err)
	}
	apiKey := "test-ipapi-secret"
	settings, err := service.Update(ctx, Update{IPAPIAPIKeyAction: "replace", IPAPIAPIKey: &apiKey})
	if err != nil || !settings.IPAPIAPIKeyConfigured || settings.ExternalOrigin != "https://center.example" {
		t.Fatalf("replace key settings = %#v, %v", settings, err)
	}

	var encrypted []byte
	var revision int64
	if err := store.Config.QueryRowContext(ctx, `
		SELECT ipapi_api_key_encrypted FROM system_state WHERE id = 1
	`).Scan(&encrypted); err != nil {
		t.Fatal(err)
	}
	if len(encrypted) == 0 || bytes.Contains(encrypted, []byte(apiKey)) {
		t.Fatal("ipapi API key was not encrypted at rest")
	}
	if err := store.Config.QueryRowContext(ctx, `
		SELECT desired_configuration_revision FROM nodes WHERE id = ?
	`, nodeID).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if revision != 2 || len(waker.nodeIDs) != 1 || waker.nodeIDs[0] != nodeID {
		t.Fatalf("configuration revision = %d, wakes = %#v", revision, waker.nodeIDs)
	}
	if retained, err := service.IPAPIAPIKey(ctx); err != nil || retained != apiKey {
		t.Fatalf("retained key = %q, %v", retained, err)
	}

	if _, err := service.Update(ctx, Update{
		IPAPIAPIKeyAction: "replace", IPAPIAPIKey: &apiKey,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Config.QueryRowContext(ctx, `
		SELECT desired_configuration_revision FROM nodes WHERE id = ?
	`, nodeID).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if revision != 2 || len(waker.nodeIDs) != 1 {
		t.Fatalf("unchanged key advanced configuration: revision = %d, wakes = %#v", revision, waker.nodeIDs)
	}

	settings, err = service.Update(ctx, Update{IPAPIAPIKeyAction: "clear"})
	if err != nil || settings.IPAPIAPIKeyConfigured || settings.ExternalOrigin != "https://center.example" {
		t.Fatalf("clear key settings = %#v, %v", settings, err)
	}
	if retained, err := service.IPAPIAPIKey(ctx); err != nil || retained != "" {
		t.Fatalf("cleared key = %q, %v", retained, err)
	}
}

func TestIPAPIAPIKeyRejectsInconsistentSecretActions(t *testing.T) {
	store, err := database.Open(context.Background(), database.PathsFromDataDirectory(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := NewService(store.Config, store.ConfigQueries, store.MasterKey, &recordingWaker{})
	value := "key"
	for _, input := range []Update{
		{IPAPIAPIKeyAction: ""},
		{IPAPIAPIKeyAction: "replace"},
		{IPAPIAPIKeyAction: "keep", IPAPIAPIKey: &value},
		{IPAPIAPIKeyAction: "clear", IPAPIAPIKey: &value},
	} {
		if _, err := service.Update(context.Background(), input); !errors.Is(err, ErrInvalidIPAPIAPIKey) {
			t.Errorf("Update(%#v) error = %v", input, err)
		}
	}
}

func stringsOfLength(length int) string {
	value := make([]byte, length)
	for index := range value {
		value[index] = 'a'
	}
	return string(value)
}

func stringPointer(value string) *string {
	return &value
}
