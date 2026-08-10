package nodes

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/center/database/configdb"
)

func TestAgentUpdateTaskLifecycleAndTerminalConfirmation(t *testing.T) {
	fixture := newProbeServiceFixture(t, 512*1024*1024)
	task := createStoredAgentUpdateTask(t, fixture, "0.1.1")
	delivered, err := fixture.service.Poll(
		fixture.ctx, fixture.registration.Credential, fixture.metadata, 0, nil, nil, nil, nil,
	)
	if err != nil || delivered.Task == nil || delivered.Task.ID != task.ID ||
		delivered.Task.Kind != "agent-update" || delivered.Task.TargetVersion == nil ||
		*delivered.Task.TargetVersion != "0.1.1" {
		t.Fatalf("delivered Agent update task = %#v, %v", delivered.Task, err)
	}

	acknowledgedAt := task.CreatedAt.Add(5 * time.Second)
	startedAt := acknowledgedAt.Add(time.Second)
	completedAt := startedAt.Add(10 * time.Second)
	previousVersion := fixture.metadata.AgentVersion
	targetVersion := "0.1.1"
	reports := []struct {
		status   string
		metadata Metadata
		report   TaskReport
	}{
		{
			status: "acknowledged", metadata: fixture.metadata,
			report: TaskReport{ID: task.ID, Status: "acknowledged", AcknowledgedAt: acknowledgedAt},
		},
		{
			status: "verifying", metadata: fixture.metadata,
			report: TaskReport{
				ID: task.ID, Status: "verifying", AcknowledgedAt: acknowledgedAt,
				StartedAt: &startedAt, PreviousVersion: &previousVersion,
			},
		},
		{
			status: "installing", metadata: fixture.metadata,
			report: TaskReport{
				ID: task.ID, Status: "installing", AcknowledgedAt: acknowledgedAt,
				StartedAt: &startedAt, PreviousVersion: &previousVersion,
			},
		},
		{
			status: "restarting", metadata: metadataWithAgentVersion(fixture.metadata, targetVersion),
			report: TaskReport{
				ID: task.ID, Status: "restarting", AcknowledgedAt: acknowledgedAt,
				StartedAt: &startedAt, PreviousVersion: &previousVersion,
			},
		},
		{
			status: "succeeded", metadata: metadataWithAgentVersion(fixture.metadata, targetVersion),
			report: TaskReport{
				ID: task.ID, Status: "succeeded", AcknowledgedAt: acknowledgedAt,
				StartedAt: &startedAt, CompletedAt: &completedAt,
				PreviousVersion: &previousVersion, ResultVersion: &targetVersion,
			},
		},
	}

	for index, step := range reports {
		fixture.now = acknowledgedAt.Add(time.Duration(index) * time.Second)
		poll, pollErr := fixture.service.Poll(
			fixture.ctx, fixture.registration.Credential, step.metadata, 0, nil, nil, nil, nil,
			AddressUpload{TaskReport: &step.report},
		)
		if pollErr != nil {
			t.Fatalf("report %s: %v", step.status, pollErr)
		}
		record, getErr := fixture.store.ConfigQueries.GetProbeTask(fixture.ctx, configdb.GetProbeTaskParams{
			ID: task.ID.String(), NodeID: fixture.registration.NodeID.String(),
		})
		if getErr != nil || record.Status != step.status {
			t.Fatalf("stored report %s = %#v, %v", step.status, record, getErr)
		}
		if step.status == "succeeded" {
			if poll.AcceptedTerminalTaskID == nil || *poll.AcceptedTerminalTaskID != task.ID {
				t.Fatalf("terminal task confirmation = %#v", poll.AcceptedTerminalTaskID)
			}
		} else if poll.AcceptedTerminalTaskID != nil {
			t.Fatalf("non-terminal report %s was confirmed", step.status)
		}
	}

	fixture.now = fixture.now.Add(time.Second)
	repeated, err := fixture.service.Poll(
		fixture.ctx, fixture.registration.Credential, reports[len(reports)-1].metadata,
		0, nil, nil, nil, nil, AddressUpload{TaskReport: &reports[len(reports)-1].report},
	)
	if err != nil || repeated.AcceptedTerminalTaskID == nil || *repeated.AcceptedTerminalTaskID != task.ID {
		t.Fatalf("repeated terminal report = %#v, %v", repeated.AcceptedTerminalTaskID, err)
	}
}

