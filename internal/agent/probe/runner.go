package probe

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	agentnetwork "github.com/ipchronicle/ipchronicle/internal/agent/network"
	"github.com/ipchronicle/ipchronicle/internal/agent/state"
	"golang.org/x/sys/unix"
)

const (
	officialScriptURL    = "https://IP.Check.Place"
	ipQualityClearPrefix = "\x1b[H\x1b[J"
	maxScriptBytes       = 2 * 1024 * 1024
	scriptDownloadLimit  = 30 * time.Second
	defaultProbeTimeout  = 15 * time.Minute
)

type processState interface {
	SetProbeProcess(state.ProbeProcess) error
	ClearProbeProcess(int) error
}

type Runner struct {
	store      processState
	discover   func() (agentnetwork.Inventory, error)
	now        func() time.Time
	scriptURL  string
	bashPath   string
	timeout    time.Duration
	httpClient func(executionPath, string) *http.Client
}

func NewRunner(store processState) *Runner {
	if store == nil {
		panic("probe runner state must not be nil")
	}
	return &Runner{
		store: store, discover: agentnetwork.Discover, now: time.Now,
		scriptURL: officialScriptURL, bashPath: "bash", timeout: defaultProbeTimeout,
		httpClient: pathHTTPClient,
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
	arguments, err := path.scriptArguments(adapterURL)
	if err != nil {
		return failure("adapter", err.Error()), nil
	}
	script, err := runner.download(ctx, path, adapterURL)
	if err != nil {
		return failure("download", err.Error()), nil
	}

	stdout := newBoundedCapture(state.MaxProbeResultBytes)
	stderr := newTailCapture(state.MaxProbeDiagnosticBytes)
	command := exec.Command(runner.bashPath, append([]string{"-s", "--"}, arguments...)...)
	command.Stdin = bytes.NewReader(script)
	command.Stdout = stdout
	command.Stderr = stderr
	command.Env = probeEnvironment(os.Environ())
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		return failure("process", joinDiagnostic(err.Error(), stderr.String())), nil
	}
	processGroupID := command.Process.Pid
	identity, err := currentProcessIdentity(command.Process.Pid, processGroupID, startedAt)
	if err != nil {
		done := make(chan error, 1)
		go func() { done <- command.Wait() }()
		_ = terminateProcessGroup(processGroupID, done)
		return state.ProbeExecutionOutcome{}, fmt.Errorf("capture probe process identity: %w", err)
	}
	if err := runner.store.SetProbeProcess(identity); err != nil {
		done := make(chan error, 1)
		go func() { done <- command.Wait() }()
		_ = terminateProcessGroup(processGroupID, done)
		return state.ProbeExecutionOutcome{}, fmt.Errorf("persist probe process identity: %w", err)
	}

	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	executionContext, cancel := context.WithTimeout(ctx, runner.timeout)
	defer cancel()
	var waitErr error
	var stoppedBy string
	select {
	case waitErr = <-done:
	case <-stdout.Overflow():
		stoppedBy = "output"
		waitErr = terminateProcessGroup(processGroupID, done)
	case <-executionContext.Done():
		if errors.Is(executionContext.Err(), context.DeadlineExceeded) {
			stoppedBy = "timeout"
		} else {
			stoppedBy = "interrupted"
		}
		waitErr = terminateProcessGroup(processGroupID, done)
	}
	if err := runner.store.ClearProbeProcess(processGroupID); err != nil {
		return state.ProbeExecutionOutcome{}, fmt.Errorf("clear probe process identity: %w", err)
	}
	diagnostic := joinDiagnostic(stderr.String(), errorText(waitErr))
	if stoppedBy == "output" || stdout.Overflowed() {
		return failure("output", joinDiagnostic("IPQuality JSON output exceeded 1 MiB", diagnostic)), nil
	}
	if stoppedBy == "timeout" {
		return failure("timeout", joinDiagnostic("IPQuality execution exceeded its time limit", diagnostic)), nil
	}
	if stoppedBy == "interrupted" {
		outcome := failure("process", joinDiagnostic("IPQuality execution was interrupted", diagnostic))
		outcome.Status = "interrupted"
		return outcome, nil
	}
	raw, err := validateProbeJSON(stdout.Bytes())
	if err != nil {
		if waitErr != nil {
			return failure("process", joinDiagnostic(err.Error(), diagnostic, errorText(waitErr))), nil
		}
		return failure("output", joinDiagnostic(err.Error(), diagnostic)), nil
	}
	// IPQuality currently exits non-zero after a successful single-family run.
	// Its bounded, valid JSON object is the result contract for a completed process.
	completedAt := runner.now().UTC()
	return state.ProbeExecutionOutcome{
		Status: "succeeded", StartedAt: &startedAt, CompletedAt: completedAt, RawResult: raw,
	}, nil
}

