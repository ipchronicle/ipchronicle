package update

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/agent/state"
)

func TestSupervisorCommitsHealthyReplacementAndPreservesState(t *testing.T) {
	fixture := prepareSupervisorFixture(t)
	lifecycle := &testLifecycle{}
	lifecycle.onStart = func(start int) error {
		if start != 1 {
			return nil
		}
		store, err := state.Open(fixture.stateDirectory)
		if err != nil {
			return err
		}
		_, commitErr := store.CommitAgentUpdateHealth(fixture.taskID, "0.1.1", time.Now().UTC())
		closeErr := store.Close()
		if commitErr != nil || closeErr != nil {
			return errors.Join(commitErr, closeErr)
		}
		return WriteHealthMarker(fixture.stateDirectory, fixture.taskID)
	}
	if err := RunSupervisor(context.Background(), SupervisorOptions{
		StateDirectory: fixture.stateDirectory, AgentPath: fixture.agentPath, UpdaterPath: fixture.updaterPath,
		InitSystem: "systemd", HealthTimeout: time.Second, Lifecycle: lifecycle,
	}); err != nil {
		t.Fatal(err)
	}
	assertFileContents(t, fixture.agentPath, fixture.newBinary)
	assertFileContents(t, fixture.updaterPath, fixture.newBinary)
	assertPreservedAgentState(t, fixture, "succeeded")
	if lifecycle.stops != 1 || lifecycle.starts != 1 {
		t.Fatalf("lifecycle stops/starts = %d/%d", lifecycle.stops, lifecycle.starts)
	}
	if exists, err := HasCheckpoint(fixture.stateDirectory, fixture.taskID); err != nil || exists {
		t.Fatalf("checkpoint after success = %v, %v", exists, err)
	}
}

func TestSupervisorRollsBackAfterHealthTimeoutAndPreservesState(t *testing.T) {
	fixture := prepareSupervisorFixture(t)
	lifecycle := &testLifecycle{}
	if err := RunSupervisor(context.Background(), SupervisorOptions{
		StateDirectory: fixture.stateDirectory, AgentPath: fixture.agentPath, UpdaterPath: fixture.updaterPath,
		InitSystem: "openrc", HealthTimeout: 20 * time.Millisecond, Lifecycle: lifecycle,
	}); err != nil {
		t.Fatal(err)
	}
	assertFileContents(t, fixture.agentPath, fixture.oldBinary)
	assertFileContents(t, fixture.updaterPath, fixture.oldUpdater)
	assertPreservedAgentState(t, fixture, "rolled-back")
	if lifecycle.stops != 2 || lifecycle.starts != 2 {
		t.Fatalf("rollback lifecycle stops/starts = %d/%d", lifecycle.stops, lifecycle.starts)
	}
}

func TestSupervisorRollsBackWhenNewAgentDoesNotStart(t *testing.T) {
	fixture := prepareSupervisorFixture(t)
	lifecycle := &testLifecycle{onStart: func(start int) error {
		if start == 1 {
			return errors.New("new Agent exited during startup")
		}
		return nil
	}}
	if err := RunSupervisor(context.Background(), SupervisorOptions{
		StateDirectory: fixture.stateDirectory, AgentPath: fixture.agentPath, UpdaterPath: fixture.updaterPath,
		InitSystem: "systemd", HealthTimeout: time.Second, Lifecycle: lifecycle,
	}); err != nil {
		t.Fatal(err)
	}
	assertFileContents(t, fixture.agentPath, fixture.oldBinary)
	assertPreservedAgentState(t, fixture, "rolled-back")
	store, err := state.Open(fixture.stateDirectory)
	if err != nil {
		t.Fatal(err)
	}
	updateState, _, err := store.PendingAgentUpdate()
	_ = store.Close()
	if err != nil || updateState.FailureCode == nil || *updateState.FailureCode != "start-failed" {
		t.Fatalf("failed-start rollback = %#v, %v", updateState, err)
	}
}

