package network

import (
	"bufio"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"slices"
	"strconv"
	"strings"
)

const (
	maxInterfaces = 64
	maxAddresses  = 128
	maxRoutes     = 256

	ifaTemporary  = 0x01
	ifaDADFailed  = 0x08
	ifaDeprecated = 0x20
	ifaTentative  = 0x40
	routeUp       = 0x01
)

type Family string

const (
	IPv4 Family = "ipv4"
	IPv6 Family = "ipv6"
)

type Interface struct {
	Name     string
	Index    int
	Up       bool
	Loopback bool
}

type Address struct {
	Interface    string
	Address      string
	PrefixLength int
	Family       Family
	Scope        string
	Temporary    bool
	Tentative    bool
	Deprecated   bool
	Duplicate    bool
}

type Route struct {
	Interface   string
	Family      Family
	Destination string
	Gateway     *string
	Metric      int64
	Default     bool
}

type Inventory struct {
	Interfaces []Interface
	Addresses  []Address
	Routes     []Route
}

type link struct {
	Interface
	addresses []netip.Prefix
}

// Discover reads the kernel's current interface and route state without
// depending on iproute2 being installed on the managed host.
func Discover() (Inventory, error) {
	links, err := systemLinks()
	if err != nil {
		return Inventory{}, err
	}
	inet6, err := os.Open("/proc/net/if_inet6")
	if err != nil {
		return Inventory{}, fmt.Errorf("open IPv6 address inventory: %w", err)
	}
	defer inet6.Close()
	ipv4Routes, err := os.Open("/proc/net/route")
	if err != nil {
		return Inventory{}, fmt.Errorf("open IPv4 route inventory: %w", err)
	}
	defer ipv4Routes.Close()
	ipv6Routes, err := os.Open("/proc/net/ipv6_route")
	if err != nil {
		return Inventory{}, fmt.Errorf("open IPv6 route inventory: %w", err)
	}
	defer ipv6Routes.Close()
	return discover(links, inet6, ipv4Routes, ipv6Routes)
}

func systemLinks() ([]link, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("list network interfaces: %w", err)
	}
	if len(interfaces) > maxInterfaces {
		return nil, fmt.Errorf("network interface inventory exceeds %d entries", maxInterfaces)
	}
	links := make([]link, 0, len(interfaces))
	for _, item := range interfaces {
		addresses, err := item.Addrs()
		if err != nil {
			return nil, fmt.Errorf("list addresses for interface %q: %w", item.Name, err)
		}
		current := link{Interface: Interface{
			Name: item.Name, Index: item.Index,
			Up: item.Flags&net.FlagUp != 0, Loopback: item.Flags&net.FlagLoopback != 0,
		}}
		for _, raw := range addresses {
			prefix, err := netip.ParsePrefix(raw.String())
			if err != nil {
				return nil, fmt.Errorf("parse address %q on interface %q: %w", raw.String(), item.Name, err)
			}
			current.addresses = append(current.addresses, prefix)
		}
		links = append(links, current)
	}
	return links, nil
}

func discover(links []link, inet6, ipv4Routes, ipv6Routes io.Reader) (Inventory, error) {
	if len(links) > maxInterfaces {
		return Inventory{}, fmt.Errorf("network interface inventory exceeds %d entries", maxInterfaces)
	}
	flags, err := parseIPv6AddressFlags(inet6)
	if err != nil {
		return Inventory{}, err
	}
	inventory := Inventory{Interfaces: make([]Interface, 0, len(links))}
	for _, item := range links {
		if item.Name == "" || item.Index <= 0 {
			return Inventory{}, errors.New("network interface inventory contains an invalid interface")
		}
		inventory.Interfaces = append(inventory.Interfaces, item.Interface)
		for _, prefix := range item.addresses {
			address := prefix.Addr().Unmap()
			family := IPv6
			if address.Is4() {
				family = IPv4
			}
			entry := Address{
				Interface: item.Name, Address: address.String(), PrefixLength: prefix.Bits(),
				Family: family, Scope: addressScope(address),
			}
			if family == IPv6 {
				value := flags[addressFlagKey(item.Name, address)]
				entry.Temporary = value&ifaTemporary != 0
				entry.Duplicate = value&ifaDADFailed != 0
				entry.Deprecated = value&ifaDeprecated != 0
				entry.Tentative = value&ifaTentative != 0
			}
			inventory.Addresses = append(inventory.Addresses, entry)
		}
	}
	if len(inventory.Addresses) > maxAddresses {
		return Inventory{}, fmt.Errorf("network address inventory exceeds %d entries", maxAddresses)
	}
	ipv4, err := parseIPv4Routes(ipv4Routes)
	if err != nil {
		return Inventory{}, err
	}
	ipv6, err := parseIPv6Routes(ipv6Routes)
	if err != nil {
		return Inventory{}, err
	}
	inventory.Routes = append(ipv4, ipv6...)
	if len(inventory.Routes) > maxRoutes {
		return Inventory{}, fmt.Errorf("network route inventory exceeds %d entries", maxRoutes)
	}
	slices.SortFunc(inventory.Interfaces, func(a, b Interface) int { return a.Index - b.Index })
	slices.SortFunc(inventory.Addresses, func(a, b Address) int {
		return strings.Compare(a.Interface+"\x00"+string(a.Family)+"\x00"+a.Address, b.Interface+"\x00"+string(b.Family)+"\x00"+b.Address)
	})
	slices.SortFunc(inventory.Routes, func(a, b Route) int {
		return strings.Compare(string(a.Family)+"\x00"+a.Interface+"\x00"+a.Destination, string(b.Family)+"\x00"+b.Interface+"\x00"+b.Destination)
	})
	return inventory, nil
}