func probeEnvironment(environment []string) []string {
	blocked := map[string]struct{}{
		"HTTP_PROXY": {}, "HTTPS_PROXY": {}, "ALL_PROXY": {}, "NO_PROXY": {},
		"http_proxy": {}, "https_proxy": {}, "all_proxy": {}, "no_proxy": {},
		"BASH_ENV": {}, "ENV": {},
	}
	result := make([]string, 0, len(environment))
	for _, item := range environment {
		name, _, found := strings.Cut(item, "=")
		if _, excluded := blocked[name]; found && excluded {
			continue
		}
		result = append(result, item)
	}
	return result
}

func (runner *Runner) download(ctx context.Context, path executionPath, adapterURL string) ([]byte, error) {
	downloadContext, cancel := context.WithTimeout(ctx, scriptDownloadLimit)
	defer cancel()
	request, err := http.NewRequestWithContext(downloadContext, http.MethodGet, runner.scriptURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "text/plain")
	request.Header.Set("Cache-Control", "no-cache")
	request.Header.Set("User-Agent", "IPChronicle-Agent/complete-probe")
	client := runner.httpClient(path, adapterURL)
	if transport, ok := client.Transport.(*http.Transport); ok {
		defer transport.CloseIdleConnections()
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("official IPQuality endpoint returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxScriptBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) == 0 || len(body) > maxScriptBytes {
		return nil, errors.New("official IPQuality script is empty or exceeds 2 MiB")
	}
	return body, nil
}

func pathHTTPClient(path executionPath, adapterURL string) *http.Client {
	transport := &http.Transport{
		DisableKeepAlives:     true,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
	}
	if adapterURL != "" {
		proxyURL, _ := url.Parse(adapterURL)
		transport.Proxy = http.ProxyURL(proxyURL)
	} else {
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
		transport.DialContext = func(ctx context.Context, _, address string) (net.Conn, error) {
			network := "tcp4"
			if path.egress.Family == "ipv6" {
				network = "tcp6"
			}
			return dialer.DialContext(ctx, network, address)
		}
	}
	return &http.Client{
		Transport: transport,
		CheckRedirect: func(_ *http.Request, previous []*http.Request) error {
			if len(previous) >= 5 {
				return errors.New("official IPQuality endpoint redirected too many times")
			}
			return nil
		},
	}
}

type boundedCapture struct {
	mu         sync.Mutex
	buffer     bytes.Buffer
	limit      int
	retainTail bool
	overflow   chan struct{}
	overflowed bool
}

func newBoundedCapture(limit int) *boundedCapture {
	return &boundedCapture{limit: limit, overflow: make(chan struct{})}
}

func newTailCapture(limit int) *boundedCapture {
	return &boundedCapture{limit: limit, retainTail: true, overflow: make(chan struct{})}
}

func (capture *boundedCapture) Write(value []byte) (int, error) {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	if capture.retainTail {
		return capture.writeTail(value)
	}
	remaining := capture.limit - capture.buffer.Len()
	if remaining > 0 {
		retained := min(remaining, len(value))
		_, _ = capture.buffer.Write(value[:retained])
	}
	if len(value) > remaining && !capture.overflowed {
		capture.overflowed = true
		close(capture.overflow)
	}
	return len(value), nil
}

func (capture *boundedCapture) writeTail(value []byte) (int, error) {
	written := len(value)
	overflow := capture.buffer.Len()+written > capture.limit
	if written >= capture.limit {
		capture.buffer.Reset()
		if capture.limit > 0 {
			_, _ = capture.buffer.Write(value[written-capture.limit:])
		}
	} else {
		discard := capture.buffer.Len() + written - capture.limit
		if discard > 0 {
			capture.buffer.Next(discard)
		}
		_, _ = capture.buffer.Write(value)
	}
	if overflow && !capture.overflowed {
		capture.overflowed = true
		close(capture.overflow)
	}
	return written, nil
}

func (capture *boundedCapture) Bytes() []byte {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return append([]byte(nil), capture.buffer.Bytes()...)
}

func (capture *boundedCapture) String() string {
	return string(capture.Bytes())
}

func (capture *boundedCapture) Overflow() <-chan struct{} {
	return capture.overflow
}

func (capture *boundedCapture) Overflowed() bool {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return capture.overflowed
}

func validateProbeJSON(raw []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(raw)
	trimmed = bytes.TrimPrefix(trimmed, []byte(ipQualityClearPrefix))
	trimmed = bytes.TrimSpace(trimmed)
	if len(trimmed) == 0 {
		return nil, errors.New("IPQuality produced no JSON output")
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	var document map[string]json.RawMessage
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("IPQuality output is not a JSON object: %w", err)
	}
	if document == nil {
		return nil, errors.New("IPQuality output must be a JSON object")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("IPQuality output contains trailing non-JSON data")
	}
	return trimmed, nil
}

func sanitizeDiagnostic(value string, configuration state.Configuration) string {
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

func joinDiagnostic(parts ...string) string {
	var retained []string
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			retained = append(retained, value)
		}
	}
	return strings.Join(retained, "\n")
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
