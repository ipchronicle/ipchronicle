package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/agent/state"
	agentupdate "github.com/ipchronicle/ipchronicle/internal/agent/update"
	"github.com/ipchronicle/ipchronicle/internal/generated/agentapi"
)

func TestConfigurationRejectsUnknownFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/agent/configuration" {
			http.NotFound(response, request)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(response, `{"schemaVersion":1,"revision":1,"enabled":true,"historyGeneration":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","unexpected":true}`)
	}))
	defer server.Close()

	client, err := NewControlClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.configuration(context.Background(), "credential", 1); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown configuration field error = %v", err)
	}
}

func TestConfigurationMapsV6TransportSemantics(t *testing.T) {
	discoveryID := uuid.New()
	targetID := uuid.New()
	pathID := uuid.New()
	proxyID := uuid.New()
	generation := strings.Repeat("a", 64)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/agent/configuration" {
			http.NotFound(response, request)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(agentapi.AgentConfigurationSnapshot{
			SchemaVersion: 7, Revision: 7, Enabled: true, HistoryGeneration: generation,
			DiscoveryPaths: []agentapi.AgentDiscoveryPath{{
				Id: discoveryID, Kind: agentapi.Source, Family: agentapi.Ipv4,
				InterfaceName: pointer("eth0"), SourceAddress: pointer("10.0.0.2"),
				LightweightIntervalSeconds: 600,
			}},
			ProbeTargets: []agentapi.AgentProbeTarget{{
				Id: targetID, PathId: pathID, PublicAddress: "203.0.113.10",
				Kind: agentapi.Proxy, Family: agentapi.Ipv4,
				ProxyId: &proxyID,
			}},
			Proxies: []agentapi.AgentProxyConfiguration{{
				Id: proxyID, Scheme: agentapi.NetworkProxySchemeSocks5, Host: "proxy.example", Port: 1080,
			}},
			DiscoveryServices: agentapi.NetworkObservationSettingsUpdate{
				Ipv4Services: []string{"https://v4-one.example", "https://v4-two.example"},
				Ipv6Services: []string{"https://v6-one.example", "https://v6-two.example"},
			},
			ProbeSchedule: agentapi.ProbeSchedule{Enabled: true, Cron: "0 0 0 * * *", Timezone: "UTC"},
		})
	}))
	defer server.Close()

	client, err := NewControlClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	configuration, err := client.configuration(context.Background(), "credential", 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(configuration.DiscoveryPaths) != 1 || !configuration.DiscoveryPaths[0].Enabled ||
		configuration.DiscoveryPaths[0].ID != discoveryID.String() || configuration.DiscoveryPaths[0].LightweightIntervalSeconds != 600 {
		t.Fatalf("discovery paths = %#v", configuration.DiscoveryPaths)
	}
	if len(configuration.ProbeTargets) != 1 || !configuration.ProbeTargets[0].Enabled ||
		configuration.ProbeTargets[0].ID != targetID.String() || configuration.ProbeTargets[0].PathID == nil ||
		*configuration.ProbeTargets[0].PathID != pathID.String() {
		t.Fatalf("probe targets = %#v", configuration.ProbeTargets)
	}
	if len(configuration.Proxies) != 1 || configuration.Proxies[0].ID != proxyID.String() {
		t.Fatalf("proxies = %#v", configuration.Proxies)
	}
	store, err := state.Open(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.SaveIdentity(state.Identity{
		CenterURL: server.URL, NodeID: uuid.NewString(), Credential: "ipc_agent_configuration-test",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.ApplyConfiguration(configuration); err != nil {
		t.Fatalf("apply mapped v6 configuration: %v", err)
	}
}

func pointer(value string) *string {
	return &value
}

func TestAgentAPIResponseBodyIsBounded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(response, strings.Repeat("x", maxAgentAPIResponseSize*2))
	}))
	defer server.Close()

	client := &http.Client{
		Transport: boundedResponseTransport{base: http.DefaultTransport, maxBytes: maxAgentAPIResponseSize},
	}
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if len(body) != maxAgentAPIResponseSize+1 {
		t.Fatalf("bounded response length = %d", len(body))
	}
}

func TestParsePhysicalMemory(t *testing.T) {
	memory, err := parsePhysicalMemory(strings.NewReader("SwapTotal: 0 kB\nMemTotal:       262144 kB\n"))
	if err != nil || memory != 256*1024*1024 {
		t.Fatalf("physical memory = %d, %v", memory, err)
	}
	for _, input := range []string{
		"SwapTotal: 0 kB\n",
		"MemTotal: unknown kB\n",
		"MemTotal: 0 kB\n",
		"MemTotal: 1024 MB\n",
	} {
		if _, err := parsePhysicalMemory(strings.NewReader(input)); err == nil {
			t.Fatalf("invalid meminfo %q unexpectedly succeeded", input)
		}
	}
}

