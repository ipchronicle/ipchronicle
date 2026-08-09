package nodes

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/center/database/configdb"
	"github.com/ipchronicle/ipchronicle/internal/center/database/historydb"
	sharedschedule "github.com/ipchronicle/ipchronicle/internal/schedule"
)

const (
	minimumProbeMemoryBytes = 256 * 1024 * 1024
	probeTaskDeliveryWindow = 2 * time.Minute
	maxProbeResultBytes     = 1024 * 1024
	maxProbeDiagnosticBytes = 64 * 1024
)

var (
	ErrInvalidProbeSettings  = errors.New("complete-probe settings are invalid")
	ErrNodeOffline           = errors.New("node is offline")
	ErrNodeDisabled          = errors.New("node is disabled")
	ErrProbeTaskSlotOccupied = errors.New("node task slot is occupied")
	ErrProbeAlreadyRunning   = errors.New("node already has an active complete-probe run")
	ErrProbePausedLowMemory  = errors.New("complete probes are paused below the memory baseline")
	ErrNoEnabledEgress       = errors.New("node has no enabled network egress")
	ErrProbeRunNotFound      = errors.New("complete-probe run does not exist")
	ErrProbeSnapshotNotFound = errors.New("complete-probe snapshot does not exist")
	ErrInvalidProbeArtifact  = errors.New("complete-probe artifact is invalid")
	ErrHistoryResetPending   = errors.New("history reset is pending completion")
)

type ProbeStatus struct {
	ActiveRunID                       *uuid.UUID
	NextScheduledAt                   *time.Time
	LastOccurrenceAt                  *time.Time
	LastOccurrenceTrigger             *string
	LastOccurrenceStatus              *string
	LastSkipReason                    *string
	HistoryResetGeneration            *string
	HistoryResetAt                    *time.Time
	HistoryResetDiscardedAddressItems int64
	HistoryResetDiscardedProbeItems   int64
}

type TaskReport struct {
	ID              uuid.UUID
	Status          string
	AcknowledgedAt  time.Time
	StartedAt       *time.Time
	CompletedAt     *time.Time
	RunID           *uuid.UUID
	RejectionReason *string
}

type Task struct {
	ID              uuid.UUID
	NodeID          uuid.UUID
	Status          string
	CreatedAt       time.Time
	ExpiresAt       time.Time
	AcknowledgedAt  *time.Time
	StartedAt       *time.Time
	CompletedAt     *time.Time
	RunID           *uuid.UUID
	RejectionReason *string
	Offline         bool
}

type ProbeSettingsUpdate struct {
	Schedule          ProbeSchedule
	LowMemoryOverride bool
}

type ProbeState struct {
	NodeID              uuid.UUID
	Schedule            ProbeSchedule
	LowMemoryOverride   bool
	PhysicalMemoryBytes *int64
	PausedLowMemory     bool
	AgentStatus         *ProbeStatus
	Task                *Task
	RecentRuns          []ProbeRunSummary
}

type ProbeRunSummary struct {
	ID                  uuid.UUID
	NodeID              uuid.UUID
	Trigger             string
	StartedAt           time.Time
	CompletedAt         *time.Time
	Status              string
	ExpectedExecutions  int64
	CompletedExecutions int64
}

type ProbeRun struct {
	ID                    uuid.UUID
	NodeID                uuid.UUID
	ConfigurationRevision int64
	HistoryGeneration     string
	Trigger               string
	TaskID                *uuid.UUID
	TriggeringEgressID    *uuid.UUID
	StartedAt             time.Time
	CompletedAt           *time.Time
	Status                string
	ExpectedExecutions    int64
	Executions            []ProbeExecution
}

type ProbeExecution struct {
	ID           uuid.UUID
	RunID        uuid.UUID
	EgressID     uuid.UUID
	Ordinal      int64
	Sequence     int64
	Status       string
	StartedAt    *time.Time
	CompletedAt  *time.Time
	FailureStage *string
	Diagnostic   *string
	SnapshotID   *uuid.UUID
}

type ProbeSnapshot struct {
	ID          uuid.UUID
	ExecutionID uuid.UUID
	EgressID    uuid.UUID
	Sequence    int64
	ObservedAt  time.Time
	RawResult   []byte
}

type ProbeExecutionManifest struct {
	ID       uuid.UUID
	EgressID uuid.UUID
	Ordinal  int64
	Sequence int64
}

type ProbeRunArtifact struct {
	ID                    uuid.UUID
	ConfigurationRevision int64
	HistoryGeneration     string
	Trigger               string
	TaskID                *uuid.UUID
	TriggeringEgressID    *uuid.UUID
	StartedAt             time.Time
	CompletedAt           *time.Time
	Status                string
	Executions            []ProbeExecutionManifest
}

type ProbeExecutionArtifact struct {
	ID           uuid.UUID
	EgressID     uuid.UUID
	Ordinal      int64
	Sequence     int64
	Status       string
	StartedAt    *time.Time
	CompletedAt  *time.Time
	FailureStage *string
	Diagnostic   *string
	RawResult    []byte
}

type ProbeGapArtifact struct {
	ID                uuid.UUID
	EgressID          uuid.UUID
	HistoryGeneration string
	DroppedCount      int64
	FirstSequence     int64
	LastSequence      int64
	FirstObservedAt   time.Time
	LastObservedAt    time.Time
}

type ProbeArtifact struct {
	ID        uuid.UUID
	Revision  int64
	Run       *ProbeRunArtifact
	Execution *ProbeExecutionArtifact
	Gap       *ProbeGapArtifact
}

type ProbeArtifactReceipt struct {
	ID          uuid.UUID
	Revision    int64
	Disposition string
}

type HistoryState struct {
	Generation string
	ResetAt    *time.Time
}

