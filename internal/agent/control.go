package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
	agentnetwork "github.com/ipchronicle/ipchronicle/internal/agent/network"
	"github.com/ipchronicle/ipchronicle/internal/agent/observation"
	"github.com/ipchronicle/ipchronicle/internal/agent/state"
	"github.com/ipchronicle/ipchronicle/internal/generated/agentapi"
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
	if registrationKey == "" {
		return state.Identity{}, errors.New("registration key must not be empty")
	}
	client, err := NewControlClient(normalized)
	if err != nil {
		return state.Identity{}, err
	}
	metadata, err := currentMetadata(version)
	if err != nil {
		return state.Identity{}, err
	}
	response, err := client.client.RegisterAgentWithResponse(ctx, agentapi.AgentRegistrationRequest{
		RegistrationKey: registrationKey,
		Metadata:        metadata,
	})
	if err != nil {
		return state.Identity{}, fmt.Errorf("register Agent: %w", err)
	}
	if response.JSON201 == nil {
		return state.Identity{}, responseError("register Agent", response.StatusCode(), response.JSON400, response.JSON401, response.JSON403)
	}
	identity := state.Identity{
		CenterURL: normalized, NodeID: response.JSON201.NodeId.String(),
		Credential: response.JSON201.Credential, AppliedConfigurationRevision: 0,
	}
	if err := store.SaveIdentity(identity); err != nil {
		return state.Identity{}, fmt.Errorf("persist Agent identity: %w", err)
	}
	return identity, nil
}

func Run(ctx context.Context, store *state.Store, version string, logger *log.Logger) error {
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
	metadata, err := currentMetadata(version)
	if err != nil {
		return err
	}
	interval := 30 * time.Second
	syncManager := newSyncManager(ctx, identity.CenterURL, identity.Credential, logger)
	defer syncManager.Close()
	observerContext, stopObserver := context.WithCancel(ctx)
	defer stopObserver()
	observerDone := make(chan error, 1)
	go func() {
		observerDone <- observation.NewObserver(store, logger).Run(observerContext)
	}()
	logger.Printf("Agent %s polling %s", identity.NodeID, identity.CenterURL)
	for {
		select {
		case observerErr := <-observerDone:
			if observerErr != nil {
				return fmt.Errorf("run lightweight address observer: %w", observerErr)
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
		outcome, err := client.poll(ctx, store, identity, metadata, controlState, inventory, inventoryError)
		if errors.Is(err, ErrAgentRevoked) {
			if markErr := store.MarkRevoked(); markErr != nil {
				return errors.Join(err, markErr)
			}
			logger.Printf("Agent %s was revoked by the center; control polling has stopped", identity.NodeID)
			return nil
		}
		if err != nil && !errors.Is(err, context.Canceled) {
			logger.Printf("control poll failed: %v", err)
		}
		if outcome.received {
			syncManager.Update(outcome.syncSession)
		}
		if outcome.interval >= 5*time.Second && outcome.interval <= time.Hour {
			interval = outcome.interval
		}
		if outcome.applied {
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
		case observerErr := <-observerDone:
			if !timer.Stop() {
				<-timer.C
			}
			if observerErr != nil {
				return fmt.Errorf("run lightweight address observer: %w", observerErr)
			}
			return nil
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
) (pollOutcome, error) {
	upload, err := store.AddressUpload(64)
	if err != nil {
		return pollOutcome{}, err
	}
	addressStates, addressEvents, addressGaps, err := addressUploadToAPI(upload)
	if err != nil {
		return pollOutcome{}, err
	}
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
	}, func(_ context.Context, request *http.Request) error {
		request.Header.Set("Authorization", "Bearer "+identity.Credential)
		return nil
	})
	if err != nil {
		return pollOutcome{}, err
	}
	if response.JSON200 == nil {
		return pollOutcome{}, responseError("poll center", response.StatusCode(), response.JSON400, response.JSON401, response.JSON403)
	}
	if err := store.AcknowledgeAddressUpload(addressReceiptFromAPI(response.JSON200.AddressUploadReceipt)); err != nil {
		return pollOutcome{}, fmt.Errorf("acknowledge address upload: %w", err)
	}
	outcome := pollOutcome{
		interval: time.Duration(response.JSON200.PollIntervalSeconds) * time.Second,
		received: true, syncSession: response.JSON200.SyncSession,
	}
	if response.JSON200.DesiredConfigurationRevision == controlState.AppliedConfigurationRevision {
		return outcome, nil
	}
	if response.JSON200.DesiredConfigurationRevision < controlState.AppliedConfigurationRevision {
		return outcome, errors.New("center desired configuration revision moved backwards")
	}
	desiredRevision := response.JSON200.DesiredConfigurationRevision
	configuration, err := c.configuration(ctx, identity.Credential, desiredRevision)
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

func (c *ControlClient) configuration(ctx context.Context, credential string, desiredRevision int64) (state.Configuration, error) {
	response, err := c.client.GetAgentConfigurationWithResponse(ctx, func(_ context.Context, request *http.Request) error {
		request.Header.Set("Authorization", "Bearer "+credential)
		return nil
	})
	if err != nil {
		return state.Configuration{}, err
	}
	if response.JSON200 == nil {
		return state.Configuration{}, responseError("fetch Agent configuration", response.StatusCode(), response.JSON401, response.JSON403)
	}
	if len(response.Body) > maxAgentAPIResponseSize {
		return state.Configuration{}, errors.New("Agent configuration snapshot exceeds 512 KiB")
	}
	var configuration state.Configuration
	decoder := json.NewDecoder(bytes.NewReader(response.Body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&configuration); err != nil {
		return state.Configuration{}, fmt.Errorf("decode Agent configuration snapshot: %w", err)
	}
	if err := ensureJSONEnd(decoder); err != nil {
		return state.Configuration{}, err
	}
	if configuration.Revision != desiredRevision {
		return state.Configuration{}, fmt.Errorf("configuration revision is %d, expected %d", configuration.Revision, desiredRevision)
	}
	return configuration, nil
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

func currentMetadata(version string) (agentapi.AgentMetadata, error) {
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
	return agentapi.AgentMetadata{
		Hostname: hostname, AgentVersion: version,
		OperatingSystem: agentapi.Linux, Architecture: agentapi.AgentArchitecture(runtime.GOARCH),
		Capabilities: []string{controlCapability, "configuration-v4", "network-inventory-v1", "address-observation-v1", syncWakeCapability},
	}, nil
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
			PreviousAddress: item.PreviousAddress, PublicAddress: item.PublicAddress,
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
