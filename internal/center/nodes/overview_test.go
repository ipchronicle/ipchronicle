package nodes

import (
	"testing"
	"time"
)

func TestOverviewAggregatesCurrentStateWithoutPerNodeHistoryRequests(t *testing.T) {
	fixture := newProbeServiceFixture(t, 512*1024*1024)
	nextScheduledAt := fixture.now.Add(12 * time.Hour)
	if _, err := fixture.service.Poll(
		fixture.ctx, fixture.registration.Credential, fixture.metadata,
		fixture.configuration.Revision, nil, nil, nil, nil,
		AddressUpload{ProbeStatus: &ProbeStatus{NextScheduledAt: &nextScheduledAt}},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.CreateCompleteProbeTask(
		fixture.ctx, fixture.registration.NodeID, fixture.probeTargetIDs(),
	); err != nil {
		t.Fatal(err)
	}
	running, terminal, executions := probeArtifacts(fixture, []string{"succeeded", "failed"})
	uploadProbeRun(t, fixture, running)
	for index := range executions {
		uploadProbeExecution(t, fixture, running, executions[index])
	}
	uploadProbeRun(t, fixture, terminal)

	overview, err := fixture.service.Overview(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !overview.CheckedAt.Equal(fixture.now) || overview.HistoryOverBudget {
		t.Fatalf("overview metadata = %#v", overview)
	}
	if len(overview.Nodes) != 1 || overview.Nodes[0].ID != fixture.registration.NodeID ||
		overview.Nodes[0].Status != "online" || overview.Nodes[0].PausedLowMemory ||
		overview.Nodes[0].NextScheduledAt == nil || !overview.Nodes[0].NextScheduledAt.Equal(nextScheduledAt) {
		t.Fatalf("overview node = %#v", overview.Nodes)
	}
	if overview.Nodes[0].LatestProbeRun == nil || overview.Nodes[0].LatestProbeRun.Status != "partial" ||
		overview.Nodes[0].LatestProbeRun.CompletedExecutions != 2 {
		t.Fatalf("latest overview run = %#v", overview.Nodes[0].LatestProbeRun)
	}
	if len(overview.Nodes[0].PublicAddresses) != 2 {
		t.Fatalf("overview addresses = %#v", overview.Nodes[0].PublicAddresses)
	}
	healthy, failed := 0, 0
	for _, address := range overview.Nodes[0].PublicAddresses {
		if !address.LikelyNAT {
			t.Fatalf("overview address lost NAT trait: %#v", address)
		}
		if address.LatestProbeOutcome == nil {
			t.Fatalf("overview address lost probe outcome: %#v", address)
		}
		switch *address.LatestProbeOutcome {
		case "healthy":
			healthy++
			if address.LatestSnapshotID == nil || address.FormatStatus == nil {
				t.Fatalf("successful overview address lacks snapshot state: %#v", address)
			}
		case "failed":
			failed++
			if address.LatestSnapshotID != nil || address.LatestProbeRunID == nil {
				t.Fatalf("failed-only overview address was hidden or linked incorrectly: %#v", address)
			}
		default:
			t.Fatalf("unexpected overview probe outcome: %#v", address)
		}
	}
	if healthy != 1 || failed != 1 {
		t.Fatalf("overview outcomes healthy=%d failed=%d", healthy, failed)
	}
	if len(overview.ActiveTasks) != 1 || overview.ActiveTasks[0].Status != "pending" ||
		len(overview.RecentProbeRuns) != 1 || overview.RecentProbeRuns[0].Status != "partial" {
		t.Fatalf("overview tasks and activity = %#v, %#v", overview.ActiveTasks, overview.RecentProbeRuns)
	}
}
