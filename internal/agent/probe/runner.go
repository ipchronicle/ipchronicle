package probe

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	agentnetwork "github.com/ipchronicle/ipchronicle/internal/agent/network"
	"github.com/ipchronicle/ipchronicle/internal/agent/observation"
	"github.com/ipchronicle/ipchronicle/internal/agent/state"
	"golang.org/x/sys/unix"
)

const defaultProbeTimeout = 15 * time.Minute

type nativeProbeInput struct {
	Target                   string
	Family                   string
	HTTPClient               *http.Client
	ExplicitLookupHTTPClient *http.Client
	DialContext              func(context.Context, string, string) (net.Conn, error)
	ProxyAdapterURL          string
	StartedAt                time.Time
	IPAPIAPIKey              string
}

type Runner struct {
	discover     func() (agentnetwork.Inventory, error)
	now          func() time.Time
	timeout      time.Duration
	httpClient   func(executionPath, string) *http.Client
	lookupClient func(executionPath, string) *http.Client
	verifyTarget func(context.Context, state.Configuration, state.Egress, time.Time) error
	execute      func(context.Context, nativeProbeInput) ([]byte, error)
}

func NewRunner() *Runner {
	checker := observation.NewChecker()
	return &Runner{
		discover: agentnetwork.Discover, now: time.Now, timeout: defaultProbeTimeout,
		httpClient: pathHTTPClient, lookupClient: explicitLookupHTTPClient,
		verifyTarget: checker.VerifyTarget, execute: runNativeProbe,
	}
}

func (runner *Runner) Run(
	ctx context.Context,
	configuration state.Configuration,
	egress state.Egress,
	startedAt time.Time,
) (outcome state.ProbeExecutionOutcome, resultErr error) {
	startedAt = startedAt.UTC().Truncate(time.Second)
	failure := func(stage, diagnostic string) state.ProbeExecutionOutcome {
		return state.ProbeExecutionOutcome{
			Status: "failed", StartedAt: &startedAt, CompletedAt: runner.now().UTC(),
			FailureStage: stage, Diagnostic: sanitizeDiagnostic(diagnostic, configuration),
		}
	}

	var inventory agentnetwork.Inventory
	if egress.Kind != "proxy" {
		var err error
		inventory, err = runner.discover()
		if err != nil {
			return failure("selector", err.Error()), nil
		}
	}
	path, err := selectExecutionPath(configuration, egress, inventory)
	if err != nil {
		return failure("selector", err.Error()), nil
	}
	if egress.PublicAddress == nil {
		return failure("selector", "complete-probe target has no public address"), nil
	}
	if err := runner.verifyTarget(ctx, configuration, egress, runner.now().UTC()); err != nil {
		return failure("selector", err.Error()), nil
	}

	var adapter *localProxyAdapter
	adapterURL := ""
	if path.proxy != nil {
		adapter, err = startLocalProxyAdapter(*path.proxy)
		if err != nil {
			return failure("adapter", err.Error()), nil
		}
		adapterURL = adapter.URL()
		defer func() {
			if err := adapter.Close(); err != nil {
				resultErr = errors.Join(resultErr, fmt.Errorf("close execution-scoped proxy adapter: %w", err))
			}
		}()
	}

	executionContext, cancel := context.WithTimeout(ctx, runner.timeout)
	defer cancel()
	raw, err := runner.execute(executionContext, nativeProbeInput{
		Target: *egress.PublicAddress, Family: egress.Family,
		HTTPClient:               runner.httpClient(path, adapterURL),
		ExplicitLookupHTTPClient: runner.lookupClient(path, adapterURL),
		DialContext:              pathDialContext(path),
		ProxyAdapterURL:          adapterURL, StartedAt: startedAt, IPAPIAPIKey: configuration.IPAPIAPIKey,
	})
	if err != nil {
		if errors.Is(executionContext.Err(), context.DeadlineExceeded) {
			return failure("timeout", "native probe execution exceeded its time limit"), nil
		}
		if executionContext.Err() != nil {
			interrupted := failure("process", "native probe execution was interrupted")
			interrupted.Status = "interrupted"
			return interrupted, nil
		}
		return failure("process", err.Error()), nil
	}
	if len(raw) < 1 || len(raw) > state.MaxProbeResultBytes {
		return failure("output", "native probe JSON output is empty or exceeds 1 MiB"), nil
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(raw, &document); err != nil || document == nil {
		return failure("output", "native probe output is not a JSON object"), nil
	}
	return state.ProbeExecutionOutcome{
		Status: "succeeded", StartedAt: &startedAt, CompletedAt: runner.now().UTC(), RawResult: raw,
	}, nil
}

func pathHTTPClient(path executionPath, adapterURL string) *http.Client {
	return probeHTTPClient(pathDialContext(path), adapterURL)
}

func explicitLookupHTTPClient(path executionPath, adapterURL string) *http.Client {
	if adapterURL != "" || path.egress.Family != "ipv6" {
		return pathHTTPClient(path, adapterURL)
	}
	dialer := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: -1}
	return probeHTTPClient(dialer.DialContext, "")
}

