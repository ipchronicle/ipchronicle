package history

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/center/database/historydb"
	"github.com/ipchronicle/ipchronicle/internal/center/notifications"
)

func Advance(
	ctx context.Context,
	queries *historydb.Queries,
	nodeID string,
	egressID string,
	historyGeneration string,
	recordedAt int64,
) error {
	progress, err := queries.GetProbeComparisonProgress(ctx, egressID)
	if errors.Is(err, sql.ErrNoRows) || err == nil && progress.HistoryGeneration != historyGeneration {
		progress = historydb.ProbeComparisonProgress{
			EgressID: egressID, NodeID: nodeID, HistoryGeneration: historyGeneration,
			NextSequence: 1, UpdatedAt: recordedAt,
		}
	} else if err != nil {
		return err
	}
	if progress.NodeID != nodeID {
		return errors.New("probe comparison progress ownership does not match")
	}

	for {
		execution, executionErr := queries.GetProbeExecutionByEgressSequence(ctx, historydb.GetProbeExecutionByEgressSequenceParams{
			EgressID: egressID, Sequence: progress.NextSequence,
		})
		if executionErr == nil {
			if execution.Status == "pending" || execution.Status == "running" {
				break
			}
			if execution.Status == "succeeded" {
				snapshot, err := queries.GetProbeSnapshotByExecution(ctx, execution.ID)
				if err != nil {
					return fmt.Errorf("load successful probe snapshot: %w", err)
				}
				if err := processProbeOutcome(
					ctx, queries, nodeID, historyGeneration, execution, snapshot.ObservedAt, recordedAt,
				); err != nil {
					return err
				}
				if err := processSuccessfulSnapshot(ctx, queries, nodeID, progress.LastSuccessSnapshotID, execution, snapshot, recordedAt); err != nil {
					return err
				}
				progress.LastSuccessSnapshotID = &snapshot.ID
			} else if err := processProbeOutcome(
				ctx, queries, nodeID, historyGeneration, execution, executionObservedAt(execution), recordedAt,
			); err != nil {
				return err
			}
			progress.NextSequence++
			progress.UpdatedAt = recordedAt
			if err := persistProgress(ctx, queries, progress); err != nil {
				return err
			}
			continue
		}
		if executionErr != nil && !errors.Is(executionErr, sql.ErrNoRows) {
			return executionErr
		}
		gap, gapErr := queries.GetProbeGapCoveringSequence(ctx, historydb.GetProbeGapCoveringSequenceParams{
			EgressID: egressID, HistoryGeneration: historyGeneration,
			FirstSequence: progress.NextSequence, LastSequence: progress.NextSequence,
		})
		if errors.Is(gapErr, sql.ErrNoRows) {
			break
		}
		if gapErr != nil {
			return gapErr
		}
		progress.NextSequence = gap.LastSequence + 1
		progress.UpdatedAt = recordedAt
		if err := persistProgress(ctx, queries, progress); err != nil {
			return err
		}
	}
	return persistProgress(ctx, queries, progress)
}

func persistProgress(ctx context.Context, queries *historydb.Queries, progress historydb.ProbeComparisonProgress) error {
	return queries.UpsertProbeComparisonProgress(ctx, historydb.UpsertProbeComparisonProgressParams{
		EgressID: progress.EgressID, NodeID: progress.NodeID,
		HistoryGeneration: progress.HistoryGeneration, NextSequence: progress.NextSequence,
		LastSuccessSnapshotID: progress.LastSuccessSnapshotID, UpdatedAt: progress.UpdatedAt,
	})
}