func TestDevelopmentAgentRunsWithoutUpdateCapability(t *testing.T) {
	pollReceived := make(chan agentapi.AgentPollRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/agent/control" || request.Header.Get("Authorization") != "Bearer ipc_agent_dev-test" {
			http.Error(response, "unauthorized", http.StatusUnauthorized)
			return
		}
		var poll agentapi.AgentPollRequest
		if err := json.NewDecoder(request.Body).Decode(&poll); err != nil {
			t.Error(err)
			http.Error(response, "invalid", http.StatusBadRequest)
			return
		}
		pollReceived <- poll
		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(agentapi.AgentPollResult{
			AddressUploadReceipt: agentapi.AgentAddressUploadReceipt{
				AcceptedEventIds: []uuid.UUID{}, DiscardedEventIds: []uuid.UUID{},
				AcceptedGaps: []agentapi.AgentAddressGapReceipt{}, DiscardedGaps: []agentapi.AgentAddressGapReceipt{},
			},
			CenterVersion: "dev", DesiredConfigurationRevision: 0, Enabled: true, PollIntervalSeconds: 30,
		})
	}))
	defer server.Close()

	store, err := state.Open(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.SaveIdentity(state.Identity{
		CenterURL: server.URL, NodeID: uuid.NewString(), Credential: "ipc_agent_dev-test",
	}); err != nil {
		t.Fatal(err)
	}

	var logs bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- RunWithOptions(ctx, store, "dev", log.New(&logs, "", 0), RunOptions{
			UpdateConfig: &agentupdate.Config{
				InitSystem: "systemd", AgentPath: "/usr/local/bin/ipchronicle-agent", UpdaterPath: "/usr/local/libexec/ipchronicle-agent-updater",
			},
		})
	}()

	select {
	case poll := <-pollReceived:
		if poll.Metadata.AgentVersion != "dev" || slices.Contains(poll.Metadata.Capabilities, "agent-update-v1") {
			t.Fatalf("development Agent metadata = %#v", poll.Metadata)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("development Agent did not poll the center")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("development Agent did not stop after cancellation")
	}
	if !strings.Contains(logs.String(), `Agent updates disabled for non-release version "dev"`) {
		t.Fatalf("development Agent log = %q", logs.String())
	}
}

func TestUpdatedAgentAuthenticatesBeforePublishingHealth(t *testing.T) {
	pollReceived := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/agent/control" || request.Header.Get("Authorization") != "Bearer ipc_agent_health-test" {
			http.Error(response, "unauthorized", http.StatusUnauthorized)
			return
		}
		var poll agentapi.AgentPollRequest
		if err := json.NewDecoder(request.Body).Decode(&poll); err != nil {
			t.Error(err)
			http.Error(response, "invalid", http.StatusBadRequest)
			return
		}
		if poll.TaskReport == nil || poll.TaskReport.Status != agentapi.AgentTaskReportStatusRestarting ||
			poll.Metadata.AgentVersion != "0.1.1" || !slices.Contains(poll.Metadata.Capabilities, "agent-update-v1") {
			t.Errorf("health poll = %#v", poll)
			http.Error(response, "invalid", http.StatusBadRequest)
			return
		}
		pollReceived <- struct{}{}
		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(agentapi.AgentPollResult{
			AddressUploadReceipt: agentapi.AgentAddressUploadReceipt{
				AcceptedEventIds: []uuid.UUID{}, DiscardedEventIds: []uuid.UUID{},
				AcceptedGaps: []agentapi.AgentAddressGapReceipt{}, DiscardedGaps: []agentapi.AgentAddressGapReceipt{},
			},
			CenterVersion: "0.1.1", DesiredConfigurationRevision: 0, Enabled: true, PollIntervalSeconds: 30,
		})
	}))
	defer server.Close()

	stateDirectory := filepath.Join(t.TempDir(), "state")
	store, err := state.Open(stateDirectory)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.SaveIdentity(state.Identity{
		CenterURL: server.URL, NodeID: uuid.NewString(), Credential: "ipc_agent_health-test",
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	taskID := uuid.NewString()
	if _, err := store.AcceptAgentUpdate(state.AgentUpdateDelivery{
		ID: taskID, TargetVersion: "0.1.1", CreatedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Minute),
	}, "0.1.0", now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.BeginAgentUpdate(taskID, "0.1.0", now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkAgentUpdateInstalling(taskID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkAgentUpdateRestarting(taskID); err != nil {
		t.Fatal(err)
	}
	checkpoint := filepath.Join(stateDirectory, "update", "checkpoint-"+taskID)
	if err := os.MkdirAll(filepath.Join(checkpoint, "results"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"state.db", "master.key"} {
		contents, err := os.ReadFile(filepath.Join(stateDirectory, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(checkpoint, name), contents, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(checkpoint, "agent.previous"), []byte("old Agent"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(checkpoint, "complete"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	triggered := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		done <- RunWithOptions(ctx, store, "0.1.1", log.New(io.Discard, "", 0), RunOptions{
			UpdateConfig: &agentupdate.Config{
				InitSystem: "systemd", AgentPath: "/usr/local/bin/ipchronicle-agent", UpdaterPath: "/usr/local/libexec/ipchronicle-agent-updater",
			},
			UpdateTrigger: func(context.Context, string) error {
				triggered <- struct{}{}
				cancel()
				return nil
			},
		})
	}()
	select {
	case <-triggered:
	case <-time.After(3 * time.Second):
		t.Fatal("updated Agent did not publish health")
	}
	select {
	case <-pollReceived:
	default:
		t.Fatal("health was published before an authenticated control poll")
	}
	updateState, found, err := store.PendingAgentUpdate()
	if err != nil || !found || updateState.Status != "succeeded" {
		t.Fatalf("healthy update state = %#v, %v, %v", updateState, found, err)
	}
	marker, err := os.ReadFile(agentupdate.HealthMarkerPath(stateDirectory, taskID))
	if err != nil || strings.TrimSpace(string(marker)) != taskID {
		t.Fatalf("health marker = %q, %v", marker, err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Agent did not stop after test cancellation")
	}
}
