package systemsettings

import (
	"context"
	"errors"
	"testing"

	"github.com/ipchronicle/ipchronicle/internal/center/database"
)

func TestExternalOriginDefaultsToRequestAndCanBeCustomized(t *testing.T) {
	store, err := database.Open(context.Background(), database.PathsFromDataDirectory(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := NewService(store.ConfigQueries)

	effective, err := service.EffectiveOrigin(context.Background(), "https://current.example")
	if err != nil || effective != "https://current.example" {
		t.Fatalf("automatic origin = %q, %v", effective, err)
	}
	settings, err := service.Update(context.Background(), " HTTPS://EXAMPLE.COM:8443/ ")
	if err != nil || settings.ExternalOrigin != "https://example.com:8443" {
		t.Fatalf("custom settings = %#v, %v", settings, err)
	}
	effective, err = service.EffectiveOrigin(context.Background(), "https://current.example")
	if err != nil || effective != "https://example.com:8443" {
		t.Fatalf("custom origin = %q, %v", effective, err)
	}
	settings, err = service.Update(context.Background(), "")
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
	service := NewService(store.ConfigQueries)

	for _, value := range []string{
		"ftp://example.com",
		"https://owner@example.com",
		"https://example.com/path",
		"https://example.com?query=yes",
		"https://example.com/#fragment",
		"https://",
		stringsOfLength(maximumExternalOriginLength + 1),
	} {
		if _, err := service.Update(context.Background(), value); !errors.Is(err, ErrInvalidExternalOrigin) {
			t.Errorf("Update(%q) error = %v", value, err)
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
