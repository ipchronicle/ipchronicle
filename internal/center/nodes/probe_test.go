package nodes

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/center/database"
	"github.com/ipchronicle/ipchronicle/internal/center/database/configdb"
)

type probeServiceFixture struct {
	ctx           context.Context
	store         *database.Store
	service       *Service
	registration  Registration
	metadata      Metadata
	configuration Configuration
	egresses      []NetworkEgress
	now           time.Time
}

func newProbeServiceFixture(t *testing.T, physicalMemoryBytes int64) *probeServiceFixture {
	t.Helper()
	fixture := &probeServiceFixture{
		ctx: context.Background(),
		now: time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC),
		metadata: Metadata{
			Hostname: "probe.example", AgentVersion: "0.1.0", OperatingSystem: "linux", Architecture: "amd64",
			Capabilities:        []string{"control-v1", "configuration-v7", "complete-probe-v1"},
			PhysicalMemoryBytes: physicalMemoryBytes,
		},
	}
	store, err := database.Open(fixture.ctx, database.PathsFromDataDirectory(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	fixture.store = store
	fixture.service = NewService(store.Config, store.History, store.ConfigQueries, store.MasterKey, &testSyncConnections{})
	fixture.service.now = func() time.Time { return fixture.now }
	enrollment, err := fixture.service.RotateEnrollmentKey(fixture.ctx, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	fixture.registration, err = fixture.service.Register(fixture.ctx, enrollment.Key, fixture.metadata)
	if err != nil {
		t.Fatal(err)
	}
	inventory := testNetworkInventory(fixture.now)
	if _, err := fixture.service.Poll(
		fixture.ctx, fixture.registration.Credential, fixture.metadata, 0, nil, nil, &inventory, nil,
	); err != nil {
		t.Fatal(err)
	}
	configuration, err := fixture.service.Configuration(fixture.ctx, fixture.registration.Credential)
	if err != nil {
		t.Fatal(err)
	}
	var ipv4Path, ipv6Path *NetworkEgress
	for index := range configuration.DiscoveryPaths {
		path := &configuration.DiscoveryPaths[index]
		if path.Kind != "default" {
			continue
		}
		if path.Family == "ipv4" {
			ipv4Path = path
		} else if path.Family == "ipv6" {
			ipv6Path = path
		}
	}
	if ipv4Path == nil || ipv6Path == nil {
		t.Fatalf("probe fixture default paths = %#v", configuration.DiscoveryPaths)
	}
	localInterface := "eth0"
	localIPv4 := "10.0.0.5"
	localIPv6 := "fd00::5"
	publicIPv4 := "8.8.8.8"
	publicIPv6 := "2001:4860:4860::8888"
	systemState, err := fixture.store.ConfigQueries.GetSystemState(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.Poll(
		fixture.ctx, fixture.registration.Credential, fixture.metadata, 0, nil, nil, nil, nil,
		AddressUpload{States: []AddressState{
			confirmedAddressState(ipv4Path.ID, systemState.HistoryGeneration, "ipv4", publicIPv4, localInterface, localIPv4, fixture.now),
			confirmedAddressState(ipv6Path.ID, systemState.HistoryGeneration, "ipv6", publicIPv6, localInterface, localIPv6, fixture.now),
		}},
	); err != nil {
		t.Fatal(err)
	}
	network, err := fixture.service.Network(fixture.ctx, fixture.registration.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	for _, address := range network.PublicAddresses {
		if _, err := fixture.service.UpdatePublicAddress(fixture.ctx, fixture.registration.NodeID, address.ID, PublicAddressUpdate{
			ProbeEnabled: true,
		}); err != nil {
			t.Fatal(err)
		}
	}
	fixture.configuration, err = fixture.service.Configuration(fixture.ctx, fixture.registration.Credential)
	if err != nil {
		t.Fatal(err)
	}
	fixture.egresses = fixture.configuration.ProbeTargets
	if len(fixture.egresses) < 2 {
		t.Fatalf("probe fixture has %d public-address targets, want at least 2", len(fixture.egresses))
	}
	return fixture
}

func (fixture *probeServiceFixture) probeTargetIDs() []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(fixture.egresses))
	for _, egress := range fixture.egresses {
		ids = append(ids, egress.ID)
	}
	return ids
}

func confirmedAddressState(
	pathID uuid.UUID,
	historyGeneration, family, publicAddress, localInterface, localAddress string,
	checkedAt time.Time,
) AddressState {
	return AddressState{
		EgressID: pathID, HistoryGeneration: historyGeneration, Family: family,
		Status: "confirmed", PublicAddress: &publicAddress,
		LocalInterface: &localInterface, LocalAddress: &localAddress,
		LikelyNAT: localAddress != publicAddress, LastCheckedAt: checkedAt,
		LastSucceededAt: &checkedAt, LastChangedAt: &checkedAt,
	}
}

func TestCreateCompleteProbeTaskEligibilityAndSingleSlot(t *testing.T) {
	t.Run("offline", func(t *testing.T) {
		fixture := newProbeServiceFixture(t, 512*1024*1024)
		fixture.now = fixture.now.Add(OnlineWindow + time.Second)
		if _, err := fixture.service.CreateCompleteProbeTask(fixture.ctx, fixture.registration.NodeID, fixture.probeTargetIDs()); !errors.Is(err, ErrNodeOffline) {
			t.Fatalf("offline task error = %v", err)
		}
	})
	t.Run("disabled", func(t *testing.T) {
		fixture := newProbeServiceFixture(t, 512*1024*1024)
		if _, err := fixture.service.SetEnabled(fixture.ctx, fixture.registration.NodeID, false); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.service.CreateCompleteProbeTask(fixture.ctx, fixture.registration.NodeID, fixture.probeTargetIDs()); !errors.Is(err, ErrNodeDisabled) {
			t.Fatalf("disabled task error = %v", err)
		}
	})
	t.Run("low memory", func(t *testing.T) {
		fixture := newProbeServiceFixture(t, 64*1024*1024)
		if _, err := fixture.service.CreateCompleteProbeTask(fixture.ctx, fixture.registration.NodeID, fixture.probeTargetIDs()); !errors.Is(err, ErrProbePausedLowMemory) {
			t.Fatalf("low-memory task error = %v", err)
		}
		if _, err := fixture.service.UpdateProbeSettings(fixture.ctx, fixture.registration.NodeID, ProbeSettingsUpdate{
			Schedule: fixture.configuration.ProbeSchedule, LowMemoryOverride: true, ProbeOnNewAddress: true,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.service.CreateCompleteProbeTask(fixture.ctx, fixture.registration.NodeID, fixture.probeTargetIDs()); err != nil {
			t.Fatalf("task after low-memory override: %v", err)
		}
	})
	t.Run("empty target selection", func(t *testing.T) {
		fixture := newProbeServiceFixture(t, 512*1024*1024)
		if _, err := fixture.service.CreateCompleteProbeTask(fixture.ctx, fixture.registration.NodeID, nil); !errors.Is(err, ErrInvalidProbeTargets) {
			t.Fatalf("empty-target task error = %v", err)
		}
	})
	t.Run("single slot", func(t *testing.T) {
		fixture := newProbeServiceFixture(t, 512*1024*1024)
		if _, err := fixture.service.CreateCompleteProbeTask(fixture.ctx, fixture.registration.NodeID, fixture.probeTargetIDs()); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.service.CreateCompleteProbeTask(fixture.ctx, fixture.registration.NodeID, fixture.probeTargetIDs()); !errors.Is(err, ErrProbeTaskSlotOccupied) {
			t.Fatalf("second task error = %v", err)
		}
	})
	t.Run("active run", func(t *testing.T) {
		fixture := newProbeServiceFixture(t, 512*1024*1024)
		activeRunID := uuid.New()
		if _, err := fixture.service.Poll(
			fixture.ctx, fixture.registration.Credential, fixture.metadata, 0, nil, nil, nil, nil,
			AddressUpload{ProbeStatus: &ProbeStatus{ActiveRunID: &activeRunID}},
		); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.service.CreateCompleteProbeTask(fixture.ctx, fixture.registration.NodeID, fixture.probeTargetIDs()); !errors.Is(err, ErrProbeAlreadyRunning) {
			t.Fatalf("active-run task error = %v", err)
		}
	})
}

func TestProbeOnNewAddressSettingUpdatesIndependentlyAndClearsPending(t *testing.T) {
	fixture := newProbeServiceFixture(t, 512*1024*1024)
	state, err := fixture.service.Probe(fixture.ctx, fixture.registration.NodeID)
	if err != nil || !state.ProbeOnNewAddress {
		t.Fatalf("initial probe settings = %#v, %v", state, err)
	}
	if err := fixture.store.ConfigQueries.UpsertPendingPublicAddressProbe(
		fixture.ctx, configdb.UpsertPendingPublicAddressProbeParams{
			PublicAddressID:               fixture.egresses[0].ID.String(),
			NodeID:                        fixture.registration.NodeID.String(),
			RequiredConfigurationRevision: fixture.configuration.Revision,
			CreatedAt:                     fixture.now.Unix(),
		},
	); err != nil {
		t.Fatal(err)
	}
	state, err = fixture.service.UpdateProbeSettings(
		fixture.ctx, fixture.registration.NodeID,
		ProbeSettingsUpdate{
			Schedule: state.Schedule, LowMemoryOverride: state.LowMemoryOverride,
			ProbeOnNewAddress: false,
		},
	)
	if err != nil || state.ProbeOnNewAddress {
		t.Fatalf("disabled new-address setting = %#v, %v", state, err)
	}
	var pending int
	if err := fixture.store.Config.QueryRowContext(
		fixture.ctx, `SELECT count(*) FROM pending_public_address_probes WHERE node_id = ?`,
		fixture.registration.NodeID.String(),
	).Scan(&pending); err != nil || pending != 0 {
		t.Fatalf("pending probes after disabling setting = %d, %v", pending, err)
	}
	state, err = fixture.service.UpdateProbeSettings(
		fixture.ctx, fixture.registration.NodeID,
		ProbeSettingsUpdate{
			Schedule: state.Schedule, LowMemoryOverride: state.LowMemoryOverride,
			ProbeOnNewAddress: true,
		},
	)
	if err != nil || !state.ProbeOnNewAddress {
		t.Fatalf("re-enabled new-address setting = %#v, %v", state, err)
	}
}

func TestCreateCompleteProbeTaskSavesTargetsAtomically(t *testing.T) {
	t.Run("selected targets replace the enabled set", func(t *testing.T) {
		fixture := newProbeServiceFixture(t, 512*1024*1024)
		before, err := fixture.store.ConfigQueries.GetNodeProbeSettings(fixture.ctx, fixture.registration.NodeID.String())
		if err != nil {
			t.Fatal(err)
		}
		selected := fixture.egresses[0].ID
		if _, err := fixture.service.CreateCompleteProbeTask(
			fixture.ctx, fixture.registration.NodeID, []uuid.UUID{selected},
		); err != nil {
			t.Fatal(err)
		}
		after, err := fixture.store.ConfigQueries.GetNodeProbeSettings(fixture.ctx, fixture.registration.NodeID.String())
		if err != nil {
			t.Fatal(err)
		}
		if after.DesiredConfigurationRevision != before.DesiredConfigurationRevision+1 {
			t.Fatalf("desired revision = %d, want %d", after.DesiredConfigurationRevision, before.DesiredConfigurationRevision+1)
		}
		configuration, err := fixture.service.Configuration(fixture.ctx, fixture.registration.Credential)
		if err != nil {
			t.Fatal(err)
		}
		if len(configuration.ProbeTargets) != 1 || configuration.ProbeTargets[0].ID != selected {
			t.Fatalf("probe targets = %#v, want only %s", configuration.ProbeTargets, selected)
		}
	})

	t.Run("an unavailable target leaves settings and task unchanged", func(t *testing.T) {
		fixture := newProbeServiceFixture(t, 512*1024*1024)
		before, err := fixture.store.ConfigQueries.GetNodeProbeSettings(fixture.ctx, fixture.registration.NodeID.String())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.service.CreateCompleteProbeTask(
			fixture.ctx, fixture.registration.NodeID, []uuid.UUID{uuid.New()},
		); !errors.Is(err, ErrProbeTargetUnavailable) {
			t.Fatalf("unavailable-target error = %v", err)
		}
		after, err := fixture.store.ConfigQueries.GetNodeProbeSettings(fixture.ctx, fixture.registration.NodeID.String())
		if err != nil {
			t.Fatal(err)
		}
		if after.DesiredConfigurationRevision != before.DesiredConfigurationRevision {
			t.Fatalf("desired revision changed from %d to %d", before.DesiredConfigurationRevision, after.DesiredConfigurationRevision)
		}
		if _, err := fixture.store.ConfigQueries.GetActiveNodeTask(fixture.ctx, fixture.registration.NodeID.String()); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("active task after rejected selection error = %v", err)
		}
		configuration, err := fixture.service.Configuration(fixture.ctx, fixture.registration.Credential)
		if err != nil {
			t.Fatal(err)
		}
		if len(configuration.ProbeTargets) != len(fixture.egresses) {
			t.Fatalf("probe target count = %d, want %d", len(configuration.ProbeTargets), len(fixture.egresses))
		}
	})
}

func TestProbeTaskExpiryAndTerminalAcknowledgement(t *testing.T) {
	t.Run("pending task expires without delivery", func(t *testing.T) {
		fixture := newProbeServiceFixture(t, 512*1024*1024)
		task, err := fixture.service.CreateCompleteProbeTask(fixture.ctx, fixture.registration.NodeID, fixture.probeTargetIDs())
		if err != nil {
			t.Fatal(err)
		}
		fixture.now = task.ExpiresAt.Add(time.Second)
		poll, err := fixture.service.Poll(
			fixture.ctx, fixture.registration.Credential, fixture.metadata, 0, nil, nil, nil, nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		if poll.Task != nil {
			t.Fatalf("expired task was delivered: %#v", poll.Task)
		}
		record, err := fixture.store.ConfigQueries.GetProbeTask(fixture.ctx, configdb.GetProbeTaskParams{
			ID: task.ID.String(), NodeID: fixture.registration.NodeID.String(),
		})
		if err != nil || record.Status != "expired" {
			t.Fatalf("expired task record = %#v, %v", record, err)
		}
	})
	t.Run("terminal report is confirmed idempotently", func(t *testing.T) {
		fixture := newProbeServiceFixture(t, 512*1024*1024)
		task, err := fixture.service.CreateCompleteProbeTask(fixture.ctx, fixture.registration.NodeID, fixture.probeTargetIDs())
		if err != nil {
			t.Fatal(err)
		}
		poll, err := fixture.service.Poll(
			fixture.ctx, fixture.registration.Credential, fixture.metadata, 0, nil, nil, nil, nil,
		)
		if err != nil || poll.Task == nil || poll.Task.ID != task.ID {
			t.Fatalf("delivered task = %#v, %v", poll.Task, err)
		}
		fixture.now = fixture.now.Add(30 * time.Second)
		reason := "disabled"
		report := TaskReport{
			ID: task.ID, Status: "rejected", AcknowledgedAt: fixture.now,
			CompletedAt: &fixture.now, RejectionReason: &reason,
		}
		for attempt := 0; attempt < 2; attempt++ {
			poll, err = fixture.service.Poll(
				fixture.ctx, fixture.registration.Credential, fixture.metadata, 0, nil, nil, nil, nil,
				AddressUpload{TaskReport: &report},
			)
			if err != nil || poll.AcceptedTerminalTaskID == nil || *poll.AcceptedTerminalTaskID != task.ID {
				t.Fatalf("terminal acknowledgement %d = %#v, %v", attempt, poll.AcceptedTerminalTaskID, err)
			}
		}
	})
	t.Run("small acknowledgement clock skew is accepted", func(t *testing.T) {
		fixture := newProbeServiceFixture(t, 512*1024*1024)
		task, err := fixture.service.CreateCompleteProbeTask(fixture.ctx, fixture.registration.NodeID, fixture.probeTargetIDs())
		if err != nil {
			t.Fatal(err)
		}
		tooEarly := task.CreatedAt.Add(-time.Second)
		reason := "disabled"
		poll, err := fixture.service.Poll(
			fixture.ctx, fixture.registration.Credential, fixture.metadata, 0, nil, nil, nil, nil,
			AddressUpload{TaskReport: &TaskReport{
				ID: task.ID, Status: "rejected", AcknowledgedAt: tooEarly,
				CompletedAt: &tooEarly, RejectionReason: &reason,
			}},
		)
		if err != nil || poll.AcceptedTerminalTaskID == nil || *poll.AcceptedTerminalTaskID != task.ID {
			t.Fatalf("clock-skewed acknowledgement = %#v, %v", poll.AcceptedTerminalTaskID, err)
		}
	})
	t.Run("acknowledgement outside clock skew tolerance is rejected", func(t *testing.T) {
		for _, test := range []struct {
			name           string
			acknowledgedAt func(Task) time.Time
		}{
			{
				name: "before creation",
				acknowledgedAt: func(task Task) time.Time {
					return task.CreatedAt.Add(-taskReportClockSkewTolerance - time.Second)
				},
			},
			{
				name: "after expiry",
				acknowledgedAt: func(task Task) time.Time {
					return task.ExpiresAt.Add(taskReportClockSkewTolerance + time.Second)
				},
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				fixture := newProbeServiceFixture(t, 512*1024*1024)
				task, err := fixture.service.CreateCompleteProbeTask(
					fixture.ctx, fixture.registration.NodeID, fixture.probeTargetIDs(),
				)
				if err != nil {
					t.Fatal(err)
				}
				acknowledgedAt := test.acknowledgedAt(task)
				reason := "disabled"
				if _, err := fixture.service.Poll(
					fixture.ctx, fixture.registration.Credential, fixture.metadata, 0, nil, nil, nil, nil,
					AddressUpload{TaskReport: &TaskReport{
						ID: task.ID, Status: "rejected", AcknowledgedAt: acknowledgedAt,
						CompletedAt: &acknowledgedAt, RejectionReason: &reason,
					}},
				); !errors.Is(err, ErrInvalidMetadata) {
					t.Fatalf("out-of-range task acknowledgement error = %v", err)
				}
			})
		}
	})
}

func TestProbeArtifactsAcceptParentChildArrivalOrders(t *testing.T) {
	for _, order := range []string{"executions-first", "terminal-run-first"} {
		t.Run(order, func(t *testing.T) {
			fixture := newProbeServiceFixture(t, 512*1024*1024)
			running, terminal, executions := probeArtifacts(fixture, []string{"succeeded", "failed"})
			if order == "executions-first" {
				uploadProbeRun(t, fixture, running)
				for index := range executions {
					uploadProbeExecution(t, fixture, running, executions[index])
				}
				stored, err := fixture.service.ProbeRun(fixture.ctx, running.ID)
				if err != nil || stored.Status != "running" {
					t.Fatalf("run before terminal summary = %#v, %v", stored, err)
				}
				uploadProbeRun(t, fixture, terminal)
			} else {
				uploadProbeRun(t, fixture, terminal)
				for index := range executions {
					uploadProbeExecution(t, fixture, terminal, executions[index])
				}
			}
			stored, err := fixture.service.ProbeRun(fixture.ctx, terminal.ID)
			if err != nil || stored.Status != "partial" || len(stored.Executions) != 2 ||
				stored.Executions[0].SnapshotID == nil || stored.Executions[1].SnapshotID != nil {
				t.Fatalf("terminal run = %#v, %v", stored, err)
			}
		})
	}
}

func TestProbeRunSummaryMustMatchExecutionOutcomes(t *testing.T) {
	tests := []struct {
		name     string
		outcomes []string
		status   string
	}{
		{name: "success", outcomes: []string{"succeeded", "succeeded"}, status: "succeeded"},
		{name: "partial", outcomes: []string{"succeeded", "failed"}, status: "partial"},
		{name: "failure", outcomes: []string{"failed", "failed"}, status: "failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newProbeServiceFixture(t, 512*1024*1024)
			running, terminal, executions := probeArtifacts(fixture, test.outcomes)
			if terminal.Status != test.status {
				t.Fatalf("fixture status = %s, want %s", terminal.Status, test.status)
			}
			uploadProbeRun(t, fixture, running)
			for index := range executions {
				uploadProbeExecution(t, fixture, running, executions[index])
			}
			uploadProbeRun(t, fixture, terminal)
			stored, err := fixture.service.ProbeRun(fixture.ctx, terminal.ID)
			if err != nil || stored.Status != test.status {
				t.Fatalf("stored run = %#v, %v", stored, err)
			}
		})
	}

	fixture := newProbeServiceFixture(t, 512*1024*1024)
	running, terminal, executions := probeArtifacts(fixture, []string{"succeeded", "failed"})
	uploadProbeRun(t, fixture, running)
	for index := range executions {
		uploadProbeExecution(t, fixture, running, executions[index])
	}
	terminal.Status = "succeeded"
	if _, err := fixture.service.UploadProbeArtifact(fixture.ctx, fixture.registration.Credential, ProbeArtifact{
		ID: terminal.ID, Revision: 2, Run: &terminal,
	}); !errors.Is(err, ErrInvalidProbeArtifact) {
		t.Fatalf("inconsistent parent status error = %v", err)
	}
}

func TestProbeArtifactRetransmissionAndGapConflict(t *testing.T) {
	fixture := newProbeServiceFixture(t, 512*1024*1024)
	running, _, executions := probeArtifacts(fixture, []string{"succeeded"})
	uploadProbeRun(t, fixture, running)
	for attempt := 0; attempt < 2; attempt++ {
		uploadProbeExecution(t, fixture, running, executions[0])
	}
	conflict := executions[0]
	conflict.RawResult = []byte(`{"changed":true}`)
	if _, err := fixture.service.UploadProbeArtifact(fixture.ctx, fixture.registration.Credential, ProbeArtifact{
		ID: conflict.ID, Revision: 2, Run: &running, Execution: &conflict,
	}); !errors.Is(err, ErrInvalidProbeArtifact) {
		t.Fatalf("conflicting execution error = %v", err)
	}

	gap := ProbeGapArtifact{
		ID: uuid.New(), EgressID: fixture.egresses[0].ID, HistoryGeneration: fixture.configuration.HistoryGeneration,
		DroppedCount: 1, FirstSequence: 1, LastSequence: 1,
		FirstObservedAt: fixture.now.Add(-time.Minute), LastObservedAt: fixture.now.Add(-time.Minute),
	}
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := fixture.service.UploadProbeArtifact(fixture.ctx, fixture.registration.Credential, ProbeArtifact{
			ID: gap.ID, Revision: 1, Gap: &gap,
		}); err != nil {
			t.Fatalf("gap upload %d: %v", attempt, err)
		}
	}
	gap.EgressID = fixture.egresses[1].ID
	if _, err := fixture.service.UploadProbeArtifact(fixture.ctx, fixture.registration.Credential, ProbeArtifact{
		ID: gap.ID, Revision: 2, Gap: &gap,
	}); !errors.Is(err, ErrInvalidProbeArtifact) {
		t.Fatalf("conflicting gap error = %v", err)
	}
}

func TestProbeRunningExecutionRetransmissionKeepsStartTime(t *testing.T) {
	fixture := newProbeServiceFixture(t, 512*1024*1024)
	running, _, _ := probeArtifacts(fixture, []string{"succeeded"})
	uploadProbeRun(t, fixture, running)
	startedAt := running.StartedAt.Add(time.Second)
	execution := ProbeExecutionArtifact{
		ID: running.Executions[0].ID, EgressID: running.Executions[0].EgressID,
		Ordinal: 0, Sequence: 1, Status: "running", StartedAt: &startedAt,
	}
	uploadProbeExecution(t, fixture, running, execution)
	uploadProbeExecution(t, fixture, running, execution)
	changed := startedAt.Add(time.Second)
	execution.StartedAt = &changed
	if _, err := fixture.service.UploadProbeArtifact(fixture.ctx, fixture.registration.Credential, ProbeArtifact{
		ID: execution.ID, Revision: 2, Run: &running, Execution: &execution,
	}); !errors.Is(err, ErrInvalidProbeArtifact) {
		t.Fatalf("changed execution start error = %v", err)
	}
}

func TestProbeArtifactRejectsForeignManifestAndInvalidTimeline(t *testing.T) {
	fixture := newProbeServiceFixture(t, 512*1024*1024)
	secondEnrollment, err := fixture.service.RotateEnrollmentKey(fixture.ctx, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	second, err := fixture.service.Register(fixture.ctx, secondEnrollment.Key, fixture.metadata)
	if err != nil {
		t.Fatal(err)
	}
	inventory := testNetworkInventory(fixture.now)
	if _, err := fixture.service.Poll(fixture.ctx, second.Credential, fixture.metadata, 0, nil, nil, &inventory, nil); err != nil {
		t.Fatal(err)
	}
	secondConfiguration, err := fixture.service.Configuration(fixture.ctx, second.Credential)
	if err != nil {
		t.Fatal(err)
	}
	running, terminal, executions := probeArtifacts(fixture, []string{"succeeded"})
	running.Executions[0].EgressID = secondConfiguration.DiscoveryPaths[0].ID
	if _, err := fixture.service.UploadProbeArtifact(fixture.ctx, fixture.registration.Credential, ProbeArtifact{
		ID: running.ID, Revision: 1, Run: &running,
	}); !errors.Is(err, ErrInvalidProbeArtifact) {
		t.Fatalf("foreign manifest error = %v", err)
	}

	running, terminal, executions = probeArtifacts(fixture, []string{"succeeded"})
	uploadProbeRun(t, fixture, running)
	uploadProbeExecution(t, fixture, running, executions[0])
	tooEarly := executions[0].CompletedAt.Add(-time.Second)
	terminal.CompletedAt = &tooEarly
	if _, err := fixture.service.UploadProbeArtifact(fixture.ctx, fixture.registration.Credential, ProbeArtifact{
		ID: terminal.ID, Revision: 2, Run: &terminal,
	}); !errors.Is(err, ErrInvalidProbeArtifact) {
		t.Fatalf("invalid parent completion timeline error = %v", err)
	}
}

func TestProbeHistoryResetInvalidatesOldGenerationAndAdvancesConfiguration(t *testing.T) {
	fixture := newProbeServiceFixture(t, 512*1024*1024)
	running, terminal, executions := probeArtifacts(fixture, []string{"succeeded"})
	uploadProbeRun(t, fixture, running)
	uploadProbeExecution(t, fixture, running, executions[0])
	uploadProbeRun(t, fixture, terminal)
	secondRunning, secondTerminal, secondExecutions := probeArtifacts(fixture, []string{"succeeded"})
	secondRunning.Executions[0].Sequence = 2
	secondTerminal.Executions[0].Sequence = 2
	secondExecutions[0].Sequence = 2
	secondExecutions[0].RawResult = []byte(`{"ip":"203.0.113.11"}`)
	uploadProbeRun(t, fixture, secondRunning)
	uploadProbeExecution(t, fixture, secondRunning, secondExecutions[0])
	uploadProbeRun(t, fixture, secondTerminal)
	nodeBefore, err := fixture.store.ConfigQueries.GetNodeByID(fixture.ctx, fixture.registration.NodeID.String())
	if err != nil {
		t.Fatal(err)
	}
	reset, err := fixture.service.ResetHistory(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if reset.Generation == fixture.configuration.HistoryGeneration || reset.ResetAt == nil {
		t.Fatalf("history reset = %#v", reset)
	}
	if _, err := fixture.service.ProbeRun(fixture.ctx, running.ID); !errors.Is(err, ErrProbeRunNotFound) {
		t.Fatalf("old run after reset error = %v", err)
	}
	nodeAfter, err := fixture.store.ConfigQueries.GetNodeByID(fixture.ctx, fixture.registration.NodeID.String())
	if err != nil || nodeAfter.DesiredConfigurationRevision != nodeBefore.DesiredConfigurationRevision+1 {
		t.Fatalf("configuration revision after reset = %d, want %d: %v", nodeAfter.DesiredConfigurationRevision, nodeBefore.DesiredConfigurationRevision+1, err)
	}
	receipt, err := fixture.service.UploadProbeArtifact(fixture.ctx, fixture.registration.Credential, ProbeArtifact{
		ID: running.ID, Revision: 1, Run: &running,
	})
	if err != nil || receipt.Disposition != "obsolete-generation" {
		t.Fatalf("obsolete upload receipt = %#v, %v", receipt, err)
	}
}

func TestProbeUploadWaitsForPendingHistoryReset(t *testing.T) {
	fixture := newProbeServiceFixture(t, 512*1024*1024)
	running, _, _ := probeArtifacts(fixture, []string{"succeeded"})
	pending := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err := fixture.store.ConfigQueries.SetPendingHistoryGeneration(fixture.ctx, &pending); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.UploadProbeArtifact(fixture.ctx, fixture.registration.Credential, ProbeArtifact{
		ID: running.ID, Revision: 1, Run: &running,
	}); !errors.Is(err, ErrHistoryResetPending) {
		t.Fatalf("pending reset upload error = %v", err)
	}
}

func TestProbeHistorySurvivesNodeDeletion(t *testing.T) {
	fixture := newProbeServiceFixture(t, 512*1024*1024)
	running, terminal, executions := probeArtifacts(fixture, []string{"succeeded", "succeeded"})
	uploadProbeRun(t, fixture, running)
	for index := range executions {
		uploadProbeExecution(t, fixture, running, executions[index])
	}
	uploadProbeRun(t, fixture, terminal)
	if _, err := fixture.service.Delete(fixture.ctx, fixture.registration.NodeID); err != nil {
		t.Fatal(err)
	}
	if err := fixture.service.processDeletions(fixture.ctx, 16); err != nil {
		t.Fatal(err)
	}
	remaining, err := fixture.service.ProbeRun(fixture.ctx, running.ID)
	if err != nil || len(remaining.Executions) != 2 {
		t.Fatalf("run after node deletion = %#v, %v", remaining, err)
	}
}

func TestTerminalProbeTasksAreCleanedAfterThirtyDays(t *testing.T) {
	fixture := newProbeServiceFixture(t, 512*1024*1024)
	task, err := fixture.service.CreateCompleteProbeTask(fixture.ctx, fixture.registration.NodeID, fixture.probeTargetIDs())
	if err != nil {
		t.Fatal(err)
	}
	fixture.now = fixture.now.Add(time.Minute)
	reason := "disabled"
	if _, err := fixture.service.Poll(
		fixture.ctx, fixture.registration.Credential, fixture.metadata, 0, nil, nil, nil, nil,
		AddressUpload{TaskReport: &TaskReport{
			ID: task.ID, Status: "rejected", AcknowledgedAt: fixture.now,
			CompletedAt: &fixture.now, RejectionReason: &reason,
		}},
	); err != nil {
		t.Fatal(err)
	}
	fixture.now = fixture.now.Add(31 * 24 * time.Hour)
	if err := fixture.service.processDeletions(fixture.ctx, 16); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.ConfigQueries.GetProbeTask(fixture.ctx, configdb.GetProbeTaskParams{
		ID: task.ID.String(), NodeID: fixture.registration.NodeID.String(),
	}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("task after retention cleanup error = %v", err)
	}
}

func probeArtifacts(fixture *probeServiceFixture, outcomes []string) (ProbeRunArtifact, ProbeRunArtifact, []ProbeExecutionArtifact) {
	startedAt := fixture.now.Add(-time.Minute)
	running := ProbeRunArtifact{
		ID: uuid.New(), ConfigurationRevision: fixture.configuration.Revision,
		HistoryGeneration: fixture.configuration.HistoryGeneration, Trigger: "schedule",
		StartedAt: startedAt, Status: "running", Executions: make([]ProbeExecutionManifest, len(outcomes)),
	}
	executions := make([]ProbeExecutionArtifact, len(outcomes))
	succeeded := 0
	for index, status := range outcomes {
		executionID := uuid.New()
		egressID := fixture.egresses[index].ID
		running.Executions[index] = ProbeExecutionManifest{
			ID: executionID, EgressID: egressID, Ordinal: int64(index), Sequence: 1,
		}
		executionStartedAt := startedAt.Add(time.Duration(index+1) * time.Second)
		executionCompletedAt := executionStartedAt.Add(time.Second)
		executions[index] = ProbeExecutionArtifact{
			ID: executionID, EgressID: egressID, Ordinal: int64(index), Sequence: 1,
			Status: status, StartedAt: &executionStartedAt, CompletedAt: &executionCompletedAt,
		}
		if status == "succeeded" {
			succeeded++
			executions[index].RawResult = []byte(`{"ip":"203.0.113.10"}`)
		} else {
			stage := "process"
			diagnostic := "exit status 1"
			executions[index].FailureStage = &stage
			executions[index].Diagnostic = &diagnostic
		}
	}
	terminal := running
	completedAt := fixture.now.Add(-10 * time.Second)
	terminal.CompletedAt = &completedAt
	switch {
	case succeeded == len(outcomes):
		terminal.Status = "succeeded"
	case succeeded > 0:
		terminal.Status = "partial"
	default:
		terminal.Status = "failed"
	}
	return running, terminal, executions
}

func uploadProbeRun(t *testing.T, fixture *probeServiceFixture, run ProbeRunArtifact) {
	t.Helper()
	if _, err := fixture.service.UploadProbeArtifact(fixture.ctx, fixture.registration.Credential, ProbeArtifact{
		ID: run.ID, Revision: 1, Run: &run,
	}); err != nil {
		t.Fatal(err)
	}
}

func uploadProbeExecution(t *testing.T, fixture *probeServiceFixture, run ProbeRunArtifact, execution ProbeExecutionArtifact) {
	t.Helper()
	if _, err := fixture.service.UploadProbeArtifact(fixture.ctx, fixture.registration.Credential, ProbeArtifact{
		ID: execution.ID, Revision: 1, Run: &run, Execution: &execution,
	}); err != nil {
		t.Fatal(err)
	}
}