func (s *Service) applyProbeControlReport(
	ctx context.Context,
	queries *configdb.Queries,
	nodeID string,
	status *ProbeStatus,
	report *TaskReport,
	now int64,
) (*uuid.UUID, error) {
	if err := validateProbeStatus(status); err != nil {
		return nil, err
	}
	if status != nil {
		params := configdb.UpsertNodeProbeStatusParams{NodeID: nodeID, ReportedAt: now}
		if status.ActiveRunID != nil {
			value := status.ActiveRunID.String()
			params.ActiveRunID = &value
		}
		params.NextScheduledAt = unixPointer(status.NextScheduledAt)
		params.LastOccurrenceAt = unixPointer(status.LastOccurrenceAt)
		params.LastOccurrenceTrigger = status.LastOccurrenceTrigger
		params.LastOccurrenceStatus = status.LastOccurrenceStatus
		params.LastSkipReason = status.LastSkipReason
		params.HistoryResetGeneration = status.HistoryResetGeneration
		params.HistoryResetAt = unixPointer(status.HistoryResetAt)
		params.HistoryResetDiscardedAddressItems = status.HistoryResetDiscardedAddressItems
		params.HistoryResetDiscardedProbeItems = status.HistoryResetDiscardedProbeItems
		if err := queries.UpsertNodeProbeStatus(ctx, params); err != nil {
			return nil, err
		}
	}
	if report == nil {
		return nil, nil
	}
	record, err := queries.GetProbeTask(ctx, configdb.GetProbeTaskParams{ID: report.ID.String(), NodeID: nodeID})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidMetadata
	}
	if err != nil {
		return nil, err
	}
	if err := validateTaskReport(record, *report); err != nil {
		return nil, err
	}
	if taskTerminal(record.Status) {
		if !sameTerminalTask(record, *report) {
			return nil, ErrInvalidMetadata
		}
		id := report.ID
		return &id, nil
	}
	if taskStatusRank(report.Status) < taskStatusRank(record.Status) {
		return nil, ErrInvalidMetadata
	}
	var runID *string
	if report.RunID != nil {
		value := report.RunID.String()
		runID = &value
	}
	terminalConfirmedAt := (*int64)(nil)
	if taskTerminal(report.Status) {
		terminalConfirmedAt = &now
	}
	updated, err := queries.UpdateProbeTaskReport(ctx, configdb.UpdateProbeTaskReportParams{
		Status: report.Status, AcknowledgedAt: unixPointer(&report.AcknowledgedAt),
		StartedAt: unixPointer(report.StartedAt), CompletedAt: unixPointer(report.CompletedAt),
		RunID: runID, RejectionReason: report.RejectionReason,
		TerminalConfirmedAt: terminalConfirmedAt, ID: report.ID.String(), NodeID: nodeID,
	})
	if err != nil {
		return nil, err
	}
	if updated != 1 {
		return nil, ErrInvalidMetadata
	}
	if taskTerminal(report.Status) {
		id := report.ID
		return &id, nil
	}
	return nil, nil
}

func (s *Service) deliverProbeTask(ctx context.Context, nodeID string, now int64) (*Task, error) {
	record, err := s.queries.GetActiveProbeTask(ctx, nodeID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if record.Status == "pending" && record.ExpiresAt <= now {
		if _, err := s.queries.ExpireProbeTask(ctx, configdb.ExpireProbeTaskParams{
			CompletedAt: &now, ID: record.ID, NodeID: nodeID, ExpiresAt: now,
		}); err != nil {
			return nil, err
		}
		return nil, nil
	}
	task, err := taskFromRecord(record, false)
	return &task, err
}

func (s *Service) CreateCompleteProbeTask(ctx context.Context, nodeID uuid.UUID) (Task, error) {
	node, err := s.queries.GetNodeProbeSettings(ctx, nodeID.String())
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, ErrNodeNotFound
	}
	if err != nil {
		return Task{}, err
	}
	if node.RevokedAt != nil {
		return Task{}, ErrNodeRevoked
	}
	if node.Enabled == 0 {
		return Task{}, ErrNodeDisabled
	}
	now := s.now().UTC().Truncate(time.Second)
	if node.LastSeenAt == nil || now.Sub(time.Unix(*node.LastSeenAt, 0)) > OnlineWindow {
		return Task{}, ErrNodeOffline
	}
	if node.PhysicalMemoryBytes != nil && *node.PhysicalMemoryBytes < minimumProbeMemoryBytes && node.ProbeLowMemoryOverride == 0 {
		return Task{}, ErrProbePausedLowMemory
	}
	if status, statusErr := s.queries.GetNodeProbeStatus(ctx, nodeID.String()); statusErr == nil && status.ActiveRunID != nil {
		return Task{}, ErrProbeAlreadyRunning
	} else if statusErr != nil && !errors.Is(statusErr, sql.ErrNoRows) {
		return Task{}, statusErr
	}
	if _, err := s.queries.GetActiveProbeTask(ctx, nodeID.String()); err == nil {
		return Task{}, ErrProbeTaskSlotOccupied
	} else if !errors.Is(err, sql.ErrNoRows) {
		return Task{}, err
	}
	egresses, err := s.queries.ListActiveNodeEgresses(ctx, nodeID.String())
	if err != nil {
		return Task{}, err
	}
	if !slices.ContainsFunc(egresses, func(egress configdb.NetworkEgress) bool { return egress.Enabled == 1 }) {
		return Task{}, ErrNoEnabledEgress
	}
	id := uuid.New()
	expiresAt := now.Add(probeTaskDeliveryWindow)
	if err := s.queries.CreateProbeTask(ctx, configdb.CreateProbeTaskParams{
		ID: id.String(), NodeID: nodeID.String(), CreatedAt: now.Unix(), ExpiresAt: expiresAt.Unix(),
	}); err != nil {
		if strings.Contains(err.Error(), "probe_tasks_active_node_idx") {
			return Task{}, ErrProbeTaskSlotOccupied
		}
		return Task{}, err
	}
	s.sync.Wake(nodeID.String())
	return Task{ID: id, NodeID: nodeID, Status: "pending", CreatedAt: now, ExpiresAt: expiresAt}, nil
}

