package updates

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/center/database/configdb"
	"github.com/ipchronicle/ipchronicle/internal/center/nodes"
	"github.com/ipchronicle/ipchronicle/internal/releaseinfo"
	"golang.org/x/mod/semver"
)

const (
	AgentUpdateCapability = "agent-update-v1"

	defaultGitHubAPIURL        = "https://api.github.com/repos/ipchronicle/ipchronicle/releases"
	defaultReleaseDownloadURL  = "https://github.com/ipchronicle/ipchronicle/releases/download"
	defaultDiscoveryCacheTTL   = 15 * time.Minute
	maximumGitHubResponseBytes = 2 * 1024 * 1024
	maximumReleaseCount        = 100
	taskDeliveryWindow         = 2 * time.Minute
)

var (
	ErrInvalidChannel    = errors.New("release channel is invalid")
	ErrInvalidTarget     = errors.New("Agent update target is invalid")
	ErrTargetUnavailable = errors.New("Agent update target is not the current discovered release")
)

type Waker interface {
	Wake(nodeID string)
}

type ServiceOptions struct {
	Queries            *configdb.Queries
	Waker              Waker
	CurrentVersion     string
	CurrentRevision    string
	HTTPClient         *http.Client
	GitHubAPIURL       string
	ReleaseDownloadURL string
	DiscoveryCacheTTL  time.Duration
	Now                func() time.Time
}

type Service struct {
	queries            *configdb.Queries
	waker              Waker
	currentVersion     string
	currentRevision    string
	httpClient         *http.Client
	githubAPIURL       string
	releaseDownloadURL string
	cacheTTL           time.Duration
	now                func() time.Time

	cacheMu sync.Mutex
	cache   *discovery
}

type Release struct {
	Version           string
	Tag               string
	Channel           string
	Revision          string
	PublishedAt       time.Time
	AgentCapabilities []string
}

type Task struct {
	ID              uuid.UUID
	NodeID          uuid.UUID
	TargetVersion   string
	Status          string
	CreatedAt       time.Time
	ExpiresAt       time.Time
	AcknowledgedAt  *time.Time
	StartedAt       *time.Time
	CompletedAt     *time.Time
	PreviousVersion *string
	ResultVersion   *string
	FailureCode     *string
	Diagnostic      *string
	Offline         bool
}

type State struct {
	Channel         string
	CurrentVersion  string
	CurrentRevision string
	CheckedAt       time.Time
	Available       *Release
	DiscoveryError  *string
	Tasks           []Task
}

type BatchItem struct {
	NodeID   uuid.UUID
	Accepted bool
	Task     *Task
	Error    *string
}

type BatchResult struct {
	TargetVersion string
	Items         []BatchItem
}

type discovery struct {
	channel   string
	checkedAt time.Time
	expiresAt time.Time
	release   *Release
	errorCode *string
}

