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
	service := NewService(store.Config, store.ConfigQueries, store.MasterKey)
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
	if _, err := service.Poll(ctx, "wrong-credential", metadata, 0, nil); !errors.Is(err, ErrAgentUnauthenticated) {
		t.Fatalf("wrong Agent credential error = %v", err)
	}
	now = now.Add(time.Minute)
	poll, err := service.Poll(ctx, registration.Credential, metadata, 0, nil)
	if err != nil || !poll.Enabled || poll.DesiredConfigurationRevision != 0 {
		t.Fatalf("poll = %#v, %v", poll, err)
	}
	listed, err = service.List(ctx)
	if err != nil || len(listed) != 1 {
		t.Fatalf("nodes after heartbeat = %#v, %v", listed, err)
	}
	if listed[0].Status != "online" || listed[0].ConfigurationStatus != "current" ||
		len(listed[0].Capabilities) != 1 || listed[0].Capabilities[0] != "control-v1" || listed[0].LastSeenAt == nil {
		t.Fatalf("unexpected online node: %#v", listed[0])
	}

	restarted := NewService(store.Config, store.ConfigQueries, store.MasterKey)
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
	if _, err := restarted.Poll(ctx, registration.Credential, metadata, 0, nil); err != nil {
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

func testMetadata() Metadata {
	return Metadata{
		Hostname: "edge.example", AgentVersion: "0.1.0",
		OperatingSystem: "linux", Architecture: "amd64",
		Capabilities: []string{"control-v1"},
	}
}