func (s *Service) Probe(ctx context.Context, nodeID uuid.UUID) (ProbeState, error) {
	settings, err := s.queries.GetNodeProbeSettings(ctx, nodeID.String())
	if errors.Is(err, sql.ErrNoRows) {
		return ProbeState{}, ErrNodeNotFound
	}
	if err != nil {
		return ProbeState{}, err
	}
	state := ProbeState{
		NodeID:              nodeID,
		Schedule:            ProbeSchedule{Enabled: settings.ProbeScheduleEnabled == 1, Cron: settings.ProbeScheduleCron, Timezone: settings.ProbeScheduleTimezone},
		LowMemoryOverride:   settings.ProbeLowMemoryOverride == 1,
		PhysicalMemoryBytes: settings.PhysicalMemoryBytes,
	}
	state.PausedLowMemory = settings.PhysicalMemoryBytes != nil && *settings.PhysicalMemoryBytes < minimumProbeMemoryBytes && !state.LowMemoryOverride
	if record, statusErr := s.queries.GetNodeProbeStatus(ctx, nodeID.String()); statusErr == nil {
		status, err := probeStatusFromRecord(record)
		if err != nil {
			return ProbeState{}, err
		}
		state.AgentStatus = &status
	} else if !errors.Is(statusErr, sql.ErrNoRows) {
		return ProbeState{}, statusErr
	}
	if record, taskErr := s.queries.GetLatestProbeTask(ctx, nodeID.String()); taskErr == nil {
		offline := settings.LastSeenAt == nil || s.now().UTC().Sub(time.Unix(*settings.LastSeenAt, 0)) > OnlineWindow
		task, err := taskFromRecord(record, offline && !taskTerminal(record.Status))
		if err != nil {
			return ProbeState{}, err
		}
		state.Task = &task
	} else if !errors.Is(taskErr, sql.ErrNoRows) {
		return ProbeState{}, taskErr
	}
	runs, err := s.historyQueries.ListNodeProbeRuns(ctx, historydb.ListNodeProbeRunsParams{NodeID: nodeID.String(), Limit: 30})
	if err != nil {
		return ProbeState{}, err
	}
	state.RecentRuns = make([]ProbeRunSummary, 0, len(runs))
	for _, record := range runs {
		executions, err := s.historyQueries.ListProbeRunExecutions(ctx, record.ID)
		if err != nil {
			return ProbeState{}, err
		}
		completed := int64(0)
		for _, execution := range executions {
			if execution.Status != "pending" && execution.Status != "running" {
				completed++
			}
		}
		summary, err := probeRunSummaryFromRecord(record, completed)
		if err != nil {
			return ProbeState{}, err
		}
		state.RecentRuns = append(state.RecentRuns, summary)
	}
	return state, nil
}

func (s *Service) UpdateProbeSettings(ctx context.Context, nodeID uuid.UUID, update ProbeSettingsUpdate) (ProbeState, error) {
	if err := sharedschedule.ValidateProbe(update.Schedule.Cron, update.Schedule.Timezone); err != nil {
		return ProbeState{}, ErrInvalidProbeSettings
	}
	current, err := s.queries.GetNodeProbeSettings(ctx, nodeID.String())
	if errors.Is(err, sql.ErrNoRows) {
		return ProbeState{}, ErrNodeNotFound
	}
	if err != nil {
		return ProbeState{}, err
	}
	if current.RevokedAt != nil {
		return ProbeState{}, ErrNodeRevoked
	}
	if current.ProbeScheduleEnabled == boolInt(update.Schedule.Enabled) && current.ProbeScheduleCron == update.Schedule.Cron &&
		current.ProbeScheduleTimezone == update.Schedule.Timezone && current.ProbeLowMemoryOverride == boolInt(update.LowMemoryOverride) {
		return s.Probe(ctx, nodeID)
	}
	updated, err := s.queries.UpdateNodeProbeSettings(ctx, configdb.UpdateNodeProbeSettingsParams{
		ProbeScheduleEnabled: boolInt(update.Schedule.Enabled), ProbeScheduleCron: update.Schedule.Cron,
		ProbeScheduleTimezone: update.Schedule.Timezone, ProbeLowMemoryOverride: boolInt(update.LowMemoryOverride), ID: nodeID.String(),
	})
	if err != nil {
		return ProbeState{}, err
	}
	if updated != 1 {
		if deletion, deletionErr := s.queries.GetNodeDeletion(ctx, nodeID.String()); deletionErr == nil && deletion.Status != "completed" {
			return ProbeState{}, ErrNodeDeletionPending
		}
		return ProbeState{}, ErrNodeNotFound
	}
	s.sync.Wake(nodeID.String())
	return s.Probe(ctx, nodeID)
}

func (s *Service) UploadProbeArtifact(ctx context.Context, credential string, artifact ProbeArtifact) (ProbeArtifactReceipt, error) {
	node, err := s.authenticateAgent(ctx, credential)
	if err != nil {
		return ProbeArtifactReceipt{}, err
	}
	if err := validateProbeArtifact(artifact); err != nil {
		return ProbeArtifactReceipt{}, err
	}
	receipt := ProbeArtifactReceipt{ID: artifact.ID, Revision: artifact.Revision, Disposition: "accepted"}
	generation := artifactGeneration(artifact)
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	systemState, err := s.queries.GetSystemState(ctx)
	if err != nil {
		return ProbeArtifactReceipt{}, err
	}
	if generation != systemState.HistoryGeneration {
		receipt.Disposition = "obsolete-generation"
		return receipt, nil
	}
	if systemState.PendingHistoryGeneration != nil {
		return ProbeArtifactReceipt{}, ErrHistoryResetPending
	}
	if artifact.Gap != nil {
		owned, deleted, err := s.egressOwnership(ctx, node.ID, artifact.Gap.EgressID)
		if err != nil {
			return ProbeArtifactReceipt{}, err
		}
		if !owned || deleted {
			receipt.Disposition = "egress-deleted"
			return receipt, nil
		}
		changed, err := s.historyQueries.UpsertProbeGap(ctx, historydb.UpsertProbeGapParams{
			ID: artifact.Gap.ID.String(), EgressID: artifact.Gap.EgressID.String(), NodeID: node.ID,
			HistoryGeneration: artifact.Gap.HistoryGeneration, DroppedCount: artifact.Gap.DroppedCount,
			FirstSequence: artifact.Gap.FirstSequence, LastSequence: artifact.Gap.LastSequence,
			FirstObservedAt: artifact.Gap.FirstObservedAt.Unix(), LastObservedAt: artifact.Gap.LastObservedAt.Unix(),
			ReceivedAt: s.now().UTC().Unix(),
		})
		if err != nil {
			return ProbeArtifactReceipt{}, err
		}
		if changed != 1 {
			return ProbeArtifactReceipt{}, ErrInvalidProbeArtifact
		}
		return receipt, nil
	}
	for _, manifest := range artifact.Run.Executions {
		owned, deleted, err := s.egressOwnership(ctx, node.ID, manifest.EgressID)
		if err != nil {
			return ProbeArtifactReceipt{}, err
		}
		if !owned {
			return ProbeArtifactReceipt{}, ErrInvalidProbeArtifact
		}
		if deleted {
			receipt.Disposition = "egress-deleted"
			return receipt, nil
		}
	}
	if artifact.Execution != nil {
		owned, deleted, err := s.egressOwnership(ctx, node.ID, artifact.Execution.EgressID)
		if err != nil {
			return ProbeArtifactReceipt{}, err
		}
		if !owned || deleted {
			receipt.Disposition = "egress-deleted"
			return receipt, nil
		}
	}
	transaction, err := s.history.BeginTx(ctx, nil)
	if err != nil {
		return ProbeArtifactReceipt{}, err
	}
	defer transaction.Rollback()
	queries := s.historyQueries.WithTx(transaction)
	if err := ingestProbeRun(ctx, queries, node.ID, *artifact.Run, s.now().UTC().Unix()); err != nil {
		return ProbeArtifactReceipt{}, err
	}
	if artifact.Execution != nil {
		if err := ingestProbeExecution(ctx, queries, *artifact.Run, *artifact.Execution, s.now().UTC().Unix()); err != nil {
			return ProbeArtifactReceipt{}, err
		}
	}
	if err := reconcileProbeRunSummary(ctx, queries, *artifact.Run, s.now().UTC().Unix()); err != nil {
		return ProbeArtifactReceipt{}, err
	}
	if err := transaction.Commit(); err != nil {
		return ProbeArtifactReceipt{}, err
	}
	return receipt, nil
}

