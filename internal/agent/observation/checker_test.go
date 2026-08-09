package observation

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	agentnetwork "github.com/ipchronicle/ipchronicle/internal/agent/network"
	"github.com/ipchronicle/ipchronicle/internal/agent/state"
)

func TestCheckerConfirmsFirstAndChangedAddressesButNotUnchanged(t *testing.T) {
	first := newMutableEchoService(t, "8.8.8.8", http.StatusOK)
	second := newMutableEchoService(t, "8.8.8.8", http.StatusOK)
	checker := NewChecker()
	checker.discover = func() (agentnetwork.Inventory, error) { return testIPv4Inventory(), nil }
	configuration := testConfiguration([]string{first.server.URL, second.server.URL})
	egress := configuration.Egresses[0]
	checkedAt := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)

	initial := checker.Check(context.Background(), configuration, egress, nil, checkedAt)
	if !initial.Confirmed || initial.PublicAddress != "8.8.8.8" || !initial.LikelyNAT {
		t.Fatalf("initial observation = %#v", initial)
	}
	if first.Count() != 1 || second.Count() != 1 {
		t.Fatalf("initial provider calls = %d, %d; want 1, 1", first.Count(), second.Count())
	}

	previous := addressState(initial)
	unchanged := checker.Check(context.Background(), configuration, egress, &previous, checkedAt.Add(time.Minute))
	if !unchanged.Confirmed || unchanged.PublicAddress != "8.8.8.8" {
		t.Fatalf("unchanged observation = %#v", unchanged)
	}
	if first.Count() != 2 || second.Count() != 1 {
		t.Fatalf("unchanged provider calls = %d, %d; want 2, 1", first.Count(), second.Count())
	}

	first.Set("1.1.1.1", http.StatusOK)
	second.Set("1.1.1.1", http.StatusOK)
	changed := checker.Check(context.Background(), configuration, egress, &previous, checkedAt.Add(2*time.Minute))
	if !changed.Confirmed || changed.PublicAddress != "1.1.1.1" {
		t.Fatalf("changed observation = %#v", changed)
	}
	if first.Count() != 3 || second.Count() != 2 {
		t.Fatalf("changed provider calls = %d, %d; want 3, 2", first.Count(), second.Count())
	}
}

func TestCheckerClassifiesProviderFailures(t *testing.T) {
	tests := []struct {
		name         string
		firstBody    string
		firstStatus  int
		secondBody   string
		secondStatus int
		want         string
	}{
		{
			name: "all invalid", firstBody: "not-an-address", firstStatus: http.StatusOK,
			secondBody: "10.0.0.1", secondStatus: http.StatusOK, want: "no-valid-response",
		},
		{
			name: "confirmation unavailable", firstBody: "8.8.8.8", firstStatus: http.StatusOK,
			secondBody: "unavailable", secondStatus: http.StatusServiceUnavailable, want: "confirmation-unavailable",
		},
		{
			name: "conflicting responses", firstBody: "8.8.8.8", firstStatus: http.StatusOK,
			secondBody: "1.1.1.1", secondStatus: http.StatusOK, want: "conflicting-responses",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			first := newMutableEchoService(t, test.firstBody, test.firstStatus)
			second := newMutableEchoService(t, test.secondBody, test.secondStatus)
			checker := NewChecker()
			checker.discover = func() (agentnetwork.Inventory, error) { return testIPv4Inventory(), nil }
			configuration := testConfiguration([]string{first.server.URL, second.server.URL})
			observation := checker.Check(context.Background(), configuration, configuration.Egresses[0], nil, time.Now())
			if observation.Confirmed || observation.FailureReason != test.want {
				t.Fatalf("observation = %#v, want failure %q", observation, test.want)
			}
		})
	}
}

