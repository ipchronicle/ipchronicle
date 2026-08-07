package agent

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/ipchronicle/ipchronicle/internal/agent/state"
	"github.com/ipchronicle/ipchronicle/internal/generated/agentapi"
)

const controlCapability = "control-v1"

type ControlClient struct {
	client *agentapi.ClientWithResponses
}

func NewControlClient(centerURL string) (*ControlClient, error) {
	normalized, err := NormalizeCenterURL(centerURL)
	if err != nil {
		return nil, err
	}
	httpClient := &http.Client{Timeout: 15 * time.Second}
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
	logger.Printf("Agent %s polling %s", identity.NodeID, identity.CenterURL)
	for {
		pollInterval, err := client.poll(ctx, identity, metadata)
		if err != nil && !errors.Is(err, context.Canceled) {
			logger.Printf("control poll failed: %v", err)
		}
		if pollInterval >= 5*time.Second && pollInterval <= time.Hour {
			interval = pollInterval
		}
		timer := time.NewTimer(interval)
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

func (c *ControlClient) poll(ctx context.Context, identity state.Identity, metadata agentapi.AgentMetadata) (time.Duration, error) {
	response, err := c.client.PollAgentWithResponse(ctx, agentapi.AgentPollRequest{
		AppliedConfigurationRevision: identity.AppliedConfigurationRevision,
		Metadata:                     metadata,
	}, func(_ context.Context, request *http.Request) error {
		request.Header.Set("Authorization", "Bearer "+identity.Credential)
		return nil
	})
	if err != nil {
		return 0, err
	}
	if response.JSON200 == nil {
		return 0, responseError("poll center", response.StatusCode(), response.JSON400, response.JSON401, response.JSON403)
	}
	return time.Duration(response.JSON200.PollIntervalSeconds) * time.Second, nil
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
		Capabilities: []string{controlCapability},
	}, nil
}

func responseError(operation string, status int, responses ...*agentapi.ErrorResponse) error {
	for _, response := range responses {
		if response != nil {
			return fmt.Errorf("%s returned HTTP %d (%s)", operation, status, response.Code)
		}
	}
	return fmt.Errorf("%s returned HTTP %d", operation, status)
}
