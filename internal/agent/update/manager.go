package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/ipchronicle/ipchronicle/internal/agent/state"
	"github.com/ipchronicle/ipchronicle/internal/releaseinfo"
	"golang.org/x/mod/semver"
)

const (
	defaultReleaseDownloadURL = "https://github.com/ipchronicle/ipchronicle/releases/download"
	maximumAgentArtifactBytes = 128 * 1024 * 1024
	maximumCommandOutputBytes = 64 * 1024
)

type Config struct {
	InitSystem  string
	AgentPath   string
	UpdaterPath string
}

func (config Config) Validate() error {
	if config.InitSystem != "systemd" && config.InitSystem != "openrc" {
		return errors.New("Agent updater init system must be systemd or openrc")
	}
	if !filepath.IsAbs(config.AgentPath) || !filepath.IsAbs(config.UpdaterPath) || config.AgentPath == config.UpdaterPath {
		return errors.New("Agent and updater paths must be distinct absolute paths")
	}
	return nil
}

type Trigger func(context.Context, string) error

type ManagerOptions struct {
	Store              *state.Store
	CurrentVersion     string
	Config             Config
	HTTPClient         *http.Client
	ReleaseDownloadURL string
	Trigger            Trigger
	Now                func() time.Time
	Logger             *log.Logger
}

type Manager struct {
	store              *state.Store
	currentVersion     string
	config             Config
	httpClient         *http.Client
	releaseDownloadURL string
	trigger            Trigger
	now                func() time.Time
	logger             *log.Logger
	wake               chan struct{}
}

func NewManager(options ManagerOptions) (*Manager, error) {
	if options.Store == nil {
		return nil, errors.New("Agent update manager store must not be nil")
	}
	if err := options.Config.Validate(); err != nil {
		return nil, err
	}
	if _, err := releaseinfo.CanonicalVersion(options.CurrentVersion); err != nil {
		return nil, fmt.Errorf("running Agent version is not updateable: %w", err)
	}
	client := options.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	downloadURL := strings.TrimRight(strings.TrimSpace(options.ReleaseDownloadURL), "/")
	if downloadURL == "" {
		downloadURL = defaultReleaseDownloadURL
	}
	trigger := options.Trigger
	if trigger == nil {
		trigger = triggerSupervisor
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	logger := options.Logger
	if logger == nil {
		logger = log.Default()
	}
	return &Manager{
		store: options.Store, currentVersion: options.CurrentVersion, config: options.Config,
		httpClient: client, releaseDownloadURL: downloadURL, trigger: trigger,
		now: now, logger: logger, wake: make(chan struct{}, 1),
	}, nil
}

func (manager *Manager) AcceptTask(delivery state.AgentUpdateDelivery) error {
	report, err := manager.store.AcceptAgentUpdate(delivery, manager.currentVersion, manager.now())
	if err != nil && !errors.Is(err, state.ErrAgentUpdateHandled) {
		return err
	}
	if report.Status != "acknowledged" {
		return nil
	}
	select {
	case manager.wake <- struct{}{}:
	default:
	}
	return nil
}

func (manager *Manager) StartSupervisor(ctx context.Context) error {
	return manager.trigger(ctx, manager.config.InitSystem)
}

func (manager *Manager) Run(ctx context.Context) error {
	if err := manager.store.CleanupAgentUpdates(manager.now()); err != nil {
		return err
	}
	if update, found, err := manager.store.PendingAgentUpdate(); err != nil {
		return err
	} else if found && (update.Status == "acknowledged" || update.Status == "verifying" || update.Status == "installing") {
		if err := manager.process(ctx, update); err != nil {
			return err
		}
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-manager.wake:
			if err := manager.store.CleanupAgentUpdates(manager.now()); err != nil {
				return err
			}
			update, found, err := manager.store.PendingAgentUpdate()
			if err != nil {
				return err
			}
			if !found || (update.Status != "acknowledged" && update.Status != "verifying" && update.Status != "installing") {
				continue
			}
			if err := manager.process(ctx, update); err != nil {
				return err
			}
		}
	}
}