func TestAgentUpdateTaskRejectsInvalidTransitionsAndMixedFields(t *testing.T) {
	t.Run("status regression", func(t *testing.T) {
		fixture := newProbeServiceFixture(t, 512*1024*1024)
		task := createStoredAgentUpdateTask(t, fixture, "0.1.1")
		acknowledgedAt := task.CreatedAt.Add(time.Second)
		startedAt := acknowledgedAt.Add(time.Second)
		previousVersion := fixture.metadata.AgentVersion
		verifying := TaskReport{
			ID: task.ID, Status: "verifying", AcknowledgedAt: acknowledgedAt,
			StartedAt: &startedAt, PreviousVersion: &previousVersion,
		}
		if _, err := pollWithTaskReport(fixture, fixture.metadata, verifying); err != nil {
			t.Fatal(err)
		}
		regression := TaskReport{ID: task.ID, Status: "acknowledged", AcknowledgedAt: acknowledgedAt}
		if _, err := pollWithTaskReport(fixture, fixture.metadata, regression); !errors.Is(err, ErrInvalidMetadata) {
			t.Fatalf("status regression error = %v", err)
		}
	})

	t.Run("update task rejects probe fields", func(t *testing.T) {
		fixture := newProbeServiceFixture(t, 512*1024*1024)
		task := createStoredAgentUpdateTask(t, fixture, "0.1.1")
		acknowledgedAt := task.CreatedAt.Add(time.Second)
		startedAt := acknowledgedAt.Add(time.Second)
		previousVersion := fixture.metadata.AgentVersion
		runID := uuid.New()
		report := TaskReport{
			ID: task.ID, Status: "verifying", AcknowledgedAt: acknowledgedAt,
			StartedAt: &startedAt, PreviousVersion: &previousVersion, RunID: &runID,
		}
		if _, err := pollWithTaskReport(fixture, fixture.metadata, report); !errors.Is(err, ErrInvalidMetadata) {
			t.Fatalf("mixed update report error = %v", err)
		}
	})

	t.Run("probe task rejects update fields", func(t *testing.T) {
		fixture := newProbeServiceFixture(t, 512*1024*1024)
		task, err := fixture.service.CreateCompleteProbeTask(fixture.ctx, fixture.registration.NodeID)
		if err != nil {
			t.Fatal(err)
		}
		previousVersion := fixture.metadata.AgentVersion
		report := TaskReport{
			ID: task.ID, Status: "acknowledged", AcknowledgedAt: task.CreatedAt.Add(time.Second),
			PreviousVersion: &previousVersion,
		}
		if _, err := pollWithTaskReport(fixture, fixture.metadata, report); !errors.Is(err, ErrInvalidMetadata) {
			t.Fatalf("mixed probe report error = %v", err)
		}
	})
}

func TestAgentUpdateTaskRequiresStatusVersionAgreement(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		metadata string
	}{
		{name: "verifying from target binary", status: "verifying", metadata: "0.1.1"},
		{name: "restarting from previous binary", status: "restarting", metadata: "0.1.0"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newProbeServiceFixture(t, 512*1024*1024)
			task := createStoredAgentUpdateTask(t, fixture, "0.1.1")
			acknowledgedAt := task.CreatedAt.Add(time.Second)
			startedAt := acknowledgedAt.Add(time.Second)
			previousVersion := fixture.metadata.AgentVersion
			report := TaskReport{
				ID: task.ID, Status: test.status, AcknowledgedAt: acknowledgedAt,
				StartedAt: &startedAt, PreviousVersion: &previousVersion,
			}
			metadata := metadataWithAgentVersion(fixture.metadata, test.metadata)
			if _, err := pollWithTaskReport(fixture, metadata, report); !errors.Is(err, ErrInvalidMetadata) {
				t.Fatalf("version mismatch error = %v", err)
			}
		})
	}
}

