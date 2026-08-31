package state

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAgentUpdateLifecycleAndConfirmation(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agent"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, time.August, 10, 1, 2, 3, 0, time.UTC)
	delivery := AgentUpdateDelivery{
		ID: uuid.NewString(), TargetVersion: "0.1.1",
		CreatedAt: now.Add(-time.Second), ExpiresAt: now.Add(2 * time.Minute),
	}
	acknowledged, err := store.AcceptAgentUpdate(delivery, "0.1.0", now)
	if err != nil || acknowledged.Status != "acknowledged" {
		t.Fatalf("accept update = %#v, %v", acknowledged, err)
	}
	if _, err := store.AcceptAgentUpdate(delivery, "0.1.0", now); !errors.Is(err, ErrAgentUpdateHandled) {
		t.Fatalf("duplicate update error = %v", err)
	}
	verifying, err := store.BeginAgentUpdate(delivery.ID, "0.1.0", now.Add(time.Second))
	if err != nil || verifying.Status != "verifying" || verifying.PreviousVersion == nil || *verifying.PreviousVersion != "0.1.0" {
		t.Fatalf("begin update = %#v, %v", verifying, err)
	}
	if _, err := store.MarkAgentUpdateInstalling(delivery.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkAgentUpdateRestarting(delivery.ID); err != nil {
		t.Fatal(err)
	}
	succeeded, err := store.CommitAgentUpdateHealth(delivery.ID, "0.1.1", now.Add(2*time.Second))
	if err != nil || succeeded.Status != "succeeded" || succeeded.ResultVersion == nil || *succeeded.ResultVersion != "0.1.1" {
		t.Fatalf("commit health = %#v, %v", succeeded, err)
	}
	report, err := store.AgentUpdateControlReport()
	if err != nil || report == nil || report.Status != "succeeded" {
		t.Fatalf("control report = %#v, %v", report, err)
	}
	found, err := store.ConfirmTerminalAgentUpdate(delivery.ID, now.Add(3*time.Second))
	if err != nil || !found {
		t.Fatalf("confirm update = %v, %v", found, err)
	}
	if err := store.CleanupAgentUpdates(now.Add(25 * time.Hour)); err != nil {
		t.Fatal(err)
	}
	if report, err := store.AgentUpdateControlReport(); err != nil || report != nil {
		t.Fatalf("cleaned report = %#v, %v", report, err)
	}
}

func TestAgentUpdateRollbackPreservesPreviousVersion(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agent"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().UTC().Truncate(time.Second)
	id := uuid.NewString()
	if _, err := store.AcceptAgentUpdate(AgentUpdateDelivery{
		ID: id, TargetVersion: "0.2.0-rc.1", CreatedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Minute),
	}, "0.1.0", now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.BeginAgentUpdate(id, "0.1.0", now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkAgentUpdateInstalling(id); err != nil {
		t.Fatal(err)
	}
	rolledBack, err := store.RollbackAgentUpdate(id, "health-timeout", "new Agent did not report healthy", now.Add(time.Minute))
	if err != nil || rolledBack.Status != "rolled-back" || rolledBack.ResultVersion == nil || *rolledBack.ResultVersion != "0.1.0" {
		t.Fatalf("rollback = %#v, %v", rolledBack, err)
	}
}

func TestAgentUpdateAndCompleteProbeAreMutuallyExclusive(t *testing.T) {
	store, _ := openProbeTestStore(t, filepath.Join(t.TempDir(), "agent"), 1)
	defer store.Close()
	now := time.Now().UTC().Truncate(time.Second)
	run, err := store.StartProbeRun("schedule", nil, now.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	rejectedID := uuid.NewString()
	rejected, err := store.AcceptAgentUpdate(AgentUpdateDelivery{
		ID: rejectedID, TargetVersion: "0.1.1", CreatedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Minute),
	}, "0.1.0", now)
	if err != nil || rejected.Status != "rejected" || rejected.FailureCode == nil || *rejected.FailureCode != "probe-active" {
		t.Fatalf("update during probe = %#v, %v", rejected, err)
	}
	if _, err := store.ReconcileProbeRun(now); err != nil {
		t.Fatal(err)
	}
	if found, err := store.ConfirmTerminalAgentUpdate(rejectedID, now); err != nil || !found {
		t.Fatalf("confirm rejected update = %v, %v", found, err)
	}
	acceptedID := uuid.NewString()
	if _, err := store.AcceptAgentUpdate(AgentUpdateDelivery{
		ID: acceptedID, TargetVersion: "0.1.1", CreatedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Minute),
	}, "0.1.0", now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.StartProbeRun("schedule", nil, now.Add(time.Second)); !errors.Is(err, ErrProbeBusy) {
		t.Fatalf("probe during update error = %v", err)
	}
	_ = run
}
