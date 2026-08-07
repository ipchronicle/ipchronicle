package nodes

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"github.com/ipchronicle/ipchronicle/internal/center/database"
)

func TestEnrollmentRegistrationAndHeartbeatLifecycle(t *testing.T) {
	ctx := context.Background()
	store, err := database.Open(ctx, database.PathsFromDataDirectory(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := NewService(store.Config, store.History, store.ConfigQueries, store.MasterKey)
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
	if _, err := service.Poll(ctx, "wrong-credential", metadata, 0, nil, nil); !errors.Is(err, ErrAgentUnauthenticated) {
		t.Fatalf("wrong Agent credential error = %v", err)
	}
	now = now.Add(time.Minute)
	poll, err := service.Poll(ctx, registration.Credential, metadata, 0, nil, nil)
	if err != nil || !poll.Enabled || poll.DesiredConfigurationRevision != 1 {
		t.Fatalf("poll = %#v, %v", poll, err)
	}
	configuration, err := service.Configuration(ctx, registration.Credential)
	if err != nil || configuration.Revision != 1 || !configuration.Enabled || len(configuration.HistoryGeneration) != 64 {
		t.Fatalf("configuration = %#v, %v", configuration, err)
	}
	if _, err := service.Poll(ctx, registration.Credential, metadata, 1, nil, nil); err != nil {
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

	restarted := NewService(store.Config, store.History, store.ConfigQueries, store.MasterKey)
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
	if _, err := restarted.Poll(ctx, registration.Credential, metadata, 1, nil, nil); err != nil {
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
	service := NewService(store.Config, store.History, store.ConfigQueries, store.MasterKey)
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
	if _, err := service.Poll(ctx, registration.Credential, testMetadata(), 1, &message, &errorRevision); err != nil {
		t.Fatal(err)
	}
	failed, err := service.Get(ctx, registration.NodeID)
	if err != nil || failed.ConfigurationStatus != "failed" || failed.ConfigurationError == nil {
		t.Fatalf("failed node = %#v, %v", failed, err)
	}
	if _, err := service.Poll(ctx, registration.Credential, testMetadata(), 2, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Poll(ctx, registration.Credential, testMetadata(), 1, nil, nil); !errors.Is(err, ErrInvalidMetadata) {
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
	if _, err := service.Poll(ctx, registration.Credential, testMetadata(), 2, nil, nil); !errors.Is(err, ErrAgentRevoked) {
		t.Fatalf("deleted credential error = %v", err)
	}
}

func testMetadata() Metadata {
	return Metadata{
		Hostname: "edge.example", AgentVersion: "0.1.0",
		OperatingSystem: "linux", Architecture: "amd64",
		Capabilities: []string{"control-v1"},
	}
}
