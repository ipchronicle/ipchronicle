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

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/agent"
	agentstate "github.com/ipchronicle/ipchronicle/internal/agent/state"
	"github.com/ipchronicle/ipchronicle/internal/center"
	"github.com/ipchronicle/ipchronicle/internal/center/admin"
	"github.com/ipchronicle/ipchronicle/internal/center/database"
	"github.com/ipchronicle/ipchronicle/internal/center/nodes"
	"github.com/ipchronicle/ipchronicle/internal/center/notifications"
	"github.com/ipchronicle/ipchronicle/internal/center/syncws"
	"github.com/ipchronicle/ipchronicle/internal/center/systemsettings"
	centerupdates "github.com/ipchronicle/ipchronicle/internal/center/updates"
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
	syncHub := syncws.NewHub()
	nodeService := nodes.NewService(centerStore.Config, centerStore.History, centerStore.ConfigQueries, centerStore.MasterKey, syncHub)
	systemSettingsService := systemsettings.NewService(centerStore.Config, centerStore.ConfigQueries, centerStore.MasterKey, syncHub)
	notificationService := notifications.NewService(notifications.ServiceOptions{
		ConfigDatabase: centerStore.Config, HistoryDatabase: centerStore.History,
		ConfigQueries: centerStore.ConfigQueries, HistoryQueries: centerStore.HistoryQueries,
		MasterKey: centerStore.MasterKey, SystemSettings: systemSettingsService, Executable: "/proc/self/exe",
	})
	updateService := centerupdates.NewService(centerupdates.ServiceOptions{
		Queries: centerStore.ConfigQueries, Waker: syncHub,
		CurrentVersion: "0.1.0-rc.1", CurrentRevision: "test-revision",
	})
	enrollment, err := nodeService.RotateEnrollmentKey(ctx, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	handler := center.NewHTTPHandler(center.HTTPOptions{
		Version: "0.1.0-test", Revision: "test-revision", Web: http.NotFoundHandler(), Administrator: administrator,
		Nodes: nodeService, Notifications: notificationService, Updates: updateService, SyncHub: syncHub,
		SystemSettings: systemSettingsService, Store: centerStore,
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
	started, err := nodeService.StartSyncSession(ctx, registrationNodeID(t, identity.NodeID))
	if err != nil || started.SyncStatus == nil || *started.SyncStatus != "pending" {
		t.Fatalf("start temporary sync = %#v, %v", started, err)
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
	expectedRevision := int64(0)
	for {
		listed, err = nodeService.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(listed) == 1 && listed[0].Status == "online" && listed[0].ConfigurationStatus == "current" &&
			listed[0].AppliedConfigurationRevision == listed[0].DesiredConfigurationRevision &&
			listed[0].SyncStatus != nil && *listed[0].SyncStatus == "connected" {
			expectedRevision = listed[0].DesiredConfigurationRevision
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("Agent did not become online: %#v", listed)
		}
		time.Sleep(20 * time.Millisecond)
	}
	configuration, err := localStore.Configuration()
	if err != nil || configuration.Revision != expectedRevision || !configuration.Enabled || len(configuration.HistoryGeneration) != 64 {
		t.Fatalf("applied local configuration = %#v, %v", configuration, err)
	}
	convergenceStarted := time.Now()
	if _, err := nodeService.SetEnabled(ctx, registrationNodeID(t, identity.NodeID), false); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(3 * time.Second)
	for {
		configuration, err = localStore.Configuration()
		if err != nil {
			t.Fatal(err)
		}
		if configuration.Revision == expectedRevision+1 && !configuration.Enabled {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("WebSocket wake did not converge configuration before normal polling: %#v", configuration)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if time.Since(convergenceStarted) >= nodes.PollInterval {
		t.Fatalf("configuration convergence took at least one normal poll interval: %s", time.Since(convergenceStarted))
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

func registrationNodeID(t *testing.T, value string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(value)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
