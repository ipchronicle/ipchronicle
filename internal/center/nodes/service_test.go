package nodes

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/center/database"
)

type testSyncConnections struct {
	connectedSession string
	wakes            []string
	disconnections   []string
}

func (s *testSyncConnections) Connected(_ string, sessionID string) bool {
	return s.connectedSession == sessionID
}

func (s *testSyncConnections) Wake(nodeID string) {
	s.wakes = append(s.wakes, nodeID)
}

func (s *testSyncConnections) Disconnect(nodeID string) {
	s.disconnections = append(s.disconnections, nodeID)
}

func TestEnrollmentRegistrationAndHeartbeatLifecycle(t *testing.T) {
	ctx := context.Background()
	store, err := database.Open(ctx, database.PathsFromDataDirectory(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := NewService(store.Config, store.History, store.ConfigQueries, store.MasterKey, &testSyncConnections{})
	now := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

	enrollment, err := service.Enrollment(ctx)
	if err != nil || enrollment.HasKey || enrollment.Enabled {
		t.Fatalf("unexpected initial enrollment: %#v, %v", enrollment, err)
	}
	if _, err := service.SetEnrollmentEnabled(ctx, true); !errors.Is(err, ErrEnrollmentKeyMissing) {
		t.Fatalf("enable without key error = %v", err)
	}
	enrollment, err = service.RotateEnrollmentKey(ctx)
	if err != nil || !enrollment.HasKey || !enrollment.Enabled || enrollment.Key == "" {
		t.Fatalf("rotated enrollment = %#v, %v", enrollment, err)
	}
	record, err := store.ConfigQueries.GetAgentEnrollment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(record.KeyEncrypted, []byte(enrollment.Key)) {
		t.Fatal("registration key is stored as plaintext")
	}

	metadata := testMetadata()
	if _, err := service.Register(ctx, "wrong-key", metadata); !errors.Is(err, ErrEnrollmentKeyInvalid) {
		t.Fatalf("wrong registration key error = %v", err)
	}
	disabled, err := service.SetEnrollmentEnabled(ctx, false)
	if err != nil || disabled.Enabled {
		t.Fatalf("disable enrollment = %#v, %v", disabled, err)
	}
	if _, err := service.Register(ctx, enrollment.Key, metadata); !errors.Is(err, ErrEnrollmentDisabled) {
		t.Fatalf("disabled registration error = %v", err)
	}
	if _, err := service.SetEnrollmentEnabled(ctx, true); err != nil {
		t.Fatal(err)
	}
	registration, err := service.Register(ctx, enrollment.Key, metadata)
	if err != nil {
		t.Fatal(err)
	}
	if registration.NodeID.String() == "" || registration.Credential == "" {
		t.Fatalf("incomplete registration: %#v", registration)
	}
	credentialDigest := sha256.Sum256([]byte(registration.Credential))
	nodeRecord, err := store.ConfigQueries.GetNodeByCredentialDigest(ctx, credentialDigest[:])
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(nodeRecord.CredentialDigest, []byte(registration.Credential)) {
		t.Fatal("Agent credential is stored as plaintext")
	}

	listed, err := service.List(ctx)
	if err != nil || len(listed) != 1 || listed[0].Status != "offline" {
		t.Fatalf("nodes before heartbeat = %#v, %v", listed, err)
	}
	if _, err := service.Poll(ctx, "wrong-credential", metadata, 0, nil, nil, nil, nil); !errors.Is(err, ErrAgentUnauthenticated) {
		t.Fatalf("wrong Agent credential error = %v", err)
	}
	now = now.Add(time.Minute)
	poll, err := service.Poll(ctx, registration.Credential, metadata, 0, nil, nil, nil, nil)
	if err != nil || !poll.Enabled || poll.DesiredConfigurationRevision != 1 {
		t.Fatalf("poll = %#v, %v", poll, err)
	}
	configuration, err := service.Configuration(ctx, registration.Credential)
	if err != nil || configuration.Revision != 1 || !configuration.Enabled || len(configuration.HistoryGeneration) != 64 {
		t.Fatalf("configuration = %#v, %v", configuration, err)
	}
	if _, err := service.Poll(ctx, registration.Credential, metadata, 1, nil, nil, nil, nil); err != nil {
		t.Fatalf("applied configuration poll: %v", err)
	}
	listed, err = service.List(ctx)
	if err != nil || len(listed) != 1 {
		t.Fatalf("nodes after heartbeat = %#v, %v", listed, err)
	}
	if listed[0].Status != "online" || listed[0].ConfigurationStatus != "current" ||
		len(listed[0].Capabilities) != 1 || listed[0].Capabilities[0] != "control-v1" || listed[0].LastSeenAt == nil {
		t.Fatalf("unexpected online node: %#v", listed[0])
	}

	restarted := NewService(store.Config, store.History, store.ConfigQueries, store.MasterKey, &testSyncConnections{})
	restartedEnrollment, err := restarted.Enrollment(ctx)
	if err != nil || restartedEnrollment.Key != enrollment.Key {
		t.Fatalf("enrollment key did not survive restart: %#v, %v", restartedEnrollment, err)
	}
	rotated, err := restarted.RotateEnrollmentKey(ctx)
	if err != nil || rotated.Key == enrollment.Key {
		t.Fatalf("second rotation = %#v, %v", rotated, err)
	}
	if _, err := restarted.Register(ctx, enrollment.Key, metadata); !errors.Is(err, ErrEnrollmentKeyInvalid) {
		t.Fatalf("old key remains valid after rotation: %v", err)
	}
	if _, err := restarted.Poll(ctx, registration.Credential, metadata, 1, nil, nil, nil, nil); err != nil {
		t.Fatalf("key rotation invalidated existing Agent: %v", err)
	}
}

func TestAgentMetadataValidation(t *testing.T) {
	valid := testMetadata()
	if _, err := validateMetadata(valid); err != nil {
		t.Fatal(err)
	}
	invalid := valid
	invalid.Capabilities = []string{"control-v1", "control-v1"}
	if _, err := validateMetadata(invalid); !errors.Is(err, ErrInvalidMetadata) {
		t.Fatalf("duplicate capability error = %v", err)
	}
	invalid = valid
	invalid.OperatingSystem = "windows"
	if _, err := validateMetadata(invalid); !errors.Is(err, ErrInvalidMetadata) {
		t.Fatalf("unsupported operating system error = %v", err)
	}
}

func TestConfigurationFailureAndNodeLifecycle(t *testing.T) {
	ctx := context.Background()
	store, err := database.Open(ctx, database.PathsFromDataDirectory(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := NewService(store.Config, store.History, store.ConfigQueries, store.MasterKey, &testSyncConnections{})
	now := time.Date(2026, 8, 7, 13, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	enrollment, err := service.RotateEnrollmentKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	registration, err := service.Register(ctx, enrollment.Key, testMetadata())
	if err != nil {
		t.Fatal(err)
	}

	disabled, err := service.SetEnabled(ctx, registration.NodeID, false)
	if err != nil || disabled.Enabled || disabled.DesiredConfigurationRevision != 2 || disabled.ConfigurationStatus != "pending" {
		t.Fatalf("disabled node = %#v, %v", disabled, err)
	}
	configuration, err := service.Configuration(ctx, registration.Credential)
	if err != nil || configuration.Enabled || configuration.Revision != 2 {
		t.Fatalf("disabled configuration = %#v, %v", configuration, err)
	}
	message := "snapshot rejected for test"
	errorRevision := int64(2)
	if _, err := service.Poll(ctx, registration.Credential, testMetadata(), 1, &message, &errorRevision, nil, nil); err != nil {
		t.Fatal(err)
	}
	failed, err := service.Get(ctx, registration.NodeID)
	if err != nil || failed.ConfigurationStatus != "failed" || failed.ConfigurationError == nil {
		t.Fatalf("failed node = %#v, %v", failed, err)
	}
	if _, err := service.Poll(ctx, registration.Credential, testMetadata(), 2, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Poll(ctx, registration.Credential, testMetadata(), 1, nil, nil, nil, nil); !errors.Is(err, ErrInvalidMetadata) {
		t.Fatalf("applied revision rollback error = %v", err)
	}
	current, err := service.Get(ctx, registration.NodeID)
	if err != nil || current.ConfigurationStatus != "current" {
		t.Fatalf("current node = %#v, %v", current, err)
	}

	revoked, err := service.Revoke(ctx, registration.NodeID)
	if err != nil || revoked.Status != "revoked" {
		t.Fatalf("revoked node = %#v, %v", revoked, err)
	}
	if _, err := service.Configuration(ctx, registration.Credential); !errors.Is(err, ErrAgentRevoked) {
		t.Fatalf("revoked configuration error = %v", err)
	}

	deletion, err := service.Delete(ctx, registration.NodeID)
	if err != nil || deletion.Status != "pending" {
		t.Fatalf("queued deletion = %#v, %v", deletion, err)
	}
	service.deleteHistory = func(context.Context, string) error {
		return errors.New("history disk unavailable")
	}
	if err := service.processDeletions(ctx, 1); err != nil {
		t.Fatal(err)
	}
	failedDeletion, err := service.Get(ctx, registration.NodeID)
	if err != nil || failedDeletion.DeletionStatus == nil || *failedDeletion.DeletionStatus != "failed" || failedDeletion.DeletionError == nil {
		t.Fatalf("failed deletion node = %#v, %v", failedDeletion, err)
	}
	service.deleteHistory = service.deleteNodeHistory
	if err := service.processDeletions(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Get(ctx, registration.NodeID); !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("deleted node lookup error = %v", err)
	}
	if _, err := service.Poll(ctx, registration.Credential, testMetadata(), 2, nil, nil, nil, nil); !errors.Is(err, ErrAgentRevoked) {
		t.Fatalf("deleted credential error = %v", err)
	}
}

func TestTemporarySyncSessionLifecycle(t *testing.T) {
	ctx := context.Background()
	store, err := database.Open(ctx, database.PathsFromDataDirectory(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	connections := &testSyncConnections{}
	service := NewService(store.Config, store.History, store.ConfigQueries, store.MasterKey, connections)
	now := time.Date(2026, 8, 9, 4, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	enrollment, err := service.RotateEnrollmentKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	metadata := testMetadata()
	metadata.Capabilities = append(metadata.Capabilities, SyncWakeCapability)
	registration, err := service.Register(ctx, enrollment.Key, metadata)
	if err != nil {
		t.Fatal(err)
	}

	started, err := service.StartSyncSession(ctx, registration.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	if started.SyncStatus == nil || *started.SyncStatus != "pending" || started.SyncExpiresAt == nil ||
		!started.SyncExpiresAt.Equal(now.Add(SyncSessionLease)) {
		t.Fatalf("started sync session = %#v", started)
	}
	if !slices.Contains(connections.disconnections, registration.NodeID.String()) {
		t.Fatalf("starting a replacement session did not disconnect the previous connection: %#v", connections.disconnections)
	}

	restarted := NewService(store.Config, store.History, store.ConfigQueries, store.MasterKey, connections)
	restarted.now = service.now
	persisted, err := restarted.Get(ctx, registration.NodeID)
	if err != nil || persisted.SyncStatus == nil || *persisted.SyncStatus != "pending" {
		t.Fatalf("persisted sync session = %#v, %v", persisted, err)
	}
	poll, err := restarted.Poll(ctx, registration.Credential, metadata, 0, nil, nil, nil, nil)
	if err != nil || poll.SyncSession == nil || !poll.SyncSession.ExpiresAt.Equal(now.Add(SyncSessionLease)) {
		t.Fatalf("sync delivery poll = %#v, %v", poll, err)
	}
	authorization, err := restarted.AuthorizeSync(ctx, registration.Credential, poll.SyncSession.ID)
	if err != nil || authorization.NodeID != registration.NodeID || authorization.SessionID != poll.SyncSession.ID {
		t.Fatalf("sync authorization = %#v, %v", authorization, err)
	}
	if _, err := restarted.AuthorizeSync(ctx, "wrong-credential", poll.SyncSession.ID); !errors.Is(err, ErrAgentUnauthenticated) {
		t.Fatalf("wrong sync credential error = %v", err)
	}
	if _, err := restarted.AuthorizeSync(ctx, registration.Credential, uuid.New()); !errors.Is(err, ErrSyncSessionUnavailable) {
		t.Fatalf("wrong sync session error = %v", err)
	}

	degraded, err := restarted.Get(ctx, registration.NodeID)
	if err != nil || degraded.SyncStatus == nil || *degraded.SyncStatus != "degraded" {
		t.Fatalf("delivered disconnected sync session = %#v, %v", degraded, err)
	}
	connections.connectedSession = poll.SyncSession.ID.String()
	connected, err := restarted.Get(ctx, registration.NodeID)
	if err != nil || connected.SyncStatus == nil || *connected.SyncStatus != "connected" {
		t.Fatalf("connected sync session = %#v, %v", connected, err)
	}

	if _, err := restarted.SetEnabled(ctx, registration.NodeID, false); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(connections.wakes, registration.NodeID.String()) {
		t.Fatalf("configuration change did not wake the sync connection: %#v", connections.wakes)
	}
	stopped, err := restarted.StopSyncSession(ctx, registration.NodeID)
	if err != nil || stopped.SyncStatus != nil || stopped.SyncExpiresAt != nil {
		t.Fatalf("stopped sync session = %#v, %v", stopped, err)
	}
	if _, err := restarted.AuthorizeSync(ctx, registration.Credential, poll.SyncSession.ID); !errors.Is(err, ErrSyncSessionUnavailable) {
		t.Fatalf("stopped sync authorization error = %v", err)
	}

	if _, err := restarted.StartSyncSession(ctx, registration.NodeID); err != nil {
		t.Fatal(err)
	}
	now = now.Add(SyncSessionLease + time.Second)
	expired, err := restarted.Get(ctx, registration.NodeID)
	if err != nil || expired.SyncStatus != nil || expired.SyncExpiresAt != nil {
		t.Fatalf("expired sync session = %#v, %v", expired, err)
	}
	expiredPoll, err := restarted.Poll(ctx, registration.Credential, metadata, 0, nil, nil, nil, nil)
	if err != nil || expiredPoll.SyncSession != nil {
		t.Fatalf("expired sync delivery poll = %#v, %v", expiredPoll, err)
	}
}

func TestTemporarySyncRequiresAdvertisedCapability(t *testing.T) {
	ctx := context.Background()
	store, err := database.Open(ctx, database.PathsFromDataDirectory(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := NewService(store.Config, store.History, store.ConfigQueries, store.MasterKey, &testSyncConnections{})
	enrollment, err := service.RotateEnrollmentKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	registration, err := service.Register(ctx, enrollment.Key, testMetadata())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartSyncSession(ctx, registration.NodeID); !errors.Is(err, ErrNodeSyncUnsupported) {
		t.Fatalf("unsupported sync session error = %v", err)
	}
}

func testMetadata() Metadata {
	return Metadata{
		Hostname: "edge.example", AgentVersion: "0.1.0",
		OperatingSystem: "linux", Architecture: "amd64",
		Capabilities: []string{"control-v1"},
	}
}
