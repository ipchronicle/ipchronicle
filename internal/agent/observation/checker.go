package observation

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"

	agentnetwork "github.com/ipchronicle/ipchronicle/internal/agent/network"
	"github.com/ipchronicle/ipchronicle/internal/agent/state"
	xproxy "golang.org/x/net/proxy"
	"golang.org/x/sys/unix"
)

const (
	requestTimeout  = 8 * time.Second
	maxResponseSize = 128
)

var sharedIPv4Prefix = netip.MustParsePrefix("100.64.0.0/10")

type Checker struct {
	discover func() (agentnetwork.Inventory, error)
}

type selectedPath struct {
	egress        state.Egress
	proxy         *state.Proxy
	inventory     agentnetwork.Inventory
	interfaceName string
	sourceAddress *netip.Addr
	bindInterface bool
}

type echoResult struct {
	address      netip.Addr
	localAddress netip.Addr
}

func NewChecker() *Checker {
	return &Checker{discover: agentnetwork.Discover}
}

func (c *Checker) Check(ctx context.Context, configuration state.Configuration, egress state.Egress, previous *state.AddressState, checkedAt time.Time) state.AddressObservation {
	observation := state.AddressObservation{
		EgressID: egress.ID, ConfigurationRevision: configuration.Revision,
		HistoryGeneration: configuration.HistoryGeneration, Family: egress.Family,
		ProxyPath: egress.Kind == "proxy", CheckedAt: checkedAt.UTC(),
	}
	path, err := c.selectPath(configuration, egress)
	if err != nil {
		observation.FailureReason = "selector-unavailable"
		return observation
	}
	services := configuration.DiscoveryServices.IPv4
	if egress.Family == "ipv6" {
		services = configuration.DiscoveryServices.IPv6
	}
	first, nextService := c.firstValid(ctx, path, services, 0)
	if !first.address.IsValid() {
		observation.FailureReason = "no-valid-response"
		return observation
	}
	if egress.Kind != "proxy" {
		if !first.localAddress.IsValid() {
			observation.FailureReason = "no-valid-response"
			return observation
		}
		c.applyLocalMapping(&observation, path.inventory, path.interfaceName, first.localAddress, first.address)
	}
	if previous != nil && previous.PublicAddress != nil {
		confirmed, parseErr := netip.ParseAddr(*previous.PublicAddress)
		if parseErr == nil && first.address == confirmed {
			observation.Confirmed = true
			observation.PublicAddress = first.address.String()
			return observation
		}
	}
	second, _ := c.firstValid(ctx, path, services, nextService)
	if !second.address.IsValid() {
		observation.FailureReason = "confirmation-unavailable"
		return observation
	}
	if second.address != first.address {
		observation.FailureReason = "conflicting-responses"
		return observation
	}
	observation.Confirmed = true
	observation.PublicAddress = first.address.String()
	return observation
}

func (c *Checker) selectPath(configuration state.Configuration, egress state.Egress) (selectedPath, error) {
	path := selectedPath{egress: egress}
	if egress.Kind == "proxy" {
		for index := range configuration.Proxies {
			if egress.ProxyID != nil && configuration.Proxies[index].ID == *egress.ProxyID {
				path.proxy = &configuration.Proxies[index]
				return path, nil
			}
		}
		return selectedPath{}, errors.New("configured proxy is unavailable")
	}
	inventory, err := c.discover()
	if err != nil {
		return selectedPath{}, err
	}
	path.inventory = inventory
	switch egress.Kind {
	case "default":
		path.interfaceName = defaultInterface(inventory, egress.Family)
	case "interface":
		if egress.InterfaceName != nil {
			path.interfaceName = *egress.InterfaceName
			path.bindInterface = true
		}
	case "source":
		if egress.InterfaceName != nil && egress.SourceAddress != nil {
			path.interfaceName = *egress.InterfaceName
			path.bindInterface = true
			address, parseErr := netip.ParseAddr(*egress.SourceAddress)
			if parseErr == nil {
				path.sourceAddress = &address
			}
		}
	}
	if path.interfaceName == "" || !selectorAvailable(inventory, path) {
		return selectedPath{}, errors.New("local selector is unavailable")
	}
	return path, nil
}

func (c *Checker) firstValid(ctx context.Context, path selectedPath, services []string, start int) (echoResult, int) {
	for index := start; index < len(services); index++ {
		result, err := queryService(ctx, path, services[index])
		if err == nil {
			return result, index + 1
		}
	}
	return echoResult{}, len(services)
}

