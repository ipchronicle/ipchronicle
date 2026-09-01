package history

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/center/database"
	"github.com/ipchronicle/ipchronicle/internal/center/database/historydb"
	"github.com/ipchronicle/ipchronicle/internal/center/notifications"
)

func TestAdvanceWaitsForPrecedingSequenceAndProcessesChronologically(t *testing.T) {
	ctx, store := openProcessorStore(t)
	nodeID := uuid.NewString()
	egressID := uuid.NewString()
	second := seedSuccessfulExecution(t, store.History, nodeID, egressID, 2, 200,
		`{"Head":{"IP":"203.0.113.*"},"Info":{"ASN":"64501"}}`)
	if err := Advance(ctx, store.HistoryQueries, nodeID, egressID, store.HistoryGeneration, 300); err != nil {
		t.Fatal(err)
	}
	if _, err := store.HistoryQueries.GetProbeChangeSetBySnapshot(ctx, second); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("out-of-order sequence was processed early: %v", err)
	}
	if events, err := store.HistoryQueries.ListPendingNotificationEvents(ctx, 10); err != nil || len(events) != 0 {
		t.Fatalf("out-of-order notifications = %#v, %v", events, err)
	}
	progress, err := store.HistoryQueries.GetProbeComparisonProgress(ctx, egressID)
	if err != nil || progress.NextSequence != 1 || progress.LastSuccessSnapshotID != nil {
		t.Fatalf("blocked progress = %#v, %v", progress, err)
	}

	first := seedSuccessfulExecution(t, store.History, nodeID, egressID, 1, 100,
		`{"Head":{"IP":"198.51.100.*"},"Info":{"ASN":"64500"}}`)
	if err := Advance(ctx, store.HistoryQueries, nodeID, egressID, store.HistoryGeneration, 301); err != nil {
		t.Fatal(err)
	}
	baseline, err := store.HistoryQueries.GetProbeChangeSetBySnapshot(ctx, first)
	if err != nil || baseline.Baseline != 1 || baseline.ChangeCount != 0 {
		t.Fatalf("baseline = %#v, %v", baseline, err)
	}
	changeSet, err := store.HistoryQueries.GetProbeChangeSetBySnapshot(ctx, second)
	if err != nil || changeSet.Baseline != 0 || changeSet.ChangeCount != 2 || changeSet.PreviousSnapshotID == nil || *changeSet.PreviousSnapshotID != first {
		t.Fatalf("second change set = %#v, %v", changeSet, err)
	}
	changes, err := store.HistoryQueries.ListProbeFieldChanges(ctx, changeSet.ID)
	if err != nil || len(changes) != 2 || changes[0].FieldID != "Head.IP" || changes[1].FieldID != "Info.ASN" {
		t.Fatalf("changes = %#v, %v", changes, err)
	}
	pendingEvents, err := store.HistoryQueries.ListPendingNotificationEvents(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	events := make([]historydb.NotificationEvent, 0, 1)
	for _, event := range pendingEvents {
		if event.EventType == notifications.EventProbeFieldChange {
			events = append(events, event)
		}
	}
	if len(events) != 1 {
		t.Fatalf("chronological field change notification events = %#v", events)
	}
	var payload notifications.ProbeChangeData
	if err := json.Unmarshal(events[0].PayloadJson, &payload); err != nil || payload.Sequence != 2 || payload.SnapshotID != second {
		t.Fatalf("field change notification payload = %#v, %v", payload, err)
	}
	progress, err = store.HistoryQueries.GetProbeComparisonProgress(ctx, egressID)
	if err != nil || progress.NextSequence != 3 || progress.LastSuccessSnapshotID == nil || *progress.LastSuccessSnapshotID != second {
		t.Fatalf("advanced progress = %#v, %v", progress, err)
	}

	firstSnapshot, err := store.HistoryQueries.GetProbeSnapshot(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.HistoryQueries.DeleteRetentionExecution(ctx, firstSnapshot.ExecutionID); err != nil {
		t.Fatal(err)
	}
	changeSet, err = store.HistoryQueries.GetProbeChangeSetBySnapshot(ctx, second)
	if err != nil || changeSet.Baseline != 0 || changeSet.PreviousSnapshotID != nil {
		t.Fatalf("change set after preceding snapshot cleanup = %#v, %v", changeSet, err)
	}
}

func TestAdvanceEmitsProbeFailureAndRecoveryOnlyOnTransitions(t *testing.T) {
	ctx, store := openProcessorStore(t)
	nodeID := uuid.NewString()
	egressID := uuid.NewString()
	firstFailure := seedFailedExecution(t, store.History, nodeID, egressID, 1, 100)
	secondFailure := seedFailedExecution(t, store.History, nodeID, egressID, 2, 200)
	seedSuccessfulExecution(t, store.History, nodeID, egressID, 3, 300,
		`{"Head":{"IP":"198.51.100.*"}}`)

	if err := Advance(ctx, store.HistoryQueries, nodeID, egressID, store.HistoryGeneration, 400); err != nil {
		t.Fatal(err)
	}
	rows, err := store.History.QueryContext(ctx, `
		SELECT event_type, source_id, observed_at
		FROM notification_events
		WHERE event_type IN (?, ?)
		ORDER BY observed_at
	`, notifications.EventProbeFailure, notifications.EventProbeRecovery)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type outcomeEvent struct {
		eventType  string
		sourceID   string
		observedAt int64
	}
	events := make([]outcomeEvent, 0, 2)
	for rows.Next() {
		var event outcomeEvent
		if err := rows.Scan(&event.eventType, &event.sourceID, &event.observedAt); err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].eventType != notifications.EventProbeFailure ||
		events[0].sourceID != firstFailure || events[0].observedAt != 100 ||
		events[1].eventType != notifications.EventProbeRecovery || events[1].observedAt != 300 {
		t.Fatalf("probe transition events = %#v", events)
	}
	if events[0].sourceID == secondFailure {
		t.Fatal("repeated probe failure produced a notification event")
	}
	state, err := store.HistoryQueries.GetProbeOutcomeState(ctx, egressID)
	if err != nil || state.Status != "healthy" || state.Sequence != 3 || state.FirstObservedAt != 300 {
		t.Fatalf("probe outcome state = %#v, %v", state, err)
	}
}

func TestAdvanceUsesExplicitGapAsComparisonBoundary(t *testing.T) {
	ctx, store := openProcessorStore(t)
	nodeID := uuid.NewString()
	egressID := uuid.NewString()
	snapshotID := seedSuccessfulExecution(t, store.History, nodeID, egressID, 2, 200,
		`{"Head":{"IP":"203.0.113.*"}}`)
	if _, err := store.HistoryQueries.UpsertProbeGap(ctx, historydb.UpsertProbeGapParams{
		ID: uuid.NewString(), EgressID: egressID, NodeID: nodeID,
		HistoryGeneration: store.HistoryGeneration, DroppedCount: 1,
		FirstSequence: 1, LastSequence: 1, FirstObservedAt: 100,
		LastObservedAt: 100, ReceivedAt: 300,
	}); err != nil {
		t.Fatal(err)
	}
	if err := Advance(ctx, store.HistoryQueries, nodeID, egressID, store.HistoryGeneration, 300); err != nil {
		t.Fatal(err)
	}
	changeSet, err := store.HistoryQueries.GetProbeChangeSetBySnapshot(ctx, snapshotID)
	if err != nil || changeSet.Baseline != 1 {
		t.Fatalf("post-gap baseline = %#v, %v", changeSet, err)
	}
}

func TestAdvanceRecordsTypeDriftWithoutSemanticFieldChange(t *testing.T) {
	ctx, store := openProcessorStore(t)
	nodeID := uuid.NewString()
	egressID := uuid.NewString()
	seedSuccessfulExecution(t, store.History, nodeID, egressID, 1, 100,
		`{"Info":{"ASN":"64500"}}`)
	second := seedSuccessfulExecution(t, store.History, nodeID, egressID, 2, 200,
		`{"Info":{"ASN":64501}}`)
	if err := Advance(ctx, store.HistoryQueries, nodeID, egressID, store.HistoryGeneration, 300); err != nil {
		t.Fatal(err)
	}
	changeSet, err := store.HistoryQueries.GetProbeChangeSetBySnapshot(ctx, second)
	if err != nil || changeSet.ChangeCount != 0 {
		t.Fatalf("type drift change set = %#v, %v", changeSet, err)
	}
	format, err := store.HistoryQueries.GetProbeSnapshotFormat(ctx, second)
	if err != nil || format.Status != "mismatch" || format.IssueCount == 0 {
		t.Fatalf("type drift format = %#v, %v", format, err)
	}
	events, err := store.HistoryQueries.ListGlobalFormatEvents(ctx, historydb.ListGlobalFormatEventsParams{
		NodeID: nodeID, EgressID: egressID, PageSize: 10, PageOffset: 0,
	})
	if err != nil || len(events) != 2 || events[0].Kind != "changed" {
		t.Fatalf("format events = %#v, %v", events, err)
	}
}

func TestAdvanceTreatsNullAsUnavailableWithoutFormatTransition(t *testing.T) {
	ctx, store := openProcessorStore(t)
	nodeID := uuid.NewString()
	egressID := uuid.NewString()
	first := seedSuccessfulExecution(t, store.History, nodeID, egressID, 1, 100,
		`{"Info":{"ASN":"64500"}}`)
	second := seedSuccessfulExecution(t, store.History, nodeID, egressID, 2, 200,
		`{"Info":{"ASN":null}}`)
	if err := Advance(ctx, store.HistoryQueries, nodeID, egressID, store.HistoryGeneration, 300); err != nil {
		t.Fatal(err)
	}
	changeSet, err := store.HistoryQueries.GetProbeChangeSetBySnapshot(ctx, second)
	if err != nil || changeSet.ChangeCount != 0 {
		t.Fatalf("null availability change set = %#v, %v", changeSet, err)
	}
	firstFormat, err := store.HistoryQueries.GetProbeSnapshotFormat(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	secondFormat, err := store.HistoryQueries.GetProbeSnapshotFormat(ctx, second)
	if err != nil {
		t.Fatal(err)
	}
	if firstFormat.Signature != secondFormat.Signature || firstFormat.IssueCount != secondFormat.IssueCount {
		t.Fatalf("null changed format state: first=%#v second=%#v", firstFormat, secondFormat)
	}
	var issues []FormatIssue
	if err := json.Unmarshal(secondFormat.IssuesJson, &issues); err != nil {
		t.Fatal(err)
	}
	for _, issue := range issues {
		if issue.Path == "Info.ASN" {
			t.Fatalf("null produced a format issue: %#v", issue)
		}
	}
	events, err := store.HistoryQueries.ListGlobalFormatEvents(ctx, historydb.ListGlobalFormatEventsParams{
		NodeID: nodeID, EgressID: egressID, PageSize: 10, PageOffset: 0,
	})
	if err != nil || len(events) != 1 || events[0].SnapshotID != first {
		t.Fatalf("null produced a format transition: %#v, %v", events, err)
	}
}

func openProcessorStore(t *testing.T) (context.Context, *database.Store) {
	t.Helper()
	ctx := context.Background()
	store, err := database.Open(ctx, database.PathsFromDataDirectory(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return ctx, store
}

func seedSuccessfulExecution(
	t *testing.T,
	database *sql.DB,
	nodeID string,
	egressID string,
	sequence int64,
	observedAt int64,
	raw string,
) string {
	t.Helper()
	runID := uuid.NewString()
	executionID := uuid.NewString()
	snapshotID := uuid.NewString()
	generationRow := database.QueryRow(`SELECT generation FROM history_metadata WHERE id = 1`)
	var generation string
	if err := generationRow.Scan(&generation); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		INSERT INTO probe_runs (
			id, node_id, history_generation, configuration_revision, trigger,
			status, expected_executions, started_at, completed_at, received_at
		) VALUES (?, ?, ?, 1, 'schedule', 'succeeded', 1, ?, ?, ?)
	`, runID, nodeID, generation, observedAt-2, observedAt, observedAt+1); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		INSERT INTO probe_executions (
			id, run_id, egress_id, ordinal, sequence, status,
			started_at, completed_at, received_at
		) VALUES (?, ?, ?, 0, ?, 'succeeded', ?, ?, ?)
	`, executionID, runID, egressID, sequence, observedAt-1, observedAt, observedAt+1); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		INSERT INTO probe_snapshots (
			id, execution_id, egress_id, sequence, observed_at,
			raw_result, encoded_size, received_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, snapshotID, executionID, egressID, sequence, observedAt, []byte(raw), len(raw), observedAt+1); err != nil {
		t.Fatal(err)
	}
	return snapshotID
}

func seedFailedExecution(
	t *testing.T,
	database *sql.DB,
	nodeID string,
	egressID string,
	sequence int64,
	observedAt int64,
) string {
	t.Helper()
	runID := uuid.NewString()
	executionID := uuid.NewString()
	var generation string
	if err := database.QueryRow(`SELECT generation FROM history_metadata WHERE id = 1`).Scan(&generation); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		INSERT INTO probe_runs (
			id, node_id, history_generation, configuration_revision, trigger,
			status, expected_executions, started_at, completed_at, received_at
		) VALUES (?, ?, ?, 1, 'schedule', 'failed', 1, ?, ?, ?)
	`, runID, nodeID, generation, observedAt-2, observedAt, observedAt+1); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		INSERT INTO probe_executions (
			id, run_id, egress_id, ordinal, sequence, status,
			started_at, completed_at, failure_stage, received_at
		) VALUES (?, ?, ?, 0, ?, 'failed', ?, ?, 'process', ?)
	`, executionID, runID, egressID, sequence, observedAt-1, observedAt, observedAt+1); err != nil {
		t.Fatal(err)
	}
	return executionID
}
