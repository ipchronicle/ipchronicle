package nodes

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/center/database"
)

const (
	releaseCapacityNodes           = 70
	releaseCapacityEgressesPerNode = 6
)

func TestReleaseCapacity70Nodes420Egresses(t *testing.T) {
	if os.Getenv("IPCHRONICLE_RELEASE_CAPACITY") != "1" {
		t.Skip("set IPCHRONICLE_RELEASE_CAPACITY=1 to run the release capacity gate")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	startedAt := time.Now()
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)

	store, err := database.Open(ctx, database.PathsFromDataDirectory(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service := NewService(store.Config, store.History, store.ConfigQueries, store.MasterKey, &testSyncConnections{})
	service.now = func() time.Time { return now }
	enrollment, err := service.RotateEnrollmentKey(ctx, "UTC")
	if err != nil {
		t.Fatal(err)
	}

	type capacityNode struct {
		registration  Registration
		configuration Configuration
		egresses      []NetworkEgress
	}
	nodes := make([]capacityNode, 0, releaseCapacityNodes)
	for index := 0; index < releaseCapacityNodes; index++ {
		metadata := Metadata{
			Hostname: fmt.Sprintf("capacity-node-%02d", index+1), AgentVersion: "0.1.0-rc.1",
			OperatingSystem: "linux", Architecture: "amd64", PhysicalMemoryBytes: 256 * 1024 * 1024,
			Capabilities: []string{"control-v1", "configuration-v9", "complete-probe-v1", "agent-update-v1"},
		}
		registration, err := service.Register(ctx, enrollment.Key, metadata)
		if err != nil {
			t.Fatalf("register node %d: %v", index, err)
		}
		inventory := releaseCapacityInventory(index, now)
		if _, err := service.Poll(ctx, registration.Credential, metadata, 0, nil, nil, &inventory, nil); err != nil {
			t.Fatalf("poll inventory for node %d: %v", index, err)
		}
		configuration, err := service.Configuration(ctx, registration.Credential)
		if err != nil {
			t.Fatalf("read configuration for node %d: %v", index, err)
		}
		if len(configuration.DiscoveryPaths) != releaseCapacityEgressesPerNode {
			t.Fatalf("node %d discovery paths = %d, want %d", index, len(configuration.DiscoveryPaths), releaseCapacityEgressesPerNode)
		}
		states := make([]AddressState, 0, len(configuration.DiscoveryPaths))
		for pathIndex, path := range configuration.DiscoveryPaths {
			publicAddress := fmt.Sprintf("198.51.%d.%d", index+1, pathIndex+1)
			localAddress := fmt.Sprintf("10.0.%d.%d", index+1, pathIndex+10)
			if path.Family == "ipv6" {
				publicAddress = fmt.Sprintf("2606:4700:%x::%x", index+1, pathIndex+1)
				localAddress = fmt.Sprintf("fd00:%x::%x", index+1, pathIndex+10)
			}
			states = append(states, confirmedAddressState(
				path.ID, configuration.HistoryGeneration, path.Family,
				publicAddress, "eth0", localAddress, now,
			))
		}
		if _, err := service.Poll(
			ctx, registration.Credential, metadata, 0, nil, nil, nil, nil,
			AddressUpload{States: states},
		); err != nil {
			t.Fatalf("upload public addresses for node %d: %v", index, err)
		}
		configuration, err = service.Configuration(ctx, registration.Credential)
		if err != nil {
			t.Fatalf("read probe configuration for node %d: %v", index, err)
		}
		if len(configuration.ProbeTargets) != releaseCapacityEgressesPerNode {
			t.Fatalf("node %d probe targets = %d, want %d", index, len(configuration.ProbeTargets), releaseCapacityEgressesPerNode)
		}
		if _, err := service.Poll(
			ctx, registration.Credential, metadata, configuration.Revision, nil, nil, nil, nil,
		); err != nil {
			t.Fatalf("poll converged configuration for node %d: %v", index, err)
		}
		nodes = append(nodes, capacityNode{registration: registration, configuration: configuration, egresses: configuration.ProbeTargets})
	}
	listed, err := service.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != releaseCapacityNodes {
		t.Fatalf("listed nodes = %d, want %d", len(listed), releaseCapacityNodes)
	}

	for index, node := range nodes {
		for sequence, runStartedAt := range []time.Time{now.Add(-72 * time.Hour), now.Add(-time.Hour)} {
			releaseCapacityUploadRun(t, ctx, service, node.registration.Credential, node.configuration, node.egresses, int64(sequence+1), runStartedAt, index)
		}
	}

	maxAgeDays := int64(1)
	history, err := service.UpdateHistoryRetention(ctx, HistoryRetentionUpdate{Mode: "age", MaxAgeDays: &maxAgeDays})
	if err != nil {
		t.Fatal(err)
	}
	if history.Retention.LastCleanupDeletedItems < releaseCapacityNodes*releaseCapacityEgressesPerNode {
		t.Fatalf("retention deleted %d items, want at least %d", history.Retention.LastCleanupDeletedItems, releaseCapacityNodes*releaseCapacityEgressesPerNode)
	}
	if history.Usage.RecordCount < releaseCapacityNodes*releaseCapacityEgressesPerNode {
		t.Fatalf("history record count after cleanup = %d, want at least %d", history.Usage.RecordCount, releaseCapacityNodes*releaseCapacityEgressesPerNode)
	}
	t.Logf(
		"release capacity gate passed: nodes=%d egresses=%d retained-records=%d elapsed=%s",
		len(nodes), len(nodes)*releaseCapacityEgressesPerNode, history.Usage.RecordCount, time.Since(startedAt).Round(time.Millisecond),
	)
}

func releaseCapacityInventory(index int, capturedAt time.Time) NetworkInventory {
	thirdOctet := index + 1
	addresses := []NetworkAddress{
		{InterfaceName: "eth0", Address: fmt.Sprintf("10.0.%d.10", thirdOctet), PrefixLength: 24, Family: "ipv4", Scope: "private"},
		{InterfaceName: "eth0", Address: fmt.Sprintf("10.0.%d.11", thirdOctet), PrefixLength: 24, Family: "ipv4", Scope: "private"},
		{InterfaceName: "eth0", Address: fmt.Sprintf("10.0.%d.12", thirdOctet), PrefixLength: 24, Family: "ipv4", Scope: "private"},
		{InterfaceName: "eth0", Address: fmt.Sprintf("fd00:%x::10", thirdOctet), PrefixLength: 64, Family: "ipv6", Scope: "unique-local"},
	}
	gateway4 := "10.0.0.1"
	gateway6 := "fe80::1"
	inventory := NetworkInventory{
		CapturedAt: capturedAt,
		Interfaces: []NetworkInterface{{Name: "eth0", Index: 2, Up: true}},
		Addresses:  addresses,
		Routes: []NetworkRoute{
			{InterfaceName: "eth0", Family: "ipv4", Destination: "0.0.0.0/0", Gateway: &gateway4, Metric: 100, Default: true},
			{InterfaceName: "eth0", Family: "ipv6", Destination: "::/0", Gateway: &gateway6, Metric: 100, Default: true},
		},
	}
	return inventory
}

func releaseCapacityUploadRun(
	t *testing.T,
	ctx context.Context,
	service *Service,
	credential string,
	configuration Configuration,
	egresses []NetworkEgress,
	sequence int64,
	startedAt time.Time,
	nodeIndex int,
) {
	t.Helper()
	runID := uuid.New()
	run := ProbeRunArtifact{
		ID: runID, ConfigurationRevision: configuration.Revision, HistoryGeneration: configuration.HistoryGeneration,
		Trigger: "schedule", StartedAt: startedAt, Status: "running",
		Executions: make([]ProbeExecutionManifest, len(egresses)),
	}
	executions := make([]ProbeExecutionArtifact, len(egresses))
	for index, egress := range egresses {
		executionID := uuid.New()
		executionStartedAt := startedAt.Add(time.Duration(index+1) * time.Second)
		executionCompletedAt := executionStartedAt.Add(time.Second)
		run.Executions[index] = ProbeExecutionManifest{
			ID: executionID, EgressID: egress.ID, Ordinal: int64(index), Sequence: sequence,
		}
		executions[index] = ProbeExecutionArtifact{
			ID: executionID, EgressID: egress.ID, Ordinal: int64(index), Sequence: sequence,
			Status: "succeeded", StartedAt: &executionStartedAt, CompletedAt: &executionCompletedAt,
			RawResult: []byte(fmt.Sprintf(`{"Head":{"IP":"198.51.%d.%d"}}`, sequence+99, nodeIndex+1)),
		}
	}
	if _, err := service.UploadProbeArtifact(ctx, credential, ProbeArtifact{ID: run.ID, Revision: 1, Run: &run}); err != nil {
		t.Fatalf("upload running probe run for node %d sequence %d: %v", nodeIndex, sequence, err)
	}
	for index := range executions {
		artifact := ProbeArtifact{ID: executions[index].ID, Revision: 1, Run: &run, Execution: &executions[index]}
		for attempt := 0; attempt < 2; attempt++ {
			if _, err := service.UploadProbeArtifact(ctx, credential, artifact); err != nil {
				t.Fatalf("upload probe execution for node %d sequence %d attempt %d: %v", nodeIndex, sequence, attempt+1, err)
			}
		}
	}
	completedAt := startedAt.Add(time.Duration(len(egresses)+2) * time.Second)
	run.Status = "succeeded"
	run.CompletedAt = &completedAt
	if _, err := service.UploadProbeArtifact(ctx, credential, ProbeArtifact{ID: run.ID, Revision: 2, Run: &run}); err != nil {
		t.Fatalf("upload terminal probe run for node %d sequence %d: %v", nodeIndex, sequence, err)
	}
}
