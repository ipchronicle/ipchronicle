package nodes

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/center/database/historydb"
)

const overviewActivityLimit = 8

type Overview struct {
	CheckedAt           time.Time
	HistoryOverBudget   bool
	Nodes               []OverviewNode
	ActiveTasks         []OverviewTask
	RecentProbeRuns     []ProbeRunSummary
	RecentAddressEvents []OverviewAddressEvent
}

type OverviewNode struct {
	ID                           uuid.UUID
	Name                         string
	Status                       string
	ConfigurationStatus          string
	DesiredConfigurationRevision int64
	AppliedConfigurationRevision int64
	LastSeenAt                   *time.Time
	PausedLowMemory              bool
	NextScheduledAt              *time.Time
	LatestProbeRun               *ProbeRunSummary
	PublicAddresses              []OverviewPublicAddress
}

type OverviewPublicAddress struct {
	ID                 uuid.UUID
	Address            string
	Family             string
	ProbeEnabled       bool
	LikelyNAT          bool
	ProxyPath          bool
	LastSeenAt         time.Time
	LatestSnapshotID   *uuid.UUID
	LatestProbeAt      *time.Time
	LatestProbeRunID   *uuid.UUID
	LatestProbeOutcome *string
	FormatStatus       *string
}

type OverviewTask struct {
	ID        uuid.UUID
	NodeID    uuid.UUID
	Kind      string
	Status    string
	CreatedAt time.Time
	ExpiresAt time.Time
	RunID     *uuid.UUID
}

type OverviewAddressEvent struct {
	ID              uuid.UUID
	NodeID          uuid.UUID
	PublicAddressID uuid.UUID
	Kind            string
	Family          string
	PublicAddress   *string
	FailureReason   *string
	ObservedAt      time.Time
}

type overviewAddressTraits struct {
	LikelyNAT bool
	ProxyPath bool
}

type overviewProbeState struct {
	LatestSnapshotID   *uuid.UUID
	LatestProbeAt      *time.Time
	LatestProbeRunID   *uuid.UUID
	LatestProbeOutcome *string
	FormatStatus       *string
}

