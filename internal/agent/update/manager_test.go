package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/agent/state"
	"github.com/ipchronicle/ipchronicle/internal/releaseinfo"
)

func TestManagerStagesValidatedAgentAndStartsSupervisor(t *testing.T) {
	stateDirectory := filepath.Join(t.TempDir(), "agent")
	store, err := state.Open(stateDirectory)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	candidate := candidateAgentScript(t, "0.1.1", strings.Repeat("a", 40))
	server := newAgentReleaseServer(t, "0.1.1", strings.Repeat("a", 40), candidate, false)
	triggered := make(chan string, 1)
	now := time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC)
	manager, err := NewManager(ManagerOptions{
		Store: store, CurrentVersion: "0.1.0",
		Config:             Config{InitSystem: "systemd", AgentPath: "/usr/local/bin/ipchronicle-agent", UpdaterPath: "/usr/local/libexec/ipchronicle-agent-updater"},
		ReleaseDownloadURL: server.URL + "/download", Now: func() time.Time { return now }, Logger: log.New(io.Discard, "", 0),
		Trigger: func(_ context.Context, initSystem string) error {
			triggered <- initSystem
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.NewString()
	if err := manager.AcceptTask(state.AgentUpdateDelivery{
		ID: id, TargetVersion: "0.1.1", CreatedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- manager.Run(ctx) }()
	select {
	case initSystem := <-triggered:
		if initSystem != "systemd" {
			t.Fatalf("trigger init system = %q", initSystem)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Agent update supervisor was not triggered")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	updateState, found, err := store.PendingAgentUpdate()
	if err != nil || !found || updateState.Status != "installing" {
		t.Fatalf("staged update = %#v, %v, %v", updateState, found, err)
	}
	staged, err := os.ReadFile(StagedBinaryPath(stateDirectory, id))
	if err != nil || string(staged) != string(candidate) {
		t.Fatalf("staged Agent mismatch: %v", err)
	}
	info, err := os.Stat(StagedBinaryPath(stateDirectory, id))
	if err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("staged Agent mode = %v, %v", info, err)
	}
}

func TestManagerRecordsChecksumFailureWithoutStartingSupervisor(t *testing.T) {
	store, err := state.Open(filepath.Join(t.TempDir(), "agent"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	candidate := candidateAgentScript(t, "0.1.1", strings.Repeat("b", 40))
	server := newAgentReleaseServer(t, "0.1.1", strings.Repeat("b", 40), candidate, true)
	triggered := make(chan struct{}, 1)
	now := time.Now().UTC().Truncate(time.Second)
	manager, err := NewManager(ManagerOptions{
		Store: store, CurrentVersion: "0.1.0",
		Config:             Config{InitSystem: "openrc", AgentPath: "/usr/local/bin/ipchronicle-agent", UpdaterPath: "/usr/local/libexec/ipchronicle-agent-updater"},
		ReleaseDownloadURL: server.URL + "/download", Now: func() time.Time { return now }, Logger: log.New(io.Discard, "", 0),
		Trigger: func(context.Context, string) error { triggered <- struct{}{}; return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.NewString()
	if err := manager.AcceptTask(state.AgentUpdateDelivery{
		ID: id, TargetVersion: "0.1.1", CreatedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- manager.Run(ctx) }()
	updateState := waitForUpdateStatus(t, store, "failed")
	if updateState.FailureCode == nil || *updateState.FailureCode != "artifact-checksum" || updateState.Diagnostic == nil {
		t.Fatalf("checksum failure = %#v", updateState)
	}
	select {
	case <-triggered:
		t.Fatal("supervisor was triggered for an invalid artifact")
	default:
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestManagerRejectsIncompatibleStateSchemaWithoutStartingSupervisor(t *testing.T) {
	store, err := state.Open(filepath.Join(t.TempDir(), "agent"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	candidate := candidateAgentScriptWithSchema(t, "0.1.1", strings.Repeat("c", 40), state.SchemaVersion()+1)
	server := newAgentReleaseServer(t, "0.1.1", strings.Repeat("c", 40), candidate, false)
	triggered := make(chan struct{}, 1)
	now := time.Now().UTC().Truncate(time.Second)
	manager, err := NewManager(ManagerOptions{
		Store: store, CurrentVersion: "0.1.0",
		Config:             Config{InitSystem: "systemd", AgentPath: "/usr/local/bin/ipchronicle-agent", UpdaterPath: "/usr/local/libexec/ipchronicle-agent-updater"},
		ReleaseDownloadURL: server.URL + "/download", Now: func() time.Time { return now }, Logger: log.New(io.Discard, "", 0),
		Trigger: func(context.Context, string) error { triggered <- struct{}{}; return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.NewString()
	if err := manager.AcceptTask(state.AgentUpdateDelivery{
		ID: id, TargetVersion: "0.1.1", CreatedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- manager.Run(ctx) }()
	updateState := waitForUpdateStatus(t, store, "failed")
	if updateState.FailureCode == nil || *updateState.FailureCode != "binary-metadata" ||
		updateState.Diagnostic == nil || !strings.Contains(*updateState.Diagnostic, "incompatible local state schema") {
		t.Fatalf("state schema failure = %#v", updateState)
	}
	if _, err := os.Stat(StagedBinaryPath(store.Directory(), id)); !os.IsNotExist(err) {
		t.Fatalf("staged incompatible Agent remains: %v", err)
	}
	select {
	case <-triggered:
		t.Fatal("supervisor was triggered for an incompatible state schema")
	default:
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func candidateAgentScript(t *testing.T, version, revision string) []byte {
	t.Helper()
	return candidateAgentScriptWithSchema(t, version, revision, state.SchemaVersion())
}

func candidateAgentScriptWithSchema(t *testing.T, version, revision string, schemaVersion int) []byte {
	t.Helper()
	info := releaseinfo.BinaryInfo{
		Version: version, Revision: revision, Component: "agent", OS: "linux", Arch: runtime.GOARCH,
		Capabilities: slices.Clone(releaseinfo.RequiredAgentCapabilities), StateSchemaVersion: schemaVersion,
	}
	encoded, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	return []byte("#!/bin/sh\nprintf '%s\\n' '" + string(encoded) + "'\n")
}

func newAgentReleaseServer(t *testing.T, version, revision string, candidate []byte, corruptChecksum bool) *httptest.Server {
	t.Helper()
	currentDigest := sha256.Sum256(candidate)
	digest := hex.EncodeToString(currentDigest[:])
	if corruptChecksum {
		digest = strings.Repeat("0", 64)
	}
	otherArch := "arm64"
	if runtime.GOARCH == "arm64" {
		otherArch = "amd64"
	}
	other := []byte("other architecture")
	otherDigest := sha256.Sum256(other)
	manifest := releaseinfo.Manifest{
		SchemaVersion: releaseinfo.ManifestSchemaVersion, Version: version, Tag: "v" + version, Channel: "stable",
		Revision: revision, SourceURL: releaseinfo.SourceRepository + "/tree/v" + version,
		AgentCapabilities: slices.Clone(releaseinfo.RequiredAgentCapabilities),
		Artifacts: []releaseinfo.Artifact{
			{Name: releaseinfo.AgentArtifactName(runtime.GOARCH), Component: "agent", OS: "linux", Arch: runtime.GOARCH, Size: int64(len(candidate)), SHA256: digest},
			{Name: releaseinfo.AgentArtifactName(otherArch), Component: "agent", OS: "linux", Arch: otherArch, Size: int64(len(other)), SHA256: hex.EncodeToString(otherDigest[:])},
		},
	}
	slices.SortFunc(manifest.Artifacts, func(left, right releaseinfo.Artifact) int { return strings.Compare(left.Name, right.Name) })
	encodedManifest, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/download/v" + version + "/" + releaseinfo.ManifestAssetName:
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write(encodedManifest)
		case "/download/v" + version + "/" + releaseinfo.AgentArtifactName(runtime.GOARCH):
			response.Header().Set("Content-Type", "application/octet-stream")
			_, _ = response.Write(candidate)
		default:
			http.NotFound(response, request)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func waitForUpdateStatus(t *testing.T, store *state.Store, status string) state.AgentUpdate {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		updateState, found, err := store.PendingAgentUpdate()
		if err != nil {
			t.Fatal(err)
		}
		if found && updateState.Status == status {
			return updateState
		}
		if time.Now().After(deadline) {
			t.Fatalf("Agent update did not reach %q: %#v", status, updateState)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
