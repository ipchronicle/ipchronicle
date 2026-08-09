package nodes

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/center/database/configdb"
	"github.com/ipchronicle/ipchronicle/internal/center/database/historydb"
	centerhistory "github.com/ipchronicle/ipchronicle/internal/center/history"
)

const (
	retentionCleanupInterval = 6 * time.Hour
	retentionCleanupBatch    = 250
)

var (
	ErrInvalidHistoryRetention = errors.New("history retention settings are invalid")
	ErrSnapshotEgressMismatch  = errors.New("probe snapshots belong to different network egresses")
)

type HistoryRetentionSettings struct {
	Mode                    string
	MaxAgeDays              *int64
	MaxLogicalBytes         *int64
	UpdatedAt               time.Time
	LastCleanupAt           *time.Time
	LastCleanupDeletedItems int64
	LastCleanupError        *string
}

type HistoryRetentionUpdate struct {
	Mode            string
	MaxAgeDays      *int64
	MaxLogicalBytes *int64
}

type HistoryUsage struct {
	LogicalBytes          int64
	ProtectedLogicalBytes int64
	RecordCount           int64
	DatabaseBytes         int64
	WALBytes              int64
	SharedMemoryBytes     int64
	OverBudget            bool
	OverageBytes          int64
}

type HistoryCleanupResult struct {
	DeletedItems int64
	CompletedAt  time.Time
	Usage        HistoryUsage
}

type HistoryFilter struct {
	NodeID       *uuid.UUID
	EgressID     *uuid.UUID
	From         *time.Time
	To           *time.Time
	RunStatus    string
	Trigger      string
	Changed      *bool
	FormatStatus string
	EventKind    string
	Family       string
	Page         int64
	GapPage      int64
	PageSize     int64
}

type HistoryOwner struct {
	NodeName   *string
	EgressName *string
}

type ProbeSnapshotSummary struct {
	ID                 uuid.UUID
	ExecutionID        uuid.UUID
	RunID              uuid.UUID
	NodeID             uuid.UUID
	EgressID           uuid.UUID
	Owner              HistoryOwner
	Sequence           int64
	Trigger            string
	RunStatus          string
	ObservedAt         time.Time
	ReceivedAt         time.Time
	EncodedSize        int64
	Starred            bool
	Current            bool
	Processed          bool
	Baseline           bool
	ChangeCount        int64
	FormatStatus       string
	FormatIssueCount   int64
	PreviousSnapshotID *uuid.UUID
}

type ProbeSnapshotPage struct {
	Items []ProbeSnapshotSummary
	Total int64
}

type AddressHistoryPage struct {
	Events   []HistoryAddressEvent
	Gaps     []HistoryAddressGap
	Total    int64
	GapTotal int64
}

type HistoryAddressEvent struct {
	NodeID uuid.UUID
	Owner  HistoryOwner
	Event  AddressEvent
}

type HistoryAddressGap struct {
	NodeID uuid.UUID
	Owner  HistoryOwner
	Gap    AddressGap
}

type ProbeHistoryGap struct {
	ID              uuid.UUID
	NodeID          uuid.UUID
	EgressID        uuid.UUID
	Owner           HistoryOwner
	DroppedCount    int64
	FirstSequence   int64
	LastSequence    int64
	FirstObservedAt time.Time
	LastObservedAt  time.Time
}

type ProbeHistoryGapPage struct {
	Items []ProbeHistoryGap
	Total int64
}

type FormatEvent struct {
	ID          uuid.UUID
	NodeID      uuid.UUID
	EgressID    uuid.UUID
	ExecutionID uuid.UUID
	SnapshotID  uuid.UUID
	Owner       HistoryOwner
	Sequence    int64
	Kind        string
	Issues      []centerhistory.FormatIssue
	ObservedAt  time.Time
	RecordedAt  time.Time
}

type FormatEventPage struct {
	Items []FormatEvent
	Total int64
}

type FieldComparison struct {
	ID            string
	Group         string
	Path          string
	ExpectedTypes []centerhistory.JSONType
	Before        centerhistory.FieldValue
	After         centerhistory.FieldValue
	Changed       bool
}

type ProbeSnapshotComparison struct {
	BeforeID uuid.UUID
	AfterID  uuid.UUID
	EgressID uuid.UUID
	Fields   []FieldComparison
}

