package nodes

import (
	"context"
	"slices"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/mod/semver"
)

type AttentionGroup struct {
	Kind             string
	Count            int
	NodeIDs          []uuid.UUID
	PublicAddressIDs []uuid.UUID
	Samples          []string
}

type attentionBuilder struct {
	AttentionGroup
	subjects map[string]bool
}

func (s *Service) overviewAttention(ctx context.Context, overview Overview, nodes []Node, history HistoryState) ([]AttentionGroup, error) {
	paths, err := s.queries.ListAttentionDiscoveryPaths(ctx)
	if err != nil {
		return nil, err
	}
	activeAddresses := map[string]bool{}
	for _, path := range paths {
		if path.PublicAddressID != nil {
			activeAddresses[overviewAddressKey(path.NodeID, *path.PublicAddressID)] = true
		}
	}
	groups := map[string]*attentionBuilder{}
	add := func(kind, subject, label string, nodeID, addressID uuid.UUID) {
		group := groups[kind]
		if group == nil {
			group = &attentionBuilder{AttentionGroup: AttentionGroup{Kind: kind, NodeIDs: []uuid.UUID{}, PublicAddressIDs: []uuid.UUID{}, Samples: []string{}}, subjects: map[string]bool{}}
			groups[kind] = group
		}
		if nodeID != uuid.Nil && !slices.Contains(group.NodeIDs, nodeID) {
			group.NodeIDs = append(group.NodeIDs, nodeID)
		}
		if addressID != uuid.Nil && !slices.Contains(group.PublicAddressIDs, addressID) {
			group.PublicAddressIDs = append(group.PublicAddressIDs, addressID)
		}
		if group.subjects[subject] {
			return
		}
		group.subjects[subject] = true
		group.Count++
		if label != "" && len(group.Samples) < 3 {
			group.Samples = append(group.Samples, label)
		}
	}
	active := map[string]Node{}
	for _, node := range nodes {
		if !node.Enabled || node.Status == "revoked" || node.DeletionStatus != nil {
			continue
		}
		active[node.ID.String()] = node
		if node.Status == "offline" {
			add("offline", node.ID.String(), node.Name, node.ID, uuid.Nil)
		}
		if node.Status == "online" && (node.ConfigurationStatus == "failed" || (node.ConfigurationStatus == "pending" && !overview.CheckedAt.Before(node.ConfigurationUpdatedAt.Add(OnlineWindow)))) {
			add("configuration", node.ID.String(), node.Name, node.ID, uuid.Nil)
		}
	}
	for _, node := range overview.Nodes {
		if _, ok := active[node.ID.String()]; !ok {
			continue
		}
		if node.PausedLowMemory {
			add("memory", node.ID.String(), node.Name, node.ID, uuid.Nil)
		}
		for _, address := range node.PublicAddresses {
			if !address.ProbeEnabled || !activeAddresses[overviewAddressKey(node.ID.String(), address.ID.String())] {
				continue
			}
			if address.LatestProbeOutcome != nil && *address.LatestProbeOutcome == "failed" {
				add("probe", address.ID.String(), node.Name+" · "+address.Address, node.ID, address.ID)
			}
			if address.FormatStatus != nil && *address.FormatStatus == "mismatch" {
				add("format", address.ID.String(), node.Name+" · "+address.Address, node.ID, address.ID)
			}
		}
	}
	failedPaths, err := s.historyQueries.ListAttentionDiscoveryFailures(ctx)
	if err != nil {
		return nil, err
	}
	failures := make(map[string]bool, len(failedPaths))
	for _, id := range failedPaths {
		failures[id] = true
	}
	for _, path := range paths {
		if node, ok := active[path.NodeID]; ok && node.Status == "online" && failures[path.ID] {
			add("discovery", node.ID.String(), node.Name, node.ID, uuid.Nil)
		}
	}
	updates, err := s.queries.ListAttentionUpdateTasks(ctx)
	if err != nil {
		return nil, err
	}
	for _, update := range updates {
		node, ok := active[update.NodeID]
		if !ok || update.TargetVersion == nil || !slices.Contains([]string{"failed", "rolled-back", "rejected", "expired"}, update.Status) {
			continue
		}
		current, target := "v"+strings.TrimPrefix(node.AgentVersion, "v"), "v"+strings.TrimPrefix(*update.TargetVersion, "v")
		if semver.IsValid(current) && semver.IsValid(target) && semver.Compare(current, target) >= 0 {
			continue
		}
		add("update", node.ID.String(), node.Name, node.ID, uuid.Nil)
	}
	failedDeliveries, err := s.historyQueries.ListAttentionDeliveryFailures(ctx)
	if err != nil {
		return nil, err
	}
	failedSenders := make(map[string]bool, len(failedDeliveries))
	for _, id := range failedDeliveries {
		failedSenders[id] = true
	}
	senders, err := s.queries.ListAttentionSenders(ctx)
	if err != nil {
		return nil, err
	}
	for _, sender := range senders {
		if failedSenders[sender.ID] {
			add("delivery", sender.ID, sender.Name, uuid.Nil, uuid.Nil)
		}
	}
	if history.Retention.LastCleanupError != nil || (history.Retention.LastCleanupAt != nil && history.Usage.OverBudget) {
		add("retention", "history", "", uuid.Nil, uuid.Nil)
	}
	result := make([]AttentionGroup, 0, len(groups))
	for _, kind := range []string{"probe", "format", "discovery", "configuration", "memory", "offline", "update", "delivery", "retention"} {
		if group := groups[kind]; group != nil {
			result = append(result, group.AttentionGroup)
		}
	}
	return result, nil
}