func (s *Service) ProbeRun(ctx context.Context, id uuid.UUID) (ProbeRun, error) {
	record, err := s.historyQueries.GetProbeRun(ctx, id.String())
	if errors.Is(err, sql.ErrNoRows) {
		return ProbeRun{}, ErrProbeRunNotFound
	}
	if err != nil {
		return ProbeRun{}, err
	}
	executionRecords, err := s.historyQueries.ListProbeRunExecutions(ctx, id.String())
	if err != nil {
		return ProbeRun{}, err
	}
	run, err := probeRunFromRecord(record)
	if err != nil {
		return ProbeRun{}, err
	}
	run.Executions = make([]ProbeExecution, 0, len(executionRecords))
	for _, executionRecord := range executionRecords {
		execution, err := probeExecutionFromRecord(executionRecord)
		if err != nil {
			return ProbeRun{}, err
		}
		if snapshot, snapshotErr := s.historyQueries.GetProbeSnapshotByExecution(ctx, executionRecord.ID); snapshotErr == nil {
			snapshotID, err := uuid.Parse(snapshot.ID)
			if err != nil {
				return ProbeRun{}, err
			}
			execution.SnapshotID = &snapshotID
		} else if !errors.Is(snapshotErr, sql.ErrNoRows) {
			return ProbeRun{}, snapshotErr
		}
		run.Executions = append(run.Executions, execution)
	}
	return run, nil
}

func (s *Service) ProbeSnapshot(ctx context.Context, id uuid.UUID) (ProbeSnapshot, error) {
	record, err := s.historyQueries.GetProbeSnapshot(ctx, id.String())
	if errors.Is(err, sql.ErrNoRows) {
		return ProbeSnapshot{}, ErrProbeSnapshotNotFound
	}
	if err != nil {
		return ProbeSnapshot{}, err
	}
	executionID, err := uuid.Parse(record.ExecutionID)
	if err != nil {
		return ProbeSnapshot{}, err
	}
	egressID, err := uuid.Parse(record.EgressID)
	if err != nil {
		return ProbeSnapshot{}, err
	}
	return ProbeSnapshot{
		ID: id, ExecutionID: executionID, EgressID: egressID, Sequence: record.Sequence,
		ObservedAt: time.Unix(record.ObservedAt, 0).UTC(), RawResult: slices.Clone(record.RawResult),
	}, nil
}

func (s *Service) History(ctx context.Context) (HistoryState, error) {
	record, err := s.queries.GetSystemState(ctx)
	if err != nil {
		return HistoryState{}, err
	}
	return HistoryState{Generation: record.HistoryGeneration, ResetAt: timePointer(record.HistoryResetAt)}, nil
}

func (s *Service) ResetHistory(ctx context.Context) (HistoryState, error) {
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	state, err := s.queries.GetSystemState(ctx)
	if err != nil {
		return HistoryState{}, err
	}
	generation := ""
	if state.PendingHistoryGeneration != nil {
		generation = *state.PendingHistoryGeneration
	} else {
		generation, err = newHistoryGeneration()
		if err != nil {
			return HistoryState{}, err
		}
		if err := s.queries.SetPendingHistoryGeneration(ctx, &generation); err != nil {
			return HistoryState{}, err
		}
	}
	now := s.now().UTC().Truncate(time.Second).Unix()
	historyTransaction, err := s.history.BeginTx(ctx, nil)
	if err != nil {
		return HistoryState{}, err
	}
	historyQueries := s.historyQueries.WithTx(historyTransaction)
	for _, reset := range []func(context.Context) error{
		historyQueries.ResetProbeHistory, historyQueries.ResetProbeGaps,
		historyQueries.ResetAddressHistory, historyQueries.ResetAddressStates, historyQueries.ResetAddressGaps,
	} {
		if err := reset(ctx); err != nil {
			_ = historyTransaction.Rollback()
			return HistoryState{}, err
		}
	}
	if changed, err := historyQueries.UpdateHistoryGeneration(ctx, historydb.UpdateHistoryGenerationParams{
		Generation: generation, CreatedAt: now,
	}); err != nil || changed != 1 {
		_ = historyTransaction.Rollback()
		if err != nil {
			return HistoryState{}, err
		}
		return HistoryState{}, errors.New("history generation metadata was not updated")
	}
	if err := historyTransaction.Commit(); err != nil {
		return HistoryState{}, err
	}
	configTransaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return HistoryState{}, err
	}
	defer configTransaction.Rollback()
	configQueries := s.queries.WithTx(configTransaction)
	changed, err := configQueries.PromotePendingHistoryGeneration(ctx, configdb.PromotePendingHistoryGenerationParams{
		HistoryResetAt: &now, PendingHistoryGeneration: &generation,
	})
	if err != nil {
		return HistoryState{}, err
	}
	if changed != 1 {
		return HistoryState{}, errors.New("pending history generation was not promoted")
	}
	if err := configQueries.IncrementAllNodeDesiredConfigurationRevisions(ctx); err != nil {
		return HistoryState{}, err
	}
	if err := configTransaction.Commit(); err != nil {
		return HistoryState{}, err
	}
	resetAt := time.Unix(now, 0).UTC()
	return HistoryState{Generation: generation, ResetAt: &resetAt}, nil
}