func (s *Service) ListHistoryProbeSnapshots(ctx context.Context, filter HistoryFilter) (ProbeSnapshotPage, error) {
	filter = normalizeHistoryFilter(filter)
	records, err := s.historyQueries.ListProbeSnapshots(ctx, historydb.ListProbeSnapshotsParams{
		NodeID: filterUUID(filter.NodeID), EgressID: filterUUID(filter.EgressID),
		FromObservedAt: timeUnixPointer(filter.From), ToObservedAt: timeUnixPointer(filter.To),
		RunStatus: filter.RunStatus, Trigger: filter.Trigger, Changed: filter.Changed,
		FormatStatus: filter.FormatStatus,
		PageOffset:   (filter.Page - 1) * filter.PageSize, PageSize: filter.PageSize,
	})
	if err != nil {
		return ProbeSnapshotPage{}, err
	}
	total, err := s.historyQueries.CountProbeSnapshots(ctx, historydb.CountProbeSnapshotsParams{
		NodeID: filterUUID(filter.NodeID), EgressID: filterUUID(filter.EgressID),
		FromObservedAt: timeUnixPointer(filter.From), ToObservedAt: timeUnixPointer(filter.To),
		RunStatus: filter.RunStatus, Trigger: filter.Trigger, Changed: filter.Changed,
		FormatStatus: filter.FormatStatus,
	})
	if err != nil {
		return ProbeSnapshotPage{}, err
	}
	items := make([]ProbeSnapshotSummary, 0, len(records))
	for _, record := range records {
		id, executionID, runID, nodeID, egressID, err := parseHistoryIDs(
			record.ID, record.ExecutionID, record.RunID, record.NodeID, record.EgressID,
		)
		if err != nil {
			return ProbeSnapshotPage{}, err
		}
		owner, err := s.historyOwner(ctx, record.NodeID, record.EgressID)
		if err != nil {
			return ProbeSnapshotPage{}, err
		}
		var previousSnapshotID *uuid.UUID
		if record.PreviousSnapshotID != "" {
			parsed, err := uuid.Parse(record.PreviousSnapshotID)
			if err != nil {
				return ProbeSnapshotPage{}, err
			}
			previousSnapshotID = &parsed
		}
		items = append(items, ProbeSnapshotSummary{
			ID: id, ExecutionID: executionID, RunID: runID, NodeID: nodeID, EgressID: egressID,
			Owner: owner, Sequence: record.Sequence, Trigger: record.Trigger, RunStatus: record.RunStatus,
			ObservedAt: time.Unix(record.ObservedAt, 0).UTC(),
			ReceivedAt: time.Unix(record.ReceivedAt, 0).UTC(), EncodedSize: record.EncodedSize,
			Starred: record.Starred, Current: record.IsCurrent, Processed: record.Processed,
			Baseline: record.Baseline == 1, ChangeCount: record.ChangeCount,
			FormatStatus: record.FormatStatus, FormatIssueCount: record.FormatIssueCount,
			PreviousSnapshotID: previousSnapshotID,
		})
	}
	return ProbeSnapshotPage{Items: items, Total: total}, nil
}

