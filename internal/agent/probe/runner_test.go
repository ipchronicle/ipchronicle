package probe

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	agentnetwork "github.com/ipchronicle/ipchronicle/internal/agent/network"
	"github.com/ipchronicle/ipchronicle/internal/agent/state"
)

func TestRunnerExecutesOneBoundedJSONProbe(t *testing.T) {
	ipapiAPIKey := "test-ipapi-key"
	runner := testRunner(t, func(_ context.Context, input nativeProbeInput) ([]byte, error) {
		if input.Target != "203.0.113.10" || input.Family != "ipv4" || input.HTTPClient == nil ||
			input.ExplicitLookupHTTPClient == nil || input.DialContext == nil || input.IPAPIAPIKey != ipapiAPIKey {
			t.Fatalf("native input = %#v", input)
		}
		return []byte(`{"field":{"value":1}}`), nil
	})
	configuration, egress := probeTestConfiguration("default", "ipv4")
	configuration.IPAPIAPIKey = ipapiAPIKey
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
}

func TestRunnerStopsNativeProbeAtTimeout(t *testing.T) {
	runner := testRunner(t, func(ctx context.Context, _ nativeProbeInput) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	runner.timeout = 50 * time.Millisecond
	configuration, egress := probeTestConfiguration("default", "ipv4")
	runner.discover = func() (agentnetwork.Inventory, error) { return probeTestInventory(), nil }
	started := time.Now()
	outcome, err := runner.Run(context.Background(), configuration, egress, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(started) > time.Second || outcome.Status != "failed" || outcome.FailureStage != "timeout" {
		t.Fatalf("outcome = %#v", outcome)
	}
}

func TestRunnerReportsInvalidAndOversizedJSON(t *testing.T) {
	tests := []struct {
		name       string
		result     []byte
		diagnostic string
	}{
		{name: "invalid", result: []byte("not-json"), diagnostic: "not a JSON object"},
		{name: "oversized", result: make([]byte, state.MaxProbeResultBytes+1), diagnostic: "exceeds 1 MiB"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := testRunner(t, func(context.Context, nativeProbeInput) ([]byte, error) {
				return test.result, nil
			})
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
	if path.sourceAddress == nil || path.sourceAddress.String() != "2001:db8::10" ||
		path.interfaceName != "eth0" || !path.bindInterface {
		t.Fatalf("interface path = %#v", path)
	}

	password := "proxy-secret-value"
	ipapiAPIKey := "ipapi-secret-value"
	proxyID := uuid.NewString()
	configuration.Proxies = []state.Proxy{{
		ID: proxyID, Scheme: "http", Host: "proxy.example", Port: 8080, Password: &password,
	}}
	configuration.IPAPIAPIKey = ipapiAPIKey
	egress.Kind = "proxy"
	egress.ProxyID = &proxyID
	egress.InterfaceName = nil
	path, err = selectExecutionPath(configuration, egress, agentnetwork.Inventory{})
	if err != nil {
		t.Fatal(err)
	}
	if path.proxy == nil || path.proxy.ID != proxyID {
		t.Fatalf("proxy path = %#v", path)
	}
	if value := sanitizeDiagnostic("failed password="+password+" key="+ipapiAPIKey, configuration); strings.Contains(value, password) || strings.Contains(value, ipapiAPIKey) {
		t.Fatalf("diagnostic contains a retained secret: %q", value)
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

func testRunner(t *testing.T, execute func(context.Context, nativeProbeInput) ([]byte, error)) *Runner {
	t.Helper()
	runner := NewRunner()
	runner.verifyTarget = func(context.Context, state.Configuration, state.Egress, time.Time) error { return nil }
	runner.execute = execute
	return runner
}

func probeTestConfiguration(kind, family string) (state.Configuration, state.Egress) {
	pathID := uuid.NewString()
	publicAddress := "203.0.113.10"
	if family == "ipv6" {
		publicAddress = "2001:db8::10"
	}
	egress := state.Egress{
		ID: uuid.NewString(), PathID: &pathID, PublicAddress: &publicAddress,
		Kind: kind, Family: family, Enabled: true,
	}
	return state.Configuration{
		SchemaVersion: 9, Revision: 1, Enabled: true,
		HistoryGeneration: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ProbeTargets:      []state.Egress{egress},
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