func (s *Service) Overview(ctx context.Context) (Overview, error) {
	checkedAt := s.now().UTC().Truncate(time.Second)
	nodeRecords, err := s.List(ctx)
	if err != nil {
		return Overview{}, err
	}
	result := Overview{
		CheckedAt: checkedAt, Nodes: make([]OverviewNode, 0, len(nodeRecords)),
		ActiveTasks: make([]OverviewTask, 0), RecentProbeRuns: make([]ProbeRunSummary, 0),
		RecentAddressEvents: make([]OverviewAddressEvent, 0),
	}
	nodeIndexes := make(map[string]int, len(nodeRecords))
	for _, node := range nodeRecords {
		nodeIndexes[node.ID.String()] = len(result.Nodes)
		result.Nodes = append(result.Nodes, OverviewNode{
			ID: node.ID, Name: node.Name, Status: node.Status,
			ConfigurationStatus:          node.ConfigurationStatus,
			DesiredConfigurationRevision: node.DesiredConfigurationRevision,
			AppliedConfigurationRevision: node.AppliedConfigurationRevision,
			LastSeenAt:                   node.LastSeenAt,
			PublicAddresses:              make([]OverviewPublicAddress, 0, len(node.PublicAddresses)),
		})
	}

	probeDetails, err := s.queries.ListOverviewNodeProbeDetails(ctx)
	if err != nil {
		return Overview{}, err
	}
	for _, detail := range probeDetails {
		index, ok := nodeIndexes[detail.NodeID]
		if !ok {
			continue
		}
		result.Nodes[index].PausedLowMemory = detail.PhysicalMemoryBytes != nil &&
			*detail.PhysicalMemoryBytes < minimumProbeMemoryBytes && detail.ProbeLowMemoryOverride != 1
		result.Nodes[index].NextScheduledAt = timePointerFromUnix(detail.NextScheduledAt)
	}

	traitRecords, err := s.queries.ListOverviewPublicAddressTraits(ctx)
	if err != nil {
		return Overview{}, err
	}
	traits := make(map[string]overviewAddressTraits, len(traitRecords))
	for _, record := range traitRecords {
		traits[overviewAddressKey(record.NodeID, record.PublicAddressID)] = overviewAddressTraits{
			LikelyNAT: record.LikelyNat == 1, ProxyPath: record.ProxyPath == 1,
		}
	}
	probeStates, err := s.overviewProbeStates(ctx)
	if err != nil {
		return Overview{}, err
	}
	for nodeIndex, node := range nodeRecords {
		for _, address := range node.PublicAddresses {
			trait := traits[overviewAddressKey(node.ID.String(), address.ID.String())]
			probe := probeStates[address.ID.String()]
			result.Nodes[nodeIndex].PublicAddresses = append(result.Nodes[nodeIndex].PublicAddresses, OverviewPublicAddress{
				ID: address.ID, Address: address.Address, Family: address.Family,
				ProbeEnabled: address.ProbeEnabled, LikelyNAT: trait.LikelyNAT,
				ProxyPath: trait.ProxyPath, LastSeenAt: address.LastSeenAt,
				LatestSnapshotID: probe.LatestSnapshotID, LatestProbeAt: probe.LatestProbeAt,
				LatestProbeRunID: probe.LatestProbeRunID, LatestProbeOutcome: probe.LatestProbeOutcome,
				FormatStatus: probe.FormatStatus,
			})
		}
	}

	latestRuns, err := s.historyQueries.ListOverviewLatestNodeProbeRuns(ctx)
	if err != nil {
		return Overview{}, err
	}
	for _, record := range latestRuns {
		index, ok := nodeIndexes[record.NodeID]
		if !ok {
			continue
		}
		run, err := overviewProbeRunSummary(
			record.ID, record.NodeID, record.Trigger, record.StartedAt, record.CompletedAt,
			record.Status, record.ExpectedExecutions, record.CompletedExecutions,
		)
		if err != nil {
			return Overview{}, err
		}
		result.Nodes[index].LatestProbeRun = &run
	}

	taskRecords, err := s.queries.ListOverviewActiveTasks(ctx)
	if err != nil {
		return Overview{}, err
	}
	for _, record := range taskRecords {
		id, err := uuid.Parse(record.ID)
		if err != nil {
			return Overview{}, fmt.Errorf("parse stored overview task ID %q: %w", record.ID, err)
		}
		nodeID, err := uuid.Parse(record.NodeID)
		if err != nil {
			return Overview{}, fmt.Errorf("parse stored overview task node ID %q: %w", record.NodeID, err)
		}
		runID, err := optionalUUID(record.RunID)
		if err != nil {
			return Overview{}, fmt.Errorf("parse stored overview task run ID: %w", err)
		}
		result.ActiveTasks = append(result.ActiveTasks, OverviewTask{
			ID: id, NodeID: nodeID, Kind: record.Kind, Status: record.Status,
			CreatedAt: time.Unix(record.CreatedAt, 0).UTC(), ExpiresAt: time.Unix(record.ExpiresAt, 0).UTC(),
			RunID: runID,
		})
	}

	recentRuns, err := s.historyQueries.ListOverviewRecentProbeRuns(ctx)
	if err != nil {
		return Overview{}, err
	}
	for _, record := range recentRuns {
		run, err := overviewProbeRunSummary(
			record.ID, record.NodeID, record.Trigger, record.StartedAt, record.CompletedAt,
			record.Status, record.ExpectedExecutions, record.CompletedExecutions,
		)
		if err != nil {
			return Overview{}, err
		}
		result.RecentProbeRuns = append(result.RecentProbeRuns, run)
	}

	eventRecords, err := s.historyQueries.ListGlobalAddressEvents(ctx, historydb.ListGlobalAddressEventsParams{
		PageSize: overviewActivityLimit,
	})
	if err != nil {
		return Overview{}, err
	}
	for _, record := range eventRecords {
		id, nodeID, publicAddressID, err := parseOverviewEventIDs(record.ID, record.NodeID, record.PublicAddressID)
		if err != nil {
			return Overview{}, err
		}
		result.RecentAddressEvents = append(result.RecentAddressEvents, OverviewAddressEvent{
			ID: id, NodeID: nodeID, PublicAddressID: publicAddressID,
			Kind: record.Kind, Family: record.Family, PublicAddress: record.PublicAddress,
			FailureReason: record.FailureReason, ObservedAt: time.Unix(record.ObservedAt, 0).UTC(),
		})
	}

	history, err := s.History(ctx)
	if err != nil {
		return Overview{}, err
	}
	result.HistoryOverBudget = history.Usage.OverBudget
	return result, nil
}

