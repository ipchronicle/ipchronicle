package updates

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/center/database"
	"github.com/ipchronicle/ipchronicle/internal/center/nodes"
	"github.com/ipchronicle/ipchronicle/internal/releaseinfo"
	"golang.org/x/mod/semver"
)

type testConnections struct {
	mu    sync.Mutex
	wakes []string
}

func (*testConnections) Connected(string, string) bool { return false }
func (*testConnections) Disconnect(string)             {}

func (connections *testConnections) Wake(nodeID string) {
	connections.mu.Lock()
	defer connections.mu.Unlock()
	connections.wakes = append(connections.wakes, nodeID)
}

func TestReleaseDiscoverySelectsStableAndReleaseCandidateChannels(t *testing.T) {
	published := time.Date(2026, time.August, 10, 1, 2, 3, 0, time.UTC)
	server := newReleaseServer(t, []githubRelease{
		{TagName: "v0.2.0-rc.2", Prerelease: true, PublishedAt: published.Add(time.Hour)},
		{TagName: "v0.1.0", PublishedAt: published},
		{TagName: "v0.3.0", Draft: true, PublishedAt: published.Add(2 * time.Hour)},
	}, map[string][]byte{
		"v0.1.0":      releaseManifest(t, "v0.1.0"),
		"v0.2.0-rc.2": releaseManifest(t, "v0.2.0-rc.2"),
	})
	store := openTestStore(t)
	connections := &testConnections{}
	service := NewService(ServiceOptions{
		Queries: store.ConfigQueries, Waker: connections,
		CurrentVersion: "0.1.0-rc.1", CurrentRevision: strings.Repeat("a", 40),
		GitHubAPIURL: server.URL + "/releases", ReleaseDownloadURL: server.URL + "/download",
	})

	stable, err := service.State(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stable.Channel != "stable" || stable.Available == nil || stable.Available.Version != "0.1.0" || stable.DiscoveryError != nil {
		t.Fatalf("stable discovery = %#v", stable)
	}
	rc, err := service.SetChannel(context.Background(), "rc")
	if err != nil {
		t.Fatal(err)
	}
	if rc.Channel != "rc" || rc.Available == nil || rc.Available.Version != "0.2.0-rc.2" || rc.DiscoveryError != nil {
		t.Fatalf("RC discovery = %#v", rc)
	}
	if _, err := service.SetChannel(context.Background(), "nightly"); !errors.Is(err, ErrInvalidChannel) {
		t.Fatalf("invalid channel error = %v", err)
	}
}

func TestReleaseDiscoveryRejectsMalformedManifest(t *testing.T) {
	server := newReleaseServer(t, []githubRelease{{
		TagName: "v0.1.1", PublishedAt: time.Now().UTC(),
	}}, map[string][]byte{"v0.1.1": []byte(`{"schemaVersion":1}`)})
	store := openTestStore(t)
	service := NewService(ServiceOptions{
		Queries: store.ConfigQueries, Waker: &testConnections{},
		CurrentVersion: "0.1.0", CurrentRevision: strings.Repeat("a", 40),
		GitHubAPIURL: server.URL + "/releases", ReleaseDownloadURL: server.URL + "/download",
	})
	state, err := service.State(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.Available != nil || state.DiscoveryError == nil || *state.DiscoveryError != "release-discovery-failed" {
		t.Fatalf("malformed discovery = %#v", state)
	}
}

func TestCreateTasksEnforcesNodeAndSharedSlotBoundaries(t *testing.T) {
	ctx := context.Background()
	server := newReleaseServer(t, []githubRelease{{
		TagName: "v0.1.1", PublishedAt: time.Now().UTC(),
	}}, map[string][]byte{"v0.1.1": releaseManifest(t, "v0.1.1")})
	store := openTestStore(t)
	connections := &testConnections{}
	nodeService := nodes.NewService(store.Config, store.History, store.ConfigQueries, store.MasterKey, connections)
	enrollment, err := nodeService.RotateEnrollmentKey(ctx, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	online := registerTestNode(t, nodeService, enrollment.Key, "online", "0.1.0", true, true)
	offline := registerTestNode(t, nodeService, enrollment.Key, "offline", "0.1.0", true, false)
	unsupported := registerTestNode(t, nodeService, enrollment.Key, "unsupported", "0.1.0", false, true)
	current := registerTestNode(t, nodeService, enrollment.Key, "current", "0.1.1", true, true)
	now := time.Now().UTC().Truncate(time.Second)

	service := NewService(ServiceOptions{
		Queries: store.ConfigQueries, Waker: connections,
		CurrentVersion: "0.1.0", CurrentRevision: strings.Repeat("a", 40),
		GitHubAPIURL: server.URL + "/releases", ReleaseDownloadURL: server.URL + "/download",
		Now: func() time.Time { return now },
	})
	result, err := service.CreateTasks(ctx, []uuid.UUID{online, offline, unsupported, current}, "0.1.1")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 4 || !result.Items[0].Accepted || result.Items[0].Task == nil ||
		result.Items[1].Error == nil || *result.Items[1].Error != "agent_update_node_offline" ||
		result.Items[2].Error == nil || *result.Items[2].Error != "agent_update_unsupported" ||
		result.Items[3].Error == nil || *result.Items[3].Error != "agent_update_not_available" {
		t.Fatalf("batch result = %#v", result)
	}
	occupied, err := service.CreateTasks(ctx, []uuid.UUID{online}, "0.1.1")
	if err != nil {
		t.Fatal(err)
	}
	if occupied.Items[0].Error == nil || *occupied.Items[0].Error != "agent_update_task_slot_occupied" {
		t.Fatalf("occupied task result = %#v", occupied)
	}
	state, err := service.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Tasks) != 1 || state.Tasks[0].NodeID != online || state.Tasks[0].TargetVersion != "0.1.1" || state.Tasks[0].Status != "pending" {
		t.Fatalf("stored update tasks = %#v", state.Tasks)
	}
	connections.mu.Lock()
	wakes := slices.Clone(connections.wakes)
	connections.mu.Unlock()
	if !slices.Contains(wakes, online.String()) {
		t.Fatalf("accepted node was not woken: %#v", wakes)
	}
	now = now.Add(taskDeliveryWindow)
	expired, err := service.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(expired.Tasks) != 1 || expired.Tasks[0].Status != "expired" || expired.Tasks[0].CompletedAt == nil {
		t.Fatalf("expired update task = %#v", expired.Tasks)
	}
	if _, err := store.ConfigQueries.GetActiveNodeTask(ctx, online.String()); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expired update task still occupies the node task slot: %v", err)
	}
	if _, err := service.CreateTasks(ctx, []uuid.UUID{online}, "0.1.2"); !errors.Is(err, ErrTargetUnavailable) {
		t.Fatalf("undiscovered target error = %v", err)
	}
	if _, err := service.CreateTasks(ctx, []uuid.UUID{online}, "latest"); !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf("invalid target error = %v", err)
	}
}

