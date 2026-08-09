package state

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	bolt "go.etcd.io/bbolt"
)

func TestProbeRunReconcilesWithoutRerunningAfterRestart(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "agent")
	store, _ := openProbeTestStore(t, directory, 2)
	startedAt := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	run, err := store.StartProbeRun("schedule", nil, nil, startedAt)
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.StartProbeExecution(run.ID, run.Executions[0].ID, startedAt.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompleteProbeExecution(run.ID, first.ID, ProbeExecutionOutcome{
		Status: "succeeded", StartedAt: first.StartedAt, CompletedAt: startedAt.Add(2 * time.Second),
		RawResult: []byte(`{"ip":"203.0.113.10"}`),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	restarted, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	finished, err := restarted.ReconcileProbeRun(startedAt.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if finished == nil || finished.Status != "partial" || finished.CompletedAt == nil {
		t.Fatalf("reconciled run = %#v", finished)
	}
	_, executions, err := restarted.ProbeRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if executions[0].Status != "succeeded" || executions[1].Status != "skipped" ||
		executions[1].FailureStage == nil || *executions[1].FailureStage != "restart" {
		t.Fatalf("reconciled executions = %#v", executions)
	}
	if _, err := restarted.StartProbeExecution(run.ID, run.Executions[1].ID, startedAt.Add(2*time.Minute)); !errors.Is(err, ErrInvalidProbeState) {
		t.Fatalf("restarting an execution error = %v", err)
	}
}

func TestProbeQueueEvictsOldestPerEgressAndKeepsGap(t *testing.T) {
	store, egresses := openProbeTestStore(t, filepath.Join(t.TempDir(), "agent"), 1)
	defer store.Close()
	start := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	var firstResult string
	for index := 0; index < maxPendingProbeResultsPerEgress+1; index++ {
		at := start.Add(time.Duration(index) * time.Minute)
		run, err := store.StartProbeRun("schedule", nil, nil, at)
		if err != nil {
			t.Fatal(err)
		}
		execution, err := store.StartProbeExecution(run.ID, run.Executions[0].ID, at.Add(time.Second))
		if err != nil {
			t.Fatal(err)
		}
		completed, err := store.CompleteProbeExecution(run.ID, execution.ID, ProbeExecutionOutcome{
			Status: "succeeded", StartedAt: execution.StartedAt, CompletedAt: at.Add(2 * time.Second),
			RawResult: []byte(`{"ok":true}`),
		})
		if err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			firstResult = completed.ResultFile
		}
		if _, err := store.FinishProbeRun(run.ID, at.Add(3*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(store.resultDirectory, firstResult)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("oldest result file still exists: %v", err)
	}

	var queuedExecutions int
	var gap ProbeGap
	if err := store.database.View(func(transaction *bolt.Tx) error {
		queue := transaction.Bucket(probeArtifactsBucket)
		if err := queue.ForEach(func(_, encoded []byte) error {
			var item queuedProbeArtifact
			if err := decodeJSON(encoded, &item, "queued probe artifact"); err != nil {
				return err
			}
			if item.Kind == "execution" {
				queuedExecutions++
			}
			return nil
		}); err != nil {
			return err
		}
		return transaction.Bucket(probeGapsBucket).ForEach(func(_, encoded []byte) error {
			return decodeProbeGap(encoded, &gap)
		})
	}); err != nil {
		t.Fatal(err)
	}
	if queuedExecutions != maxPendingProbeResultsPerEgress || gap.EgressID != egresses[0] ||
		gap.DroppedCount != 1 || gap.FirstSequence != 1 || gap.LastSequence != 1 {
		t.Fatalf("queue count = %d, gap = %#v", queuedExecutions, gap)
	}
}

func TestMissingProbeResultBecomesGapOnOpen(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "agent")
	store, egresses := openProbeTestStore(t, directory, 1)
	start := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	run, err := store.StartProbeRun("schedule", nil, nil, start)
	if err != nil {
		t.Fatal(err)
	}
	execution, err := store.StartProbeExecution(run.ID, run.Executions[0].ID, start.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	completed, err := store.CompleteProbeExecution(run.ID, execution.ID, ProbeExecutionOutcome{
		Status: "succeeded", StartedAt: execution.StartedAt, CompletedAt: start.Add(2 * time.Second),
		RawResult: []byte(`{"ok":true}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.FinishProbeRun(run.ID, start.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(directory, "results", completed.ResultFile)); err != nil {
		t.Fatal(err)
	}

	restarted, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	artifact, err := restarted.NextProbeArtifact()
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Gap == nil || artifact.Gap.EgressID != egresses[0] || artifact.Gap.DroppedCount != 1 {
		t.Fatalf("artifact after missing result = %#v", artifact)
	}
}

func TestProbeTaskDeliveryIsDeduplicated(t *testing.T) {
	store, _ := openProbeTestStore(t, filepath.Join(t.TempDir(), "agent"), 1)
	defer store.Close()
	start := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	task := ProbeTaskDelivery{ID: uuid.NewString(), CreatedAt: start, ExpiresAt: start.Add(2 * time.Minute)}
	run, err := store.StartProbeRun("manual", &task, nil, start)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.StartProbeRun("manual", &task, nil, start.Add(time.Second)); !errors.Is(err, ErrProbeTaskHandled) {
		t.Fatalf("duplicate active task error = %v", err)
	}
	execution, err := store.StartProbeExecution(run.ID, run.Executions[0].ID, start.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompleteProbeExecution(run.ID, execution.ID, ProbeExecutionOutcome{
		Status: "failed", StartedAt: execution.StartedAt, CompletedAt: start.Add(2 * time.Second),
		FailureStage: "process", Diagnostic: "exit status 1",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.FinishProbeRun(run.ID, start.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.StartProbeRun("manual", &task, nil, start.Add(4*time.Second)); !errors.Is(err, ErrProbeTaskHandled) {
		t.Fatalf("duplicate terminal task error = %v", err)
	}
}

func TestHistoryGenerationChangeDiscardsQueuedProbeArtifactsAndFinishesActiveRun(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "agent")
	store, egresses := openProbeTestStore(t, directory, 1)
	defer store.Close()
	start := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	if _, err := store.RecordAddressObservation(AddressObservation{
		EgressID: egresses[0], ConfigurationRevision: 1, HistoryGeneration: testHistoryGeneration,
		Family: "ipv4", FailureReason: "no-valid-response", CheckedAt: start,
	}); err != nil {
		t.Fatal(err)
	}
	run, err := store.StartProbeRun("schedule", nil, nil, start)
	if err != nil {
		t.Fatal(err)
	}
	execution, err := store.StartProbeExecution(run.ID, run.Executions[0].ID, start.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	completed, err := store.CompleteProbeExecution(run.ID, execution.ID, ProbeExecutionOutcome{
		Status: "succeeded", StartedAt: execution.StartedAt, CompletedAt: start.Add(2 * time.Second),
		RawResult: []byte(`{"ok":true}`),
	})
	if err != nil {
		t.Fatal(err)
	}

	configuration, err := store.Configuration()
	if err != nil {
		t.Fatal(err)
	}
	configuration.Revision++
	configuration.HistoryGeneration = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	if err := store.ApplyConfiguration(configuration); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(directory, "results", completed.ResultFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("obsolete result file still exists: %v", err)
	}
	status, _, err := store.ProbeControlReport()
	if err != nil {
		t.Fatal(err)
	}
	if status.HistoryResetGeneration == nil || *status.HistoryResetGeneration != configuration.HistoryGeneration ||
		status.HistoryResetAt == nil || status.HistoryResetDiscardedAddressItems != 2 ||
		status.HistoryResetDiscardedProbeItems != 2 {
		t.Fatalf("history reset status = %#v", status)
	}
	if _, err := store.FinishProbeRun(run.ID, start.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	status, _, err = store.ProbeControlReport()
	if err != nil {
		t.Fatal(err)
	}
	if status.ActiveRunID != nil || status.HistoryResetDiscardedProbeItems != 3 {
		t.Fatalf("finished obsolete run status = %#v", status)
	}
	if _, _, err := store.ProbeRun(run.ID); !errors.Is(err, ErrProbeRunNotFound) {
		t.Fatalf("obsolete completed run remains locally: %v", err)
	}
	artifact, err := store.NextProbeArtifact()
	if err != nil || artifact.ID != "" {
		t.Fatalf("obsolete artifact remains queued: %#v, %v", artifact, err)
	}
}

func TestProbeArtifactAcknowledgementIsRevisionAware(t *testing.T) {
	store, _ := openProbeTestStore(t, filepath.Join(t.TempDir(), "agent"), 1)
	defer store.Close()
	start := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	run, err := store.StartProbeRun("schedule", nil, nil, start)
	if err != nil {
		t.Fatal(err)
	}
	execution, err := store.StartProbeExecution(run.ID, run.Executions[0].ID, start.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompleteProbeExecution(run.ID, execution.ID, ProbeExecutionOutcome{
		Status: "failed", StartedAt: execution.StartedAt, CompletedAt: start.Add(2 * time.Second),
		FailureStage: "process", Diagnostic: "exit status 1",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.FinishProbeRun(run.ID, start.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	artifact, err := store.NextProbeArtifact()
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Revision < 2 {
		t.Fatalf("artifact revision = %d", artifact.Revision)
	}
	if err := store.AcknowledgeProbeArtifact(ProbeArtifactReceipt{ID: artifact.ID, Revision: artifact.Revision - 1}); err != nil {
		t.Fatal(err)
	}
	retained, err := store.NextProbeArtifact()
	if err != nil || retained.ID != artifact.ID || retained.Revision != artifact.Revision {
		t.Fatalf("artifact after stale receipt = %#v, %v", retained, err)
	}
	if err := store.AcknowledgeProbeArtifact(ProbeArtifactReceipt{ID: artifact.ID, Revision: artifact.Revision + 1}); !errors.Is(err, ErrInvalidProbeState) {
		t.Fatalf("future receipt error = %v", err)
	}
	if err := store.AcknowledgeProbeArtifact(ProbeArtifactReceipt{ID: artifact.ID, Revision: artifact.Revision}); err != nil {
		t.Fatal(err)
	}
}

func TestDecodeJSONRejectsTrailingValue(t *testing.T) {
	var target queuedProbeArtifact
	if err := decodeJSON([]byte(`{"kind":"run","revision":1} {}`), &target, "test value"); err == nil {
		t.Fatal("JSON with a trailing value was accepted")
	}
}

func TestOpenRejectsInvalidProbeStatus(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "agent")
	store, _ := openProbeTestStore(t, directory, 1)
	if err := store.database.Update(func(transaction *bolt.Tx) error {
		return transaction.Bucket(probeControlBucket).Put(probeStatusKey, []byte(`{"lastOccurrenceAt":"2026-08-09T12:00:00Z"}`))
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(directory); err == nil {
		t.Fatal("opening Agent state with an incomplete probe status unexpectedly succeeded")
	}
}

func openProbeTestStore(t *testing.T, directory string, egressCount int) (*Store, []string) {
	t.Helper()
	store, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveIdentity(Identity{
		CenterURL: "https://center.example", NodeID: uuid.NewString(), Credential: "ipc_agent_secret-probe-test",
	}); err != nil {
		t.Fatal(err)
	}
	configuration := Configuration{
		SchemaVersion: 5, Revision: 1, Enabled: true, HistoryGeneration: testHistoryGeneration,
		ProbeSchedule:     ProbeSchedule{Enabled: true, Cron: "0 0 0 * * *", Timezone: "agent-local"},
		DiscoveryServices: testDiscoveryServices(),
	}
	var egresses []string
	for index := 0; index < egressCount; index++ {
		id := uuid.NewString()
		egresses = append(egresses, id)
		family := "ipv4"
		if index%2 == 1 {
			family = "ipv6"
		}
		configuration.Egresses = append(configuration.Egresses, Egress{
			ID: id, Kind: "default", Family: family, Enabled: true,
			LightweightIntervalSeconds: 600, ProbeOnAddressChange: true,
		})
	}
	if err := store.ApplyConfiguration(configuration); err != nil {
		t.Fatal(err)
	}
	return store, egresses
}
