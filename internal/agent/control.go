package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"maps"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/agent/agentlogs"
	agentnetwork "github.com/ipchronicle/ipchronicle/internal/agent/network"
	"github.com/ipchronicle/ipchronicle/internal/agent/observation"
	agentprobe "github.com/ipchronicle/ipchronicle/internal/agent/probe"
	"github.com/ipchronicle/ipchronicle/internal/agent/state"
	agentupdate "github.com/ipchronicle/ipchronicle/internal/agent/update"
	"github.com/ipchronicle/ipchronicle/internal/generated/agentapi"
	"github.com/ipchronicle/ipchronicle/internal/releaseinfo"
	productversion "github.com/ipchronicle/ipchronicle/internal/version"
)

const controlCapability = "control-v1"

const maxAgentAPIResponseSize = 512 * 1024

var ErrAgentRevoked = errors.New("Agent identity is revoked")

type ControlClient struct {
	client *agentapi.ClientWithResponses
}

type pollOutcome struct {
	interval    time.Duration
	applied     bool
	received    bool
	syncSession *agentapi.AgentSyncSession
	probeTask   *state.ProbeTaskDelivery
	updateTask  *state.AgentUpdateDelivery
}

func NewControlClient(centerURL string) (*ControlClient, error) {
	normalized, err := NormalizeCenterURL(centerURL)
	if err != nil {
		return nil, err
	}
	httpClient := &http.Client{
		Transport: boundedResponseTransport{base: http.DefaultTransport, maxBytes: maxAgentAPIResponseSize},
		Timeout:   15 * time.Second,
	}
	client, err := agentapi.NewClientWithResponses(normalized, agentapi.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("create Agent API client: %w", err)
	}
	return &ControlClient{client: client}, nil
}

