package agentlogs

import (
	"bytes"
	"context"
	"log"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ipchronicle/ipchronicle/internal/agent/state"
)

func TestRecorderAppliesLevelAndPersistsThroughOneEventSource(t *testing.T) {
	store, err := state.Open(filepath.Join(t.TempDir(), "agent"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	var output bytes.Buffer
	recorder := NewRecorder(store, log.New(&output, "", 0))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- recorder.Run(ctx) }()
	recorder.Emit(Event{Level: "debug", Component: "test", EventType: "hidden", Message: "hidden"})
	recorder.Emit(Event{Level: "error", Component: "test", EventType: "visible", Message: "visible"})
	select {
	case <-recorder.UploadWake():
	case <-time.After(time.Second):
		t.Fatal("recorder did not persist the visible event")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	batch, err := store.PendingAgentLogs(64, 8*1024*1024)
	if err != nil || len(batch) != 1 || batch[0].EventType != "visible" {
		t.Fatalf("recorded events = %#v, %v", batch, err)
	}
	if text := output.String(); !strings.Contains(text, `event=visible`) || strings.Contains(text, `event=hidden`) {
		t.Fatalf("local log output = %q", text)
	}
}

func TestSanitizeRequestTargetRemovesCredentialsAndQuery(t *testing.T) {
	target := SanitizeRequestTarget("https://user:password@example.com/check?api_key=secret#result")
	if target == nil || *target != "https://example.com/check" {
		t.Fatalf("sanitized request target = %v", target)
	}
}