type githubRelease struct {
	TagName     string    `json:"tag_name"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
}

func NewService(options ServiceOptions) *Service {
	if options.Queries == nil || options.Waker == nil {
		panic("Agent update service dependencies must not be nil")
	}
	client := options.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	apiURL := strings.TrimSpace(options.GitHubAPIURL)
	if apiURL == "" {
		apiURL = defaultGitHubAPIURL
	}
	downloadURL := strings.TrimRight(strings.TrimSpace(options.ReleaseDownloadURL), "/")
	if downloadURL == "" {
		downloadURL = defaultReleaseDownloadURL
	}
	cacheTTL := options.DiscoveryCacheTTL
	if cacheTTL <= 0 {
		cacheTTL = defaultDiscoveryCacheTTL
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &Service{
		queries: options.Queries, waker: options.Waker,
		currentVersion: options.CurrentVersion, currentRevision: options.CurrentRevision,
		httpClient: client, githubAPIURL: apiURL, releaseDownloadURL: downloadURL,
		cacheTTL: cacheTTL, now: now,
	}
}

func (s *Service) State(ctx context.Context) (State, error) {
	systemState, err := s.queries.GetSystemState(ctx)
	if err != nil {
		return State{}, err
	}
	discovered := s.discover(ctx, systemState.ReleaseChannel)
	tasks, err := s.listTasks(ctx)
	if err != nil {
		return State{}, err
	}
	return State{
		Channel: systemState.ReleaseChannel, CurrentVersion: s.currentVersion,
		CurrentRevision: s.currentRevision, CheckedAt: discovered.checkedAt,
		Available: cloneRelease(discovered.release), DiscoveryError: cloneString(discovered.errorCode),
		Tasks: tasks,
	}, nil
}

func (s *Service) SetChannel(ctx context.Context, channel string) (State, error) {
	if channel != "stable" && channel != "rc" {
		return State{}, ErrInvalidChannel
	}
	if _, err := s.queries.SetReleaseChannel(ctx, configdb.SetReleaseChannelParams{
		ReleaseChannel: channel, ReleaseChannel_2: channel,
	}); err != nil {
		return State{}, err
	}
	s.cacheMu.Lock()
	s.cache = nil
	s.cacheMu.Unlock()
	return s.State(ctx)
}

func (s *Service) CreateTasks(ctx context.Context, nodeIDs []uuid.UUID, targetVersion string) (BatchResult, error) {
	canonical, err := releaseinfo.CanonicalVersion(targetVersion)
	if err != nil || canonical != targetVersion || len(nodeIDs) == 0 || len(nodeIDs) > 70 {
		return BatchResult{}, ErrInvalidTarget
	}
	seen := make(map[uuid.UUID]struct{}, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		if nodeID == uuid.Nil {
			return BatchResult{}, ErrInvalidTarget
		}
		if _, exists := seen[nodeID]; exists {
			return BatchResult{}, ErrInvalidTarget
		}
		seen[nodeID] = struct{}{}
	}
	systemState, err := s.queries.GetSystemState(ctx)
	if err != nil {
		return BatchResult{}, err
	}
	discovered := s.discover(ctx, systemState.ReleaseChannel)
	if discovered.errorCode != nil || discovered.release == nil || discovered.release.Version != targetVersion {
		return BatchResult{}, ErrTargetUnavailable
	}

	result := BatchResult{TargetVersion: targetVersion, Items: make([]BatchItem, 0, len(nodeIDs))}
	for _, nodeID := range nodeIDs {
		item := s.createTask(ctx, nodeID, targetVersion)
		result.Items = append(result.Items, item)
	}
	return result, nil
}

func (s *Service) createTask(ctx context.Context, nodeID uuid.UUID, targetVersion string) BatchItem {
	item := BatchItem{NodeID: nodeID}
	fail := func(code string) BatchItem {
		item.Error = &code
		return item
	}
	record, err := s.queries.GetNodeByID(ctx, nodeID.String())
	if errors.Is(err, sql.ErrNoRows) {
		return fail("agent_update_node_not_found")
	}
	if err != nil {
		return fail("agent_update_target_invalid")
	}
	if record.RevokedAt != nil {
		return fail("agent_update_node_revoked")
	}
	if record.Enabled != 1 {
		return fail("agent_update_node_disabled")
	}
	now := s.now().UTC().Truncate(time.Second)
	if record.LastSeenAt == nil || now.Sub(time.Unix(*record.LastSeenAt, 0)) > nodes.OnlineWindow {
		return fail("agent_update_node_offline")
	}
	if _, err := s.queries.GetNodeCapability(ctx, configdb.GetNodeCapabilityParams{
		NodeID: nodeID.String(), Capability: AgentUpdateCapability,
	}); errors.Is(err, sql.ErrNoRows) {
		return fail("agent_update_unsupported")
	} else if err != nil {
		return fail("agent_update_target_invalid")
	}
	currentVersion, err := releaseinfo.CanonicalVersion(record.AgentVersion)
	if err != nil || !releaseinfo.SameMajor("v"+currentVersion, "v"+targetVersion) {
		return fail("agent_update_unsupported")
	}
	if semver.Compare("v"+targetVersion, "v"+currentVersion) <= 0 {
		return fail("agent_update_not_available")
	}

	id := uuid.New()
	expiresAt := now.Add(taskDeliveryWindow)
	target := targetVersion
	if err := s.queries.CreateAgentUpdateTask(ctx, configdb.CreateAgentUpdateTaskParams{
		ID: id.String(), NodeID: nodeID.String(), TargetVersion: &target,
		CreatedAt: now.Unix(), ExpiresAt: expiresAt.Unix(),
	}); err != nil {
		if strings.Contains(err.Error(), "probe_tasks_active_node_idx") || strings.Contains(err.Error(), "UNIQUE constraint failed: probe_tasks.node_id") {
			return fail("agent_update_task_slot_occupied")
		}
		return fail("agent_update_target_invalid")
	}
	task := Task{
		ID: id, NodeID: nodeID, TargetVersion: targetVersion, Status: "pending",
		CreatedAt: now, ExpiresAt: expiresAt,
	}
	item.Accepted = true
	item.Task = &task
	s.waker.Wake(nodeID.String())
	return item
}

func (s *Service) listTasks(ctx context.Context) ([]Task, error) {
	records, err := s.queries.ListLatestAgentUpdateTasks(ctx)
	if err != nil {
		return nil, err
	}
	tasks := make([]Task, 0, len(records))
	now := s.now().UTC().Truncate(time.Second)
	for _, original := range records {
		record, err := s.expirePendingTask(ctx, original, now)
		if err != nil {
			return nil, err
		}
		node, err := s.queries.GetNodeByID(ctx, record.NodeID)
		if err != nil {
			return nil, err
		}
		offline := node.LastSeenAt == nil || now.Sub(time.Unix(*node.LastSeenAt, 0)) > nodes.OnlineWindow
		task, err := taskFromRecord(record, offline && !terminalStatus(record.Status))
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func (s *Service) expirePendingTask(ctx context.Context, record configdb.ProbeTask, now time.Time) (configdb.ProbeTask, error) {
	if record.Status != "pending" || record.ExpiresAt > now.Unix() {
		return record, nil
	}
	completedAt := now.Unix()
	updated, err := s.queries.ExpireProbeTask(ctx, configdb.ExpireProbeTaskParams{
		CompletedAt: &completedAt, ID: record.ID, NodeID: record.NodeID, ExpiresAt: completedAt,
	})
	if err != nil {
		return configdb.ProbeTask{}, err
	}
	if updated == 1 {
		record.Status = "expired"
		record.CompletedAt = &completedAt
		return record, nil
	}
	return s.queries.GetProbeTask(ctx, configdb.GetProbeTaskParams{ID: record.ID, NodeID: record.NodeID})
}

func (s *Service) discover(ctx context.Context, channel string) discovery {
	now := s.now().UTC().Truncate(time.Second)
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if s.cache != nil && s.cache.channel == channel && now.Before(s.cache.expiresAt) {
		return cloneDiscovery(*s.cache)
	}
	result := s.discoverFresh(ctx, channel, now)
	result.expiresAt = now.Add(s.cacheTTL)
	s.cache = &result
	return cloneDiscovery(result)
}

func (s *Service) discoverFresh(ctx context.Context, channel string, checkedAt time.Time) discovery {
	result := discovery{channel: channel, checkedAt: checkedAt}
	current, err := releaseinfo.CanonicalVersion(s.currentVersion)
	if err != nil {
		result.errorCode = stringPointer("current-version-invalid")
		return result
	}
	releases, err := s.fetchGitHubReleases(ctx)
	if err != nil {
		result.errorCode = stringPointer("release-discovery-failed")
		return result
	}
	candidates := make([]githubRelease, 0, len(releases))
	for _, release := range releases {
		if release.Draft || release.PublishedAt.IsZero() || !semver.IsValid(release.TagName) ||
			!releaseinfo.SameMajor(release.TagName, "v"+current) {
			continue
		}
		isRC := releaseinfo.IsReleaseCandidate(release.TagName)
		isStable := semver.Prerelease(release.TagName) == ""
		if (!isRC && !isStable) || release.Prerelease != isRC || (channel == "stable" && !isStable) {
			continue
		}
		candidates = append(candidates, release)
	}
	if len(candidates) == 0 {
		return result
	}
	slices.SortFunc(candidates, func(left, right githubRelease) int {
		return semver.Compare(right.TagName, left.TagName)
	})
	selected := candidates[0]
	if semver.Compare(selected.TagName, "v"+current) < 0 {
		return result
	}
	manifest, err := s.fetchManifest(ctx, selected.TagName)
	if err != nil || manifest.Tag != selected.TagName {
		result.errorCode = stringPointer("release-discovery-failed")
		return result
	}
	result.release = &Release{
		Version: manifest.Version, Tag: manifest.Tag, Channel: manifest.Channel,
		Revision: manifest.Revision, PublishedAt: selected.PublishedAt.UTC(),
		AgentCapabilities: slices.Clone(manifest.AgentCapabilities),
	}
	return result
}

func (s *Service) fetchGitHubReleases(ctx context.Context) ([]githubRelease, error) {
	parsed, err := url.Parse(s.githubAPIURL)
	if err != nil {
		return nil, fmt.Errorf("parse GitHub releases URL: %w", err)
	}
	query := parsed.Query()
	query.Set("per_page", fmt.Sprint(maximumReleaseCount))
	parsed.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "IPChronicle/"+s.currentVersion)
	response, err := s.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub releases returned HTTP %d", response.StatusCode)
	}
	encoded, err := io.ReadAll(io.LimitReader(response.Body, maximumGitHubResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(encoded) > maximumGitHubResponseBytes {
		return nil, errors.New("GitHub releases response exceeds 2 MiB")
	}
	var releases []githubRelease
	if err := json.Unmarshal(encoded, &releases); err != nil {
		return nil, err
	}
	if len(releases) > maximumReleaseCount {
		return nil, errors.New("GitHub releases response exceeds the release limit")
	}
	return releases, nil
}

func (s *Service) fetchManifest(ctx context.Context, tag string) (releaseinfo.Manifest, error) {
	manifestURL := s.releaseDownloadURL + "/" + url.PathEscape(tag) + "/" + releaseinfo.ManifestAssetName
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
	if err != nil {
		return releaseinfo.Manifest{}, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "IPChronicle/"+s.currentVersion)
	response, err := s.httpClient.Do(request)
	if err != nil {
		return releaseinfo.Manifest{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return releaseinfo.Manifest{}, fmt.Errorf("release manifest returned HTTP %d", response.StatusCode)
	}
	return releaseinfo.ParseManifest(response.Body)
}

func taskFromRecord(record configdb.ProbeTask, offline bool) (Task, error) {
	if record.Kind != "agent-update" || record.TargetVersion == nil {
		return Task{}, errors.New("stored Agent update task is invalid")
	}
	id, err := uuid.Parse(record.ID)
	if err != nil {
		return Task{}, err
	}
	nodeID, err := uuid.Parse(record.NodeID)
	if err != nil {
		return Task{}, err
	}
	return Task{
		ID: id, NodeID: nodeID, TargetVersion: *record.TargetVersion, Status: record.Status,
		CreatedAt: time.Unix(record.CreatedAt, 0).UTC(), ExpiresAt: time.Unix(record.ExpiresAt, 0).UTC(),
		AcknowledgedAt: timePointer(record.AcknowledgedAt), StartedAt: timePointer(record.StartedAt),
		CompletedAt: timePointer(record.CompletedAt), PreviousVersion: record.PreviousVersion,
		ResultVersion: record.ResultVersion, FailureCode: record.FailureCode, Diagnostic: record.Diagnostic,
		Offline: offline,
	}, nil
}

func terminalStatus(status string) bool {
	switch status {
	case "succeeded", "partial", "failed", "rolled-back", "rejected", "expired":
		return true
	default:
		return false
	}
}

func timePointer(value *int64) *time.Time {
	if value == nil {
		return nil
	}
	result := time.Unix(*value, 0).UTC()
	return &result
}

func cloneDiscovery(value discovery) discovery {
	value.release = cloneRelease(value.release)
	value.errorCode = cloneString(value.errorCode)
	return value
}

func cloneRelease(value *Release) *Release {
	if value == nil {
		return nil
	}
	result := *value
	result.AgentCapabilities = slices.Clone(value.AgentCapabilities)
	return &result
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func stringPointer(value string) *string {
	return &value
}