func probeHTTPClient(
	dialContext func(context.Context, string, string) (net.Conn, error),
	adapterURL string,
) *http.Client {
	transport := &http.Transport{
		DisableKeepAlives:     true,
		ForceAttemptHTTP2:     true,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
	}
	if adapterURL != "" {
		proxyURL, _ := url.Parse(adapterURL)
		transport.Proxy = http.ProxyURL(proxyURL)
	} else {
		transport.DialContext = dialContext
	}
	return &http.Client{
		Transport: transport,
		CheckRedirect: func(_ *http.Request, previous []*http.Request) error {
			if len(previous) >= 5 {
				return errors.New("probe endpoint redirected too many times")
			}
			return nil
		},
	}
}

func pathDialContext(path executionPath) func(context.Context, string, string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: -1}
	if path.sourceAddress != nil {
		dialer.LocalAddr = &net.TCPAddr{IP: net.IP(path.sourceAddress.AsSlice())}
	}
	if path.bindInterface {
		interfaceName := path.interfaceName
		dialer.Control = func(_, _ string, raw syscall.RawConn) error {
			var socketError error
			if err := raw.Control(func(fileDescriptor uintptr) {
				socketError = unix.SetsockoptString(int(fileDescriptor), unix.SOL_SOCKET, unix.SO_BINDTODEVICE, interfaceName)
			}); err != nil {
				return err
			}
			return socketError
		}
	}
	return func(ctx context.Context, _, address string) (net.Conn, error) {
		network := "tcp4"
		if path.egress.Family == "ipv6" {
			network = "tcp6"
		}
		return dialer.DialContext(ctx, network, address)
	}
}

func sanitizeDiagnostic(value string, configuration state.Configuration) string {
	if configuration.IPAPIAPIKey != "" {
		value = strings.ReplaceAll(value, configuration.IPAPIAPIKey, "[REDACTED]")
		value = strings.ReplaceAll(value, url.QueryEscape(configuration.IPAPIAPIKey), "[REDACTED]")
	}
	for _, proxy := range configuration.Proxies {
		for _, secret := range []*string{proxy.Password} {
			if secret == nil || *secret == "" {
				continue
			}
			value = strings.ReplaceAll(value, *secret, "[REDACTED]")
			value = strings.ReplaceAll(value, url.QueryEscape(*secret), "[REDACTED]")
		}
	}
	value = strings.ToValidUTF8(value, "?")
	var cleaned strings.Builder
	cleaned.Grow(min(len(value), state.MaxProbeDiagnosticBytes))
	for _, current := range value {
		if current == '\n' || current == '\r' || current == '\t' || current >= 0x20 {
			if cleaned.Len()+utf8.RuneLen(current) > state.MaxProbeDiagnosticBytes {
				break
			}
			cleaned.WriteRune(current)
		}
	}
	return strings.TrimSpace(cleaned.String())
}