func (s *Service) ListHistoryAddressEvents(ctx context.Context, filter HistoryFilter) (AddressHistoryPage, error) {
	filter = normalizeHistoryFilter(filter)
	params := historydb.ListGlobalAddressEventsParams{
		NodeID: filterUUID(filter.NodeID), EgressID: filterUUID(filter.EgressID),
		FromObservedAt: timeUnixPointer(filter.From), ToObservedAt: timeUnixPointer(filter.To),
		EventKind: filter.EventKind, Family: filter.Family,
		PageOffset: (filter.Page - 1) * filter.PageSize, PageSize: filter.PageSize,
	}
	records, err := s.historyQueries.ListGlobalAddressEvents(ctx, params)
	if err != nil {
		return AddressHistoryPage{}, err
	}
	total, err := s.historyQueries.CountGlobalAddressEvents(ctx, historydb.CountGlobalAddressEventsParams{
		NodeID: params.NodeID, EgressID: params.EgressID,
		FromObservedAt: params.FromObservedAt, ToObservedAt: params.ToObservedAt,
		EventKind: params.EventKind, Family: params.Family,
	})
	if err != nil {
		return AddressHistoryPage{}, err
	}
	gapRecords, err := s.historyQueries.ListGlobalAddressGaps(ctx, historydb.ListGlobalAddressGapsParams{
		NodeID: params.NodeID, EgressID: params.EgressID,
		FromObservedAt: params.FromObservedAt, ToObservedAt: params.ToObservedAt,
		PageOffset: (filter.GapPage - 1) * filter.PageSize, PageSize: params.PageSize,
	})
	if err != nil {
		return AddressHistoryPage{}, err
	}
	gapTotal, err := s.historyQueries.CountGlobalAddressGaps(ctx, historydb.CountGlobalAddressGapsParams{
		NodeID: params.NodeID, EgressID: params.EgressID,
		FromObservedAt: params.FromObservedAt, ToObservedAt: params.ToObservedAt,
	})
	if err != nil {
		return AddressHistoryPage{}, err
	}
	result := AddressHistoryPage{
		Events: make([]HistoryAddressEvent, 0, len(records)),
		Gaps:   make([]HistoryAddressGap, 0, len(gapRecords)), Total: total, GapTotal: gapTotal,
	}
	for _, record := range records {
		item, err := addressEventFromRecord(record)
		if err != nil {
			return AddressHistoryPage{}, err
		}
		nodeID, err := uuid.Parse(record.NodeID)
		if err != nil {
			return AddressHistoryPage{}, err
		}
		owner, err := s.historyOwner(ctx, record.NodeID, record.EgressID)
		if err != nil {
			return AddressHistoryPage{}, err
		}
		result.Events = append(result.Events, HistoryAddressEvent{NodeID: nodeID, Owner: owner, Event: item})
	}
	for _, record := range gapRecords {
		item, err := addressGapFromRecord(record)
		if err != nil {
			return AddressHistoryPage{}, err
		}
		nodeID, err := uuid.Parse(record.NodeID)
		if err != nil {
			return AddressHistoryPage{}, err
		}
		owner, err := s.historyOwner(ctx, record.NodeID, record.EgressID)
		if err != nil {
			return AddressHistoryPage{}, err
		}
		result.Gaps = append(result.Gaps, HistoryAddressGap{NodeID: nodeID, Owner: owner, Gap: item})
	}
	return result, nil
}

func (s *Service) ListHistoryProbeGaps(ctx context.Context, filter HistoryFilter) (ProbeHistoryGapPage, error) {
	filter = normalizeHistoryFilter(filter)
	records, err := s.historyQueries.ListGlobalProbeGaps(ctx, historydb.ListGlobalProbeGapsParams{
		NodeID: filterUUID(filter.NodeID), EgressID: filterUUID(filter.EgressID),
		FromObservedAt: timeUnixPointer(filter.From), ToObservedAt: timeUnixPointer(filter.To),
		PageOffset: (filter.Page - 1) * filter.PageSize, PageSize: filter.PageSize,
	})
	if err != nil {
		return ProbeHistoryGapPage{}, err
	}
	total, err := s.historyQueries.CountGlobalProbeGaps(ctx, historydb.CountGlobalProbeGapsParams{
		NodeID: filterUUID(filter.NodeID), EgressID: filterUUID(filter.EgressID),
		FromObservedAt: timeUnixPointer(filter.From), ToObservedAt: timeUnixPointer(filter.To),
	})
	if err != nil {
		return ProbeHistoryGapPage{}, err
	}
	result := make([]ProbeHistoryGap, 0, len(records))
	for _, record := range records {
		id, err := uuid.Parse(record.ID)
		if err != nil {
			return ProbeHistoryGapPage{}, err
		}
		nodeID, err := uuid.Parse(record.NodeID)
		if err != nil {
			return ProbeHistoryGapPage{}, err
		}
		egressID, err := uuid.Parse(record.EgressID)
		if err != nil {
			return ProbeHistoryGapPage{}, err
		}
		owner, err := s.historyOwner(ctx, record.NodeID, record.EgressID)
		if err != nil {
			return ProbeHistoryGapPage{}, err
		}
		result = append(result, ProbeHistoryGap{
			ID: id, NodeID: nodeID, EgressID: egressID, Owner: owner,
			DroppedCount: record.DroppedCount, FirstSequence: record.FirstSequence, LastSequence: record.LastSequence,
			FirstObservedAt: time.Unix(record.FirstObservedAt, 0).UTC(),
			LastObservedAt:  time.Unix(record.LastObservedAt, 0).UTC(),
		})
	}
	return ProbeHistoryGapPage{Items: result, Total: total}, nil
}

