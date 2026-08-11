package probe

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/google/uuid"
	agentnetwork "github.com/ipchronicle/ipchronicle/internal/agent/network"
	"github.com/ipchronicle/ipchronicle/internal/agent/state"
)

func TestRunnerExecutesOneBoundedJSONProbe(t *testing.T) {
	processes := &fakeProcessState{}
	runner := testRunner(t, processes, `printf '%s\n' '{"field":{"value":1}}'`)
	configuration, egress := probeTestConfiguration("default", "ipv4")
	runner.discover = func() (agentnetwork.Inventory, error) { return probeTestInventory(), nil }
	startedAt := time.Now().UTC().Truncate(time.Second)
	outcome, err := runner.Run(context.Background(), configuration, egress, startedAt)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != "succeeded" || string(outcome.RawResult) != `{"field":{"value":1}}` ||
		outcome.StartedAt == nil || !outcome.StartedAt.Equal(startedAt) {
		t.Fatalf("outcome = %#v", outcome)
	}
	if processes.setCount != 1 || processes.clearCount != 1 || processes.current != nil {
		t.Fatalf("process state = %#v", processes)
	}
}

func TestRunnerAcceptsIPQualitySingleFamilyCompletion(t *testing.T) {
	runner := testRunner(t, &fakeProcessState{}, `printf '\033[H\033[J\r\r%s\r\n' '{"field":{"value":1}}'; exit 1`)
	configuration, egress := probeTestConfiguration("default", "ipv4")
	runner.discover = func() (agentnetwork.Inventory, error) { return probeTestInventory(), nil }
	outcome, err := runner.Run(context.Background(), configuration, egress, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != "succeeded" || string(outcome.RawResult) != `{"field":{"value":1}}` {
		t.Fatalf("outcome = %#v", outcome)
	}
}

func TestRunnerReportsInvalidAndOversizedJSON(t *testing.T) {
	tests := []struct {
		name       string
		script     string
		diagnostic string
	}{
		{name: "invalid", script: `printf 'not-json'`, diagnostic: "not a JSON object"},
		{name: "oversized", script: `printf '{"value":"'; head -c 1048577 /dev/zero | tr '\0' x; printf '"}'`, diagnostic: "exceeded 1 MiB"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := testRunner(t, &fakeProcessState{}, test.script)
			configuration, egress := probeTestConfiguration("default", "ipv4")
			runner.discover = func() (agentnetwork.Inventory, error) { return probeTestInventory(), nil }
			outcome, err := runner.Run(context.Background(), configuration, egress, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if outcome.Status != "failed" || outcome.FailureStage != "output" ||
				!strings.Contains(outcome.Diagnostic, test.diagnostic) || len(outcome.RawResult) != 0 {
				t.Fatalf("outcome = %#v", outcome)
			}
		})
	}
}

func TestTailCaptureRetainsRecentDiagnostic(t *testing.T) {
	capture := newTailCapture(5)
	if _, err := capture.Write([]byte("abc")); err != nil {
		t.Fatal(err)
	}
	if _, err := capture.Write([]byte("defg")); err != nil {
		t.Fatal(err)
	}
	if got := capture.String(); got != "cdefg" || !capture.Overflowed() {
		t.Fatalf("capture = %q, overflowed = %t", got, capture.Overflowed())
	}
}

func TestRunnerClearsProxyAndShellHookEnvironment(t *testing.T) {
	for _, name := range []string{
		"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY",
		"http_proxy", "https_proxy", "all_proxy", "no_proxy", "BASH_ENV", "ENV",
	} {
		t.Setenv(name, "must-not-reach-probe")
	}
	t.Setenv("IPCHRONICLE_RUNNER_TEST", "retained")
	runner := testRunner(t, &fakeProcessState{}, `
if env | grep -Eq '^(HTTP_PROXY|HTTPS_PROXY|ALL_PROXY|NO_PROXY|http_proxy|https_proxy|all_proxy|no_proxy|BASH_ENV|ENV)='; then
  printf 'blocked environment leaked' >&2
  exit 1
fi
printf '{"retained":"%s"}' "$IPCHRONICLE_RUNNER_TEST"
`)
	configuration, egress := probeTestConfiguration("default", "ipv4")
	runner.discover = func() (agentnetwork.Inventory, error) { return probeTestInventory(), nil }
	outcome, err := runner.Run(context.Background(), configuration, egress, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != "succeeded" || string(outcome.RawResult) != `{"retained":"retained"}` {
		t.Fatalf("outcome = %#v", outcome)
	}
}

func TestRunnerTerminatesProcessGroupAtTimeout(t *testing.T) {
	processes := &fakeProcessState{}
	runner := testRunner(t, processes, `sleep 30`)
	runner.timeout = 100 * time.Millisecond
	configuration, egress := probeTestConfiguration("default", "ipv4")
	runner.discover = func() (agentnetwork.Inventory, error) { return probeTestInventory(), nil }
	started := time.Now()
	outcome, err := runner.Run(context.Background(), configuration, egress, started)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(started) > 2*time.Second {
		t.Fatal("timed-out process group did not terminate promptly")
	}
	if outcome.Status != "failed" || outcome.FailureStage != "timeout" || processes.current != nil {
		t.Fatalf("outcome = %#v, process = %#v", outcome, processes.current)
	}
}

func TestRecoverRetainedProcessTerminatesMatchingProcessGroup(t *testing.T) {
	command := exec.Command("bash", "-c", "sleep 30")
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	identity, err := currentProcessIdentity(command.Process.Pid, command.Process.Pid, time.Now())
	if err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		t.Fatal(err)
	}
	processes := &fakeProcessState{current: &identity}
	waited := make(chan error, 1)
	go func() { waited <- command.Wait() }()
	if err := recoverRetainedProcess(context.Background(), processes); err != nil {
		t.Fatal(err)
	}
	select {
	case <-waited:
	case <-time.After(2 * time.Second):
		t.Fatal("retained process was not terminated")
	}
	if processes.current != nil || processes.clearCount != 1 {
		t.Fatalf("retained process state = %#v", processes)
	}
}

func TestExecutionPathUsesOneDeterministicSourceAndHidesProxyCredentials(t *testing.T) {
	configuration, egress := probeTestConfiguration("interface", "ipv6")
	interfaceName := "eth0"
	egress.InterfaceName = &interfaceName
	inventory := probeTestInventory()
	inventory.Addresses = append(inventory.Addresses,
		agentnetwork.Address{Interface: "eth0", Address: "2001:db8::20", Family: agentnetwork.IPv6, Scope: "global", Temporary: true},
		agentnetwork.Address{Interface: "eth0", Address: "2001:db8::10", Family: agentnetwork.IPv6, Scope: "global"},
	)
	inventory.Routes = append(inventory.Routes, agentnetwork.Route{Interface: "eth0", Family: agentnetwork.IPv6, Destination: "::/0", Default: true})
	path, err := selectExecutionPath(configuration, egress, inventory)
	if err != nil {
		t.Fatal(err)
	}
	arguments, err := path.scriptArguments("")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(arguments, " ") != "-n -j -p -i 2001:db8::10" {
		t.Fatalf("interface arguments = %q", arguments)
	}

	password := "proxy-secret-value"
	proxyID := uuid.NewString()
	configuration.Proxies = []state.Proxy{{
		ID: proxyID, Scheme: "http", Host: "proxy.example", Port: 8080, Password: &password,
	}}
	egress.Kind = "proxy"
	egress.ProxyID = &proxyID
	egress.InterfaceName = nil
	path, err = selectExecutionPath(configuration, egress, agentnetwork.Inventory{})
	if err != nil {
		t.Fatal(err)
	}
	arguments, err = path.scriptArguments("http://127.0.0.1:12345")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(arguments, " ")
	if strings.Contains(joined, password) || joined != "-n -j -p -6 -x http://127.0.0.1:12345" {
		t.Fatalf("proxy arguments = %q", arguments)
	}
	if value := sanitizeDiagnostic("failed password="+password, configuration); strings.Contains(value, password) {
		t.Fatalf("diagnostic contains proxy password: %q", value)
	}
}

func TestLocalProxyAdapterAuthenticatesHTTPAndConnect(t *testing.T) {
	target := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte("through-proxy"))
	}))
	defer target.Close()
	username := "agent"
	password := "secret"
	wantAuthorization := "Basic " + basicToken(username, password)
	upstream := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Proxy-Authorization") != wantAuthorization {
			response.WriteHeader(http.StatusProxyAuthRequired)
			return
		}
		if request.Method != http.MethodConnect {
			response.WriteHeader(http.StatusBadRequest)
			return
		}
		destination, err := net.DialTimeout("tcp", request.Host, time.Second)
		if err != nil {
			response.WriteHeader(http.StatusBadGateway)
			return
		}
		client, _, err := response.(http.Hijacker).Hijack()
		if err != nil {
			_ = destination.Close()
			return
		}
		_, _ = client.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
		go proxyCopy(destination, client)
		go proxyCopy(client, destination)
	}))
	defer upstream.Close()
	parsed, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	host, portText, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatal(err)
	}
	var port int64
	if _, err := fmt.Sscan(portText, &port); err != nil {
		t.Fatal(err)
	}
	adapter, err := startLocalProxyAdapter(state.Proxy{
		Scheme: "http", Host: host, Port: port, Username: &username, Password: &password,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer adapter.Close()
	adapterURL, err := url.Parse(adapter.URL())
	if err != nil {
		t.Fatal(err)
	}
	client := target.Client()
	client.Transport = &http.Transport{
		Proxy: http.ProxyURL(adapterURL), TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // test target only
	}
	response, err := client.Get(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "through-proxy" {
		t.Fatalf("response body = %q", body)
	}
}

func testRunner(t *testing.T, processes processState, script string) *Runner {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "text/plain")
		_, _ = response.Write([]byte(script))
	}))
	t.Cleanup(server.Close)
	runner := NewRunner(processes)
	runner.scriptURL = server.URL
	runner.httpClient = func(executionPath, string) *http.Client { return server.Client() }
	return runner
}

