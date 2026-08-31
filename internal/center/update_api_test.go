package center

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/center/admin"
	"github.com/ipchronicle/ipchronicle/internal/center/database"
	"github.com/ipchronicle/ipchronicle/internal/center/nodes"
	"github.com/ipchronicle/ipchronicle/internal/center/notifications"
	"github.com/ipchronicle/ipchronicle/internal/center/syncws"
	"github.com/ipchronicle/ipchronicle/internal/center/systemsettings"
	centerupdates "github.com/ipchronicle/ipchronicle/internal/center/updates"
	"github.com/ipchronicle/ipchronicle/internal/generated/api"
	"github.com/ipchronicle/ipchronicle/internal/releaseinfo"
)

func TestAgentUpdateAPIAuthenticationChannelAndBatchResults(t *testing.T) {
	fixture := newUpdateHTTPFixture(t)
	unauthenticatedState := performRequest(fixture.handler, http.MethodGet, "/api/v1/agent-updates", nil, "", nil)
	assertErrorCode(t, unauthenticatedState, http.StatusUnauthorized, api.Unauthenticated)
	unauthenticatedCreate := performRequest(
		fixture.handler, http.MethodPost, "/api/v1/agent-updates",
		[]byte(`{"nodeIds":["00000000-0000-0000-0000-000000000001"],"targetVersion":"0.1.1"}`),
		"http://example.test", nil,
	)
	assertErrorCode(t, unauthenticatedCreate, http.StatusUnauthorized, api.Unauthenticated)

	cookie, session := loginTestAdministrator(t, fixture.handler)
	stateResponse := performRequest(fixture.handler, http.MethodGet, "/api/v1/agent-updates", nil, "", cookie)
	if stateResponse.Code != http.StatusOK {
		t.Fatalf("Agent update state status = %d, body = %s", stateResponse.Code, stateResponse.Body.String())
	}
	var state api.AgentUpdateState
	if err := json.NewDecoder(stateResponse.Body).Decode(&state); err != nil {
		t.Fatal(err)
	}
	if state.Channel != api.Stable || state.CurrentVersion != "0.1.0" ||
		state.CurrentRevision != fixture.centerRevision || state.AvailableRelease == nil ||
		state.AvailableRelease.Version != "0.1.1" || state.AvailableRelease.Revision != fixture.releaseRevision {
		t.Fatalf("stable Agent update state = %#v", state)
	}

	batchBody, err := json.Marshal(map[string]any{
		"nodeIds":       []uuid.UUID{fixture.onlineNode, fixture.offlineNode, fixture.missingNode},
		"targetVersion": "0.1.1",
	})
	if err != nil {
		t.Fatal(err)
	}
	missingBatchCSRF := performRequest(
		fixture.handler, http.MethodPost, "/api/v1/agent-updates", batchBody,
		"http://example.test", cookie,
	)
	assertErrorCode(t, missingBatchCSRF, http.StatusForbidden, api.CsrfFailed)
	batchResponse := performRequestWithCSRF(
		fixture.handler, http.MethodPost, "/api/v1/agent-updates", batchBody,
		"http://example.test", cookie, session.CsrfToken,
	)
	if batchResponse.Code != http.StatusAccepted {
		t.Fatalf("Agent update batch status = %d, body = %s", batchResponse.Code, batchResponse.Body.String())
	}
	var batch api.AgentUpdateBatchResult
	if err := json.NewDecoder(batchResponse.Body).Decode(&batch); err != nil {
		t.Fatal(err)
	}
	if batch.TargetVersion != "0.1.1" || len(batch.Items) != 3 || !batch.Items[0].Accepted ||
		batch.Items[0].Task == nil || batch.Items[0].Task.Status != api.AgentUpdateTaskStatusPending ||
		batch.Items[1].Accepted || batch.Items[1].Error == nil || *batch.Items[1].Error != api.AgentUpdateNodeOffline ||
		batch.Items[2].Accepted || batch.Items[2].Error == nil || *batch.Items[2].Error != api.AgentUpdateNodeNotFound {
		t.Fatalf("Agent update batch = %#v", batch)
	}

	missingChannelCSRF := performRequest(
		fixture.handler, http.MethodPut, "/api/v1/agent-updates/channel", []byte(`{"channel":"rc"}`),
		"http://example.test", cookie,
	)
	assertErrorCode(t, missingChannelCSRF, http.StatusForbidden, api.CsrfFailed)
	channelResponse := performRequestWithCSRF(
		fixture.handler, http.MethodPut, "/api/v1/agent-updates/channel", []byte(`{"channel":"rc"}`),
		"http://example.test", cookie, session.CsrfToken,
	)
	if channelResponse.Code != http.StatusOK {
		t.Fatalf("release channel update status = %d, body = %s", channelResponse.Code, channelResponse.Body.String())
	}
	var rcState api.AgentUpdateState
	if err := json.NewDecoder(channelResponse.Body).Decode(&rcState); err != nil {
		t.Fatal(err)
	}
	if rcState.Channel != api.Rc || rcState.AvailableRelease == nil ||
		rcState.AvailableRelease.Version != "0.2.0-rc.1" || len(rcState.Tasks) != 1 ||
		rcState.Tasks[0].NodeId != fixture.onlineNode {
		t.Fatalf("RC Agent update state = %#v", rcState)
	}
}

