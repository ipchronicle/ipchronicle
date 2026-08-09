package network

import (
	"net/netip"
	"strings"
	"testing"
)

func TestDiscoverParsesLinuxInventoryAndLifecycleFlags(t *testing.T) {
	links := []link{
		{Interface: Interface{Name: "lo", Index: 1, Up: true, Loopback: true}, addresses: prefixes("127.0.0.1/8", "::1/128")},
		{Interface: Interface{Name: "eth0", Index: 2, Up: true}, addresses: prefixes("10.0.0.5/24", "2001:db8::10/64", "2001:db8::99/64")},
	}
	inet6 := strings.NewReader("20010db8000000000000000000000010 02 40 00 00 eth0\n" +
		"20010db8000000000000000000000099 02 40 00 61 eth0\n")
	ipv4 := strings.NewReader("Iface Destination Gateway Flags RefCnt Use Metric Mask MTU Window IRTT\n" +
		"eth0 00000000 0100000A 0003 0 0 100 00000000 0 0 0\n" +
		"eth0 0000000A 00000000 0001 0 0 100 00FFFFFF 0 0 0\n")
	ipv6 := strings.NewReader("00000000000000000000000000000000 00 00000000000000000000000000000000 00 fe800000000000000000000000000001 00000064 00000000 00000000 00000001 eth0\n")

	inventory, err := discover(links, inet6, ipv4, ipv6)
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Interfaces) != 2 || len(inventory.Addresses) != 5 || len(inventory.Routes) != 3 {
		t.Fatalf("unexpected inventory sizes: %#v", inventory)
	}
	if inventory.Routes[0].Family != IPv4 || !inventory.Routes[0].Default || inventory.Routes[0].Destination != "0.0.0.0/0" {
		t.Fatalf("unexpected default IPv4 route: %#v", inventory.Routes[0])
	}
	var temporary Address
	for _, address := range inventory.Addresses {
		if address.Address == "2001:db8::99" {
			temporary = address
		}
	}
	if !temporary.Temporary || !temporary.Deprecated || !temporary.Tentative || temporary.Duplicate {
		t.Fatalf("unexpected temporary address flags: %#v", temporary)
	}
}

func TestAddressScopesCoverSupportedCandidateRanges(t *testing.T) {
	tests := map[string]string{
		"10.0.0.1": "private", "100.64.0.1": "shared", "8.8.8.8": "global",
		"fd00::1": "unique-local", "2001:4860:4860::8888": "global",
		"fe80::1": "link-local", "127.0.0.1": "loopback",
	}
	for value, expected := range tests {
		if actual := addressScope(netip.MustParseAddr(value)); actual != expected {
			t.Errorf("scope(%s) = %q, want %q", value, actual, expected)
		}
	}
}

func TestInvalidRouteDataFailsExplicitly(t *testing.T) {
	_, err := parseIPv4Routes(strings.NewReader("Iface Destination Gateway Flags RefCnt Use Metric Mask MTU Window IRTT\neth0 bad\n"))
	if err == nil {
		t.Fatal("invalid route row was accepted")
	}
	_, err = parseIPv6Routes(strings.NewReader("not-a-route\n"))
	if err == nil {
		t.Fatal("invalid IPv6 route row was accepted")
	}
}

func prefixes(values ...string) []netip.Prefix {
	result := make([]netip.Prefix, len(values))
	for index, value := range values {
		result[index] = netip.MustParsePrefix(value)
	}
	return result
}