func processSuccessfulSnapshot(
	ctx context.Context,
	queries *historydb.Queries,
	nodeID string,
	previousSnapshotID *string,
	execution historydb.ProbeExecution,
	snapshot historydb.ProbeSnapshot,
	recordedAt int64,
) error {
	current, err := Interpret(snapshot.RawResult)
	if err != nil {
		return err
	}
	changeSetID := stableID("probe-change-set", execution.ID)
	changes := []FieldChange(nil)
	baseline := int64(1)
	if previousSnapshotID != nil {
		previous, err := queries.GetProbeSnapshot(ctx, *previousSnapshotID)
		if err != nil {
			return fmt.Errorf("load preceding probe snapshot: %w", err)
		}
		previousReport, err := Interpret(previous.RawResult)
		if err != nil {
			return err
		}
		changes = Compare(previousReport, current)
		baseline = 0
	}
	inserted, err := queries.CreateProbeChangeSet(ctx, historydb.CreateProbeChangeSetParams{
		ID: changeSetID, ExecutionID: execution.ID, SnapshotID: snapshot.ID,
		EgressID: execution.EgressID, Sequence: execution.Sequence,
		PreviousSnapshotID: previousSnapshotID, Baseline: baseline,
		ChangeCount: int64(len(changes)), ObservedAt: snapshot.ObservedAt, RecordedAt: recordedAt,
	})
	if err != nil {
		return err
	}
	if inserted == 1 {
		for _, change := range changes {
			if _, err := queries.CreateProbeFieldChange(ctx, historydb.CreateProbeFieldChangeParams{
				ChangeSetID: changeSetID, FieldID: change.FieldID, GroupName: change.Group,
				JsonPath: change.Path, ValueType: string(change.ValueType),
				BeforeValue: change.Before, AfterValue: change.After,
			}); err != nil {
				return err
			}
		}
	}
	if len(changes) > 0 {
		egressID := execution.EgressID
		data := notifications.ProbeChangeData{
			ExecutionID: execution.ID, SnapshotID: snapshot.ID,
			PreviousSnapshotID: previousSnapshotID, Sequence: execution.Sequence,
			Changes: make([]notifications.FieldChange, 0, len(changes)),
		}
		for _, change := range changes {
			data.Changes = append(data.Changes, notifications.FieldChange{
				FieldID: change.FieldID, Group: change.Group, Path: change.Path,
				ValueType: string(change.ValueType), Before: change.Before, After: change.After,
			})
		}
		if err := notifications.CreateEvent(ctx, queries, notifications.EventInput{
			Type: notifications.EventProbeFieldChange, SourceKind: "probe-change-set",
			SourceID: changeSetID, NodeID: &nodeID, EgressID: &egressID,
			Payload: data, ObservedAt: snapshot.ObservedAt, RecordedAt: recordedAt,
		}); err != nil {
			return err
		}
	}
	return processFormat(ctx, queries, nodeID, execution, snapshot, current.Issues, recordedAt)
}