func TestCheckerRejectsUnavailableSelector(t *testing.T) {
	checker := NewChecker()
	checker.discover = func() (agentnetwork.Inventory, error) {
		inventory := testIPv4Inventory()
		inventory.Interfaces[0].Up = false
		return inventory, nil
	}
	configuration := testConfiguration([]string{"http://one.invalid", "http://two.invalid"})
	observation := checker.Check(context.Background(), configuration, configuration.Egresses[0], nil, time.Now())
	if observation.Confirmed || observation.FailureReason != "selector-unavailable" {
		t.Fatalf("observation = %#v", observation)
	}
}

func TestCheckerPreservesSelectedSourceAndLocalMapping(t *testing.T) {
	checker := NewChecker()
	inventory := testIPv4Inventory()
	checker.discover = func() (agentnetwork.Inventory, error) { return inventory, nil }
	interfaceName := "eth0"
	sourceAddress := "10.0.0.5"
	egress := state.Egress{
		ID: "d099bad9-e7c4-42a9-bd19-ad85408321c5", Kind: "source", Family: "ipv4",
		InterfaceName: &interfaceName, SourceAddress: &sourceAddress, Enabled: true,
		LightweightIntervalSeconds: 600, ProbeOnAddressChange: true,
	}
	path, err := checker.selectPath(state.Configuration{}, egress)
	if err != nil {
		t.Fatal(err)
	}
	if path.interfaceName != interfaceName || path.sourceAddress == nil || path.sourceAddress.String() != sourceAddress || !path.bindInterface {
		t.Fatalf("selected source path = %#v", path)
	}

	observation := state.AddressObservation{}
	checker.applyLocalMapping(&observation, inventory, interfaceName, netip.MustParseAddr(sourceAddress), netip.MustParseAddr("8.8.8.8"))
	if observation.LocalInterface == nil || *observation.LocalInterface != interfaceName ||
		observation.LocalAddress == nil || *observation.LocalAddress != sourceAddress || !observation.LikelyNAT || observation.Temporary {
		t.Fatalf("NAT mapping = %#v", observation)
	}

	temporaryInventory := agentnetwork.Inventory{Addresses: []agentnetwork.Address{{
		Interface: "eth1", Address: "2001:4860:4860::8844", Family: agentnetwork.IPv6,
		Scope: "global", Temporary: true,
	}}}
	observation = state.AddressObservation{}
	public := netip.MustParseAddr("2001:4860:4860::8844")
	checker.applyLocalMapping(&observation, temporaryInventory, "eth1", public, public)
	if observation.LikelyNAT || !observation.Temporary || observation.LocalInterface == nil || *observation.LocalInterface != "eth1" {
		t.Fatalf("temporary direct IPv6 mapping = %#v", observation)
	}
}

func TestQueryServiceEnforcesResponseBoundary(t *testing.T) {
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(response, "8.8.8.8")
	}))
	t.Cleanup(redirectTarget.Close)
	tests := []struct {
		name   string
		status int
		body   string
		header http.Header
	}{
		{name: "private address", status: http.StatusOK, body: "10.0.0.1"},
		{name: "shared address", status: http.StatusOK, body: "100.64.0.1"},
		{name: "wrong family", status: http.StatusOK, body: "2001:4860:4860::8888"},
		{name: "oversized", status: http.StatusOK, body: strings.Repeat("1", maxResponseSize+1)},
		{name: "redirect", status: http.StatusFound, header: http.Header{"Location": []string{redirectTarget.URL}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				for name, values := range test.header {
					for _, value := range values {
						response.Header().Add(name, value)
					}
				}
				response.WriteHeader(test.status)
				_, _ = io.WriteString(response, test.body)
			}))
			defer server.Close()
			_, err := queryService(context.Background(), selectedPath{egress: state.Egress{Family: "ipv4"}}, server.URL)
			if err == nil {
				t.Fatal("invalid address service response was accepted")
			}
		})
	}
}