func (s *Service) overviewProbeStates(ctx context.Context) (map[string]overviewProbeState, error) {
	records, err := s.historyQueries.ListOverviewCurrentProbeStates(ctx)
	if err != nil {
		return nil, err
	}
	states := make(map[string]overviewProbeState, len(records))
	for _, record := range records {
		state := overviewProbeState{
			LatestProbeAt:      timePointerFromUnix(record.LatestProbeAt),
			LatestProbeOutcome: record.LatestProbeOutcome, FormatStatus: record.FormatStatus,
		}
		if record.SnapshotID != nil {
			id, err := uuid.Parse(*record.SnapshotID)
			if err != nil {
				return nil, fmt.Errorf("parse stored overview snapshot ID %q: %w", *record.SnapshotID, err)
			}
			state.LatestSnapshotID = &id
			if state.LatestProbeAt == nil {
				state.LatestProbeAt = timePointerFromUnix(record.ObservedAt)
			}
		}
		if record.LatestProbeRunID != nil {
			id, err := uuid.Parse(*record.LatestProbeRunID)
			if err != nil {
				return nil, fmt.Errorf("parse stored overview probe run ID %q: %w", *record.LatestProbeRunID, err)
			}
			state.LatestProbeRunID = &id
		}
		states[record.EgressID] = state
	}
	return states, nil
}

func overviewProbeRunSummary(
	idValue, nodeIDValue, trigger string,
	startedAt int64,
	completedAt *int64,
	status string,
	expectedExecutions, completedExecutions int64,
) (ProbeRunSummary, error) {
	id, err := uuid.Parse(idValue)
	if err != nil {
		return ProbeRunSummary{}, fmt.Errorf("parse stored overview probe run ID %q: %w", idValue, err)
	}
	nodeID, err := uuid.Parse(nodeIDValue)
	if err != nil {
		return ProbeRunSummary{}, fmt.Errorf("parse stored overview probe run node ID %q: %w", nodeIDValue, err)
	}
	return ProbeRunSummary{
		ID: id, NodeID: nodeID, Trigger: trigger, StartedAt: time.Unix(startedAt, 0).UTC(),
		CompletedAt: timePointerFromUnix(completedAt), Status: status,
		ExpectedExecutions: expectedExecutions, CompletedExecutions: completedExecutions,
	}, nil
}

func parseOverviewEventIDs(idValue, nodeIDValue, publicAddressIDValue string) (uuid.UUID, uuid.UUID, uuid.UUID, error) {
	id, err := uuid.Parse(idValue)
	if err != nil {
		return uuid.Nil, uuid.Nil, uuid.Nil, fmt.Errorf("parse stored overview event ID %q: %w", idValue, err)
	}
	nodeID, err := uuid.Parse(nodeIDValue)
	if err != nil {
		return uuid.Nil, uuid.Nil, uuid.Nil, fmt.Errorf("parse stored overview event node ID %q: %w", nodeIDValue, err)
	}
	publicAddressID, err := uuid.Parse(publicAddressIDValue)
	if err != nil {
		return uuid.Nil, uuid.Nil, uuid.Nil, fmt.Errorf("parse stored overview event public address ID %q: %w", publicAddressIDValue, err)
	}
	return id, nodeID, publicAddressID, nil
}

func overviewAddressKey(nodeID, publicAddressID string) string {
	return nodeID + "\x00" + publicAddressID
}

func timePointerFromUnix(value *int64) *time.Time {
	if value == nil {
		return nil
	}
	parsed := time.Unix(*value, 0).UTC()
	return &parsed
}

func optionalUUID(value *string) (*uuid.UUID, error) {
	if value == nil {
		return nil, nil
	}
	parsed, err := uuid.Parse(*value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}