func TestAgentUpdateTaskTerminalOutcomes(t *testing.T) {
	tests := []struct {
		name        string
		status      string
		prepare     []string
		metadata    string
		result      *string
		failureCode string
	}{
		{name: "failed", status: "failed", prepare: []string{"verifying"}, metadata: "0.1.0", failureCode: "artifact-checksum"},
		{name: "rejected", status: "rejected", metadata: "0.1.0", failureCode: "probe-active"},
		{name: "rolled back", status: "rolled-back", prepare: []string{"verifying", "installing", "restarting"}, metadata: "0.1.0", result: stringPointerForTest("0.1.0"), failureCode: "health-timeout"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newProbeServiceFixture(t, 512*1024*1024)
			task := createStoredAgentUpdateTask(t, fixture, "0.1.1")
			acknowledgedAt := task.CreatedAt.Add(time.Second)
			startedAt := acknowledgedAt.Add(time.Second)
			completedAt := startedAt.Add(10 * time.Second)
			previousVersion := fixture.metadata.AgentVersion
			for _, status := range test.prepare {
				metadata := fixture.metadata
				if status == "restarting" {
					metadata = metadataWithAgentVersion(metadata, "0.1.1")
				}
				report := TaskReport{
					ID: task.ID, Status: status, AcknowledgedAt: acknowledgedAt,
					StartedAt: &startedAt, PreviousVersion: &previousVersion,
				}
				if _, err := pollWithTaskReport(fixture, metadata, report); err != nil {
					t.Fatalf("prepare %s: %v", status, err)
				}
			}
			diagnostic := "bounded update diagnostic"
			report := TaskReport{
				ID: task.ID, Status: test.status, AcknowledgedAt: acknowledgedAt,
				CompletedAt: &completedAt, PreviousVersion: &previousVersion,
				ResultVersion: test.result, FailureCode: &test.failureCode, Diagnostic: &diagnostic,
			}
			if test.status != "rejected" {
				report.StartedAt = &startedAt
			}
			poll, err := pollWithTaskReport(fixture, metadataWithAgentVersion(fixture.metadata, test.metadata), report)
			if err != nil || poll.AcceptedTerminalTaskID == nil || *poll.AcceptedTerminalTaskID != task.ID {
				t.Fatalf("terminal %s = %#v, %v", test.status, poll.AcceptedTerminalTaskID, err)
			}
			record, err := fixture.store.ConfigQueries.GetProbeTask(fixture.ctx, configdb.GetProbeTaskParams{
				ID: task.ID.String(), NodeID: fixture.registration.NodeID.String(),
			})
			if err != nil || record.Status != test.status || record.FailureCode == nil || *record.FailureCode != test.failureCode {
				t.Fatalf("stored terminal %s = %#v, %v", test.status, record, err)
			}
		})
	}
}

func TestAgentSourceRevisionPersistsWithHeartbeat(t *testing.T) {
	fixture := newProbeServiceFixture(t, 512*1024*1024)
	revision := strings.Repeat("a", 40)
	metadata := fixture.metadata
	metadata.AgentRevision = &revision
	if _, err := fixture.service.Poll(
		fixture.ctx, fixture.registration.Credential, metadata, 0, nil, nil, nil, nil,
	); err != nil {
		t.Fatal(err)
	}
	nodes, err := fixture.service.List(fixture.ctx)
	if err != nil || len(nodes) != 1 || nodes[0].AgentRevision == nil || *nodes[0].AgentRevision != revision {
		t.Fatalf("node source revision = %#v, %v", nodes, err)
	}
}

func createStoredAgentUpdateTask(t *testing.T, fixture *probeServiceFixture, targetVersion string) Task {
	t.Helper()
	id := uuid.New()
	createdAt := fixture.now.UTC().Truncate(time.Second)
	expiresAt := createdAt.Add(2 * time.Minute)
	if err := fixture.store.ConfigQueries.CreateAgentUpdateTask(fixture.ctx, configdb.CreateAgentUpdateTaskParams{
		ID: id.String(), NodeID: fixture.registration.NodeID.String(), TargetVersion: &targetVersion,
		CreatedAt: createdAt.Unix(), ExpiresAt: expiresAt.Unix(),
	}); err != nil {
		t.Fatal(err)
	}
	return Task{
		ID: id, NodeID: fixture.registration.NodeID, Kind: "agent-update", Status: "pending",
		CreatedAt: createdAt, ExpiresAt: expiresAt, TargetVersion: &targetVersion,
	}
}

func pollWithTaskReport(fixture *probeServiceFixture, metadata Metadata, report TaskReport) (Poll, error) {
	fixture.now = fixture.now.Add(time.Second)
	return fixture.service.Poll(
		fixture.ctx, fixture.registration.Credential, metadata, 0, nil, nil, nil, nil,
		AddressUpload{TaskReport: &report},
	)
}

func metadataWithAgentVersion(metadata Metadata, version string) Metadata {
	metadata.AgentVersion = version
	return metadata
}

func stringPointerForTest(value string) *string {
	return &value
}