func NormalizeCenterURL(value string) (string, error) {
	parsed, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("parse center URL: %w", err)
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil ||
		(parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("center URL must be an HTTP or HTTPS origin without credentials, path, query, or fragment")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Path = ""
	return strings.TrimSuffix(parsed.String(), "/"), nil
}

func Enroll(ctx context.Context, store *state.Store, centerURL, registrationKey, version string) (state.Identity, error) {
	return EnrollWithCapabilities(ctx, store, centerURL, registrationKey, version, false)
}

func EnrollWithCapabilities(ctx context.Context, store *state.Store, centerURL, registrationKey, version string, updateCapable bool) (state.Identity, error) {
	return enrollWithCredential(ctx, store, centerURL, registrationKey, "registration", version, updateCapable)
}

func RecoverWithCapabilities(ctx context.Context, store *state.Store, centerURL, recoveryKey, version string, updateCapable bool) (state.Identity, error) {
	return enrollWithCredential(ctx, store, centerURL, recoveryKey, "recovery", version, updateCapable)
}

func enrollWithCredential(ctx context.Context, store *state.Store, centerURL, key, mode, version string, updateCapable bool) (state.Identity, error) {
	normalized, err := NormalizeCenterURL(centerURL)
	if err != nil {
		return state.Identity{}, err
	}
	existing, err := store.Identity()
	if err == nil {
		if existing.CenterURL != normalized {
			return state.Identity{}, fmt.Errorf("Agent is already enrolled with %s", existing.CenterURL)
		}
		return existing, nil
	}
	if !errors.Is(err, state.ErrNotEnrolled) {
		return state.Identity{}, err
	}
	if key == "" || (mode != "registration" && mode != "recovery") {
		return state.Identity{}, errors.New("registration or recovery key must not be empty")
	}
	client, err := NewControlClient(normalized)
	if err != nil {
		return state.Identity{}, err
	}
	metadata, err := currentMetadata(version, updateCapable)
	if err != nil {
		return state.Identity{}, err
	}
	var nodeID uuid.UUID
	var credential string
	if mode == "registration" {
		response, err := client.client.RegisterAgentWithResponse(ctx, agentapi.AgentRegistrationRequest{
			RegistrationKey: key, Metadata: metadata,
		})
		if err != nil {
			return state.Identity{}, fmt.Errorf("register Agent: %w", err)
		}
		if response.JSON201 == nil {
			return state.Identity{}, responseError("register Agent", response.StatusCode(), response.JSON400, response.JSON401, response.JSON403)
		}
		nodeID = response.JSON201.NodeId
		credential = response.JSON201.Credential
	} else {
		response, err := client.client.RecoverAgentWithResponse(ctx, agentapi.AgentRecoveryRequest{
			RecoveryKey: key, Metadata: metadata,
		})
		if err != nil {
			return state.Identity{}, fmt.Errorf("recover Agent: %w", err)
		}
		if response.JSON200 == nil {
			return state.Identity{}, responseError("recover Agent", response.StatusCode(), response.JSON400, response.JSON401, nil)
		}
		nodeID = response.JSON200.NodeId
		credential = response.JSON200.Credential
	}
	identity := state.Identity{
		CenterURL: normalized, NodeID: nodeID.String(),
		Credential: credential, AppliedConfigurationRevision: 0,
	}
	if err := store.SaveIdentity(identity); err != nil {
		return state.Identity{}, fmt.Errorf("persist Agent identity: %w", err)
	}
	return identity, nil
}

type RunOptions struct {
	UpdateConfig             *agentupdate.Config
	UpdateHTTPClient         *http.Client
	UpdateReleaseDownloadURL string
	UpdateTrigger            agentupdate.Trigger
	UpdateNow                func() time.Time
}

func Run(ctx context.Context, store *state.Store, version string, logger *log.Logger) error {
	return RunWithOptions(ctx, store, version, logger, RunOptions{})
}

func RunWithOptions(ctx context.Context, store *state.Store, version string, logger *log.Logger, options RunOptions) error {
	if logger == nil {
		logger = log.Default()
	}
	identity, err := store.Identity()
	if err != nil {
		return err
	}
	client, err := NewControlClient(identity.CenterURL)
	if err != nil {
		return err
	}
	logRecorder := agentlogs.NewRecorder(store, logger)
	if configuration, configurationErr := store.Configuration(); configurationErr == nil && configuration.LogLevel != "" {
		if err := logRecorder.SetLevel(configuration.LogLevel); err != nil {
			return err
		}
	}
	var updateManager *agentupdate.Manager
	if options.UpdateConfig != nil {
		if err := options.UpdateConfig.Validate(); err != nil {
			return err
		}
		if _, versionErr := releaseinfo.CanonicalVersion(version); versionErr != nil {
			logger.Printf("Agent updates disabled for non-release version %q: %v", version, versionErr)
		} else {
			updateManager, err = agentupdate.NewManager(agentupdate.ManagerOptions{
				Store: store, CurrentVersion: version, Config: *options.UpdateConfig, Logger: logger, Events: logRecorder,
				HTTPClient: options.UpdateHTTPClient, ReleaseDownloadURL: options.UpdateReleaseDownloadURL,
				Trigger: options.UpdateTrigger, Now: options.UpdateNow,
			})
			if err != nil {
				return err
			}
		}
	}
	metadata, err := currentMetadata(version, updateManager != nil)
	if err != nil {
		return err
	}
	interval := 30 * time.Second
	syncManager := newSyncManager(ctx, identity.CenterURL, identity.Credential, logger)
	defer syncManager.Close()
	pendingUpdate, hasPendingUpdate, err := store.PendingAgentUpdate()
	if err != nil {
		return err
	}
	requiresHealthCommit := hasPendingUpdate && pendingUpdate.Status == "restarting"
	if requiresHealthCommit {
		hasCheckpoint, checkpointErr := agentupdate.HasCheckpoint(store.Directory(), pendingUpdate.ID)
		if checkpointErr != nil {
			return checkpointErr
		}
		if !hasCheckpoint {
			return errors.New("Agent update checkpoint is missing while the new Agent is restarting")
		}
	}
	if hasPendingUpdate && pendingUpdate.Status == "succeeded" {
		hasCheckpoint, checkpointErr := agentupdate.HasCheckpoint(store.Directory(), pendingUpdate.ID)
		if checkpointErr != nil {
			return checkpointErr
		}
		requiresHealthCommit = hasCheckpoint
	}
	if requiresHealthCommit {
		if updateManager == nil {
			return errors.New("Agent update health commitment requires an installed update supervisor")
		}
		if pendingUpdate.TargetVersion != version || pendingUpdate.Status == "succeeded" &&
			(pendingUpdate.ResultVersion == nil || *pendingUpdate.ResultVersion != version) {
			return errors.New("running Agent version does not match its pending update")
		}
		outcome, healthErr := waitForUpdateHealth(ctx, client, store, identity, metadata, pendingUpdate, syncManager, logger)
		if healthErr != nil {
			return healthErr
		}
		if outcome.interval >= 5*time.Second && outcome.interval <= time.Hour {
			interval = outcome.interval
		}
		if pendingUpdate.Status == "restarting" {
			if _, err := store.CommitAgentUpdateHealth(pendingUpdate.ID, version, time.Now().UTC()); err != nil {
				return fmt.Errorf("commit Agent update health: %w", err)
			}
		}
		if err := agentupdate.WriteHealthMarker(store.Directory(), pendingUpdate.ID); err != nil {
			return fmt.Errorf("publish Agent update health: %w", err)
		}
		if err := updateManager.StartSupervisor(ctx); err != nil {
			return fmt.Errorf("resume Agent update supervisor: %w", err)
		}
	}
	workerContext, stopWorkers := context.WithCancel(ctx)
	var workers sync.WaitGroup
	defer func() {
		stopWorkers()
		workers.Wait()
	}()
	logRecorderDone := make(chan error, 1)
	workers.Add(1)
	go func() {
		defer workers.Done()
		logRecorderDone <- logRecorder.Run(workerContext)
	}()
	logUploaderDone := make(chan error, 1)
	workers.Add(1)
	go func() {
		defer workers.Done()
		logUploaderDone <- client.runLogUploader(workerContext, store, identity, logRecorder)
	}()
	probeManager := agentprobe.NewManager(store, metadata.PhysicalMemoryBytes, logger, logRecorder)
	probeDone := make(chan error, 1)
	workers.Add(1)
	go func() {
		defer workers.Done()
		probeDone <- probeManager.Run(workerContext)
	}()
	observerDone := make(chan error, 1)
	workers.Add(1)
	go func() {
		defer workers.Done()
		observer := observation.NewObserver(store, logger, logRecorder)
		observerDone <- observer.Run(workerContext)
	}()
	uploaderDone := make(chan error, 1)
	workers.Add(1)
	go func() {
		defer workers.Done()
		uploaderDone <- client.runProbeUploader(workerContext, store, identity, probeManager.UploadWake(), logger, logRecorder)
	}()
	var updateDone <-chan error
	if updateManager != nil {
		done := make(chan error, 1)
		updateDone = done
		workers.Add(1)
		go func() {
			defer workers.Done()
			done <- updateManager.Run(workerContext)
		}()
	}
	logger.Printf("Agent %s polling %s", identity.NodeID, identity.CenterURL)
	logRecorder.Emit(agentlogs.Event{
		Level: "info", Component: "agent", EventType: "agent-started",
		Message: "Agent control loop started",
	})
	for {
		select {
		case recorderErr := <-logRecorderDone:
			if recorderErr != nil {
				return fmt.Errorf("run Agent log recorder: %w", recorderErr)
			}
			return nil
		case logUploadErr := <-logUploaderDone:
			if errors.Is(logUploadErr, ErrAgentRevoked) {
				if markErr := store.MarkRevoked(); markErr != nil {
					return errors.Join(logUploadErr, markErr)
				}
				return nil
			}
			if logUploadErr != nil {
				return fmt.Errorf("run Agent log uploader: %w", logUploadErr)
			}
			return nil
		case probeErr := <-probeDone:
			if probeErr != nil {
				return fmt.Errorf("run complete-probe manager: %w", probeErr)
			}
			return nil
		case observerErr := <-observerDone:
			if observerErr != nil {
				return fmt.Errorf("run lightweight address observer: %w", observerErr)
			}
			return nil
		case uploaderErr := <-uploaderDone:
			if errors.Is(uploaderErr, ErrAgentRevoked) {
				if markErr := store.MarkRevoked(); markErr != nil {
					return errors.Join(uploaderErr, markErr)
				}
				return nil
			}
			if uploaderErr != nil {
				return fmt.Errorf("run complete-probe uploader: %w", uploaderErr)
			}
			return nil
		case updateErr := <-updateDone:
			if updateErr != nil {
				return fmt.Errorf("run Agent update manager: %w", updateErr)
			}
			return nil
		default:
		}
		controlState, err := store.ControlState()
		if err != nil {
			return err
		}
		if controlState.Revoked {
			logger.Printf("Agent %s is revoked; control polling has stopped", identity.NodeID)
			return nil
		}
		inventory, inventoryError := captureNetworkInventory()
		outcome, err := client.poll(ctx, store, identity, metadata, controlState, inventory, inventoryError, logRecorder)
		if errors.Is(err, ErrAgentRevoked) {
			if markErr := store.MarkRevoked(); markErr != nil {
				return errors.Join(err, markErr)
			}
			logger.Printf("Agent %s was revoked by the center; control polling has stopped", identity.NodeID)
			return nil
		}
		if outcome.received {
			syncManager.Update(outcome.syncSession)
		}
		if err == nil && outcome.probeTask != nil {
			if taskErr := probeManager.AcceptTask(workerContext, *outcome.probeTask); taskErr != nil {
				return fmt.Errorf("accept complete-probe task: %w", taskErr)
			}
		}
		if err == nil && outcome.updateTask != nil {
			if updateManager == nil {
				return errors.New("Agent update task received without an installed update supervisor")
			}
			if taskErr := updateManager.AcceptTask(*outcome.updateTask); taskErr != nil {
				return fmt.Errorf("accept Agent update task: %w", taskErr)
			}
		}
		if outcome.interval >= 5*time.Second && outcome.interval <= time.Hour {
			interval = outcome.interval
		}
		if outcome.applied {
			configuration, configurationErr := store.Configuration()
			if configurationErr != nil {
				return configurationErr
			}
			level := configuration.LogLevel
			if level == "" {
				level = "info"
			}
			if err := logRecorder.SetLevel(level); err != nil {
				return err
			}
			logRecorder.Emit(agentlogs.Event{
				Level: "info", Component: "configuration", EventType: "configuration-applied",
				Message: "Agent configuration applied", ConfigurationRevision: &configuration.Revision,
			})
			continue
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil
		case <-timer.C:
		case <-syncManager.Wake():
			if !timer.Stop() {
				<-timer.C
			}
		case <-probeManager.Wake():
			if !timer.Stop() {
				<-timer.C
			}
		case probeErr := <-probeDone:
			if !timer.Stop() {
				<-timer.C
			}
			if probeErr != nil {
				return fmt.Errorf("run complete-probe manager: %w", probeErr)
			}
			return nil
		case observerErr := <-observerDone:
			if !timer.Stop() {
				<-timer.C
			}
			if observerErr != nil {
				return fmt.Errorf("run lightweight address observer: %w", observerErr)
			}
			return nil
		case uploaderErr := <-uploaderDone:
			if !timer.Stop() {
				<-timer.C
			}
			if errors.Is(uploaderErr, ErrAgentRevoked) {
				if markErr := store.MarkRevoked(); markErr != nil {
					return errors.Join(uploaderErr, markErr)
				}
				return nil
			}
			if uploaderErr != nil {
				return fmt.Errorf("run complete-probe uploader: %w", uploaderErr)
			}
			return nil
		case updateErr := <-updateDone:
			if !timer.Stop() {
				<-timer.C
			}
			if updateErr != nil {
				return fmt.Errorf("run Agent update manager: %w", updateErr)
			}
			return nil
		case recorderErr := <-logRecorderDone:
			if !timer.Stop() {
				<-timer.C
			}
			if recorderErr != nil {
				return fmt.Errorf("run Agent log recorder: %w", recorderErr)
			}
			return nil
		case logUploadErr := <-logUploaderDone:
			if !timer.Stop() {
				<-timer.C
			}
			if errors.Is(logUploadErr, ErrAgentRevoked) {
				if markErr := store.MarkRevoked(); markErr != nil {
					return errors.Join(logUploadErr, markErr)
				}
				return nil
			}
			if logUploadErr != nil {
				return fmt.Errorf("run Agent log uploader: %w", logUploadErr)
			}
			return nil
		}
	}
}

func waitForUpdateHealth(
	ctx context.Context,
	client *ControlClient,
	store *state.Store,
	identity state.Identity,
	metadata agentapi.AgentMetadata,
	pending state.AgentUpdate,
	syncManager *syncManager,
	logger *log.Logger,
) (pollOutcome, error) {
	const retryInterval = 5 * time.Second
	for {
		controlState, err := store.ControlState()
		if err != nil {
			return pollOutcome{}, err
		}
		inventory, inventoryError := captureNetworkInventory()
		outcome, pollErr := client.poll(ctx, store, identity, metadata, controlState, inventory, inventoryError, nil)
		if outcome.received {
			syncManager.Update(outcome.syncSession)
		}
		if pollErr == nil {
			if outcome.probeTask != nil {
				return pollOutcome{}, errors.New("center delivered a complete-probe task while an Agent update was restarting")
			}
			if outcome.updateTask != nil && (outcome.updateTask.ID != pending.ID || outcome.updateTask.TargetVersion != pending.TargetVersion) {
				return pollOutcome{}, errors.New("center delivered a different task while an Agent update was restarting")
			}
			return outcome, nil
		}
		if errors.Is(pollErr, ErrAgentRevoked) {
			return pollOutcome{}, pollErr
		}
		if !errors.Is(pollErr, context.Canceled) {
			logger.Printf("Agent update health poll failed: %v", pollErr)
		}
		timer := time.NewTimer(retryInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return pollOutcome{}, ctx.Err()
		case <-timer.C:
		}
	}
}

func (c *ControlClient) poll(
	ctx context.Context,
	store *state.Store,
	identity state.Identity,
	metadata agentapi.AgentMetadata,
	controlState state.ControlState,
	inventory *agentapi.NetworkInventory,
	inventoryError *string,
	events agentlogs.Sink,
) (pollOutcome, error) {
	upload, err := store.AddressUpload(64)
	if err != nil {
		return pollOutcome{}, err
	}
	addressStates, addressEvents, addressGaps, err := addressUploadToAPI(upload)
	if err != nil {
		return pollOutcome{}, err
	}
	probeStatus, probeTaskReport, err := store.ProbeControlReport()
	if err != nil {
		return pollOutcome{}, err
	}
	probeStatusAPI, err := probeStatusToAPI(probeStatus)
	if err != nil {
		return pollOutcome{}, err
	}
	updateTaskReport, err := store.AgentUpdateControlReport()
	if err != nil {
		return pollOutcome{}, err
	}
	if probeTaskReport != nil && updateTaskReport != nil {
		return pollOutcome{}, errors.New("multiple unconfirmed Agent task reports")
	}
	taskReportAPI, err := taskReportToAPI(probeTaskReport)
	if err != nil {
		return pollOutcome{}, err
	}
	if updateTaskReport != nil {
		taskReportAPI, err = agentUpdateReportToAPI(updateTaskReport)
		if err != nil {
			return pollOutcome{}, err
		}
	}
	startedAt := time.Now()
	response, err := c.client.PollAgentWithResponse(ctx, agentapi.AgentPollRequest{
		AppliedConfigurationRevision: controlState.AppliedConfigurationRevision,
		ConfigurationError:           controlState.ConfigurationError,
		ConfigurationErrorRevision:   controlState.ConfigurationErrorRevision,
		Metadata:                     metadata,
		NetworkInventory:             inventory,
		NetworkInventoryError:        inventoryError,
		AddressStates:                &addressStates,
		AddressEvents:                &addressEvents,
		AddressGaps:                  &addressGaps,
		ProbeStatus:                  probeStatusAPI,
		TaskReport:                   taskReportAPI,
	}, func(_ context.Context, request *http.Request) error {
		request.Header.Set("Authorization", "Bearer "+identity.Credential)
		return nil
	})
	duration := time.Since(startedAt).Milliseconds()
	target := identity.CenterURL + "/api/v1/agent/control"
	if err != nil {
		emitAgentRequestFailure(events, "control", "poll-failed", "Center control poll failed", http.MethodPost, target, 0, "", nil, duration, err)
		return pollOutcome{}, err
	}
	if response.JSON200 == nil {
		emitAgentRequestFailure(events, "control", "poll-rejected", "Center control poll was rejected", http.MethodPost,
			target, response.StatusCode(), response.ContentType(), response.Body, duration, nil)
		return pollOutcome{}, responseError("poll center", response.StatusCode(), response.JSON400, response.JSON401, response.JSON403)
	}
	emitAgentRequestSuccess(events, "control", "poll-succeeded", "Center control poll succeeded", http.MethodPost, target,
		response.StatusCode(), duration)
	if err := store.AcknowledgeAddressUpload(addressReceiptFromAPI(response.JSON200.AddressUploadReceipt)); err != nil {
		return pollOutcome{}, fmt.Errorf("acknowledge address upload: %w", err)
	}
	if response.JSON200.AcceptedTerminalTaskId != nil {
		acceptedID := response.JSON200.AcceptedTerminalTaskId.String()
		found, err := store.ConfirmTerminalAgentUpdate(acceptedID, time.Now().UTC())
		if err != nil {
			return pollOutcome{}, fmt.Errorf("confirm terminal Agent update: %w", err)
		}
		if !found {
			if err := store.ConfirmTerminalProbeTask(acceptedID, time.Now().UTC()); err != nil {
				return pollOutcome{}, fmt.Errorf("confirm terminal probe task: %w", err)
			}
		}
	}
	outcome := pollOutcome{
		interval: time.Duration(response.JSON200.PollIntervalSeconds) * time.Second,
		received: true, syncSession: response.JSON200.SyncSession,
	}
	if response.JSON200.Task != nil {
		switch response.JSON200.Task.Kind {
		case agentapi.AgentTaskKindCompleteProbe:
			if response.JSON200.Task.TargetVersion != nil {
				return pollOutcome{}, errors.New("complete-probe task contains an update target")
			}
			outcome.probeTask = &state.ProbeTaskDelivery{
				ID: response.JSON200.Task.Id.String(), Trigger: string(response.JSON200.Task.Trigger),
				CreatedAt: response.JSON200.Task.CreatedAt,
				ExpiresAt: response.JSON200.Task.ExpiresAt,
			}
			if response.JSON200.Task.PublicAddressIds == nil {
				return pollOutcome{}, errors.New("complete-probe task omits its public-address targets")
			}
			outcome.probeTask.PublicAddressIDs = make([]string, 0, len(*response.JSON200.Task.PublicAddressIds))
			for _, id := range *response.JSON200.Task.PublicAddressIds {
				outcome.probeTask.PublicAddressIDs = append(outcome.probeTask.PublicAddressIDs, id.String())
			}
		case agentapi.AgentTaskKindAgentUpdate:
			if response.JSON200.Task.TargetVersion == nil {
				return pollOutcome{}, errors.New("Agent update task omits its target version")
			}
			outcome.updateTask = &state.AgentUpdateDelivery{
				ID: response.JSON200.Task.Id.String(), TargetVersion: *response.JSON200.Task.TargetVersion,
				CreatedAt: response.JSON200.Task.CreatedAt, ExpiresAt: response.JSON200.Task.ExpiresAt,
			}
		default:
			return pollOutcome{}, errors.New("center returned an unsupported Agent task kind")
		}
	}
	if response.JSON200.DesiredConfigurationRevision == controlState.AppliedConfigurationRevision {
		return outcome, nil
	}
	if response.JSON200.DesiredConfigurationRevision < controlState.AppliedConfigurationRevision {
		return outcome, errors.New("center desired configuration revision moved backwards")
	}
	desiredRevision := response.JSON200.DesiredConfigurationRevision
	configuration, err := c.configuration(ctx, identity, desiredRevision, events)
	if err == nil {
		err = store.ApplyConfiguration(configuration)
	}
	if err != nil {
		if errors.Is(err, ErrAgentRevoked) {
			return outcome, err
		}
		if recordErr := store.RecordConfigurationFailure(desiredRevision, err); recordErr != nil {
			return outcome, errors.Join(err, recordErr)
		}
		return outcome, fmt.Errorf("apply configuration revision %d: %w", desiredRevision, err)
	}
	outcome.applied = true
	return outcome, nil
}

func (c *ControlClient) configuration(
	ctx context.Context,
	identity state.Identity,
	desiredRevision int64,
	events agentlogs.Sink,
) (state.Configuration, error) {
	startedAt := time.Now()
	response, err := c.client.GetAgentConfigurationWithResponse(ctx, func(_ context.Context, request *http.Request) error {
		request.Header.Set("Authorization", "Bearer "+identity.Credential)
		return nil
	})
	duration := time.Since(startedAt).Milliseconds()
	target := identity.CenterURL + "/api/v1/agent/configuration"
	if err != nil {
		emitAgentRequestFailure(events, "configuration", "fetch-failed", "Agent configuration request failed", http.MethodGet,
			target, 0, "", nil, duration, err)
		return state.Configuration{}, err
	}
	if response.JSON200 == nil {
		emitAgentRequestFailure(events, "configuration", "fetch-rejected", "Agent configuration request was rejected", http.MethodGet,
			target, response.StatusCode(), response.ContentType(), response.Body, duration, nil)
		return state.Configuration{}, responseError("fetch Agent configuration", response.StatusCode(), response.JSON401, response.JSON403)
	}
	if len(response.Body) > maxAgentAPIResponseSize {
		emitAgentInvalidResponse(events, "configuration", "snapshot-too-large", "Agent configuration response exceeded 512 KiB",
			http.MethodGet, target, response.StatusCode(), response.ContentType(), response.Body, duration, "response-too-large")
		return state.Configuration{}, errors.New("Agent configuration snapshot exceeds 512 KiB")
	}
	var snapshot agentapi.AgentConfigurationSnapshot
	decoder := json.NewDecoder(bytes.NewReader(response.Body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&snapshot); err != nil {
		emitAgentInvalidResponse(events, "configuration", "snapshot-invalid", "Agent configuration response was invalid",
			http.MethodGet, target, response.StatusCode(), response.ContentType(), response.Body, duration, "invalid-response")
		return state.Configuration{}, fmt.Errorf("decode Agent configuration snapshot: %w", err)
	}
	if err := ensureJSONEnd(decoder); err != nil {
		emitAgentInvalidResponse(events, "configuration", "snapshot-invalid", "Agent configuration response was invalid",
			http.MethodGet, target, response.StatusCode(), response.ContentType(), response.Body, duration, "invalid-response")
		return state.Configuration{}, err
	}
	configuration := configurationFromAPI(snapshot)
	if configuration.Revision != desiredRevision {
		emitAgentInvalidResponse(events, "configuration", "revision-mismatch", "Agent configuration revision did not match",
			http.MethodGet, target, response.StatusCode(), response.ContentType(), response.Body, duration, "invalid-response")
		return state.Configuration{}, fmt.Errorf("configuration revision is %d, expected %d", configuration.Revision, desiredRevision)
	}
	emitAgentRequestSuccess(events, "configuration", "fetch-succeeded", "Agent configuration request succeeded", http.MethodGet,
		target, response.StatusCode(), duration)
	return configuration, nil
}

func (c *ControlClient) runLogUploader(
	ctx context.Context,
	store *state.Store,
	identity state.Identity,
	recorder *agentlogs.Recorder,
) error {
	const (
		batchSize       = 64
		batchBytes      = 8*1024*1024 - 1024
		idleInterval    = 30 * time.Second
		failureInterval = 10 * time.Second
	)
	for {
		events, err := store.PendingAgentLogs(batchSize, batchBytes)
		if err != nil {
			return err
		}
		if len(events) == 0 {
			timer := time.NewTimer(idleInterval)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return nil
			case <-timer.C:
			case <-recorder.UploadWake():
				if !timer.Stop() {
					<-timer.C
				}
			}
			continue
		}
		err = c.uploadAgentLogs(ctx, store, identity, events, recorder)
		if errors.Is(err, ErrAgentRevoked) {
			return err
		}
		if err == nil {
			continue
		}
		timer := time.NewTimer(failureInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil
		case <-timer.C:
		}
	}
}

func (c *ControlClient) uploadAgentLogs(
	ctx context.Context,
	store *state.Store,
	identity state.Identity,
	events []state.LogEvent,
	eventSink agentlogs.Sink,
) error {
	payload := agentapi.AgentLogBatch{Events: make([]agentapi.AgentLogEvent, 0, len(events))}
	for _, event := range events {
		converted, err := agentLogEventToAPI(event)
		if err != nil {
			return err
		}
		payload.Events = append(payload.Events, converted)
	}
	startedAt := time.Now()
	response, err := c.client.UploadAgentLogsWithResponse(ctx, payload, func(_ context.Context, request *http.Request) error {
		request.Header.Set("Authorization", "Bearer "+identity.Credential)
		return nil
	})
	duration := time.Since(startedAt).Milliseconds()
	target := agentlogs.SanitizeRequestTarget(identity.CenterURL + "/api/v1/agent/logs")
	method := http.MethodPost
	if err != nil {
		category := agentlogs.ClassifyRequestError(err)
		eventSink.Emit(agentlogs.Event{
			Level: "warn", Component: "log-uploader", EventType: "upload-failed",
			Message: "Agent log upload failed: " + err.Error(), FailureCategory: &category,
			RequestMethod: &method, RequestTarget: target, DurationMilliseconds: &duration,
		})
		return err
	}
	if response.JSON200 == nil {
		category := "http-status"
		if response.StatusCode() == http.StatusTooManyRequests {
			category = "rate-limit"
		}
		status := response.StatusCode()
		contentType := response.ContentType()
		eventSink.Emit(agentlogs.Event{
			Level: "warn", Component: "log-uploader", EventType: "upload-rejected",
			Message:         fmt.Sprintf("Center rejected Agent log upload with HTTP %d", status),
			FailureCategory: &category, RequestMethod: &method, RequestTarget: target,
			HTTPStatus: &status, DurationMilliseconds: &duration,
			ResponseContentType: &contentType, RateLimitHeaders: agentlogs.RateLimitHeaders(response.HTTPResponse.Header),
			ResponseBody: response.Body,
		})
		return responseError("upload Agent logs", status, response.JSON400, response.JSON401, response.JSON403)
	}
	acknowledged := make([]string, 0, len(response.JSON200.AcceptedEventIds)+len(response.JSON200.DiscardedEventIds))
	for _, id := range response.JSON200.AcceptedEventIds {
		acknowledged = append(acknowledged, id.String())
	}
	for _, id := range response.JSON200.DiscardedEventIds {
		acknowledged = append(acknowledged, id.String())
	}
	if !sameLogEventIDs(events, acknowledged) {
		category := "invalid-response"
		status := response.StatusCode()
		contentType := response.ContentType()
		eventSink.Emit(agentlogs.Event{
			Level: "warn", Component: "log-uploader", EventType: "upload-receipt-invalid",
			Message:         "Center Agent log receipt did not match the uploaded batch",
			FailureCategory: &category, RequestMethod: &method, RequestTarget: target,
			HTTPStatus: &status, DurationMilliseconds: &duration,
			ResponseContentType: &contentType, RateLimitHeaders: agentlogs.RateLimitHeaders(response.HTTPResponse.Header),
			ResponseBody: response.Body,
		})
		return errors.New("Center Agent log receipt does not match the uploaded batch")
	}
	return store.AcknowledgeAgentLogs(acknowledged)
}

func agentLogEventToAPI(event state.LogEvent) (agentapi.AgentLogEvent, error) {
	id, err := uuid.Parse(event.ID)
	if err != nil {
		return agentapi.AgentLogEvent{}, err
	}
	result := agentapi.AgentLogEvent{
		Id: id, OccurredAt: event.OccurredAt, Level: agentapi.LogLevel(event.Level),
		Component: event.Component, EventType: event.EventType, Message: event.Message,
		PublicAddress: event.PublicAddress, ConfigurationRevision: event.ConfigurationRevision,
		DiscoveryPath: event.DiscoveryPath, RequestMethod: event.RequestMethod,
		RequestTarget: event.RequestTarget, HttpStatus: event.HTTPStatus,
		DurationMilliseconds: event.DurationMilliseconds,
		ResponseContentType:  event.ResponseContentType,
		DroppedCount:         event.DroppedCount, DroppedFrom: event.DroppedFrom, DroppedTo: event.DroppedTo,
	}
	if len(event.RateLimitHeaders) > 0 {
		value := maps.Clone(event.RateLimitHeaders)
		result.RateLimitHeaders = &value
	}
	if event.PublicAddressID != nil {
		value, err := uuid.Parse(*event.PublicAddressID)
		if err != nil {
			return agentapi.AgentLogEvent{}, err
		}
		result.PublicAddressId = &value
	}
	if event.TaskID != nil {
		value, err := uuid.Parse(*event.TaskID)
		if err != nil {
			return agentapi.AgentLogEvent{}, err
		}
		result.TaskId = &value
	}
	if event.ProxyID != nil {
		value, err := uuid.Parse(*event.ProxyID)
		if err != nil {
			return agentapi.AgentLogEvent{}, err
		}
		result.ProxyId = &value
	}
	if event.Family != nil {
		value := agentapi.AddressFamily(*event.Family)
		result.Family = &value
	}
	if event.FailureCategory != nil {
		value := agentapi.LogFailureCategory(*event.FailureCategory)
		result.FailureCategory = &value
	}
	if event.ResponseBody != nil {
		value := append([]byte(nil), event.ResponseBody...)
		result.ResponseBody = &value
	}
	if event.ResponseTruncated {
		value := true
		result.ResponseTruncated = &value
	}
	return result, nil
}

func sameLogEventIDs(events []state.LogEvent, acknowledged []string) bool {
	if len(events) != len(acknowledged) {
		return false
	}
	expected := make(map[string]struct{}, len(events))
	for _, event := range events {
		expected[event.ID] = struct{}{}
	}
	for _, id := range acknowledged {
		if _, exists := expected[id]; !exists {
			return false
		}
		delete(expected, id)
	}
	return len(expected) == 0
}

func configurationFromAPI(snapshot agentapi.AgentConfigurationSnapshot) state.Configuration {
	configuration := state.Configuration{
		SchemaVersion: int(snapshot.SchemaVersion), Revision: snapshot.Revision,
		Enabled: snapshot.Enabled, HistoryGeneration: snapshot.HistoryGeneration,
		DiscoveryPaths: make([]state.Egress, 0, len(snapshot.DiscoveryPaths)),
		ProbeTargets:   make([]state.Egress, 0, len(snapshot.ProbeTargets)),
		Proxies:        make([]state.Proxy, 0, len(snapshot.Proxies)),
		DiscoveryServices: state.DiscoveryServices{
			IPv4: slices.Clone(snapshot.DiscoveryServices.Ipv4Services),
			IPv6: slices.Clone(snapshot.DiscoveryServices.Ipv6Services),
		},
		ProbeSchedule: state.ProbeSchedule{
			Enabled: snapshot.ProbeSchedule.Enabled,
			Cron:    snapshot.ProbeSchedule.Cron, Timezone: snapshot.ProbeSchedule.Timezone,
		},
		ProbeLowMemoryOverride: snapshot.ProbeLowMemoryOverride,
	}
	if snapshot.LogLevel != nil {
		configuration.LogLevel = string(*snapshot.LogLevel)
	}
	if snapshot.IpapiApiKey != nil {
		configuration.IPAPIAPIKey = *snapshot.IpapiApiKey
	}
	for _, path := range snapshot.DiscoveryPaths {
		configuration.DiscoveryPaths = append(configuration.DiscoveryPaths, state.Egress{
			ID: path.Id.String(), Kind: string(path.Kind), Family: string(path.Family),
			InterfaceName: path.InterfaceName, SourceAddress: path.SourceAddress,
			ProxyID: uuidString(path.ProxyId), Enabled: true,
			LightweightIntervalSeconds: path.LightweightIntervalSeconds,
		})
	}
	for _, target := range snapshot.ProbeTargets {
		pathID := target.PathId.String()
		publicAddress := target.PublicAddress
		configuration.ProbeTargets = append(configuration.ProbeTargets, state.Egress{
			ID: target.Id.String(), PathID: &pathID, PublicAddress: &publicAddress,
			Kind: string(target.Kind), Family: string(target.Family),
			InterfaceName: target.InterfaceName, SourceAddress: target.SourceAddress,
			ProxyID: uuidString(target.ProxyId), Enabled: target.Enabled,
		})
	}
	for _, proxy := range snapshot.Proxies {
		configuration.Proxies = append(configuration.Proxies, state.Proxy{
			ID: proxy.Id.String(), Scheme: string(proxy.Scheme), Host: proxy.Host, Port: proxy.Port,
			Username: proxy.Username, Password: proxy.Password,
		})
	}
	return configuration
}

func uuidString(value *uuid.UUID) *string {
	if value == nil {
		return nil
	}
	result := value.String()
	return &result
}

type boundedResponseTransport struct {
	base     http.RoundTripper
	maxBytes int64
}

func (transport boundedResponseTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := transport.base.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	if response.Body != nil {
		response.Body = struct {
			io.Reader
			io.Closer
		}{Reader: io.LimitReader(response.Body, transport.maxBytes+1), Closer: response.Body}
	}
	return response, nil
}

func ensureJSONEnd(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return fmt.Errorf("decode Agent configuration suffix: %w", err)
	}
	return errors.New("Agent configuration contains multiple JSON values")
}