func (s *Service) ListHistoryFormatEvents(ctx context.Context, filter HistoryFilter) (FormatEventPage, error) {
	filter = normalizeHistoryFilter(filter)
	records, err := s.historyQueries.ListGlobalFormatEvents(ctx, historydb.ListGlobalFormatEventsParams{
		NodeID: filterUUID(filter.NodeID), EgressID: filterUUID(filter.EgressID),
		FromObservedAt: timeUnixPointer(filter.From), ToObservedAt: timeUnixPointer(filter.To),
		PageOffset: (filter.Page - 1) * filter.PageSize, PageSize: filter.PageSize,
	})
	if err != nil {
		return FormatEventPage{}, err
	}
	total, err := s.historyQueries.CountGlobalFormatEvents(ctx, historydb.CountGlobalFormatEventsParams{
		NodeID: filterUUID(filter.NodeID), EgressID: filterUUID(filter.EgressID),
		FromObservedAt: timeUnixPointer(filter.From), ToObservedAt: timeUnixPointer(filter.To),
	})
	if err != nil {
		return FormatEventPage{}, err
	}
	result := make([]FormatEvent, 0, len(records))
	for _, record := range records {
		ids := []string{record.ID, record.NodeID, record.EgressID, record.ExecutionID, record.SnapshotID}
		parsed := make([]uuid.UUID, len(ids))
		for index, value := range ids {
			parsed[index], err = uuid.Parse(value)
			if err != nil {
				return FormatEventPage{}, err
			}
		}
		var issues []centerhistory.FormatIssue
		if err := json.Unmarshal(record.IssuesJson, &issues); err != nil {
			return FormatEventPage{}, err
		}
		owner, err := s.historyOwner(ctx, record.NodeID, record.EgressID)
		if err != nil {
			return FormatEventPage{}, err
		}
		result = append(result, FormatEvent{
			ID: parsed[0], NodeID: parsed[1], EgressID: parsed[2], ExecutionID: parsed[3], SnapshotID: parsed[4],
			Owner: owner, Sequence: record.Sequence, Kind: record.Kind, Issues: issues,
			ObservedAt: time.Unix(record.ObservedAt, 0).UTC(), RecordedAt: time.Unix(record.RecordedAt, 0).UTC(),
		})
	}
	return FormatEventPage{Items: result, Total: total}, nil
}

func (s *Service) CompareProbeSnapshots(ctx context.Context, beforeID, afterID uuid.UUID) (ProbeSnapshotComparison, error) {
	before, err := s.ProbeSnapshot(ctx, beforeID)
	if err != nil {
		return ProbeSnapshotComparison{}, err
	}
	after, err := s.ProbeSnapshot(ctx, afterID)
	if err != nil {
		return ProbeSnapshotComparison{}, err
	}
	if before.EgressID != after.EgressID {
		return ProbeSnapshotComparison{}, ErrSnapshotEgressMismatch
	}
	afterByID := make(map[string]centerhistory.FieldValue, len(after.Fields))
	for _, field := range after.Fields {
		afterByID[field.ID] = field
	}
	semanticChanges := centerhistory.Compare(
		centerhistory.Report{Fields: before.Fields}, centerhistory.Report{Fields: after.Fields},
	)
	changed := make(map[string]bool, len(semanticChanges))
	for _, change := range semanticChanges {
		changed[change.FieldID] = true
	}
	fields := make([]FieldComparison, 0, len(before.Fields))
	for _, previous := range before.Fields {
		current := afterByID[previous.ID]
		fields = append(fields, FieldComparison{
			ID: previous.ID, Group: previous.Group, Path: previous.Path,
			ExpectedTypes: previous.ExpectedTypes, Before: previous, After: current, Changed: changed[previous.ID],
		})
	}
	return ProbeSnapshotComparison{
		BeforeID: beforeID, AfterID: afterID, EgressID: before.EgressID, Fields: fields,
	}, nil
}