func TestHTTPAndHTTPSProxyTransportsAuthenticate(t *testing.T) {
	for _, scheme := range []string{"http", "https"} {
		t.Run(scheme, func(t *testing.T) {
			username := "proxy-user"
			password := "proxy-password"
			wantAuthorization := "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password))
			origin := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(response, "8.8.8.8")
			}))
			defer origin.Close()
			authorization := make(chan string, 1)
			handler := connectProxyHandler(authorization)
			var server *httptest.Server
			if scheme == "https" {
				server = httptest.NewTLSServer(handler)
			} else {
				server = httptest.NewServer(handler)
			}
			defer server.Close()
			parsed, err := url.Parse(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			host, rawPort, err := net.SplitHostPort(parsed.Host)
			if err != nil {
				t.Fatal(err)
			}
			port, err := net.LookupPort("tcp", rawPort)
			if err != nil {
				t.Fatal(err)
			}
			path := selectedPath{egress: state.Egress{Kind: "proxy", Family: "ipv4"}, proxy: &state.Proxy{
				Scheme: scheme, Host: host, Port: int64(port), Username: &username, Password: &password,
			}}
			transport, err := transportForPath(path)
			if err != nil {
				t.Fatal(err)
			}
			defer transport.CloseIdleConnections()
			roots := x509.NewCertPool()
			roots.AddCert(origin.Certificate())
			if scheme == "https" {
				roots.AddCert(server.Certificate())
			}
			transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
			client := &http.Client{Transport: transport, Timeout: time.Second}
			response, err := client.Get(origin.URL)
			if err != nil {
				t.Fatal(err)
			}
			body, readErr := io.ReadAll(response.Body)
			_ = response.Body.Close()
			var gotAuthorization string
			select {
			case gotAuthorization = <-authorization:
			case <-time.After(time.Second):
				t.Fatal("proxy did not receive CONNECT request")
			}
			if readErr != nil || string(body) != "8.8.8.8" || gotAuthorization != wantAuthorization {
				t.Fatalf("proxy response = %q, auth = %q, read error = %v", body, gotAuthorization, readErr)
			}
		})
	}
}

func TestSOCKS5ProxyTransportAuthenticatesAndCarriesTraffic(t *testing.T) {
	origin := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(response, "8.8.8.8")
	}))
	defer origin.Close()
	username := "socks-user"
	password := "socks-password"
	proxyAddress := startSOCKS5Proxy(t, username, password)
	host, rawPort, err := net.SplitHostPort(proxyAddress)
	if err != nil {
		t.Fatal(err)
	}
	port, err := net.LookupPort("tcp", rawPort)
	if err != nil {
		t.Fatal(err)
	}
	path := selectedPath{egress: state.Egress{Kind: "proxy", Family: "ipv4"}, proxy: &state.Proxy{
		Scheme: "socks5", Host: host, Port: int64(port), Username: &username, Password: &password,
	}}
	transport, err := transportForPath(path)
	if err != nil {
		t.Fatal(err)
	}
	defer transport.CloseIdleConnections()
	roots := x509.NewCertPool()
	roots.AddCert(origin.Certificate())
	transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	response, err := (&http.Client{Transport: transport, Timeout: time.Second}).Get(origin.URL)
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if readErr != nil || string(body) != "8.8.8.8" {
		t.Fatalf("SOCKS5 response = %q, read error = %v", body, readErr)
	}
}

func TestProxyConnectionErrorsDoNotExposeCredentials(t *testing.T) {
	username := "secret-user"
	password := "secret-password"
	path := selectedPath{egress: state.Egress{Kind: "proxy", Family: "ipv4"}, proxy: &state.Proxy{
		Scheme: "http", Host: "127.0.0.1", Port: 1, Username: &username, Password: &password,
	}}
	_, err := queryService(context.Background(), path, "http://origin.invalid/address")
	if err == nil {
		t.Fatal("unreachable proxy unexpectedly succeeded")
	}
	if strings.Contains(err.Error(), username) || strings.Contains(err.Error(), password) {
		t.Fatalf("proxy credentials appeared in error: %v", err)
	}
}

type mutableEchoService struct {
	server *httptest.Server
	mu     sync.Mutex
	body   string
	status int
	count  int
}