func currentMetadata(version string, updateCapable bool) (agentapi.AgentMetadata, error) {
	if runtime.GOOS != "linux" || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		return agentapi.AgentMetadata{}, fmt.Errorf("unsupported Agent platform %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	hostname, err := os.Hostname()
	if err != nil {
		return agentapi.AgentMetadata{}, fmt.Errorf("read hostname: %w", err)
	}
	hostname = strings.TrimSpace(hostname)
	if hostname == "" {
		return agentapi.AgentMetadata{}, errors.New("hostname must not be empty")
	}
	physicalMemoryBytes, err := readPhysicalMemory("/proc/meminfo")
	if err != nil {
		return agentapi.AgentMetadata{}, fmt.Errorf("read physical memory: %w", err)
	}
	capabilities := []string{controlCapability, "configuration-v9", "configuration-v10", "agent-logs-v1", "network-inventory-v1", "address-observation-v1", "complete-probe-v1", syncWakeCapability}
	if updateCapable {
		capabilities = append(capabilities, "agent-update-v1")
	}
	slices.Sort(capabilities)
	return agentapi.AgentMetadata{
		Hostname: hostname, AgentVersion: version,
		SourceRevision:  &productversion.Revision,
		OperatingSystem: agentapi.Linux, Architecture: agentapi.AgentArchitecture(runtime.GOARCH),
		Capabilities:        capabilities,
		PhysicalMemoryBytes: physicalMemoryBytes,
	}, nil
}

func readPhysicalMemory(path string) (int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	return parsePhysicalMemory(file)
}

func parsePhysicalMemory(input io.Reader) (int64, error) {
	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 || fields[0] != "MemTotal:" {
			continue
		}
		if len(fields) != 3 || fields[2] != "kB" {
			return 0, errors.New("MemTotal has an unexpected format")
		}
		kilobytes, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil || kilobytes < 1 || kilobytes > (1<<63-1)/1024 {
			return 0, errors.New("MemTotal is outside the supported range")
		}
		return kilobytes * 1024, nil
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return 0, errors.New("MemTotal is missing")
}

func addressUploadToAPI(upload state.AddressUpload) ([]agentapi.AgentAddressState, []agentapi.AgentAddressEvent, []agentapi.AgentAddressGap, error) {
	states := make([]agentapi.AgentAddressState, 0, len(upload.States))
	for _, item := range upload.States {
		egressID, err := uuid.Parse(item.EgressID)
		if err != nil {
			return nil, nil, nil, err
		}
		var failureReason *agentapi.AddressFailureReason
		if item.FailureReason != nil {
			value := agentapi.AddressFailureReason(*item.FailureReason)
			failureReason = &value
		}
		states = append(states, agentapi.AgentAddressState{
			EgressId: egressID, HistoryGeneration: item.HistoryGeneration,
			Family: agentapi.AddressFamily(item.Family), Status: agentapi.AddressObservationStatus(item.Status),
			Sequence: item.Sequence, PublicAddress: item.PublicAddress, LocalInterface: item.LocalInterface,
			LocalAddress: item.LocalAddress, ProxyPath: item.ProxyPath, LikelyNat: item.LikelyNAT,
			Temporary: item.Temporary, FailureReason: failureReason, LastCheckedAt: item.LastCheckedAt,
			LastSucceededAt: item.LastSucceededAt, LastChangedAt: item.LastChangedAt,
		})
	}
	events := make([]agentapi.AgentAddressEvent, 0, len(upload.Events))
	for _, item := range upload.Events {
		id, err := uuid.Parse(item.ID)
		if err != nil {
			return nil, nil, nil, err
		}
		egressID, err := uuid.Parse(item.EgressID)
		if err != nil {
			return nil, nil, nil, err
		}
		var failureReason *agentapi.AddressFailureReason
		if item.FailureReason != nil {
			value := agentapi.AddressFailureReason(*item.FailureReason)
			failureReason = &value
		}
		events = append(events, agentapi.AgentAddressEvent{
			Id: id, EgressId: egressID, HistoryGeneration: item.HistoryGeneration,
			Sequence: item.Sequence, Kind: agentapi.AddressEventKind(item.Kind), Family: agentapi.AddressFamily(item.Family),
			PublicAddress:  item.PublicAddress,
			LocalInterface: item.LocalInterface, LocalAddress: item.LocalAddress,
			ProxyPath: item.ProxyPath, LikelyNat: item.LikelyNAT, Temporary: item.Temporary,
			FailureReason: failureReason, ObservedAt: item.ObservedAt,
		})
	}
	gaps := make([]agentapi.AgentAddressGap, 0, len(upload.Gaps))
	for _, item := range upload.Gaps {
		id, err := uuid.Parse(item.ID)
		if err != nil {
			return nil, nil, nil, err
		}
		egressID, err := uuid.Parse(item.EgressID)
		if err != nil {
			return nil, nil, nil, err
		}
		gaps = append(gaps, agentapi.AgentAddressGap{
			Id: id, EgressId: egressID, HistoryGeneration: item.HistoryGeneration,
			DroppedCount: item.DroppedCount, FirstSequence: item.FirstSequence, LastSequence: item.LastSequence,
			FirstObservedAt: item.FirstObservedAt, LastObservedAt: item.LastObservedAt,
		})
	}
	return states, events, gaps, nil
}

func addressReceiptFromAPI(receipt agentapi.AgentAddressUploadReceipt) state.AddressUploadReceipt {
	result := state.AddressUploadReceipt{
		AcceptedEventIDs:  make([]string, 0, len(receipt.AcceptedEventIds)),
		DiscardedEventIDs: make([]string, 0, len(receipt.DiscardedEventIds)),
		AcceptedGaps:      make([]state.AddressGapReceipt, 0, len(receipt.AcceptedGaps)),
		DiscardedGaps:     make([]state.AddressGapReceipt, 0, len(receipt.DiscardedGaps)),
	}
	for _, id := range receipt.AcceptedEventIds {
		result.AcceptedEventIDs = append(result.AcceptedEventIDs, id.String())
	}
	for _, id := range receipt.DiscardedEventIds {
		result.DiscardedEventIDs = append(result.DiscardedEventIDs, id.String())
	}
	for _, gap := range receipt.AcceptedGaps {
		result.AcceptedGaps = append(result.AcceptedGaps, state.AddressGapReceipt{ID: gap.Id.String(), LastSequence: gap.LastSequence})
	}
	for _, gap := range receipt.DiscardedGaps {
		result.DiscardedGaps = append(result.DiscardedGaps, state.AddressGapReceipt{ID: gap.Id.String(), LastSequence: gap.LastSequence})
	}
	return result
}

func captureNetworkInventory() (*agentapi.NetworkInventory, *string) {
	inventory, err := agentnetwork.Discover()
	if err != nil {
		message := boundedMessage(err.Error(), 1024)
		return nil, &message
	}
	result := &agentapi.NetworkInventory{CapturedAt: time.Now().UTC()}
	result.Interfaces = make([]agentapi.NetworkInterface, 0, len(inventory.Interfaces))
	for _, item := range inventory.Interfaces {
		result.Interfaces = append(result.Interfaces, agentapi.NetworkInterface{
			Name: item.Name, Index: item.Index, Up: item.Up, Loopback: item.Loopback,
		})
	}
	result.Addresses = make([]agentapi.NetworkAddress, 0, len(inventory.Addresses))
	for _, item := range inventory.Addresses {
		result.Addresses = append(result.Addresses, agentapi.NetworkAddress{
			InterfaceName: item.Interface, Address: item.Address, PrefixLength: item.PrefixLength,
			Family: agentapi.AddressFamily(item.Family), Scope: agentapi.NetworkAddressScope(item.Scope),
			Temporary: item.Temporary, Tentative: item.Tentative, Deprecated: item.Deprecated, Duplicate: item.Duplicate,
		})
	}
	result.Routes = make([]agentapi.NetworkRoute, 0, len(inventory.Routes))
	for _, item := range inventory.Routes {
		result.Routes = append(result.Routes, agentapi.NetworkRoute{
			InterfaceName: item.Interface, Family: agentapi.AddressFamily(item.Family), Destination: item.Destination,
			Gateway: item.Gateway, Metric: item.Metric, Default: item.Default,
		})
	}
	return result, nil
}

func boundedMessage(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) > limit {
		runes = runes[:limit]
	}
	if len(runes) == 0 {
		return "network inventory failed without diagnostics"
	}
	return string(runes)
}