func ingestProbeRun(ctx context.Context, queries *historydb.Queries, nodeID string, run ProbeRunArtifact, receivedAt int64) error {
	var taskID, triggeringEgressID *string
	if run.TaskID != nil {
		value := run.TaskID.String()
		taskID = &value
	}
	if run.TriggeringEgressID != nil {
		value := run.TriggeringEgressID.String()
		triggeringEgressID = &value
	}
	inserted, err := queries.CreateProbeRun(ctx, historydb.CreateProbeRunParams{
		ID: run.ID.String(), NodeID: nodeID, HistoryGeneration: run.HistoryGeneration,
		ConfigurationRevision: run.ConfigurationRevision, Trigger: run.Trigger, TaskID: taskID,
		TriggeringEgressID: triggeringEgressID, Status: "running", ExpectedExecutions: int64(len(run.Executions)),
		StartedAt: run.StartedAt.Unix(), ReceivedAt: receivedAt,
	})
	if err != nil {
		return err
	}
	record, err := queries.GetProbeRun(ctx, run.ID.String())
	if err != nil {
		return err
	}
	if !sameProbeRun(record, nodeID, run) {
		return ErrInvalidProbeArtifact
	}
	for _, execution := range run.Executions {
		insertedExecution, err := queries.CreateProbeExecution(ctx, historydb.CreateProbeExecutionParams{
			ID: execution.ID.String(), RunID: run.ID.String(), EgressID: execution.EgressID.String(),
			Ordinal: execution.Ordinal, Sequence: execution.Sequence, Status: "pending", ReceivedAt: receivedAt,
		})
		if err != nil {
			return err
		}
		existing, err := queries.GetProbeExecution(ctx, execution.ID.String())
		if err != nil {
			return err
		}
		if !sameProbeExecutionManifest(existing, run.ID, execution) {
			return ErrInvalidProbeArtifact
		}
		_ = insertedExecution
	}
	if run.Status == "running" && (run.CompletedAt != nil || record.Status != "running" && inserted == 1) {
		return ErrInvalidProbeArtifact
	}
	return nil
}

func ingestProbeExecution(ctx context.Context, queries *historydb.Queries, run ProbeRunArtifact, execution ProbeExecutionArtifact, receivedAt int64) error {
	record, err := queries.GetProbeExecution(ctx, execution.ID.String())
	if err != nil {
		return err
	}
	manifestIndex := slices.IndexFunc(run.Executions, func(item ProbeExecutionManifest) bool { return item.ID == execution.ID })
	if manifestIndex < 0 || !sameProbeExecutionArtifactIdentity(record, run.ID, execution) {
		return ErrInvalidProbeArtifact
	}
	if record.Status != "pending" && record.Status != "running" {
		if !sameTerminalExecution(record, execution) {
			return ErrInvalidProbeArtifact
		}
		return ensureSnapshot(ctx, queries, execution, receivedAt)
	}
	if record.Status == "running" && execution.Status == "running" {
		if pointerInt64(record.StartedAt) != unixValue(execution.StartedAt) {
			return ErrInvalidProbeArtifact
		}
		return nil
	}
	updated, err := queries.UpdateProbeExecution(ctx, historydb.UpdateProbeExecutionParams{
		Status: execution.Status, StartedAt: unixPointer(execution.StartedAt), CompletedAt: unixPointer(execution.CompletedAt),
		FailureStage: execution.FailureStage, Diagnostic: execution.Diagnostic, ReceivedAt: receivedAt, ID: execution.ID.String(),
	})
	if err != nil {
		return err
	}
	if updated != 1 {
		return ErrInvalidProbeArtifact
	}
	return ensureSnapshot(ctx, queries, execution, receivedAt)
}

func reconcileProbeRunSummary(ctx context.Context, queries *historydb.Queries, run ProbeRunArtifact, receivedAt int64) error {
	record, err := queries.GetProbeRun(ctx, run.ID.String())
	if err != nil {
		return err
	}
	if run.Status == "running" {
		return nil
	}
	executions, err := queries.ListProbeRunExecutions(ctx, run.ID.String())
	if err != nil {
		return err
	}
	if int64(len(executions)) != record.ExpectedExecutions {
		return ErrInvalidProbeArtifact
	}
	succeeded := 0
	for _, execution := range executions {
		if execution.Status == "pending" || execution.Status == "running" {
			if record.Status != "running" {
				return ErrInvalidProbeArtifact
			}
			return nil
		}
		if execution.CompletedAt == nil || run.CompletedAt == nil || *execution.CompletedAt > run.CompletedAt.Unix() {
			return ErrInvalidProbeArtifact
		}
		if execution.Status == "succeeded" {
			succeeded++
		}
	}
	expectedStatus := "failed"
	if succeeded == len(executions) {
		expectedStatus = "succeeded"
	} else if succeeded > 0 {
		expectedStatus = "partial"
	}
	if run.Status != expectedStatus {
		return ErrInvalidProbeArtifact
	}
	completedAt := run.CompletedAt.Unix()
	if record.Status == "running" {
		updated, err := queries.CompleteProbeRun(ctx, historydb.CompleteProbeRunParams{
			Status: run.Status, CompletedAt: &completedAt, ReceivedAt: receivedAt, ID: run.ID.String(),
		})
		if err != nil {
			return err
		}
		if updated != 1 {
			return ErrInvalidProbeArtifact
		}
		return nil
	}
	if record.Status != run.Status || record.CompletedAt == nil || *record.CompletedAt != completedAt {
		return ErrInvalidProbeArtifact
	}
	return nil
}