func TestNodeAPIExposesAgentSourceRevision(t *testing.T) {
	fixture := newUpdateHTTPFixture(t)
	cookie, _ := loginTestAdministrator(t, fixture.handler)
	response := performRequest(fixture.handler, http.MethodGet, "/api/v1/nodes", nil, "", cookie)
	if response.Code != http.StatusOK {
		t.Fatalf("node list status = %d, body = %s", response.Code, response.Body.String())
	}
	var list api.NodeList
	if err := json.NewDecoder(response.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	for _, node := range list.Items {
		if node.Id == fixture.onlineNode {
			if node.SourceRevision == nil || *node.SourceRevision != fixture.agentRevision {
				t.Fatalf("online node source revision = %#v", node.SourceRevision)
			}
			return
		}
	}
	t.Fatalf("online node %s is missing from %#v", fixture.onlineNode, list.Items)
}

type updateHTTPFixture struct {
	handler         http.Handler
	onlineNode      uuid.UUID
	offlineNode     uuid.UUID
	missingNode     uuid.UUID
	centerRevision  string
	agentRevision   string
	releaseRevision string
}

func newUpdateHTTPFixture(t *testing.T) updateHTTPFixture {
	t.Helper()
	ctx := context.Background()
	publishedAt := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	releaseRevision := strings.Repeat("c", 40)
	manifests := map[string][]byte{
		"v0.1.1":      updateAPIReleaseManifest(t, "0.1.1", releaseRevision),
		"v0.2.0-rc.1": updateAPIReleaseManifest(t, "0.2.0-rc.1", releaseRevision),
	}
	releaseServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch {
		case request.URL.Path == "/releases":
			response.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(response).Encode([]map[string]any{
				{"tag_name": "v0.1.1", "draft": false, "prerelease": false, "published_at": publishedAt},
				{"tag_name": "v0.2.0-rc.1", "draft": false, "prerelease": true, "published_at": publishedAt.Add(time.Hour)},
			})
		case strings.HasPrefix(request.URL.Path, "/download/") && strings.HasSuffix(request.URL.Path, "/"+releaseinfo.ManifestAssetName):
			tag := strings.TrimSuffix(strings.TrimPrefix(request.URL.Path, "/download/"), "/"+releaseinfo.ManifestAssetName)
			manifest, ok := manifests[tag]
			if !ok {
				http.NotFound(response, request)
				return
			}
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write(manifest)
		default:
			http.NotFound(response, request)
		}
	}))
	t.Cleanup(releaseServer.Close)

	store, err := database.Open(ctx, database.PathsFromDataDirectory(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	administrator := admin.NewService(store.Config, store.ConfigQueries, store.MasterKey)
	if err := administrator.Bootstrap(ctx, "admin", "admin"); err != nil {
		t.Fatal(err)
	}
	syncHub := syncws.NewHub()
	nodeService := nodes.NewService(store.Config, store.History, store.ConfigQueries, store.MasterKey, syncHub)
	systemSettingsService := systemsettings.NewService(store.ConfigQueries)
	notificationService := notifications.NewService(notifications.ServiceOptions{
		ConfigDatabase: store.Config, HistoryDatabase: store.History,
		ConfigQueries: store.ConfigQueries, HistoryQueries: store.HistoryQueries,
		MasterKey: store.MasterKey, SystemSettings: systemSettingsService, Executable: "/proc/self/exe",
	})
	centerRevision := strings.Repeat("b", 40)
	updateService := centerupdates.NewService(centerupdates.ServiceOptions{
		Queries: store.ConfigQueries, Waker: syncHub,
		CurrentVersion: "0.1.0", CurrentRevision: centerRevision,
		GitHubAPIURL: releaseServer.URL + "/releases", ReleaseDownloadURL: releaseServer.URL + "/download",
	})
	handler := NewHTTPHandler(HTTPOptions{
		Version: "0.1.0", Revision: centerRevision, Web: http.NotFoundHandler(),
		Administrator: administrator, Nodes: nodeService, Notifications: notificationService,
		Updates: updateService, SyncHub: syncHub, SystemSettings: systemSettingsService, Store: store,
	})

	agentRevision := strings.Repeat("a", 40)
	metadata := nodes.Metadata{
		Hostname: "update-online", AgentVersion: "0.1.0", AgentRevision: &agentRevision,
		OperatingSystem: "linux", Architecture: "amd64",
		Capabilities: []string{
			"address-observation-v1", centerupdates.AgentUpdateCapability, "complete-probe-v1",
			"configuration-v7", "control-v1", "network-inventory-v1", "sync-wakeup-v1",
		},
		PhysicalMemoryBytes: 512 * 1024 * 1024,
	}
	enrollment, err := nodeService.RotateEnrollmentKey(ctx, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	online, err := nodeService.Register(ctx, enrollment.Key, metadata)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := nodeService.Poll(ctx, online.Credential, metadata, 0, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	metadata.Hostname = "update-offline"
	offline, err := nodeService.Register(ctx, enrollment.Key, metadata)
	if err != nil {
		t.Fatal(err)
	}
	return updateHTTPFixture{
		handler: handler, onlineNode: online.NodeID, offlineNode: offline.NodeID, missingNode: uuid.New(),
		centerRevision: centerRevision, agentRevision: agentRevision, releaseRevision: releaseRevision,
	}
}

func updateAPIReleaseManifest(t *testing.T, version, revision string) []byte {
	t.Helper()
	capabilities := slices.Clone(releaseinfo.RequiredAgentCapabilities)
	slices.Sort(capabilities)
	channel := "stable"
	if strings.Contains(version, "-rc.") {
		channel = "rc"
	}
	manifest := releaseinfo.Manifest{
		SchemaVersion: releaseinfo.ManifestSchemaVersion,
		Version:       version, Tag: "v" + version, Channel: channel, Revision: revision,
		SourceURL:         releaseinfo.SourceRepository + "/tree/v" + version,
		AgentCapabilities: capabilities,
		Artifacts: []releaseinfo.Artifact{
			{Name: releaseinfo.AgentArtifactName("amd64"), Component: "agent", OS: "linux", Arch: "amd64", Size: 1, SHA256: strings.Repeat("a", 64)},
			{Name: releaseinfo.AgentArtifactName("arm64"), Component: "agent", OS: "linux", Arch: "arm64", Size: 1, SHA256: strings.Repeat("b", 64)},
		},
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