func TestSupervisorFinalizesSucceededUpdateAfterInterruption(t *testing.T) {
	fixture := prepareSupervisorFixture(t)
	checkpoint := checkpointPath(fixture.stateDirectory, fixture.taskID)
	if err := ensureCheckpoint(fixture.stateDirectory, fixture.agentPath, checkpoint); err != nil {
		t.Fatal(err)
	}
	if err := replaceFile(StagedBinaryPath(fixture.stateDirectory, fixture.taskID), fixture.agentPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := markRestarting(fixture.stateDirectory, fixture.taskID); err != nil {
		t.Fatal(err)
	}
	store, err := state.Open(fixture.stateDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitAgentUpdateHealth(fixture.taskID, "0.1.1", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := WriteHealthMarker(fixture.stateDirectory, fixture.taskID); err != nil {
		t.Fatal(err)
	}
	lifecycle := &testLifecycle{}
	if err := RunSupervisor(context.Background(), SupervisorOptions{
		StateDirectory: fixture.stateDirectory, AgentPath: fixture.agentPath, UpdaterPath: fixture.updaterPath,
		InitSystem: "systemd", Lifecycle: lifecycle,
	}); err != nil {
		t.Fatal(err)
	}
	assertFileContents(t, fixture.agentPath, fixture.newBinary)
	assertFileContents(t, fixture.updaterPath, fixture.newBinary)
	assertPreservedAgentState(t, fixture, "succeeded")
	if lifecycle.stops != 1 || lifecycle.starts != 1 {
		t.Fatalf("recovery lifecycle stops/starts = %d/%d", lifecycle.stops, lifecycle.starts)
	}
}

func TestInitLifecycleCommandsAreDistinctForSystemdAndOpenRC(t *testing.T) {
	root := t.TempDir()
	logPath := filepath.Join(root, "commands.log")
	for _, name := range []string{"systemctl", "rc-service"} {
		path := filepath.Join(root, name)
		script := "#!/bin/sh\nprintf '%s %s\\n' '" + name + "' \"$*\" >> \"$COMMAND_LOG\"\n"
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", root+":"+os.Getenv("PATH"))
	t.Setenv("COMMAND_LOG", logPath)
	ctx := context.Background()
	if err := triggerSupervisor(ctx, "systemd"); err != nil {
		t.Fatal(err)
	}
	if err := triggerSupervisor(ctx, "openrc"); err != nil {
		t.Fatal(err)
	}
	for _, initSystem := range []string{"systemd", "openrc"} {
		lifecycle := commandLifecycle{initSystem: initSystem}
		if err := lifecycle.StopAgent(ctx); err != nil {
			t.Fatal(err)
		}
		if err := lifecycle.StartAgent(ctx); err != nil {
			t.Fatal(err)
		}
	}
	commands, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"systemctl start --no-block ipchronicle-agent-updater.service",
		"rc-service ipchronicle-agent-updater start",
		"systemctl stop ipchronicle-agent.service",
		"systemctl start ipchronicle-agent.service",
		"rc-service ipchronicle-agent stop",
		"rc-service ipchronicle-agent start",
	} {
		if !strings.Contains(string(commands), expected) {
			t.Fatalf("commands %q omit %q", commands, expected)
		}
	}
}

type supervisorFixture struct {
	stateDirectory string
	agentPath      string
	updaterPath    string
	taskID         string
	nodeID         string
	egressID       string
	resultFile     string
	oldBinary      []byte
	newBinary      []byte
	oldUpdater     []byte
}

func prepareSupervisorFixture(t *testing.T) supervisorFixture {
	t.Helper()
	root := t.TempDir()
	fixture := supervisorFixture{
		stateDirectory: filepath.Join(root, "state"), agentPath: filepath.Join(root, "bin", "ipchronicle-agent"),
		updaterPath: filepath.Join(root, "libexec", "ipchronicle-agent-updater"), taskID: uuid.NewString(),
		nodeID: uuid.NewString(), egressID: uuid.NewString(), oldBinary: []byte("old Agent\n"),
		newBinary: []byte("new Agent\n"), oldUpdater: []byte("old updater\n"),
	}
	if err := os.MkdirAll(filepath.Dir(fixture.agentPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(fixture.updaterPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixture.agentPath, fixture.oldBinary, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixture.updaterPath, fixture.oldUpdater, 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := state.Open(fixture.stateDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveIdentity(state.Identity{
		CenterURL: "https://center.example", NodeID: fixture.nodeID, Credential: "ipc_agent_update-test",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.ApplyConfiguration(state.Configuration{
		SchemaVersion: 5, Revision: 1, Enabled: true,
		HistoryGeneration: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		DiscoveryServices: state.DiscoveryServices{
			IPv4: []string{"https://one.example/ip", "https://two.example/ip"},
			IPv6: []string{"https://six-one.example/ip", "https://six-two.example/ip"},
		},
		ProbeSchedule: state.ProbeSchedule{Enabled: true, Cron: "0 0 0 * * *", Timezone: "agent-local"},
		Egresses: []state.Egress{{
			ID: fixture.egressID, Kind: "default", Family: "ipv4", Enabled: true,
			LightweightIntervalSeconds: 600, ProbeOnAddressChange: true,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	run, err := store.StartProbeRun("schedule", nil, nil, now.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	execution, err := store.StartProbeExecution(run.ID, run.Executions[0].ID, now.Add(-59*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	completed, err := store.CompleteProbeExecution(run.ID, execution.ID, state.ProbeExecutionOutcome{
		Status: "succeeded", StartedAt: execution.StartedAt, CompletedAt: now.Add(-58 * time.Second),
		RawResult: []byte(`{"ip":"203.0.113.20"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.resultFile = completed.ResultFile
	if _, err := store.FinishProbeRun(run.ID, now.Add(-57*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AcceptAgentUpdate(state.AgentUpdateDelivery{
		ID: fixture.taskID, TargetVersion: "0.1.1", CreatedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Minute),
	}, "0.1.0", now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.BeginAgentUpdate(fixture.taskID, "0.1.0", now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkAgentUpdateInstalling(fixture.taskID); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := ensureUpdateDirectory(fixture.stateDirectory); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(StagedBinaryPath(fixture.stateDirectory, fixture.taskID), fixture.newBinary, 0o755); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func assertPreservedAgentState(t *testing.T, fixture supervisorFixture, wantStatus string) {
	t.Helper()
	store, err := state.Open(fixture.stateDirectory)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	identity, err := store.Identity()
	if err != nil || identity.NodeID != fixture.nodeID || identity.Credential != "ipc_agent_update-test" {
		t.Fatalf("preserved identity = %#v, %v", identity, err)
	}
	configuration, err := store.Configuration()
	if err != nil || configuration.Revision != 1 || len(configuration.Egresses) != 1 || configuration.Egresses[0].ID != fixture.egressID {
		t.Fatalf("preserved configuration = %#v, %v", configuration, err)
	}
	updateState, found, err := store.PendingAgentUpdate()
	if err != nil || !found || updateState.Status != wantStatus {
		t.Fatalf("preserved update state = %#v, %v, %v", updateState, found, err)
	}
	result, err := os.ReadFile(filepath.Join(fixture.stateDirectory, "results", fixture.resultFile))
	if err != nil || string(result) != `{"ip":"203.0.113.20"}` {
		t.Fatalf("preserved queued result = %q, %v", result, err)
	}
}

func assertFileContents(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(want) {
		t.Fatalf("file %s = %q, %v; want %q", path, got, err, want)
	}
}

type testLifecycle struct {
	stops   int
	starts  int
	onStart func(int) error
}

func (lifecycle *testLifecycle) StopAgent(context.Context) error {
	lifecycle.stops++
	return nil
}

func (lifecycle *testLifecycle) StartAgent(context.Context) error {
	lifecycle.starts++
	if lifecycle.onStart != nil {
		return lifecycle.onStart(lifecycle.starts)
	}
	return nil
}