func queryService(ctx context.Context, path selectedPath, service string) (echoResult, error) {
	transport, err := transportForPath(path)
	if err != nil {
		return echoResult{}, err
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport, Timeout: requestTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
	requestContext, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, service, nil)
	if err != nil {
		return echoResult{}, err
	}
	request.Header.Set("Accept", "text/plain")
	request.Header.Set("User-Agent", "IPChronicle-Agent/address-observation")
	var localAddress netip.Addr
	request = request.WithContext(httptrace.WithClientTrace(request.Context(), &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			if address, ok := socketAddress(info.Conn.LocalAddr()); ok {
				localAddress = address
			}
		},
	}))
	response, err := client.Do(request)
	if err != nil {
		return echoResult{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return echoResult{}, fmt.Errorf("address service returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseSize+1))
	if err != nil {
		return echoResult{}, err
	}
	if len(body) > maxResponseSize {
		return echoResult{}, errors.New("address service response is too large")
	}
	value := strings.TrimSpace(string(body))
	address, err := netip.ParseAddr(value)
	if err != nil || value != address.String() || !publicAddressAllowed(address, path.egress.Family) {
		return echoResult{}, errors.New("address service response is not one public address of the expected family")
	}
	return echoResult{address: address, localAddress: localAddress}, nil
}

func transportForPath(path selectedPath) (*http.Transport, error) {
	transport := &http.Transport{
		DisableKeepAlives: true, ForceAttemptHTTP2: true,
		TLSHandshakeTimeout: requestTimeout, ResponseHeaderTimeout: requestTimeout,
	}
	baseDialer := &net.Dialer{Timeout: requestTimeout, KeepAlive: -1}
	if path.proxy != nil {
		proxyAddress := net.JoinHostPort(path.proxy.Host, strconv.FormatInt(path.proxy.Port, 10))
		switch path.proxy.Scheme {
		case "http", "https":
			proxyURL := &url.URL{Scheme: path.proxy.Scheme, Host: proxyAddress}
			if path.proxy.Username != nil || path.proxy.Password != nil {
				username := ""
				if path.proxy.Username != nil {
					username = *path.proxy.Username
				}
				password := ""
				if path.proxy.Password != nil {
					password = *path.proxy.Password
				}
				proxyURL.User = url.UserPassword(username, password)
			}
			transport.Proxy = http.ProxyURL(proxyURL)
			transport.DialContext = baseDialer.DialContext
		case "socks5":
			var authentication *xproxy.Auth
			if path.proxy.Username != nil || path.proxy.Password != nil {
				username := ""
				if path.proxy.Username != nil {
					username = *path.proxy.Username
				}
				authentication = &xproxy.Auth{User: username}
				if path.proxy.Password != nil {
					authentication.Password = *path.proxy.Password
				}
			}
			dialer, err := xproxy.SOCKS5("tcp", proxyAddress, authentication, baseDialer)
			if err != nil {
				return nil, err
			}
			contextDialer, ok := dialer.(xproxy.ContextDialer)
			if !ok {
				return nil, errors.New("SOCKS5 dialer does not support cancellation")
			}
			transport.DialContext = contextDialer.DialContext
		default:
			return nil, errors.New("unsupported proxy scheme")
		}
		return transport, nil
	}
	if path.sourceAddress != nil {
		baseDialer.LocalAddr = &net.TCPAddr{IP: net.IP(path.sourceAddress.AsSlice())}
	}
	if path.bindInterface {
		interfaceName := path.interfaceName
		baseDialer.Control = func(_, _ string, raw syscall.RawConn) error {
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
		return baseDialer.DialContext(ctx, network, address)
	}
	return transport, nil
}

func (c *Checker) applyLocalMapping(observation *state.AddressObservation, inventory agentnetwork.Inventory, selectedInterface string, localAddress, publicAddress netip.Addr) {
	interfaceName := selectedInterface
	temporary := false
	for _, address := range inventory.Addresses {
		if address.Address == localAddress.String() {
			interfaceName = address.Interface
			temporary = address.Temporary
			break
		}
	}
	local := localAddress.String()
	observation.LocalInterface = &interfaceName
	observation.LocalAddress = &local
	observation.Temporary = temporary
	observation.LikelyNAT = localAddress != publicAddress || !globallyRoutable(localAddress)
}

func defaultInterface(inventory agentnetwork.Inventory, family string) string {
	var selected string
	var metric int64
	for _, route := range inventory.Routes {
		if route.Family != agentnetwork.Family(family) || !route.Default || !interfaceUsable(inventory, route.Interface, family) {
			continue
		}
		if selected == "" || route.Metric < metric || (route.Metric == metric && route.Interface < selected) {
			selected = route.Interface
			metric = route.Metric
		}
	}
	return selected
}

func selectorAvailable(inventory agentnetwork.Inventory, path selectedPath) bool {
	if !interfaceUsable(inventory, path.interfaceName, path.egress.Family) {
		return false
	}
	if path.sourceAddress == nil {
		return true
	}
	for _, address := range inventory.Addresses {
		if address.Interface == path.interfaceName && address.Family == agentnetwork.Family(path.egress.Family) &&
			address.Address == path.sourceAddress.String() && usableLocalAddress(address) {
			return true
		}
	}
	return false
}

func interfaceUsable(inventory agentnetwork.Inventory, interfaceName, family string) bool {
	up := false
	for _, item := range inventory.Interfaces {
		if item.Name == interfaceName {
			up = item.Up && !item.Loopback
			break
		}
	}
	if !up {
		return false
	}
	hasRoute := false
	for _, route := range inventory.Routes {
		if route.Interface == interfaceName && route.Family == agentnetwork.Family(family) {
			hasRoute = true
			break
		}
	}
	if !hasRoute {
		return false
	}
	for _, address := range inventory.Addresses {
		if address.Interface == interfaceName && address.Family == agentnetwork.Family(family) && usableLocalAddress(address) {
			return true
		}
	}
	return false
}

func usableLocalAddress(address agentnetwork.Address) bool {
	if address.Tentative || address.Deprecated || address.Duplicate {
		return false
	}
	switch address.Scope {
	case "global", "private", "shared", "unique-local":
		return true
	default:
		return false
	}
}

func socketAddress(value net.Addr) (netip.Addr, bool) {
	switch address := value.(type) {
	case *net.TCPAddr:
		result, ok := netip.AddrFromSlice(address.IP)
		return result.Unmap(), ok
	default:
		return netip.Addr{}, false
	}
}

func publicAddressAllowed(address netip.Addr, family string) bool {
	if (family == "ipv4") != address.Is4() || !globallyRoutable(address) {
		return false
	}
	return true
}

func globallyRoutable(address netip.Addr) bool {
	return address.IsGlobalUnicast() && !address.IsPrivate() && !address.IsLoopback() &&
		!address.IsLinkLocalUnicast() && !sharedIPv4Prefix.Contains(address)
}