func parseIPv6AddressFlags(reader io.Reader) (map[string]uint64, error) {
	result := make(map[string]uint64)
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 6 {
			return nil, fmt.Errorf("invalid /proc/net/if_inet6 row %q", scanner.Text())
		}
		address, err := parseIPv6Hex(fields[0])
		if err != nil {
			return nil, fmt.Errorf("parse IPv6 address inventory row: %w", err)
		}
		value, err := strconv.ParseUint(fields[4], 16, 32)
		if err != nil {
			return nil, fmt.Errorf("parse IPv6 address flags %q: %w", fields[4], err)
		}
		result[addressFlagKey(fields[5], address)] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read IPv6 address inventory: %w", err)
	}
	return result, nil
}

func parseIPv4Routes(reader io.Reader) ([]Route, error) {
	scanner := bufio.NewScanner(reader)
	first := true
	var routes []Route
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if first {
			first = false
			if len(fields) > 0 && fields[0] == "Iface" {
				continue
			}
		}
		if len(fields) < 11 {
			return nil, fmt.Errorf("invalid /proc/net/route row %q", scanner.Text())
		}
		flags, err := strconv.ParseUint(fields[3], 16, 32)
		if err != nil {
			return nil, fmt.Errorf("parse IPv4 route flags %q: %w", fields[3], err)
		}
		if flags&routeUp == 0 {
			continue
		}
		destination, err := parseIPv4LittleEndian(fields[1])
		if err != nil {
			return nil, err
		}
		gateway, err := parseIPv4LittleEndian(fields[2])
		if err != nil {
			return nil, err
		}
		mask, err := parseIPv4LittleEndian(fields[7])
		if err != nil {
			return nil, err
		}
		bits, width := net.IPMask(mask.AsSlice()).Size()
		if width != 32 {
			return nil, fmt.Errorf("IPv4 route mask %q is not contiguous", fields[7])
		}
		metric, err := strconv.ParseInt(fields[6], 10, 64)
		if err != nil || metric < 0 {
			return nil, fmt.Errorf("parse IPv4 route metric %q", fields[6])
		}
		route := Route{
			Interface: fields[0], Family: IPv4,
			Destination: netip.PrefixFrom(destination, bits).Masked().String(),
			Metric:      metric, Default: destination.IsUnspecified() && bits == 0,
		}
		if !gateway.IsUnspecified() {
			value := gateway.String()
			route.Gateway = &value
		}
		routes = append(routes, route)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read IPv4 route inventory: %w", err)
	}
	return routes, nil
}

func parseIPv6Routes(reader io.Reader) ([]Route, error) {
	scanner := bufio.NewScanner(reader)
	var routes []Route
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 10 {
			return nil, fmt.Errorf("invalid /proc/net/ipv6_route row %q", scanner.Text())
		}
		flags, err := strconv.ParseUint(fields[8], 16, 32)
		if err != nil {
			return nil, fmt.Errorf("parse IPv6 route flags %q: %w", fields[8], err)
		}
		if flags&routeUp == 0 {
			continue
		}
		destination, err := parseIPv6Hex(fields[0])
		if err != nil {
			return nil, err
		}
		bits, err := strconv.ParseUint(fields[1], 16, 8)
		if err != nil || bits > 128 {
			return nil, fmt.Errorf("parse IPv6 route prefix length %q", fields[1])
		}
		gateway, err := parseIPv6Hex(fields[4])
		if err != nil {
			return nil, err
		}
		metric, err := strconv.ParseInt(fields[5], 16, 64)
		if err != nil || metric < 0 {
			return nil, fmt.Errorf("parse IPv6 route metric %q", fields[5])
		}
		route := Route{
			Interface: fields[9], Family: IPv6,
			Destination: netip.PrefixFrom(destination, int(bits)).Masked().String(),
			Metric:      metric, Default: destination.IsUnspecified() && bits == 0,
		}
		if !gateway.IsUnspecified() {
			value := gateway.String()
			route.Gateway = &value
		}
		routes = append(routes, route)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read IPv6 route inventory: %w", err)
	}
	return routes, nil
}

func parseIPv4LittleEndian(value string) (netip.Addr, error) {
	parsed, err := strconv.ParseUint(value, 16, 32)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("parse IPv4 route address %q: %w", value, err)
	}
	return netip.AddrFrom4([4]byte{byte(parsed), byte(parsed >> 8), byte(parsed >> 16), byte(parsed >> 24)}), nil
}

func parseIPv6Hex(value string) (netip.Addr, error) {
	encoded, err := hex.DecodeString(value)
	if err != nil || len(encoded) != 16 {
		return netip.Addr{}, fmt.Errorf("parse IPv6 route address %q", value)
	}
	var bytes [16]byte
	copy(bytes[:], encoded)
	return netip.AddrFrom16(bytes), nil
}

func addressFlagKey(interfaceName string, address netip.Addr) string {
	return interfaceName + "\x00" + address.String()
}

func addressScope(address netip.Addr) string {
	if address.IsUnspecified() {
		return "unspecified"
	}
	if address.IsLoopback() {
		return "loopback"
	}
	if address.IsMulticast() {
		return "multicast"
	}
	if address.IsLinkLocalUnicast() {
		return "link-local"
	}
	if address.Is4() {
		if netip.MustParsePrefix("100.64.0.0/10").Contains(address) {
			return "shared"
		}
		if address.IsPrivate() {
			return "private"
		}
	} else if address.IsPrivate() {
		return "unique-local"
	}
	if address.IsGlobalUnicast() {
		return "global"
	}
	return "other"
}