func processFormat(
	ctx context.Context,
	queries *historydb.Queries,
	nodeID string,
	execution historydb.ProbeExecution,
	snapshot historydb.ProbeSnapshot,
	issues []FormatIssue,
	recordedAt int64,
) error {
	issuesJSON, err := json.Marshal(issues)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(issuesJSON)
	signature := hex.EncodeToString(digest[:])
	status := "compatible"
	if len(issues) > 0 {
		status = "mismatch"
	}
	if _, err := queries.CreateProbeSnapshotFormat(ctx, historydb.CreateProbeSnapshotFormatParams{
		SnapshotID: snapshot.ID, Status: status, Signature: signature,
		IssueCount: int64(len(issues)), IssuesJson: issuesJSON,
	}); err != nil {
		return err
	}

	previous, err := queries.GetProbeFormatState(ctx, execution.EgressID)
	firstObservedAt := snapshot.ObservedAt
	var eventKind string
	var previousSignature *string
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if status == "mismatch" {
			eventKind = "mismatch"
		}
	case err != nil:
		return err
	default:
		previousSignature = &previous.Signature
		if previous.Signature == signature {
			firstObservedAt = previous.FirstObservedAt
		} else if previous.Status == "mismatch" && status == "compatible" {
			eventKind = "recovered"
		} else if previous.Status == "compatible" && status == "mismatch" {
			eventKind = "mismatch"
		} else if previous.Status == "mismatch" && status == "mismatch" {
			eventKind = "changed"
		}
	}
	if eventKind != "" {
		eventID := stableID("probe-format-event", execution.ID)
		if _, err := queries.CreateProbeFormatEvent(ctx, historydb.CreateProbeFormatEventParams{
			ID: eventID, ExecutionID: execution.ID,
			SnapshotID: snapshot.ID, EgressID: execution.EgressID, Sequence: execution.Sequence,
			Kind: eventKind, PreviousSignature: previousSignature, CurrentSignature: signature,
			IssueCount: int64(len(issues)), IssuesJson: issuesJSON,
			ObservedAt: snapshot.ObservedAt, RecordedAt: recordedAt,
		}); err != nil {
			return err
		}
		eventType := map[string]string{
			"mismatch":  notifications.EventFormatMismatch,
			"changed":   notifications.EventFormatChanged,
			"recovered": notifications.EventFormatRecovery,
		}[eventKind]
		egressID := execution.EgressID
		if err := notifications.CreateEvent(ctx, queries, notifications.EventInput{
			Type: eventType, SourceKind: "format-event", SourceID: eventID,
			NodeID: &nodeID, EgressID: &egressID,
			Payload: notifications.FormatData{
				ExecutionID: execution.ID, SnapshotID: snapshot.ID,
				Sequence: execution.Sequence, Kind: eventKind, IssueCount: int64(len(issues)),
			},
			ObservedAt: snapshot.ObservedAt, RecordedAt: recordedAt,
		}); err != nil {
			return err
		}
	}
	return queries.UpsertProbeFormatState(ctx, historydb.UpsertProbeFormatStateParams{
		EgressID: execution.EgressID, SnapshotID: snapshot.ID, Sequence: execution.Sequence,
		Status: status, Signature: signature, IssueCount: int64(len(issues)), IssuesJson: issuesJSON,
		FirstObservedAt: firstObservedAt, LastObservedAt: snapshot.ObservedAt, UpdatedAt: recordedAt,
	})
}

func processProbeOutcome(
	ctx context.Context,
	queries *historydb.Queries,
	nodeID string,
	historyGeneration string,
	execution historydb.ProbeExecution,
	observedAt int64,
	recordedAt int64,
) error {
	status := ""
	switch execution.Status {
	case "succeeded":
		status = "healthy"
	case "failed", "interrupted":
		status = "failed"
	default:
		return nil
	}
	previous, err := queries.GetProbeOutcomeState(ctx, execution.EgressID)
	missing := errors.Is(err, sql.ErrNoRows) || err == nil && previous.HistoryGeneration != historyGeneration
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	firstObservedAt := observedAt
	eventType := ""
	if missing {
		if status == "failed" {
			eventType = notifications.EventProbeFailure
		}
	} else if previous.Status == status {
		firstObservedAt = previous.FirstObservedAt
	} else if status == "failed" {
		eventType = notifications.EventProbeFailure
	} else {
		eventType = notifications.EventProbeRecovery
	}
	if eventType != "" {
		egressID := execution.EgressID
		if err := notifications.CreateEvent(ctx, queries, notifications.EventInput{
			Type: eventType, SourceKind: "probe-execution", SourceID: execution.ID,
			NodeID: &nodeID, EgressID: &egressID,
			Payload: notifications.ProbeOutcomeData{
				ExecutionID: execution.ID, Sequence: execution.Sequence,
				Status: execution.Status, FailureStage: execution.FailureStage,
			},
			ObservedAt: observedAt, RecordedAt: recordedAt,
		}); err != nil {
			return err
		}
	}
	return queries.UpsertProbeOutcomeState(ctx, historydb.UpsertProbeOutcomeStateParams{
		EgressID: execution.EgressID, NodeID: nodeID, HistoryGeneration: historyGeneration,
		ExecutionID: execution.ID, Sequence: execution.Sequence, Status: status,
		FirstObservedAt: firstObservedAt, LastObservedAt: observedAt, UpdatedAt: recordedAt,
	})
}

func executionObservedAt(execution historydb.ProbeExecution) int64 {
	if execution.CompletedAt != nil {
		return *execution.CompletedAt
	}
	if execution.StartedAt != nil {
		return *execution.StartedAt
	}
	return execution.ReceivedAt
}

func stableID(kind, source string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(kind+":"+source)).String()
}