func registerTestNode(t *testing.T, service *nodes.Service, key, hostname, version string, updateCapable, online bool) uuid.UUID {
	t.Helper()
	capabilities := []string{"control-v1", "configuration-v8"}
	if updateCapable {
		capabilities = append(capabilities, AgentUpdateCapability)
	}
	metadata := nodes.Metadata{
		Hostname: hostname, AgentVersion: version, OperatingSystem: "linux", Architecture: "amd64",
		Capabilities: capabilities, PhysicalMemoryBytes: 512 * 1024 * 1024,
	}
	registration, err := service.Register(context.Background(), key, metadata)
	if err != nil {
		t.Fatal(err)
	}
	if online {
		if _, err := service.Poll(context.Background(), registration.Credential, metadata, 0, nil, nil, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	return registration.NodeID
}

func openTestStore(t *testing.T) *database.Store {
	t.Helper()
	store, err := database.Open(context.Background(), database.PathsFromDataDirectory(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func newReleaseServer(t *testing.T, releases []githubRelease, manifests map[string][]byte) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch {
		case request.URL.Path == "/releases":
			response.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(response).Encode(releases); err != nil {
				t.Error(err)
			}
		case strings.HasPrefix(request.URL.Path, "/download/") && strings.HasSuffix(request.URL.Path, "/"+releaseinfo.ManifestAssetName):
			tag := strings.TrimSuffix(strings.TrimPrefix(request.URL.Path, "/download/"), "/"+releaseinfo.ManifestAssetName)
			manifest, exists := manifests[tag]
			if !exists {
				http.NotFound(response, request)
				return
			}
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write(manifest)
		default:
			http.NotFound(response, request)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func releaseManifest(t *testing.T, tag string) []byte {
	t.Helper()
	capabilities := slices.Clone(releaseinfo.RequiredAgentCapabilities)
	slices.Sort(capabilities)
	channel := "stable"
	if semver.Prerelease(tag) != "" {
		channel = "rc"
	}
	manifest := releaseinfo.Manifest{
		SchemaVersion: releaseinfo.ManifestSchemaVersion,
		Version:       strings.TrimPrefix(tag, "v"), Tag: tag, Channel: channel,
		Revision: strings.Repeat("a", 40), SourceURL: releaseinfo.SourceRepository + "/tree/" + tag,
		AgentCapabilities: capabilities,
		Artifacts: []releaseinfo.Artifact{
			{Name: releaseinfo.AgentArtifactName("amd64"), Component: "agent", OS: "linux", Arch: "amd64", Size: 123, SHA256: strings.Repeat("a", 64)},
			{Name: releaseinfo.AgentArtifactName("arm64"), Component: "agent", OS: "linux", Arch: "arm64", Size: 124, SHA256: strings.Repeat("b", 64)},
		},
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
