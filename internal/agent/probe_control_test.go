package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/agent/state"
	"github.com/ipchronicle/ipchronicle/internal/generated/agentapi"
)

func TestPollCarriesProbeStatusAndTaskReportAndAcceptsTask(t *testing.T) {
	oldTaskID := uuid.New()
	newTaskID := uuid.New()
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	var received agentapi.AgentPollRequest
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/agent/control" {
			http.NotFound(response, request)
			return
		}
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Error(err)
			response.WriteHeader(http.StatusBadRequest)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(agentapi.AgentPollResult{
			CenterVersion: "test", DesiredConfigurationRevision: 1, Enabled: true, PollIntervalSeconds: 30,
			AddressUploadReceipt: agentapi.AgentAddressUploadReceipt{
				AcceptedEventIds: []uuid.UUID{}, DiscardedEventIds: []uuid.UUID{},
				AcceptedGaps: []agentapi.AgentAddressGapReceipt{}, DiscardedGaps: []agentapi.AgentAddressGapReceipt{},
			},
			AcceptedTerminalTaskId: &oldTaskID,
			Task: &agentapi.AgentTask{
				Id: newTaskID, Kind: agentapi.CompleteProbe, CreatedAt: now, ExpiresAt: now.Add(2 * time.Minute),
			},
		})
	}))
	defer server.Close()
	store, identity, configuration := openProbeControlTestStore(t, server.URL)
	defer store.Close()
	if _, err := store.RejectProbeTask(state.ProbeTaskDelivery{
		ID: oldTaskID.String(), CreatedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Minute),
	}, "busy", now); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordSkippedProbe("schedule", "busy", now); err != nil {
		t.Fatal(err)
	}
	client, err := NewControlClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	controlState, err := store.ControlState()
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := client.poll(context.Background(), store, identity, agentapi.AgentMetadata{
		Hostname: "node", AgentVersion: "test", OperatingSystem: agentapi.Linux,
		Architecture: agentapi.Amd64, Capabilities: []string{"complete-probe-v1"}, PhysicalMemoryBytes: minimumTestMemory,
	}, controlState, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.probeTask == nil || outcome.probeTask.ID != newTaskID.String() || outcome.probeTask.ExpiresAt != now.Add(2*time.Minute) {
		t.Fatalf("poll task = %#v", outcome.probeTask)
	}
	if received.ProbeStatus == nil || received.ProbeStatus.LastSkipReason == nil ||
		*received.ProbeStatus.LastSkipReason != agentapi.AgentProbeSkipReason("busy") {
		t.Fatalf("probe status = %#v", received.ProbeStatus)
	}
	if received.TaskReport == nil || received.TaskReport.Id != oldTaskID ||
		received.TaskReport.Status != agentapi.AgentTaskReportStatusRejected {
		t.Fatalf("task report = %#v", received.TaskReport)
	}
	if err := store.CleanupProbeTasks(time.Now().UTC().Add(25 * time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, exists, err := store.ProbeTask(oldTaskID.String()); err != nil || exists {
		t.Fatalf("confirmed task remains retained: exists=%v, err=%v", exists, err)
	}
	if configuration.Revision != controlState.AppliedConfigurationRevision {
		t.Fatalf("configuration revision = %d, control = %d", configuration.Revision, controlState.AppliedConfigurationRevision)
	}
}

func TestProbeUploaderRetransmitsArtifactsWithoutExecuting(t *testing.T) {
	var received []agentapi.AgentProbeArtifact
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/agent/probe-artifacts" {
			http.NotFound(response, request)
			return
		}
		var artifact agentapi.AgentProbeArtifact
		if err := json.NewDecoder(request.Body).Decode(&artifact); err != nil {
			t.Error(err)
			response.WriteHeader(http.StatusBadRequest)
			return
		}
		received = append(received, artifact)
		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(agentapi.AgentProbeArtifactReceipt{
			ArtifactId: artifact.ArtifactId, Revision: artifact.Revision,
			Disposition: agentapi.AgentProbeArtifactDisposition("accepted"),
		})
	}))
	defer server.Close()
	store, identity, _ := openProbeControlTestStore(t, server.URL)
	defer store.Close()
	startedAt := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	run, err := store.StartProbeRun("schedule", nil, nil, startedAt)
	if err != nil {
		t.Fatal(err)
	}
	execution, err := store.StartProbeExecution(run.ID, run.Executions[0].ID, startedAt.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompleteProbeExecution(run.ID, execution.ID, state.ProbeExecutionOutcome{
		Status: "succeeded", StartedAt: execution.StartedAt, CompletedAt: startedAt.Add(2 * time.Second),
		RawResult: []byte(`{"field":{"value":1}}`),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.FinishProbeRun(run.ID, startedAt.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	client, err := NewControlClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	for {
		found, err := client.uploadNextProbeArtifact(context.Background(), store, identity)
		if err != nil {
			t.Fatal(err)
		}
		if !found {
			break
		}
	}
	if len(received) < 2 {
		t.Fatalf("uploaded artifacts = %#v", received)
	}
	foundResult := false
	for _, artifact := range received {
		if artifact.Execution != nil && artifact.Execution.Status == agentapi.ProbeExecutionStatusSucceeded {
			foundResult = artifact.Execution.RawResult != nil && string(*artifact.Execution.RawResult) == `{"field":{"value":1}}`
		}
	}
	if !foundResult {
		t.Fatalf("successful result was not uploaded: %#v", received)
	}
	remaining, err := store.NextProbeArtifact()
	if err != nil || remaining.ID != "" {
		t.Fatalf("artifact remains after receipts: %#v, %v", remaining, err)
	}
}

const minimumTestMemory = 256 * 1024 * 1024

func openProbeControlTestStore(t *testing.T, centerURL string) (*state.Store, state.Identity, state.Configuration) {
	t.Helper()
	store, err := state.Open(filepath.Join(t.TempDir(), "agent"))
	if err != nil {
		t.Fatal(err)
	}
	identity := state.Identity{
		CenterURL: centerURL, NodeID: uuid.NewString(), Credential: "ipc_agent_secret-probe-control-test",
	}
	if err := store.SaveIdentity(identity); err != nil {
		t.Fatal(err)
	}
	configuration := state.Configuration{
		SchemaVersion: 5, Revision: 1, Enabled: true,
		HistoryGeneration: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ProbeSchedule:     state.ProbeSchedule{Enabled: true, Cron: "0 0 0 * * *", Timezone: "agent-local"},
		DiscoveryServices: state.DiscoveryServices{
			IPv4: []string{"https://one.example/ip", "https://two.example/ip"},
			IPv6: []string{"https://six-one.example/ip", "https://six-two.example/ip"},
		},
		Egresses: []state.Egress{{
			ID: uuid.NewString(), Kind: "default", Family: "ipv4", Enabled: true,
			LightweightIntervalSeconds: 600, ProbeOnAddressChange: true,
		}},
	}
	if err := store.ApplyConfiguration(configuration); err != nil {
		t.Fatal(err)
	}
	identity.AppliedConfigurationRevision = configuration.Revision
	return store, identity, configuration
}
