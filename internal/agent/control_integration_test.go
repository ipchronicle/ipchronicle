package agent_test

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/ipchronicle/ipchronicle/internal/agent"
	agentstate "github.com/ipchronicle/ipchronicle/internal/agent/state"
	"github.com/ipchronicle/ipchronicle/internal/center"
	"github.com/ipchronicle/ipchronicle/internal/center/admin"
	"github.com/ipchronicle/ipchronicle/internal/center/database"
	"github.com/ipchronicle/ipchronicle/internal/center/nodes"
)

func TestAgentEnrollsOnceAndBecomesOnline(t *testing.T) {
	ctx := context.Background()
	centerStore, err := database.Open(ctx, database.PathsFromDataDirectory(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = centerStore.Close() })
	administrator := admin.NewService(centerStore.Config, centerStore.ConfigQueries, centerStore.MasterKey)
	if err := administrator.Bootstrap(ctx, "admin", "admin"); err != nil {
		t.Fatal(err)
	}
	nodeService := nodes.NewService(centerStore.Config, centerStore.ConfigQueries, centerStore.MasterKey)
	enrollment, err := nodeService.RotateEnrollmentKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	handler := center.NewHTTPHandler(center.HTTPOptions{
		Version: "0.1.0-test", Web: http.NotFoundHandler(), Administrator: administrator,
		Nodes: nodeService, Store: centerStore,
	})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	localStore, err := agentstate.Open(filepath.Join(t.TempDir(), "agent"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = localStore.Close() })
	identity, err := agent.Enroll(ctx, localStore, server.URL, enrollment.Key, "0.1.0-test")
	if err != nil {
		t.Fatal(err)
	}
	second, err := agent.Enroll(ctx, localStore, server.URL, "invalid-after-first-enrollment", "0.1.0-test")
	if err != nil || second.NodeID != identity.NodeID {
		t.Fatalf("repeat enrollment did not preserve identity: %#v, %v", second, err)
	}
	listed, err := nodeService.List(ctx)
	if err != nil || len(listed) != 1 {
		t.Fatalf("registered nodes = %#v, %v", listed, err)
	}

	runContext, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- agent.Run(runContext, localStore, "0.1.0-test", log.New(io.Discard, "", 0))
	}()
	deadline := time.Now().Add(3 * time.Second)
	for {
		listed, err = nodeService.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(listed) == 1 && listed[0].Status == "online" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("Agent did not become online: %#v", listed)
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Agent did not stop after cancellation")
	}
}
