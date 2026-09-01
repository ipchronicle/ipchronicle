package nodes

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/center/database/historydb"
	centerhistory "github.com/ipchronicle/ipchronicle/internal/center/history"
)

func TestAgeRetentionProtectsStarredAndCurrentSnapshots(t *testing.T) {
	fixture := newProbeServiceFixture(t, 512*1024*1024)
	egressID := fixture.egresses[0].ID.String()
	oldUnstarred := seedRetentionSnapshot(t, fixture, egressID, 1, fixture.now.Add(-72*time.Hour), 128)
	oldStarred := seedRetentionSnapshot(t, fixture, egressID, 2, fixture.now.Add(-48*time.Hour), 128)
	current := seedRetentionSnapshot(t, fixture, egressID, 3, fixture.now.Add(-time.Hour), 128)
	if err := centerhistory.Advance(
		fixture.ctx, fixture.store.HistoryQueries, fixture.registration.NodeID.String(),
		egressID, fixture.configuration.HistoryGeneration, fixture.now.Unix(),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.HistoryQueries.StarProbeSnapshot(fixture.ctx, historydb.StarProbeSnapshotParams{
		SnapshotID: oldStarred, StarredAt: fixture.now.Unix(),
	}); err != nil {
		t.Fatal(err)
	}
	setCurrentRetentionSnapshot(t, fixture, egressID, current, 3)

	ageDays := int64(1)
	state, err := fixture.service.UpdateHistoryRetention(fixture.ctx, HistoryRetentionUpdate{
		Mode: "age", MaxAgeDays: &ageDays,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.HistoryQueries.GetProbeSnapshot(fixture.ctx, oldUnstarred); err == nil {
		t.Fatal("old unstarred snapshot survived age cleanup")
	}
	for _, id := range []string{oldStarred, current} {
		if _, err := fixture.store.HistoryQueries.GetProbeSnapshot(fixture.ctx, id); err != nil {
			t.Fatalf("protected snapshot %s was deleted: %v", id, err)
		}
		if _, err := fixture.store.HistoryQueries.GetProbeChangeSetBySnapshot(fixture.ctx, id); err != nil {
			t.Fatalf("protected snapshot %s lost its change set: %v", id, err)
		}
		if _, err := fixture.store.HistoryQueries.GetProbeSnapshotFormat(fixture.ctx, id); err != nil {
			t.Fatalf("protected snapshot %s lost its format interpretation: %v", id, err)
		}
	}
	if state.Retention.LastCleanupAt == nil || state.Retention.LastCleanupDeletedItems == 0 || state.Usage.DatabaseBytes == 0 {
		t.Fatalf("history state after cleanup = %#v", state)
	}
}

func TestSizeRetentionReportsProtectedOverage(t *testing.T) {
	fixture := newProbeServiceFixture(t, 512*1024*1024)
	egressID := fixture.egresses[0].ID.String()
	starred := seedRetentionSnapshot(t, fixture, egressID, 1, fixture.now.Add(-2*time.Hour), 600*1024)
	current := seedRetentionSnapshot(t, fixture, egressID, 2, fixture.now.Add(-time.Hour), 600*1024)
	if _, err := fixture.store.HistoryQueries.StarProbeSnapshot(fixture.ctx, historydb.StarProbeSnapshotParams{
		SnapshotID: starred, StarredAt: fixture.now.Unix(),
	}); err != nil {
		t.Fatal(err)
	}
	setCurrentRetentionSnapshot(t, fixture, egressID, current, 2)

	budget := int64(1024 * 1024)
	state, err := fixture.service.UpdateHistoryRetention(fixture.ctx, HistoryRetentionUpdate{
		Mode: "size", MaxLogicalBytes: &budget,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !state.Usage.OverBudget || state.Usage.OverageBytes <= 0 || state.Usage.ProtectedLogicalBytes <= budget {
		t.Fatalf("protected overage was not reported: %#v", state.Usage)
	}
	for _, id := range []string{starred, current} {
		if _, err := fixture.store.HistoryQueries.GetProbeSnapshot(fixture.ctx, id); err != nil {
			t.Fatalf("protected snapshot %s was deleted: %v", id, err)
		}
	}
}

func TestSizeRetentionStopsWhenLogicalBudgetIsReached(t *testing.T) {
	fixture := newProbeServiceFixture(t, 512*1024*1024)
	egressID := fixture.egresses[0].ID.String()
	ids := make([]string, 5)
	for index := range ids {
		ids[index] = seedRetentionSnapshot(
			t, fixture, egressID, int64(index+1),
			fixture.now.Add(time.Duration(index-5)*time.Hour), 400*1024,
		)
	}
	setCurrentRetentionSnapshot(t, fixture, egressID, ids[4], 5)

	budget := int64(1024 * 1024)
	state, err := fixture.service.UpdateHistoryRetention(fixture.ctx, HistoryRetentionUpdate{
		Mode: "size", MaxLogicalBytes: &budget,
	})
	if err != nil {
		t.Fatal(err)
	}
	if state.Usage.LogicalBytes > budget {
		t.Fatalf("logical usage = %d, want at most %d", state.Usage.LogicalBytes, budget)
	}
	for index, id := range ids {
		_, err := fixture.store.HistoryQueries.GetProbeSnapshot(fixture.ctx, id)
		if index < 3 && err == nil {
			t.Fatalf("old snapshot %d survived size cleanup", index+1)
		}
		if index >= 3 && err != nil {
			t.Fatalf("snapshot %d was deleted after reaching size budget: %v", index+1, err)
		}
	}
}

func TestRetentionProtectsPendingComparisonBaseline(t *testing.T) {
	fixture := newProbeServiceFixture(t, 512*1024*1024)
	egressID := fixture.egresses[0].ID.String()
	baseline := seedRetentionSnapshot(t, fixture, egressID, 1, fixture.now.Add(-72*time.Hour), 128)
	if err := centerhistory.Advance(
		fixture.ctx, fixture.store.HistoryQueries, fixture.registration.NodeID.String(),
		egressID, fixture.configuration.HistoryGeneration, fixture.now.Unix(),
	); err != nil {
		t.Fatal(err)
	}
	current := seedRetentionSnapshot(t, fixture, egressID, 2, fixture.now.Add(-time.Hour), 128)
	setCurrentRetentionSnapshot(t, fixture, egressID, current, 2)

	ageDays := int64(1)
	state, err := fixture.service.UpdateHistoryRetention(fixture.ctx, HistoryRetentionUpdate{
		Mode: "age", MaxAgeDays: &ageDays,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.HistoryQueries.GetProbeSnapshot(fixture.ctx, baseline); err != nil {
		t.Fatalf("comparison baseline was deleted: %v", err)
	}
	if state.Usage.ProtectedLogicalBytes <= 128+160 {
		t.Fatalf("protected usage excludes baseline dependencies: %#v", state.Usage)
	}
}

func TestAgeRetentionProtectsCurrentProbeOutcomeState(t *testing.T) {
	fixture := newProbeServiceFixture(t, 512*1024*1024)
	egressID := fixture.egresses[0].ID.String()
	observedAt := fixture.now.Add(-72 * time.Hour)
	runID := uuid.NewString()
	executionID := uuid.NewString()
	if _, err := fixture.store.History.ExecContext(fixture.ctx, `
		INSERT INTO probe_runs (
			id, node_id, history_generation, configuration_revision, trigger,
			status, expected_executions, started_at, completed_at, received_at
		) VALUES (?, ?, ?, 1, 'schedule', 'failed', 1, ?, ?, ?)
	`, runID, fixture.registration.NodeID.String(), fixture.configuration.HistoryGeneration,
		observedAt.Add(-time.Second).Unix(), observedAt.Unix(), observedAt.Add(time.Second).Unix()); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.History.ExecContext(fixture.ctx, `
		INSERT INTO probe_executions (
			id, run_id, egress_id, ordinal, sequence, status,
			started_at, completed_at, failure_stage, received_at
		) VALUES (?, ?, ?, 0, 1, 'failed', ?, ?, 'process', ?)
	`, executionID, runID, egressID, observedAt.Add(-time.Second).Unix(),
		observedAt.Unix(), observedAt.Add(time.Second).Unix()); err != nil {
		t.Fatal(err)
	}
	if err := centerhistory.Advance(
		fixture.ctx, fixture.store.HistoryQueries, fixture.registration.NodeID.String(),
		egressID, fixture.configuration.HistoryGeneration, fixture.now.Unix(),
	); err != nil {
		t.Fatal(err)
	}

	ageDays := int64(1)
	state, err := fixture.service.UpdateHistoryRetention(fixture.ctx, HistoryRetentionUpdate{
		Mode: "age", MaxAgeDays: &ageDays,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.HistoryQueries.GetProbeExecution(fixture.ctx, executionID); err != nil {
		t.Fatalf("current probe outcome execution was deleted: %v", err)
	}
	outcome, err := fixture.store.HistoryQueries.GetProbeOutcomeState(fixture.ctx, egressID)
	if err != nil || outcome.ExecutionID != executionID || outcome.Status != "failed" {
		t.Fatalf("current probe outcome state = %#v, %v", outcome, err)
	}
	if state.Usage.ProtectedLogicalBytes <= 224 {
		t.Fatalf("protected usage excludes current probe outcome dependencies: %#v", state.Usage)
	}
}

func TestHistoryRetentionValidation(t *testing.T) {
	age := int64(0)
	size := int64(1024)
	for _, update := range []HistoryRetentionUpdate{
		{Mode: "unknown"},
		{Mode: "indefinite", MaxAgeDays: &age},
		{Mode: "age"},
		{Mode: "age", MaxAgeDays: &age},
		{Mode: "size", MaxLogicalBytes: &size},
	} {
		if err := validateHistoryRetention(update); err == nil {
			t.Fatalf("invalid retention accepted: %#v", update)
		}
	}
}

func TestHistoryProbeSnapshotTimeRange(t *testing.T) {
	fixture := newProbeServiceFixture(t, 512*1024*1024)
	egressID := fixture.egresses[0].ID.String()
	outside := seedRetentionSnapshot(t, fixture, egressID, 1, fixture.now.Add(-3*time.Hour), 128)
	inside := seedRetentionSnapshot(t, fixture, egressID, 2, fixture.now.Add(-2*time.Hour), 128)
	tooNew := seedRetentionSnapshot(t, fixture, egressID, 3, fixture.now.Add(-time.Hour), 128)
	snapshot, err := fixture.store.HistoryQueries.GetProbeSnapshot(fixture.ctx, inside)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.History.ExecContext(fixture.ctx, `
		UPDATE probe_runs
		SET status = 'partial', trigger = 'manual'
		WHERE id = (SELECT run_id FROM probe_executions WHERE id = ?)
	`, snapshot.ExecutionID); err != nil {
		t.Fatal(err)
	}
	from := fixture.now.Add(-150 * time.Minute)
	to := fixture.now.Add(-90 * time.Minute)
	unchanged := false

	page, err := fixture.service.ListHistoryProbeSnapshots(fixture.ctx, HistoryFilter{
		From: &from, To: &to, RunStatus: "partial", Trigger: "manual",
		Changed: &unchanged, FormatStatus: "mismatch", Page: 1, PageSize: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID.String() != inside {
		t.Fatalf("time-filtered page = %#v, want only %s (outside: %s, too new: %s)", page, inside, outside, tooNew)
	}
}

func seedRetentionSnapshot(
	t *testing.T,
	fixture *probeServiceFixture,
	egressID string,
	sequence int64,
	observedAt time.Time,
	targetBytes int,
) string {
	t.Helper()
	runID := uuid.NewString()
	executionID := uuid.NewString()
	snapshotID := uuid.NewString()
	raw := `{"padding":"` + strings.Repeat("x", max(0, targetBytes-14)) + `"}`
	if _, err := fixture.store.History.ExecContext(fixture.ctx, `
		INSERT INTO probe_runs (
			id, node_id, history_generation, configuration_revision, trigger,
			status, expected_executions, started_at, completed_at, received_at
		) VALUES (?, ?, ?, 1, 'schedule', 'succeeded', 1, ?, ?, ?)
	`, runID, fixture.registration.NodeID.String(), fixture.configuration.HistoryGeneration,
		observedAt.Add(-time.Second).Unix(), observedAt.Unix(), observedAt.Add(time.Second).Unix()); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.History.ExecContext(fixture.ctx, `
		INSERT INTO probe_executions (
			id, run_id, egress_id, ordinal, sequence, status,
			started_at, completed_at, received_at
		) VALUES (?, ?, ?, 0, ?, 'succeeded', ?, ?, ?)
	`, executionID, runID, egressID, sequence, observedAt.Add(-time.Second).Unix(),
		observedAt.Unix(), observedAt.Add(time.Second).Unix()); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.History.ExecContext(fixture.ctx, `
		INSERT INTO probe_snapshots (
			id, execution_id, egress_id, sequence, observed_at,
			raw_result, encoded_size, received_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, snapshotID, executionID, egressID, sequence, observedAt.Unix(), []byte(raw), len(raw), observedAt.Add(time.Second).Unix()); err != nil {
		t.Fatal(err)
	}
	return snapshotID
}

func setCurrentRetentionSnapshot(t *testing.T, fixture *probeServiceFixture, egressID, snapshotID string, sequence int64) {
	t.Helper()
	snapshot, err := fixture.store.HistoryQueries.GetProbeSnapshot(fixture.ctx, snapshotID)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.HistoryQueries.UpsertCurrentProbeSnapshot(fixture.ctx, historydb.UpsertCurrentProbeSnapshotParams{
		EgressID: egressID, ExecutionID: snapshot.ExecutionID, SnapshotID: snapshotID,
		Sequence: sequence, ObservedAt: snapshot.ObservedAt, ReceivedAt: snapshot.ReceivedAt,
	}); err != nil {
		t.Fatal(err)
	}
}
