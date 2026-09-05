package nodes

import (
	"github.com/google/uuid"
	"testing"
	"time"
)

func TestOverviewAttentionLatestEnabledSenderOutcome(t *testing.T) {
	f := newProbeServiceFixture(t, 512*1024*1024)
	senderID := uuid.NewString()
	if _, err := f.service.database.ExecContext(f.ctx, `INSERT INTO notification_senders(id,name,kind,enabled,configuration_encrypted,created_at,updated_at) VALUES(?, 'Test sender', 'webhook', 1, zeroblob(30), ?, ?)`, senderID, f.now.Unix(), f.now.Unix()); err != nil {
		t.Fatal(err)
	}
	deliver := func(status string, at int64) {
		t.Helper()
		eventID := uuid.NewString()
		if _, err := f.store.History.ExecContext(f.ctx, `INSERT INTO notification_events(id,event_type,source_kind,source_id,payload_json,observed_at,recorded_at) VALUES(?,'test','test',?,'{}',?,?)`, eventID, eventID, at, at); err != nil {
			t.Fatal(err)
		}
		var code *string
		if status == "failed" {
			value := "http_status"
			code = &value
		}
		if _, err := f.store.History.ExecContext(f.ctx, `INSERT INTO notification_deliveries(id,event_id,sender_id,sender_name,sender_kind,event_type,is_test,status,completed_at,error_code,matched_rule_ids_json,event_json,title,body,created_at,updated_at) VALUES(?,?,?,'Test sender','webhook','test',1,?,?,?,'[]','{}','Test','Test',?,?)`, uuid.NewString(), eventID, senderID, status, at, code, at, at); err != nil {
			t.Fatal(err)
		}
	}
	deliver("failed", f.now.Unix())
	if attentionCount(t, f, "delivery") != 1 {
		t.Fatal("failed sender not counted")
	}
	if _, err := f.service.database.ExecContext(f.ctx, `UPDATE notification_senders SET enabled=0`); err != nil {
		t.Fatal(err)
	}
	if attentionCount(t, f, "delivery") != 0 {
		t.Fatal("disabled sender counted")
	}
	if _, err := f.service.database.ExecContext(f.ctx, `UPDATE notification_senders SET enabled=1`); err != nil {
		t.Fatal(err)
	}
	deliver("succeeded", f.now.Add(time.Second).Unix())
	if attentionCount(t, f, "delivery") != 0 {
		t.Fatal("recovered sender counted")
	}
}

func attentionCount(t *testing.T, f *probeServiceFixture, kind string) int {
	t.Helper()
	overview, err := f.service.Overview(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, group := range overview.Attention {
		if group.Kind == kind {
			return group.Count
		}
	}
	return 0
}

func TestOverviewAttentionIgnoresNATAndFreshConfiguration(t *testing.T) {
	f := newProbeServiceFixture(t, 512*1024*1024)
	overview, err := f.service.Overview(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(overview.Attention) != 0 {
		t.Fatalf("normal unprobed NAT paths raised attention: %+v", overview.Attention)
	}
	if _, err := f.service.database.ExecContext(f.ctx, `UPDATE nodes SET applied_configuration_revision = 0, desired_configuration_updated_at = ?`, f.now.Add(-2*time.Minute).Unix()); err != nil {
		t.Fatal(err)
	}
	if attentionCount(t, f, "configuration") != 1 {
		t.Fatal("stale configuration was not counted")
	}
	if _, err := f.service.database.ExecContext(f.ctx, `UPDATE nodes SET last_seen_at = ?`, f.now.Add(-3*time.Minute).Unix()); err != nil {
		t.Fatal(err)
	}
	if attentionCount(t, f, "offline") != 1 || attentionCount(t, f, "configuration") != 0 {
		t.Fatal("offline node duplicated configuration attention")
	}
	if _, err := f.service.SetEnabled(f.ctx, f.registration.NodeID, false); err != nil {
		t.Fatal(err)
	}
	if attentionCount(t, f, "offline") != 0 {
		t.Fatal("disabled node reported offline")
	}
}

func TestOverviewAttentionDiscoveryAggregatesAndRecovers(t *testing.T) {
	f := newProbeServiceFixture(t, 512*1024*1024)
	if _, err := f.store.History.ExecContext(f.ctx, `UPDATE address_states SET status='failed', failure_reason='no-valid-response'`); err != nil {
		t.Fatal(err)
	}
	if attentionCount(t, f, "discovery") != 1 {
		t.Fatal("failed paths did not aggregate by node")
	}
	if _, err := f.service.database.ExecContext(f.ctx, `UPDATE network_egresses SET enabled=0`); err != nil {
		t.Fatal(err)
	}
	if attentionCount(t, f, "discovery") != 0 {
		t.Fatal("disabled paths raised attention")
	}
	if _, err := f.service.database.ExecContext(f.ctx, `UPDATE network_egresses SET enabled=1`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.History.ExecContext(f.ctx, `UPDATE address_states SET status='confirmed', failure_reason=NULL`); err != nil {
		t.Fatal(err)
	}
	if attentionCount(t, f, "discovery") != 0 {
		t.Fatal("recovered discovery still raised attention")
	}
}

func TestOverviewAttentionCountsFailedIPsInPartialRun(t *testing.T) {
	f := newProbeServiceFixture(t, 512*1024*1024)
	running, terminal, executions := probeArtifacts(f, []string{"succeeded", "failed"})
	uploadProbeRun(t, f, running)
	for _, execution := range executions {
		uploadProbeExecution(t, f, running, execution)
	}
	uploadProbeRun(t, f, terminal)
	if attentionCount(t, f, "probe") != 1 {
		t.Fatal("partial run lost its failed public IP")
	}
	if _, err := f.service.database.ExecContext(f.ctx, `UPDATE public_addresses SET probe_enabled=0`); err != nil {
		t.Fatal(err)
	}
	if attentionCount(t, f, "probe") != 0 || attentionCount(t, f, "format") != 0 {
		t.Fatal("disabled IP raised report attention")
	}
}

func TestOverviewAttentionUpdateClearsAtTargetVersion(t *testing.T) {
	f := newProbeServiceFixture(t, 512*1024*1024)
	task := createStoredAgentUpdateTask(t, f, "0.2.0")
	if _, err := f.service.database.ExecContext(f.ctx, `UPDATE probe_tasks SET status='failed', acknowledged_at=?, completed_at=? WHERE id=?`, f.now.Unix(), f.now.Unix(), task.ID.String()); err != nil {
		t.Fatal(err)
	}
	if attentionCount(t, f, "update") != 1 {
		t.Fatal("failed update was not counted")
	}
	if _, err := f.service.database.ExecContext(f.ctx, `UPDATE nodes SET agent_version='0.2.0'`); err != nil {
		t.Fatal(err)
	}
	if attentionCount(t, f, "update") != 0 {
		t.Fatal("installed target still reported failure")
	}
}

func TestOverviewAttentionRetentionAfterCleanup(t *testing.T) {
	f := newProbeServiceFixture(t, 512*1024*1024)
	history := HistoryState{Usage: HistoryUsage{OverBudget: true}}
	check := func(want int) {
		t.Helper()
		groups, err := f.service.overviewAttention(f.ctx, Overview{}, nil, history)
		if err != nil {
			t.Fatal(err)
		}
		if len(groups) != want {
			t.Fatalf("retention attention = %+v", groups)
		}
	}
	check(0)
	now := f.now
	history.Retention.LastCleanupAt = &now
	check(1)
	history.Usage.OverBudget = false
	check(0)
	failure := "disk full"
	history.Retention.LastCleanupError = &failure
	check(1)
}
