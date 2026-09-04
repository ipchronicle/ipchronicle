package agentlogs

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/center/database"
	"github.com/ipchronicle/ipchronicle/internal/center/database/configdb"
)

func TestUploadListAndGetAgentLogs(t *testing.T) {
	service, store, nodeID := newLogService(t)
	now := time.Date(2026, 9, 4, 14, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	body := []byte("upstream failure")
	address := "203.0.113.9"
	family := "ipv4"
	category := "http-status"
	method := "GET"
	target := "https://example.com/check"
	status := 429
	rateLimits := map[string]string{"Retry-After": "60", "X-RateLimit-Remaining": "0"}
	event := Event{
		ID: uuid.New(), OccurredAt: now.Add(-time.Second), Level: "warn",
		Component: "ip-quality", EventType: "provider-request-failed", Message: "provider request failed",
		PublicAddress: &address, Family: &family, FailureCategory: &category,
		RequestMethod: &method, RequestTarget: &target, HTTPStatus: &status,
		RateLimitHeaders: rateLimits, ResponseBody: body,
	}
	invalid := event
	invalid.ID = uuid.New()
	invalid.Component = ""
	accepted, discarded, err := service.Upload(context.Background(), nodeID, []Event{event, invalid})
	if err != nil || len(accepted) != 1 || accepted[0] != event.ID || len(discarded) != 1 || discarded[0] != invalid.ID {
		t.Fatalf("upload receipt = %v/%v, %v", accepted, discarded, err)
	}
	if _, _, err := service.Upload(context.Background(), nodeID, []Event{event}); err != nil {
		t.Fatal(err)
	}
	count, err := store.LogsQueries.CountLogEvents(context.Background())
	if err != nil || count != 1 {
		t.Fatalf("idempotent event count = %d, %v", count, err)
	}
	keyword := "request"
	page, err := service.List(context.Background(), Filter{NodeID: &nodeID, Keyword: &keyword, PageSize: 10})
	if err != nil || len(page.Items) != 1 || page.Items[0].NodeName == nil ||
		*page.Items[0].NodeName != "edge-one" || page.Items[0].ResponseBodyBytes != int64(len(body)) ||
		len(page.Items[0].ResponseBody) != 0 || len(page.Items[0].RateLimitHeaders) != 0 {
		t.Fatalf("log page = %#v, %v", page, err)
	}
	detail, err := service.Get(context.Background(), event.ID)
	if err != nil || !bytes.Equal(detail.Summary.ResponseBody, body) || detail.Summary.ResponseBodyBytes != int64(len(body)) ||
		detail.Summary.RateLimitHeaders["Retry-After"] != "60" {
		t.Fatalf("log detail = %#v, %v", detail, err)
	}
}

func TestLogFiltersAndCursorAreValidated(t *testing.T) {
	service, _, nodeID := newLogService(t)
	now := time.Now().UTC()
	for index := 0; index < 3; index++ {
		event := Event{
			ID: uuid.New(), OccurredAt: now.Add(time.Duration(index) * time.Second), Level: "info",
			Component: "test", EventType: "sequence", Message: "literal 100%_value",
		}
		if _, _, err := service.Upload(context.Background(), nodeID, []Event{event}); err != nil {
			t.Fatal(err)
		}
	}
	keyword := "%_"
	page, err := service.List(context.Background(), Filter{Keyword: &keyword, PageSize: 2})
	if err != nil || len(page.Items) != 2 || page.NextCursor == nil {
		t.Fatalf("first page = %#v, %v", page, err)
	}
	second, err := service.List(context.Background(), Filter{Keyword: &keyword, PageSize: 2, Cursor: page.NextCursor})
	if err != nil || len(second.Items) != 1 {
		t.Fatalf("second page = %#v, %v", second, err)
	}
	invalidCursor := "not-a-cursor"
	if _, err := service.List(context.Background(), Filter{Cursor: &invalidCursor}); !errors.Is(err, ErrInvalidFilter) {
		t.Fatalf("invalid cursor error = %v", err)
	}
}

func TestLogRetentionAgeAndSize(t *testing.T) {
	service, _, nodeID := newLogService(t)
	now := time.Date(2026, 9, 4, 14, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	for _, occurredAt := range []time.Time{now.Add(-48 * time.Hour), now.Add(-time.Hour)} {
		event := Event{ID: uuid.New(), OccurredAt: occurredAt, Level: "info", Component: "test", EventType: "retention", Message: "event"}
		if _, _, err := service.Upload(context.Background(), nodeID, []Event{event}); err != nil {
			t.Fatal(err)
		}
	}
	oneDay := int64(1)
	state, err := service.UpdateRetention(context.Background(), RetentionUpdate{Mode: "age", MaxAgeDays: &oneDay})
	if err != nil || state.RecordCount != 1 || state.LastCleanupDeletedItems != 1 {
		t.Fatalf("age retention = %#v, %v", state, err)
	}
	largeBody := bytes.Repeat([]byte("x"), 700*1024)
	for index := 0; index < 2; index++ {
		event := Event{
			ID: uuid.New(), OccurredAt: now.Add(time.Duration(index+1) * time.Second), Level: "error",
			Component: "test", EventType: "large", Message: "large", ResponseBody: largeBody,
		}
		if _, _, err := service.Upload(context.Background(), nodeID, []Event{event}); err != nil {
			t.Fatal(err)
		}
	}
	oneMiB := int64(1024 * 1024)
	state, err = service.UpdateRetention(context.Background(), RetentionUpdate{Mode: "size", MaxLogicalBytes: &oneMiB})
	if err != nil || state.LogicalBytes > oneMiB || state.RecordCount != 1 {
		t.Fatalf("size retention = %#v, %v", state, err)
	}
}

func newLogService(t *testing.T) (*Service, *database.Store, uuid.UUID) {
	t.Helper()
	store, err := database.Open(context.Background(), database.PathsFromDataDirectory(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	nodeID := uuid.New()
	now := time.Now().UTC().Truncate(time.Second)
	if err := store.ConfigQueries.CreateNode(context.Background(), configdb.CreateNodeParams{
		ID: nodeID.String(), Name: "edge-one", Hostname: "edge-one",
		CredentialDigest: make([]byte, 32), AgentVersion: "test", OperatingSystem: "linux",
		Architecture: "amd64", ProbeScheduleTimezone: "UTC", RegisteredAt: now.Unix(),
		DesiredConfigurationUpdatedAt: now.Unix(),
	}); err != nil {
		t.Fatal(err)
	}
	return NewService(store.Logs, store.LogsQueries, store.ConfigQueries), store, nodeID
}

func TestCorruptLogsDatabaseFailsExplicitly(t *testing.T) {
	paths := database.PathsFromDataDirectory(t.TempDir())
	store, err := database.Open(context.Background(), paths)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := databaseCorruptFile(paths.LogsDatabase); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Open(context.Background(), paths); err == nil || !strings.Contains(err.Error(), "logs database") {
		t.Fatalf("corrupt logs database error = %v", err)
	}
}

func databaseCorruptFile(path string) error {
	return os.WriteFile(filepath.Clean(path), []byte("not a SQLite database"), 0o600)
}