func ensureSnapshot(ctx context.Context, queries *historydb.Queries, execution ProbeExecutionArtifact, receivedAt int64) error {
	if execution.Status != "succeeded" {
		return nil
	}
	observedAt := execution.CompletedAt.Unix()
	inserted, err := queries.CreateProbeSnapshot(ctx, historydb.CreateProbeSnapshotParams{
		ID: execution.ID.String(), ExecutionID: execution.ID.String(), EgressID: execution.EgressID.String(),
		Sequence: execution.Sequence, ObservedAt: observedAt, RawResult: execution.RawResult,
		EncodedSize: int64(len(execution.RawResult)), ReceivedAt: receivedAt,
	})
	if err != nil {
		return err
	}
	if inserted == 0 {
		record, err := queries.GetProbeSnapshotByExecution(ctx, execution.ID.String())
		if err != nil {
			return err
		}
		if record.ID != execution.ID.String() || record.EgressID != execution.EgressID.String() ||
			record.Sequence != execution.Sequence || record.ObservedAt != observedAt || !bytes.Equal(record.RawResult, execution.RawResult) {
			return ErrInvalidProbeArtifact
		}
	}
	return queries.UpsertCurrentProbeSnapshot(ctx, historydb.UpsertCurrentProbeSnapshotParams{
		EgressID: execution.EgressID.String(), ExecutionID: execution.ID.String(), SnapshotID: execution.ID.String(),
		Sequence: execution.Sequence, ObservedAt: observedAt, ReceivedAt: receivedAt,
	})
}

func validateProbeArtifact(artifact ProbeArtifact) error {
	if artifact.ID == uuid.Nil || artifact.Revision < 1 {
		return ErrInvalidProbeArtifact
	}
	parts := 0
	if artifact.Run != nil {
		parts++
	}
	if artifact.Gap != nil {
		parts++
	}
	if parts != 1 || (artifact.Execution != nil && artifact.Run == nil) {
		return ErrInvalidProbeArtifact
	}
	if artifact.Gap != nil {
		gap := artifact.Gap
		if artifact.ID != gap.ID || gap.ID == uuid.Nil || gap.EgressID == uuid.Nil || !validHistoryGeneration(gap.HistoryGeneration) ||
			gap.DroppedCount < 1 || gap.FirstSequence < 1 || gap.LastSequence < gap.FirstSequence ||
			gap.FirstObservedAt.IsZero() || gap.LastObservedAt.Before(gap.FirstObservedAt) {
			return ErrInvalidProbeArtifact
		}
		return nil
	}
	run := artifact.Run
	if run.ID == uuid.Nil || run.ConfigurationRevision < 1 || !validHistoryGeneration(run.HistoryGeneration) ||
		!validProbeTrigger(run.Trigger) || run.StartedAt.IsZero() || !validRunTerminal(run.Status, run.CompletedAt) ||
		len(run.Executions) < 1 || len(run.Executions) > 64 || artifact.ID != run.ID && artifact.Execution == nil {
		return ErrInvalidProbeArtifact
	}
	if run.CompletedAt != nil && run.CompletedAt.Before(run.StartedAt) {
		return ErrInvalidProbeArtifact
	}
	if (run.Trigger == "manual") != (run.TaskID != nil) || (run.Trigger == "address-change") != (run.TriggeringEgressID != nil) {
		return ErrInvalidProbeArtifact
	}
	seenIDs := make(map[uuid.UUID]struct{}, len(run.Executions))
	seenEgresses := make(map[uuid.UUID]struct{}, len(run.Executions))
	for index, item := range run.Executions {
		if item.ID == uuid.Nil || item.EgressID == uuid.Nil || item.Ordinal != int64(index) || item.Sequence < 1 {
			return ErrInvalidProbeArtifact
		}
		if _, exists := seenIDs[item.ID]; exists {
			return ErrInvalidProbeArtifact
		}
		if _, exists := seenEgresses[item.EgressID]; exists {
			return ErrInvalidProbeArtifact
		}
		seenIDs[item.ID] = struct{}{}
		seenEgresses[item.EgressID] = struct{}{}
	}
	if run.TriggeringEgressID != nil {
		if _, exists := seenEgresses[*run.TriggeringEgressID]; !exists {
			return ErrInvalidProbeArtifact
		}
	}
	if artifact.Execution == nil {
		return nil
	}
	execution := artifact.Execution
	if artifact.ID != execution.ID || execution.ID == uuid.Nil || execution.EgressID == uuid.Nil || execution.Ordinal < 0 ||
		execution.Sequence < 1 || !validExecutionState(*execution) {
		return ErrInvalidProbeArtifact
	}
	return nil
}

func validExecutionState(execution ProbeExecutionArtifact) bool {
	diagnosticBytes := 0
	if execution.Diagnostic != nil {
		if !utf8.ValidString(*execution.Diagnostic) {
			return false
		}
		diagnosticBytes = len([]byte(*execution.Diagnostic))
	}
	if diagnosticBytes > maxProbeDiagnosticBytes || len(execution.RawResult) > maxProbeResultBytes {
		return false
	}
	switch execution.Status {
	case "running":
		return execution.StartedAt != nil && execution.CompletedAt == nil && execution.FailureStage == nil && execution.Diagnostic == nil && len(execution.RawResult) == 0
	case "succeeded":
		return execution.StartedAt != nil && execution.CompletedAt != nil && !execution.CompletedAt.Before(*execution.StartedAt) &&
			execution.FailureStage == nil && execution.Diagnostic == nil && validProbeJSONObject(execution.RawResult)
	case "failed", "interrupted":
		return execution.StartedAt != nil && execution.CompletedAt != nil && !execution.CompletedAt.Before(*execution.StartedAt) &&
			execution.FailureStage != nil && validFailureStage(*execution.FailureStage) && len(execution.RawResult) == 0
	case "skipped":
		return execution.StartedAt == nil && execution.CompletedAt != nil && execution.FailureStage != nil &&
			validFailureStage(*execution.FailureStage) && len(execution.RawResult) == 0
	default:
		return false
	}
}