func (s *Service) SetProbeSnapshotStarred(ctx context.Context, id uuid.UUID, starred bool) (ProbeSnapshot, error) {
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	if _, err := s.historyQueries.GetProbeSnapshot(ctx, id.String()); errors.Is(err, sql.ErrNoRows) {
		return ProbeSnapshot{}, ErrProbeSnapshotNotFound
	} else if err != nil {
		return ProbeSnapshot{}, err
	}
	if starred {
		if _, err := s.historyQueries.StarProbeSnapshot(ctx, historydb.StarProbeSnapshotParams{
			SnapshotID: id.String(), StarredAt: s.now().UTC().Unix(),
		}); err != nil {
			return ProbeSnapshot{}, err
		}
	} else if _, err := s.historyQueries.UnstarProbeSnapshot(ctx, id.String()); err != nil {
		return ProbeSnapshot{}, err
	}
	return s.ProbeSnapshot(ctx, id)
}

func (s *Service) UpdateHistoryRetention(ctx context.Context, update HistoryRetentionUpdate) (HistoryState, error) {
	if err := validateHistoryRetention(update); err != nil {
		return HistoryState{}, err
	}
	s.retentionMu.Lock()
	defer s.retentionMu.Unlock()
	now := s.now().UTC().Truncate(time.Second).Unix()
	changed, err := s.queries.UpdateHistoryRetentionSettings(ctx, configdb.UpdateHistoryRetentionSettingsParams{
		Mode: update.Mode, MaxAgeDays: update.MaxAgeDays,
		MaxLogicalBytes: update.MaxLogicalBytes, UpdatedAt: now,
	})
	if err != nil {
		return HistoryState{}, err
	}
	if changed != 1 {
		return HistoryState{}, errors.New("history retention settings were not updated")
	}
	if _, err := s.cleanupHistory(ctx); err != nil {
		return HistoryState{}, err
	}
	return s.History(ctx)
}

func (s *Service) CleanupHistory(ctx context.Context) (HistoryCleanupResult, error) {
	s.retentionMu.Lock()
	defer s.retentionMu.Unlock()
	return s.cleanupHistory(ctx)
}

func (s *Service) cleanupHistory(ctx context.Context) (HistoryCleanupResult, error) {
	settings, err := s.historyRetentionSettings(ctx)
	if err != nil {
		return HistoryCleanupResult{}, err
	}
	var deleted int64
	var cutoff *int64
	if settings.Mode == "age" {
		value := s.now().UTC().Add(-time.Duration(*settings.MaxAgeDays) * 24 * time.Hour).Unix()
		cutoff = &value
	}
	for settings.Mode != "indefinite" {
		if err := ctx.Err(); err != nil {
			return HistoryCleanupResult{}, s.recordCleanupFailure(ctx, deleted, err)
		}
		if settings.Mode == "size" {
			usage, err := s.historyQueries.GetHistoryLogicalUsage(ctx)
			if err != nil {
				return HistoryCleanupResult{}, s.recordCleanupFailure(ctx, deleted, err)
			}
			if usage.LogicalBytes <= *settings.MaxLogicalBytes {
				break
			}
		}
		var maxLogicalBytes *int64
		if settings.Mode == "size" {
			maxLogicalBytes = settings.MaxLogicalBytes
		}
		removed, available, err := s.cleanupHistoryBatch(ctx, cutoff, maxLogicalBytes)
		if err != nil {
			return HistoryCleanupResult{}, s.recordCleanupFailure(ctx, deleted, err)
		}
		deleted += removed
		if available == 0 || removed == 0 || settings.Mode == "age" && available < retentionCleanupBatch {
			break
		}
	}
	completedAt := s.now().UTC().Truncate(time.Second)
	if err := s.queries.RecordHistoryRetentionCleanup(ctx, configdb.RecordHistoryRetentionCleanupParams{
		LastCleanupAt: timeUnixPointer(&completedAt), LastCleanupDeletedItems: deleted,
	}); err != nil {
		return HistoryCleanupResult{}, err
	}
	currentSettings, err := s.historyRetentionSettings(ctx)
	if err != nil {
		return HistoryCleanupResult{}, err
	}
	usage, err := s.historyUsage(ctx, currentSettings)
	if err != nil {
		return HistoryCleanupResult{}, err
	}
	return HistoryCleanupResult{DeletedItems: deleted, CompletedAt: completedAt, Usage: usage}, nil
}