func newMutableEchoService(t *testing.T, body string, status int) *mutableEchoService {
	t.Helper()
	service := &mutableEchoService{body: body, status: status}
	service.server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		service.mu.Lock()
		defer service.mu.Unlock()
		service.count++
		response.WriteHeader(service.status)
		_, _ = io.WriteString(response, service.body)
	}))
	t.Cleanup(service.server.Close)
	return service
}

func (s *mutableEchoService) Set(body string, status int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.body = body
	s.status = status
}

func (s *mutableEchoService) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.count
}

func testConfiguration(services []string) state.Configuration {
	return state.Configuration{
		SchemaVersion: 5, Revision: 1, Enabled: true,
		ProbeSchedule:     state.ProbeSchedule{Enabled: true, Cron: "0 0 0 * * *", Timezone: "agent-local"},
		HistoryGeneration: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		DiscoveryServices: state.DiscoveryServices{IPv4: services, IPv6: []string{"https://six-one.invalid", "https://six-two.invalid"}},
		Egresses: []state.Egress{{
			ID: "d099bad9-e7c4-42a9-bd19-ad85408321c5", Kind: "default", Family: "ipv4", Enabled: true,
			LightweightIntervalSeconds: 600, ProbeOnAddressChange: true,
		}},
	}
}

func testIPv4Inventory() agentnetwork.Inventory {
	return agentnetwork.Inventory{
		Interfaces: []agentnetwork.Interface{{Name: "eth0", Index: 2, Up: true}},
		Addresses: []agentnetwork.Address{{
			Interface: "eth0", Address: "10.0.0.5", PrefixLength: 24, Family: agentnetwork.IPv4, Scope: "private",
		}},
		Routes: []agentnetwork.Route{{
			Interface: "eth0", Family: agentnetwork.IPv4, Destination: "0.0.0.0/0", Metric: 100, Default: true,
		}},
	}
}

func addressState(observation state.AddressObservation) state.AddressState {
	publicAddress := observation.PublicAddress
	return state.AddressState{
		EgressID: observation.EgressID, HistoryGeneration: observation.HistoryGeneration,
		Family: observation.Family, Status: "confirmed", PublicAddress: &publicAddress,
	}
}

func startSOCKS5Proxy(t *testing.T, username, password string) string {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	done := make(chan error, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			done <- acceptErr
			return
		}
		defer connection.Close()
		done <- serveSOCKS5Connection(connection, username, password)
	}()
	t.Cleanup(func() {
		select {
		case serveErr := <-done:
			if serveErr != nil && !expectedNetworkClose(serveErr) {
				t.Errorf("SOCKS5 proxy: %v", serveErr)
			}
		case <-time.After(time.Second):
			t.Error("SOCKS5 proxy did not stop")
		}
	})
	return listener.Addr().String()
}

func connectProxyHandler(authorization chan<- string) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodConnect {
			http.Error(response, "CONNECT required", http.StatusMethodNotAllowed)
			return
		}
		select {
		case authorization <- request.Header.Get("Proxy-Authorization"):
		default:
		}
		upstream, err := net.DialTimeout("tcp", request.Host, time.Second)
		if err != nil {
			http.Error(response, "upstream unavailable", http.StatusBadGateway)
			return
		}
		defer upstream.Close()
		hijacker, ok := response.(http.Hijacker)
		if !ok {
			http.Error(response, "hijacking unavailable", http.StatusInternalServerError)
			return
		}
		client, _, err := hijacker.Hijack()
		if err != nil {
			return
		}
		defer client.Close()
		if _, err := io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
			return
		}
		copyDone := make(chan error, 1)
		go func() {
			_, copyErr := io.Copy(upstream, client)
			copyDone <- copyErr
		}()
		_, downstreamErr := io.Copy(client, upstream)
		_ = upstream.Close()
		_ = client.Close()
		upstreamErr := <-copyDone
		_ = downstreamErr
		_ = upstreamErr
	})
}

