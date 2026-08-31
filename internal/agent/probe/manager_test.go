package probe

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/agent/state"
)

func TestManagerRunsFrozenEgressesSequentiallyAndRejectsOverlaps(t *testing.T) {
	store, configuration := openManagerTestStore(t, 2)
	defer store.Close()
	runner := &blockingRunner{started: make(chan string, 2), release: make(chan struct{}, 2)}
	manager := NewManager(store, minimumProbeMemoryBytes, nil)
	manager.runner = runner
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	manager.now = func() time.Time { return now }

	if err := manager.tryStart(context.Background(), "schedule", nil, now); err != nil {
		t.Fatal(err)
	}
	if actual := receiveWithin(t, runner.started); actual != configuration.ProbeTargets[0].ID {
		t.Fatalf("first execution egress = %s", actual)
	}
	status, _, err := store.ProbeControlReport()
	if err != nil || status.ActiveRunID == nil {
		t.Fatalf("active status = %#v, %v", status, err)
	}
	runID := *status.ActiveRunID
	task := state.ProbeTaskDelivery{ID: uuid.NewString(), Trigger: "manual", PublicAddressIDs: []string{configuration.ProbeTargets[0].ID}, CreatedAt: now, ExpiresAt: now.Add(2 * time.Minute)}
	if err := manager.AcceptTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	report, exists, err := store.ProbeTask(task.ID)
	if err != nil || !exists || report.Status != "rejected" || report.RejectionReason == nil || *report.RejectionReason != "busy" {
		t.Fatalf("overlapping task report = %#v, %v", report, err)
	}

	runner.release <- struct{}{}
	if actual := receiveWithin(t, runner.started); actual != configuration.ProbeTargets[1].ID {
		t.Fatalf("second execution egress = %s", actual)
	}
	runner.release <- struct{}{}
	waitManagerIdle(t, manager)
	run, executions, err := store.ProbeRun(runID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != "succeeded" || len(executions) != 2 || executions[0].Status != "succeeded" || executions[1].Status != "succeeded" {
		t.Fatalf("completed run = %#v, executions = %#v", run, executions)
	}
}

func TestManagerPausesLowMemoryUntilOverride(t *testing.T) {
	store, configuration := openManagerTestStore(t, 1)
	defer store.Close()
	runner := &blockingRunner{started: make(chan string, 1), release: make(chan struct{}, 1)}
	manager := NewManager(store, 64*1024*1024, nil)
	manager.runner = runner
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	manager.now = func() time.Time { return now }
	task := state.ProbeTaskDelivery{ID: uuid.NewString(), Trigger: "manual", PublicAddressIDs: []string{configuration.ProbeTargets[0].ID}, CreatedAt: now, ExpiresAt: now.Add(2 * time.Minute)}
	if err := manager.AcceptTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	report, _, err := store.ProbeTask(task.ID)
	if err != nil || report.RejectionReason == nil || *report.RejectionReason != "low-memory" {
		t.Fatalf("low-memory report = %#v, %v", report, err)
	}
	if err := store.ConfirmTerminalProbeTask(task.ID, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}

	configuration.Revision++
	configuration.ProbeLowMemoryOverride = true
	if err := store.ApplyConfiguration(configuration); err != nil {
		t.Fatal(err)
	}
	second := state.ProbeTaskDelivery{ID: uuid.NewString(), Trigger: "manual", PublicAddressIDs: []string{configuration.ProbeTargets[0].ID}, CreatedAt: now, ExpiresAt: now.Add(2 * time.Minute)}
	if err := manager.AcceptTask(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	if actual := receiveWithin(t, runner.started); actual != configuration.ProbeTargets[0].ID {
		t.Fatalf("override execution egress = %s", actual)
	}
	runner.release <- struct{}{}
	waitManagerIdle(t, manager)
}

func TestManagerSkipsPersistedMissedScheduleWithoutCatchUp(t *testing.T) {
	store, _ := openManagerTestStore(t, 1)
	defer store.Close()
	manager := NewManager(store, minimumProbeMemoryBytes, nil)
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	manager.now = func() time.Time { return now }
	past := now.Add(-time.Hour)
	if err := store.SetNextProbeSchedule(&past); err != nil {
		t.Fatal(err)
	}
	fingerprint := ""
	var next *time.Time
	initialized := false
	if err := manager.reconcileSchedule(context.Background(), &fingerprint, &next, &initialized); err != nil {
		t.Fatal(err)
	}
	if next == nil || !next.After(now) {
		t.Fatalf("next schedule = %v", next)
	}
	status, _, err := store.ProbeControlReport()
	if err != nil || status.LastOccurrenceStatus == nil || *status.LastOccurrenceStatus != "skipped" ||
		status.LastSkipReason == nil || *status.LastSkipReason != "missed" {
		t.Fatalf("missed schedule status = %#v, %v", status, err)
	}
	if manager.active {
		t.Fatal("missed schedule started a catch-up run")
	}
}

func TestManagerCleansConfirmedTasksHourly(t *testing.T) {
	store, configuration := openManagerTestStore(t, 1)
	defer store.Close()
	manager := NewManager(store, minimumProbeMemoryBytes, nil)
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	manager.now = func() time.Time { return now }
	task := state.ProbeTaskDelivery{ID: uuid.NewString(), Trigger: "manual", PublicAddressIDs: []string{configuration.ProbeTargets[0].ID}, CreatedAt: now, ExpiresAt: now.Add(2 * time.Minute)}
	if _, err := store.RejectProbeTask(task, "disabled", now); err != nil {
		t.Fatal(err)
	}
	if err := store.ConfirmTerminalProbeTask(task.ID, now); err != nil {
		t.Fatal(err)
	}
	next := now.Add(probeTaskCleanupInterval)
	now = now.Add(25 * time.Hour)
	if err := manager.cleanupTasksIfDue(&next); err != nil {
		t.Fatal(err)
	}
	if _, exists, err := store.ProbeTask(task.ID); err != nil || exists {
		t.Fatalf("cleaned task exists = %v, error = %v", exists, err)
	}
}

func openManagerTestStore(t *testing.T, egressCount int) (*state.Store, state.Configuration) {
	t.Helper()
	store, err := state.Open(filepath.Join(t.TempDir(), "agent"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveIdentity(state.Identity{
		CenterURL: "https://center.example", NodeID: uuid.NewString(), Credential: "ipc_agent_secret-manager-test",
	}); err != nil {
		t.Fatal(err)
	}
	configuration := state.Configuration{
		SchemaVersion: 7, Revision: 1, Enabled: true,
		HistoryGeneration: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ProbeSchedule:     state.ProbeSchedule{Enabled: true, Cron: "0 0 0 * * *", Timezone: "UTC"},
		DiscoveryServices: state.DiscoveryServices{
			IPv4: []string{"https://one.example/ip", "https://two.example/ip"},
			IPv6: []string{"https://six-one.example/ip", "https://six-two.example/ip"},
		},
	}
	for index := 0; index < egressCount; index++ {
		family := "ipv4"
		publicAddress := "8.8.8.8"
		if index%2 == 1 {
			family = "ipv6"
			publicAddress = "2606:4700:4700::1111"
		}
		pathID := uuid.NewString()
		configuration.ProbeTargets = append(configuration.ProbeTargets, state.Egress{
			ID: uuid.NewString(), PathID: &pathID, PublicAddress: &publicAddress,
			Kind: "default", Family: family, Enabled: true,
		})
	}
	if err := store.ApplyConfiguration(configuration); err != nil {
		t.Fatal(err)
	}
	return store, configuration
}

type blockingRunner struct {
	started chan string
	release chan struct{}
	mu      sync.Mutex
	calls   []string
}

func (runner *blockingRunner) Run(
	ctx context.Context,
	_ state.Configuration,
	egress state.Egress,
	startedAt time.Time,
) (state.ProbeExecutionOutcome, error) {
	runner.mu.Lock()
	runner.calls = append(runner.calls, egress.ID)
	runner.mu.Unlock()
	runner.started <- egress.ID
	select {
	case <-ctx.Done():
		return state.ProbeExecutionOutcome{
			Status: "interrupted", StartedAt: &startedAt, CompletedAt: startedAt, FailureStage: "process",
		}, nil
	case <-runner.release:
		return state.ProbeExecutionOutcome{
			Status: "succeeded", StartedAt: &startedAt, CompletedAt: startedAt, RawResult: []byte(`{"ok":true}`),
		}, nil
	}
}

func receiveWithin(t *testing.T, values <-chan string) string {
	t.Helper()
	select {
	case value := <-values:
		return value
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for probe execution")
		return ""
	}
}

func waitManagerIdle(t *testing.T, manager *Manager) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		manager.mu.Lock()
		active := manager.active
		manager.mu.Unlock()
		if !active {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal(errors.New("probe manager did not become idle"))
}
