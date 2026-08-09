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
				if err := processSuccessfulSnapshot(ctx, queries, progress.LastSuccessSnapshotID, execution, snapshot, recordedAt); err != nil {
					return err
				}
				progress.LastSuccessSnapshotID = &snapshot.ID
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
	return processFormat(ctx, queries, execution, snapshot, current.Issues, recordedAt)
}

func processFormat(
	ctx context.Context,
	queries *historydb.Queries,
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
		if _, err := queries.CreateProbeFormatEvent(ctx, historydb.CreateProbeFormatEventParams{
			ID: stableID("probe-format-event", execution.ID), ExecutionID: execution.ID,
			SnapshotID: snapshot.ID, EgressID: execution.EgressID, Sequence: execution.Sequence,
			Kind: eventKind, PreviousSignature: previousSignature, CurrentSignature: signature,
			IssueCount: int64(len(issues)), IssuesJson: issuesJSON,
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

func stableID(kind, source string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(kind+":"+source)).String()
}