func validateProbeStatus(status *ProbeStatus) error {
	if status == nil {
		return nil
	}
	if (status.LastOccurrenceAt == nil) != (status.LastOccurrenceStatus == nil) ||
		(status.LastOccurrenceStatus != nil && *status.LastOccurrenceStatus == "skipped") != (status.LastSkipReason != nil) {
		return ErrInvalidMetadata
	}
	if status.LastOccurrenceTrigger != nil && !validProbeTrigger(*status.LastOccurrenceTrigger) {
		return ErrInvalidMetadata
	}
	if status.LastOccurrenceStatus != nil && *status.LastOccurrenceStatus != "started" && *status.LastOccurrenceStatus != "skipped" {
		return ErrInvalidMetadata
	}
	if status.LastSkipReason != nil && !validSkipReason(*status.LastSkipReason) {
		return ErrInvalidMetadata
	}
	if (status.HistoryResetGeneration == nil) != (status.HistoryResetAt == nil) ||
		status.HistoryResetDiscardedAddressItems < 0 || status.HistoryResetDiscardedProbeItems < 0 {
		return ErrInvalidMetadata
	}
	if status.HistoryResetGeneration == nil {
		if status.HistoryResetDiscardedAddressItems != 0 || status.HistoryResetDiscardedProbeItems != 0 {
			return ErrInvalidMetadata
		}
	} else if !validHistoryGeneration(*status.HistoryResetGeneration) || status.HistoryResetAt.IsZero() {
		return ErrInvalidMetadata
	}
	return nil
}

func validateTaskReport(record configdb.ProbeTask, report TaskReport) error {
	if report.ID == uuid.Nil || report.AcknowledgedAt.IsZero() || report.AcknowledgedAt.Unix() < record.CreatedAt ||
		report.AcknowledgedAt.Unix() > record.ExpiresAt ||
		!validTaskReportStatus(report.Status) {
		return ErrInvalidMetadata
	}
	if report.StartedAt != nil && report.StartedAt.Before(report.AcknowledgedAt) || report.CompletedAt != nil && report.CompletedAt.Before(report.AcknowledgedAt) {
		return ErrInvalidMetadata
	}
	if report.Status == "acknowledged" {
		if report.StartedAt != nil || report.CompletedAt != nil || report.RunID != nil || report.RejectionReason != nil {
			return ErrInvalidMetadata
		}
		return nil
	}
	if report.Status == "rejected" {
		if report.StartedAt != nil || report.CompletedAt == nil || report.RunID != nil ||
			report.RejectionReason == nil || !validSkipReason(*report.RejectionReason) {
			return ErrInvalidMetadata
		}
		return nil
	}
	if report.StartedAt == nil || report.RunID == nil || report.RejectionReason != nil {
		return ErrInvalidMetadata
	}
	if report.Status == "running" {
		if report.CompletedAt != nil {
			return ErrInvalidMetadata
		}
		return nil
	}
	if report.CompletedAt == nil || report.CompletedAt.Before(*report.StartedAt) {
		return ErrInvalidMetadata
	}
	return nil
}

func validProbeJSONObject(raw []byte) bool {
	if len(raw) == 0 || !json.Valid(raw) {
		return false
	}
	var object map[string]json.RawMessage
	return json.Unmarshal(raw, &object) == nil && object != nil
}

func (s *Service) egressOwnership(ctx context.Context, nodeID string, egressID uuid.UUID) (bool, bool, error) {
	_, err := s.queries.GetNodeEgress(ctx, configdb.GetNodeEgressParams{NodeID: nodeID, ID: egressID.String()})
	if err == nil {
		if deletion, deletionErr := s.queries.GetEgressDeletion(ctx, configdb.GetEgressDeletionParams{
			EgressID: egressID.String(), NodeID: nodeID,
		}); deletionErr == nil && deletion.Status != "completed" {
			return true, true, nil
		} else if deletionErr != nil && !errors.Is(deletionErr, sql.ErrNoRows) {
			return false, false, deletionErr
		}
		return true, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, false, err
	}
	_, err = s.queries.GetEgressDeletion(ctx, configdb.GetEgressDeletionParams{EgressID: egressID.String(), NodeID: nodeID})
	if err == nil {
		return true, true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, false, nil
	}
	return false, false, err
}

func artifactGeneration(artifact ProbeArtifact) string {
	if artifact.Run != nil {
		return artifact.Run.HistoryGeneration
	}
	return artifact.Gap.HistoryGeneration
}

func taskFromRecord(record configdb.ProbeTask, offline bool) (Task, error) {
	id, err := uuid.Parse(record.ID)
	if err != nil {
		return Task{}, err
	}
	nodeID, err := uuid.Parse(record.NodeID)
	if err != nil {
		return Task{}, err
	}
	var runID *uuid.UUID
	if record.RunID != nil {
		value, err := uuid.Parse(*record.RunID)
		if err != nil {
			return Task{}, err
		}
		runID = &value
	}
	return Task{
		ID: id, NodeID: nodeID, Status: record.Status,
		CreatedAt: time.Unix(record.CreatedAt, 0).UTC(), ExpiresAt: time.Unix(record.ExpiresAt, 0).UTC(),
		AcknowledgedAt: timePointer(record.AcknowledgedAt), StartedAt: timePointer(record.StartedAt),
		CompletedAt: timePointer(record.CompletedAt), RunID: runID, RejectionReason: record.RejectionReason,
		Offline: offline,
	}, nil
}

func probeStatusFromRecord(record configdb.NodeProbeStatus) (ProbeStatus, error) {
	status := ProbeStatus{
		NextScheduledAt: timePointer(record.NextScheduledAt), LastOccurrenceAt: timePointer(record.LastOccurrenceAt),
		LastOccurrenceTrigger: record.LastOccurrenceTrigger, LastOccurrenceStatus: record.LastOccurrenceStatus,
		LastSkipReason: record.LastSkipReason, HistoryResetGeneration: record.HistoryResetGeneration,
		HistoryResetAt:                    timePointer(record.HistoryResetAt),
		HistoryResetDiscardedAddressItems: record.HistoryResetDiscardedAddressItems,
		HistoryResetDiscardedProbeItems:   record.HistoryResetDiscardedProbeItems,
	}
	if record.ActiveRunID != nil {
		value, err := uuid.Parse(*record.ActiveRunID)
		if err != nil {
			return ProbeStatus{}, err
		}
		status.ActiveRunID = &value
	}
	return status, nil
}

func probeRunSummaryFromRecord(record historydb.ProbeRun, completed int64) (ProbeRunSummary, error) {
	id, err := uuid.Parse(record.ID)
	if err != nil {
		return ProbeRunSummary{}, err
	}
	nodeID, err := uuid.Parse(record.NodeID)
	if err != nil {
		return ProbeRunSummary{}, err
	}
	return ProbeRunSummary{
		ID: id, NodeID: nodeID, Trigger: record.Trigger, StartedAt: time.Unix(record.StartedAt, 0).UTC(),
		CompletedAt: timePointer(record.CompletedAt), Status: record.Status,
		ExpectedExecutions: record.ExpectedExecutions, CompletedExecutions: completed,
	}, nil
}