func (manager *Manager) process(ctx context.Context, update state.AgentUpdate) error {
	if update.Status == "installing" {
		if err := manager.trigger(ctx, manager.config.InitSystem); err != nil {
			return manager.recordFailure(update.ID, "supervisor-start", err)
		}
		return nil
	}
	if update.Status == "acknowledged" {
		var err error
		update, err = manager.store.BeginAgentUpdate(update.ID, manager.currentVersion, manager.now())
		if err != nil {
			return err
		}
	}
	if update.Status != "verifying" {
		return state.ErrInvalidAgentUpdate
	}
	if err := manager.stage(ctx, update); err != nil {
		return manager.recordFailure(update.ID, classifyStageFailure(err), err)
	}
	if _, err := manager.store.MarkAgentUpdateInstalling(update.ID); err != nil {
		return err
	}
	if err := manager.trigger(ctx, manager.config.InitSystem); err != nil {
		return manager.recordFailure(update.ID, "supervisor-start", err)
	}
	return nil
}

func (manager *Manager) stage(ctx context.Context, update state.AgentUpdate) error {
	if !releaseinfo.SameMajor("v"+manager.currentVersion, "v"+update.TargetVersion) ||
		semver.Compare("v"+update.TargetVersion, "v"+manager.currentVersion) <= 0 {
		return stageError{code: "target-version", cause: errors.New("target version is not a newer same-major release")}
	}
	manifest, err := manager.fetchManifest(ctx, update.TargetVersion)
	if err != nil {
		return stageError{code: "manifest-download", cause: err}
	}
	if manifest.Version != update.TargetVersion {
		return stageError{code: "manifest-invalid", cause: errors.New("release manifest target does not match the task")}
	}
	artifact, ok := manifest.AgentArtifact(runtime.GOARCH)
	if !ok || artifact.OS != runtime.GOOS || artifact.Size > maximumAgentArtifactBytes {
		return stageError{code: "artifact-platform", cause: errors.New("release has no supported artifact for this Agent")}
	}
	updateDirectory, err := ensureUpdateDirectory(manager.store.Directory())
	if err != nil {
		return stageError{code: "local-storage", cause: err}
	}
	stagedPath := StagedBinaryPath(manager.store.Directory(), update.ID)
	if err := manager.downloadArtifact(ctx, manifest.Tag, artifact, updateDirectory, stagedPath); err != nil {
		return err
	}
	info, err := inspectAgentBinary(ctx, stagedPath)
	if err != nil {
		_ = os.Remove(stagedPath)
		return stageError{code: "binary-metadata", cause: err}
	}
	if err := info.ValidateAgent(manifest.Version, manifest.Revision, runtime.GOARCH); err != nil {
		_ = os.Remove(stagedPath)
		return stageError{code: "binary-metadata", cause: err}
	}
	if info.StateSchemaVersion != state.SchemaVersion() {
		_ = os.Remove(stagedPath)
		return stageError{code: "binary-metadata", cause: errors.New("Agent update requires an incompatible local state schema")}
	}
	return nil
}

func (manager *Manager) fetchManifest(ctx context.Context, targetVersion string) (releaseinfo.Manifest, error) {
	tag := "v" + targetVersion
	manifestURL := manager.releaseDownloadURL + "/" + url.PathEscape(tag) + "/" + releaseinfo.ManifestAssetName
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
	if err != nil {
		return releaseinfo.Manifest{}, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "IPChronicle-Agent/"+manager.currentVersion)
	response, err := manager.httpClient.Do(request)
	if err != nil {
		return releaseinfo.Manifest{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return releaseinfo.Manifest{}, fmt.Errorf("release manifest returned HTTP %d", response.StatusCode)
	}
	return releaseinfo.ParseManifest(response.Body)
}

func (manager *Manager) downloadArtifact(
	ctx context.Context,
	tag string,
	artifact releaseinfo.Artifact,
	updateDirectory string,
	stagedPath string,
) error {
	artifactURL := manager.releaseDownloadURL + "/" + url.PathEscape(tag) + "/" + url.PathEscape(artifact.Name)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, artifactURL, nil)
	if err != nil {
		return stageError{code: "artifact-download", cause: err}
	}
	request.Header.Set("Accept", "application/octet-stream")
	request.Header.Set("User-Agent", "IPChronicle-Agent/"+manager.currentVersion)
	response, err := manager.httpClient.Do(request)
	if err != nil {
		return stageError{code: "artifact-download", cause: err}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return stageError{code: "artifact-download", cause: fmt.Errorf("Agent artifact returned HTTP %d", response.StatusCode)}
	}
	temporary, err := os.CreateTemp(updateDirectory, ".staged-agent-*")
	if err != nil {
		return stageError{code: "local-storage", cause: err}
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(temporary, hash), io.LimitReader(response.Body, artifact.Size+1))
	if copyErr != nil {
		return stageError{code: "artifact-download", cause: copyErr}
	}
	if written != artifact.Size {
		return stageError{code: "artifact-length", cause: fmt.Errorf("Agent artifact length is %d, expected %d", written, artifact.Size)}
	}
	if actual := hex.EncodeToString(hash.Sum(nil)); actual != artifact.SHA256 {
		return stageError{code: "artifact-checksum", cause: errors.New("Agent artifact SHA-256 does not match its release manifest")}
	}
	if err := temporary.Chmod(0o755); err != nil {
		return stageError{code: "local-storage", cause: err}
	}
	if err := temporary.Sync(); err != nil {
		return stageError{code: "local-storage", cause: err}
	}
	if err := temporary.Close(); err != nil {
		return stageError{code: "local-storage", cause: err}
	}
	if err := os.Rename(temporaryPath, stagedPath); err != nil {
		return stageError{code: "local-storage", cause: err}
	}
	if err := syncDirectory(updateDirectory); err != nil {
		return stageError{code: "local-storage", cause: err}
	}
	return nil
}