func expectedNetworkClose(err error) bool {
	return errors.Is(err, net.ErrClosed) || errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE)
}

func serveSOCKS5Connection(connection net.Conn, username, password string) error {
	_ = connection.SetDeadline(time.Now().Add(3 * time.Second))
	reader := bufio.NewReader(connection)
	header := make([]byte, 2)
	if _, err := io.ReadFull(reader, header); err != nil {
		return err
	}
	methods := make([]byte, int(header[1]))
	if header[0] != 5 {
		return fmt.Errorf("SOCKS version = %d", header[0])
	}
	if _, err := io.ReadFull(reader, methods); err != nil {
		return err
	}
	if !strings.ContainsRune(string(methods), rune(2)) {
		return errors.New("SOCKS client did not offer username/password authentication")
	}
	if _, err := connection.Write([]byte{5, 2}); err != nil {
		return err
	}
	if err := verifySOCKS5Credentials(reader, connection, username, password); err != nil {
		return err
	}
	target, err := readSOCKS5Target(reader)
	if err != nil {
		return err
	}
	upstream, err := net.DialTimeout("tcp", target, time.Second)
	if err != nil {
		return err
	}
	defer upstream.Close()
	if _, err := connection.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0}); err != nil {
		return err
	}
	_ = connection.SetDeadline(time.Time{})
	errorChannel := make(chan error, 1)
	go func() {
		_, copyErr := io.Copy(upstream, reader)
		errorChannel <- copyErr
	}()
	_, downstreamErr := io.Copy(connection, upstream)
	_ = upstream.Close()
	upstreamErr := <-errorChannel
	if downstreamErr != nil {
		if !expectedNetworkClose(downstreamErr) {
			return downstreamErr
		}
	}
	if expectedNetworkClose(upstreamErr) {
		return nil
	}
	return upstreamErr
}

func verifySOCKS5Credentials(reader *bufio.Reader, connection net.Conn, username, password string) error {
	header := make([]byte, 2)
	if _, err := io.ReadFull(reader, header); err != nil {
		return err
	}
	user := make([]byte, int(header[1]))
	if header[0] != 1 {
		return fmt.Errorf("SOCKS auth version = %d", header[0])
	}
	if _, err := io.ReadFull(reader, user); err != nil {
		return err
	}
	passwordLength, err := reader.ReadByte()
	if err != nil {
		return err
	}
	secret := make([]byte, int(passwordLength))
	if _, err := io.ReadFull(reader, secret); err != nil {
		return err
	}
	status := byte(0)
	if string(user) != username || string(secret) != password {
		status = 1
	}
	if _, err := connection.Write([]byte{1, status}); err != nil {
		return err
	}
	if status != 0 {
		return errors.New("SOCKS credentials do not match")
	}
	return nil
}

func readSOCKS5Target(reader *bufio.Reader) (string, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(reader, header); err != nil {
		return "", err
	}
	if header[0] != 5 || header[1] != 1 {
		return "", fmt.Errorf("unsupported SOCKS request %v", header)
	}
	var host string
	switch header[3] {
	case 1:
		address := make([]byte, net.IPv4len)
		if _, err := io.ReadFull(reader, address); err != nil {
			return "", err
		}
		host = net.IP(address).String()
	case 3:
		length, err := reader.ReadByte()
		if err != nil {
			return "", err
		}
		name := make([]byte, int(length))
		if _, err := io.ReadFull(reader, name); err != nil {
			return "", err
		}
		host = string(name)
	case 4:
		address := make([]byte, net.IPv6len)
		if _, err := io.ReadFull(reader, address); err != nil {
			return "", err
		}
		host = net.IP(address).String()
	default:
		return "", fmt.Errorf("unsupported SOCKS address type %d", header[3])
	}
	portBytes := make([]byte, 2)
	if _, err := io.ReadFull(reader, portBytes); err != nil {
		return "", err
	}
	return net.JoinHostPort(host, fmt.Sprintf("%d", binary.BigEndian.Uint16(portBytes))), nil
}