func probeTestConfiguration(kind, family string) (state.Configuration, state.Egress) {
	egress := state.Egress{ID: uuid.NewString(), Kind: kind, Family: family, Enabled: true}
	return state.Configuration{
		SchemaVersion: 5, Revision: 1, Enabled: true,
		HistoryGeneration: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Egresses:          []state.Egress{egress},
	}, egress
}

func probeTestInventory() agentnetwork.Inventory {
	return agentnetwork.Inventory{
		Interfaces: []agentnetwork.Interface{{Name: "eth0", Index: 2, Up: true}},
		Addresses: []agentnetwork.Address{{
			Interface: "eth0", Address: "192.0.2.10", Family: agentnetwork.IPv4, Scope: "global",
		}},
		Routes: []agentnetwork.Route{{
			Interface: "eth0", Family: agentnetwork.IPv4, Destination: "0.0.0.0/0", Default: true,
		}},
	}
}

type fakeProcessState struct {
	mu         sync.Mutex
	current    *state.ProbeProcess
	setCount   int
	clearCount int
}

func (store *fakeProcessState) SetProbeProcess(process state.ProbeProcess) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.current != nil {
		return errors.New("process already retained")
	}
	value := process
	store.current = &value
	store.setCount++
	return nil
}

func (store *fakeProcessState) ClearProbeProcess(processGroupID int) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.current == nil || store.current.ProcessGroupID != processGroupID {
		return errors.New("wrong process group cleared")
	}
	store.current = nil
	store.clearCount++
	return nil
}

func (store *fakeProcessState) ProbeProcess() (*state.ProbeProcess, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.current == nil {
		return nil, nil
	}
	value := *store.current
	return &value, nil
}

func basicToken(username, password string) string {
	request, _ := http.NewRequest(http.MethodGet, "http://example.test", nil)
	request.SetBasicAuth(username, password)
	return strings.TrimPrefix(request.Header.Get("Authorization"), "Basic ")
}

func proxyCopy(destination, source net.Conn) {
	defer destination.Close()
	defer source.Close()
	_, _ = io.Copy(destination, source)
}