func (s *Service) RunRetentionWorker(ctx context.Context, logger *log.Logger) {
	if logger == nil {
		panic("history retention worker logger must not be nil")
	}
	run := func() {
		if _, err := s.CleanupHistory(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Printf("history retention cleanup failed: %v", err)
		}
	}
	run()
	ticker := time.NewTicker(retentionCleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

func (s *Service) cleanupHistoryBatch(
	ctx context.Context,
	cutoff *int64,
	maxLogicalBytes *int64,
) (int64, int, error) {
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	transaction, err := s.history.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer transaction.Rollback()
	queries := s.historyQueries.WithTx(transaction)
	candidates, err := queries.ListRetentionCandidates(ctx, historydb.ListRetentionCandidatesParams{
		OlderThan: cutoff, PageSize: retentionCleanupBatch,
	})
	if err != nil {
		return 0, 0, err
	}
	var deleted int64
	for _, candidate := range candidates {
		var changed int64
		switch candidate.Category {
		case "execution":
			changed, err = queries.DeleteRetentionExecution(ctx, candidate.ID)
		case "address-event":
			changed, err = queries.DeleteRetentionAddressEvent(ctx, candidate.ID)
		case "address-gap":
			changed, err = queries.DeleteRetentionAddressGap(ctx, candidate.ID)
		case "probe-gap":
			changed, err = queries.DeleteRetentionProbeGap(ctx, candidate.ID)
		default:
			return 0, 0, fmt.Errorf("unknown history retention category %q", candidate.Category)
		}
		if err != nil {
			return 0, 0, err
		}
		deleted += changed
		if changed > 0 && maxLogicalBytes != nil {
			if err := queries.DeleteEmptyProbeRuns(ctx); err != nil {
				return 0, 0, err
			}
			usage, err := queries.GetHistoryLogicalUsage(ctx)
			if err != nil {
				return 0, 0, err
			}
			if usage.LogicalBytes <= *maxLogicalBytes {
				break
			}
		}
	}
	if err := queries.DeleteEmptyProbeRuns(ctx); err != nil {
		return 0, 0, err
	}
	if err := transaction.Commit(); err != nil {
		return 0, 0, err
	}
	return deleted, len(candidates), nil
}

func (s *Service) historyRetentionSettings(ctx context.Context) (HistoryRetentionSettings, error) {
	record, err := s.queries.GetHistoryRetentionSettings(ctx)
	if err != nil {
		return HistoryRetentionSettings{}, err
	}
	return HistoryRetentionSettings{
		Mode: record.Mode, MaxAgeDays: record.MaxAgeDays, MaxLogicalBytes: record.MaxLogicalBytes,
		UpdatedAt: time.Unix(record.UpdatedAt, 0).UTC(), LastCleanupAt: timePointer(record.LastCleanupAt),
		LastCleanupDeletedItems: record.LastCleanupDeletedItems, LastCleanupError: record.LastCleanupError,
	}, nil
}

func (s *Service) historyUsage(ctx context.Context, settings HistoryRetentionSettings) (HistoryUsage, error) {
	logical, err := s.historyQueries.GetHistoryLogicalUsage(ctx)
	if err != nil {
		return HistoryUsage{}, err
	}
	protected, err := s.historyQueries.GetProtectedHistoryLogicalBytes(ctx)
	if err != nil {
		return HistoryUsage{}, err
	}
	databaseBytes, walBytes, sharedMemoryBytes, err := sqlitePhysicalUsage(ctx, s.history)
	if err != nil {
		return HistoryUsage{}, err
	}
	result := HistoryUsage{
		LogicalBytes: logical.LogicalBytes, ProtectedLogicalBytes: protected,
		RecordCount: logical.RecordCount, DatabaseBytes: databaseBytes,
		WALBytes: walBytes, SharedMemoryBytes: sharedMemoryBytes,
	}
	if settings.Mode == "size" && result.LogicalBytes > *settings.MaxLogicalBytes {
		result.OverBudget = true
		result.OverageBytes = result.LogicalBytes - *settings.MaxLogicalBytes
	}
	return result, nil
}

func (s *Service) recordCleanupFailure(ctx context.Context, deleted int64, cause error) error {
	now := s.now().UTC().Truncate(time.Second)
	message := cause.Error()
	for len(message) > 4096 {
		_, size := utf8.DecodeLastRuneInString(message)
		message = message[:len(message)-size]
	}
	if err := s.queries.RecordHistoryRetentionCleanup(ctx, configdb.RecordHistoryRetentionCleanupParams{
		LastCleanupAt: timeUnixPointer(&now), LastCleanupDeletedItems: deleted, LastCleanupError: &message,
	}); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}

func sqlitePhysicalUsage(ctx context.Context, database *sql.DB) (int64, int64, int64, error) {
	rows, err := database.QueryContext(ctx, "PRAGMA database_list")
	if err != nil {
		return 0, 0, 0, err
	}
	defer rows.Close()
	path := ""
	for rows.Next() {
		var sequence int
		var name, candidate string
		if err := rows.Scan(&sequence, &name, &candidate); err != nil {
			return 0, 0, 0, err
		}
		if name == "main" {
			path = candidate
		}
	}
	if err := rows.Err(); err != nil {
		return 0, 0, 0, err
	}
	if path == "" {
		return 0, 0, 0, errors.New("history database path is unavailable")
	}
	usage := make([]int64, 3)
	for index, candidate := range []string{path, path + "-wal", path + "-shm"} {
		info, err := os.Stat(candidate)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return 0, 0, 0, err
		}
		usage[index] = info.Size()
	}
	return usage[0], usage[1], usage[2], nil
}

func validateHistoryRetention(update HistoryRetentionUpdate) error {
	switch update.Mode {
	case "indefinite":
		if update.MaxAgeDays != nil || update.MaxLogicalBytes != nil {
			return ErrInvalidHistoryRetention
		}
	case "age":
		if update.MaxAgeDays == nil || *update.MaxAgeDays < 1 || *update.MaxAgeDays > 36500 || update.MaxLogicalBytes != nil {
			return ErrInvalidHistoryRetention
		}
	case "size":
		if update.MaxLogicalBytes == nil || *update.MaxLogicalBytes < 1024*1024 ||
			*update.MaxLogicalBytes > 1024*1024*1024*1024 || update.MaxAgeDays != nil {
			return ErrInvalidHistoryRetention
		}
	default:
		return ErrInvalidHistoryRetention
	}
	return nil
}

func normalizeHistoryFilter(filter HistoryFilter) HistoryFilter {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.GapPage < 1 {
		filter.GapPage = 1
	}
	if filter.PageSize < 1 || filter.PageSize > 100 {
		filter.PageSize = 50
	}
	return filter
}

func filterUUID(value *uuid.UUID) string {
	if value == nil {
		return ""
	}
	return value.String()
}

func (s *Service) historyOwner(ctx context.Context, nodeID, egressID string) (HistoryOwner, error) {
	owner := HistoryOwner{}
	node, err := s.queries.GetNodeByID(ctx, nodeID)
	if err == nil {
		owner.NodeName = &node.Name
	} else if !errors.Is(err, sql.ErrNoRows) {
		return HistoryOwner{}, err
	}
	egress, err := s.queries.GetNodeEgress(ctx, configdb.GetNodeEgressParams{NodeID: nodeID, ID: egressID})
	if err == nil {
		owner.EgressName = &egress.Name
	} else if !errors.Is(err, sql.ErrNoRows) {
		return HistoryOwner{}, err
	}
	return owner, nil
}

func parseHistoryIDs(values ...string) (uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, error) {
	if len(values) != 5 {
		return uuid.Nil, uuid.Nil, uuid.Nil, uuid.Nil, uuid.Nil, errors.New("five history identifiers are required")
	}
	parsed := make([]uuid.UUID, len(values))
	for index, value := range values {
		id, err := uuid.Parse(value)
		if err != nil {
			return uuid.Nil, uuid.Nil, uuid.Nil, uuid.Nil, uuid.Nil, err
		}
		parsed[index] = id
	}
	return parsed[0], parsed[1], parsed[2], parsed[3], parsed[4], nil
}

func timeUnixPointer(value *time.Time) *int64 {
	if value == nil {
		return nil
	}
	result := value.Unix()
	return &result
}