func probeRunFromRecord(record historydb.ProbeRun) (ProbeRun, error) {
	summary, err := probeRunSummaryFromRecord(record, 0)
	if err != nil {
		return ProbeRun{}, err
	}
	var taskID, triggeringEgressID *uuid.UUID
	if record.TaskID != nil {
		value, err := uuid.Parse(*record.TaskID)
		if err != nil {
			return ProbeRun{}, err
		}
		taskID = &value
	}
	if record.TriggeringEgressID != nil {
		value, err := uuid.Parse(*record.TriggeringEgressID)
		if err != nil {
			return ProbeRun{}, err
		}
		triggeringEgressID = &value
	}
	return ProbeRun{
		ID: summary.ID, NodeID: summary.NodeID, ConfigurationRevision: record.ConfigurationRevision,
		HistoryGeneration: record.HistoryGeneration, Trigger: record.Trigger, TaskID: taskID,
		TriggeringEgressID: triggeringEgressID, StartedAt: summary.StartedAt, CompletedAt: summary.CompletedAt,
		Status: summary.Status, ExpectedExecutions: summary.ExpectedExecutions,
	}, nil
}

func probeExecutionFromRecord(record historydb.ProbeExecution) (ProbeExecution, error) {
	id, err := uuid.Parse(record.ID)
	if err != nil {
		return ProbeExecution{}, err
	}
	runID, err := uuid.Parse(record.RunID)
	if err != nil {
		return ProbeExecution{}, err
	}
	egressID, err := uuid.Parse(record.EgressID)
	if err != nil {
		return ProbeExecution{}, err
	}
	return ProbeExecution{
		ID: id, RunID: runID, EgressID: egressID, Ordinal: record.Ordinal, Sequence: record.Sequence,
		Status: record.Status, StartedAt: timePointer(record.StartedAt), CompletedAt: timePointer(record.CompletedAt),
		FailureStage: record.FailureStage, Diagnostic: record.Diagnostic,
	}, nil
}

func sameProbeRun(record historydb.ProbeRun, nodeID string, run ProbeRunArtifact) bool {
	var taskID, triggeringEgressID string
	if run.TaskID != nil {
		taskID = run.TaskID.String()
	}
	if run.TriggeringEgressID != nil {
		triggeringEgressID = run.TriggeringEgressID.String()
	}
	return record.ID == run.ID.String() && record.NodeID == nodeID && record.HistoryGeneration == run.HistoryGeneration &&
		record.ConfigurationRevision == run.ConfigurationRevision && record.Trigger == run.Trigger &&
		pointerString(record.TaskID) == taskID && pointerString(record.TriggeringEgressID) == triggeringEgressID &&
		record.ExpectedExecutions == int64(len(run.Executions)) && record.StartedAt == run.StartedAt.Unix()
}

func sameProbeExecutionManifest(record historydb.ProbeExecution, runID uuid.UUID, execution ProbeExecutionManifest) bool {
	return record.ID == execution.ID.String() && record.RunID == runID.String() && record.EgressID == execution.EgressID.String() &&
		record.Ordinal == execution.Ordinal && record.Sequence == execution.Sequence
}

func sameProbeExecutionArtifactIdentity(record historydb.ProbeExecution, runID uuid.UUID, execution ProbeExecutionArtifact) bool {
	return record.ID == execution.ID.String() && record.RunID == runID.String() && record.EgressID == execution.EgressID.String() &&
		record.Ordinal == execution.Ordinal && record.Sequence == execution.Sequence
}

func sameTerminalExecution(record historydb.ProbeExecution, execution ProbeExecutionArtifact) bool {
	return record.Status == execution.Status && pointerInt64(record.StartedAt) == unixValue(execution.StartedAt) &&
		pointerInt64(record.CompletedAt) == unixValue(execution.CompletedAt) && pointerString(record.FailureStage) == pointerString(execution.FailureStage) &&
		pointerString(record.Diagnostic) == pointerString(execution.Diagnostic)
}

func sameTerminalTask(record configdb.ProbeTask, report TaskReport) bool {
	return record.Status == report.Status && pointerInt64(record.AcknowledgedAt) == report.AcknowledgedAt.Unix() &&
		pointerInt64(record.StartedAt) == unixValue(report.StartedAt) && pointerInt64(record.CompletedAt) == unixValue(report.CompletedAt) &&
		pointerString(record.RunID) == uuidPointerString(report.RunID) && pointerString(record.RejectionReason) == pointerString(report.RejectionReason)
}

func validProbeTrigger(value string) bool {
	return value == "manual" || value == "schedule" || value == "address-change"
}

func validFailureStage(value string) bool {
	switch value {
	case "download", "selector", "adapter", "process", "timeout", "output", "restart":
		return true
	default:
		return false
	}
}

func validSkipReason(value string) bool {
	switch value {
	case "busy", "disabled", "low-memory", "no-egress", "missed":
		return true
	default:
		return false
	}
}

func validRunTerminal(status string, completedAt *time.Time) bool {
	if status == "running" {
		return completedAt == nil
	}
	return (status == "succeeded" || status == "partial" || status == "failed") && completedAt != nil
}

func validTaskReportStatus(value string) bool {
	switch value {
	case "acknowledged", "running", "succeeded", "partial", "failed", "rejected":
		return true
	default:
		return false
	}
}

func taskTerminal(value string) bool {
	return value == "succeeded" || value == "partial" || value == "failed" || value == "rejected" || value == "expired"
}

func taskStatusRank(value string) int {
	switch value {
	case "pending":
		return 0
	case "acknowledged":
		return 1
	case "running":
		return 2
	default:
		return 3
	}
}

func unixValue(value *time.Time) int64 {
	if value == nil {
		return 0
	}
	return value.UTC().Truncate(time.Second).Unix()
}

func timePointer(value *int64) *time.Time {
	if value == nil {
		return nil
	}
	result := time.Unix(*value, 0).UTC()
	return &result
}

func pointerString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func pointerInt64(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func uuidPointerString(value *uuid.UUID) string {
	if value == nil {
		return ""
	}
	return value.String()
}

func boolInt(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

func newHistoryGeneration() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate history generation: %w", err)
	}
	return hex.EncodeToString(value), nil
}