func stringPointer(value string) *string {
	return &value
}

func emitAgentRequestFailure(
	events agentlogs.Sink,
	component, eventType, message, method, target string,
	status int,
	contentType string,
	body []byte,
	duration int64,
	requestErr error,
) {
	if events == nil {
		return
	}
	category := "http-status"
	if requestErr != nil {
		category = agentlogs.ClassifyRequestError(requestErr)
	} else if status == http.StatusTooManyRequests {
		category = "rate-limit"
	}
	event := agentlogs.Event{
		Level: "warn", Component: component, EventType: eventType, Message: message,
		FailureCategory: &category, RequestMethod: &method, RequestTarget: agentlogs.SanitizeRequestTarget(target),
		DurationMilliseconds: &duration, ResponseBody: append([]byte(nil), body...),
		ResponseTruncated: len(body) > maxAgentAPIResponseSize,
	}
	if status != 0 {
		event.HTTPStatus = &status
	}
	if contentType != "" {
		event.ResponseContentType = &contentType
	}
	events.Emit(event)
}

func emitAgentInvalidResponse(
	events agentlogs.Sink,
	component, eventType, message, method, target string,
	status int,
	contentType string,
	body []byte,
	duration int64,
	category string,
) {
	if events == nil {
		return
	}
	event := agentlogs.Event{
		Level: "warn", Component: component, EventType: eventType, Message: message,
		FailureCategory: &category, RequestMethod: &method, RequestTarget: agentlogs.SanitizeRequestTarget(target),
		DurationMilliseconds: &duration, ResponseBody: append([]byte(nil), body...),
		ResponseTruncated: len(body) > maxAgentAPIResponseSize,
	}
	if status != 0 {
		event.HTTPStatus = &status
	}
	if contentType != "" {
		event.ResponseContentType = &contentType
	}
	events.Emit(event)
}

func emitAgentRequestSuccess(
	events agentlogs.Sink,
	component, eventType, message, method, target string,
	status int,
	duration int64,
) {
	if events == nil {
		return
	}
	events.Emit(agentlogs.Event{
		Level: "debug", Component: component, EventType: eventType, Message: message,
		RequestMethod: &method, RequestTarget: agentlogs.SanitizeRequestTarget(target),
		HTTPStatus: &status, DurationMilliseconds: &duration,
	})
}

func responseError(operation string, status int, responses ...*agentapi.ErrorResponse) error {
	for _, response := range responses {
		if response != nil {
			if response.Code == agentapi.AgentRevoked {
				return fmt.Errorf("%w: %s returned HTTP %d", ErrAgentRevoked, operation, status)
			}
			return fmt.Errorf("%s returned HTTP %d (%s)", operation, status, response.Code)
		}
	}
	return fmt.Errorf("%s returned HTTP %d", operation, status)
}