func (manager *Manager) recordFailure(id, code string, cause error) error {
	diagnostic := boundedDiagnostic(cause)
	if _, err := manager.store.FailAgentUpdate(id, code, diagnostic, manager.now()); err != nil {
		return errors.Join(cause, err)
	}
	manager.logger.Printf("Agent update %s failed during %s: %s", id, code, diagnostic)
	return nil
}

type stageError struct {
	code  string
	cause error
}

func (err stageError) Error() string { return err.cause.Error() }
func (err stageError) Unwrap() error { return err.cause }

func classifyStageFailure(err error) string {
	var typed stageError
	if errors.As(err, &typed) {
		return typed.code
	}
	return "update-failed"
}

func triggerSupervisor(ctx context.Context, initSystem string) error {
	var command *exec.Cmd
	if initSystem == "systemd" {
		command = exec.CommandContext(ctx, "systemctl", "start", "--no-block", "ipchronicle-agent-updater.service")
	} else {
		command = exec.CommandContext(ctx, "rc-service", "ipchronicle-agent-updater", "start")
	}
	output := &boundedBuffer{limit: 4096}
	command.Stdout = output
	command.Stderr = output
	if err := command.Run(); err != nil {
		return fmt.Errorf("start Agent update supervisor: %w: %s", err, strings.TrimSpace(output.String()))
	}
	return nil
}

func inspectAgentBinary(ctx context.Context, path string) (releaseinfo.BinaryInfo, error) {
	commandContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	command := exec.CommandContext(commandContext, path, "version", "--json")
	stdout := &boundedBuffer{limit: maximumCommandOutputBytes}
	stderr := &boundedBuffer{limit: maximumCommandOutputBytes}
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		return releaseinfo.BinaryInfo{}, fmt.Errorf("read candidate metadata: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if stdout.exceeded || stderr.exceeded {
		return releaseinfo.BinaryInfo{}, errors.New("candidate metadata output exceeds 64 KiB")
	}
	if strings.TrimSpace(stderr.String()) != "" {
		return releaseinfo.BinaryInfo{}, errors.New("candidate metadata wrote unexpected diagnostics")
	}
	return releaseinfo.ParseBinaryInfo(strings.NewReader(stdout.String()))
}

type boundedBuffer struct {
	mu       sync.Mutex
	builder  strings.Builder
	limit    int
	exceeded bool
}

func (buffer *boundedBuffer) Write(value []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	remaining := buffer.limit - buffer.builder.Len()
	if remaining < len(value) {
		buffer.exceeded = true
		if remaining > 0 {
			_, _ = buffer.builder.Write(value[:remaining])
		}
		return len(value), nil
	}
	_, _ = buffer.builder.Write(value)
	return len(value), nil
}

func (buffer *boundedBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.builder.String()
}

func ensureUpdateDirectory(stateDirectory string) (string, error) {
	directory := filepath.Join(stateDirectory, "update")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", err
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return "", err
	}
	return directory, nil
}

func StagedBinaryPath(stateDirectory, taskID string) string {
	return filepath.Join(stateDirectory, "update", "staged-"+taskID)
}

func HealthMarkerPath(stateDirectory, taskID string) string {
	return filepath.Join(stateDirectory, "update", "healthy-"+taskID)
}

func boundedDiagnostic(err error) string {
	value := strings.ReplaceAll(err.Error(), "\x00", "")
	runes := []rune(value)
	if len(runes) > 4096 {
		runes = runes[:4096]
	}
	if len(runes) == 0 {
		return "Agent update failed without diagnostics"
	}
	return string(runes)
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
