package probe

import (
	"errors"
	"net/netip"
	"slices"
	"strings"

	agentnetwork "github.com/ipchronicle/ipchronicle/internal/agent/network"
	"github.com/ipchronicle/ipchronicle/internal/agent/state"
)

type executionPath struct {
	egress        state.Egress
	proxy         *state.Proxy
	interfaceName string
	sourceAddress *netip.Addr
	bindInterface bool
}

func selectExecutionPath(configuration state.Configuration, egress state.Egress, inventory agentnetwork.Inventory) (executionPath, error) {
	path := executionPath{egress: egress}
	if egress.Kind == "proxy" {
		for index := range configuration.Proxies {
			if egress.ProxyID != nil && configuration.Proxies[index].ID == *egress.ProxyID {
				path.proxy = &configuration.Proxies[index]
				return path, nil
			}
		}
		return executionPath{}, errors.New("configured proxy is unavailable")
	}

	switch egress.Kind {
	case "default":
		if !defaultPathAvailable(inventory, egress.Family) {
			return executionPath{}, errors.New("default route selector is unavailable")
		}
	case "interface":
		if egress.InterfaceName == nil {
			return executionPath{}, errors.New("interface selector is incomplete")
		}
		address, ok := preferredInterfaceAddress(inventory, *egress.InterfaceName, egress.Family)
		if !ok {
			return executionPath{}, errors.New("interface has no usable source address for the requested family")
		}
		path.interfaceName = *egress.InterfaceName
		path.sourceAddress = &address
		path.bindInterface = true
	case "source":
		if egress.InterfaceName == nil || egress.SourceAddress == nil {
			return executionPath{}, errors.New("source selector is incomplete")
		}
		address, err := netip.ParseAddr(*egress.SourceAddress)
		if err != nil || !inventoryContainsAddress(inventory, *egress.InterfaceName, egress.Family, address) {
			return executionPath{}, errors.New("configured source address is unavailable")
		}
		path.interfaceName = *egress.InterfaceName
		path.sourceAddress = &address
		path.bindInterface = true
	default:
		return executionPath{}, errors.New("unsupported egress kind")
	}
	return path, nil
}

func defaultPathAvailable(inventory agentnetwork.Inventory, family string) bool {
	for _, route := range inventory.Routes {
		if route.Default && string(route.Family) == family && interfaceAvailable(inventory, route.Interface, family) {
			return true
		}
	}
	return false
}

func preferredInterfaceAddress(inventory agentnetwork.Inventory, interfaceName, family string) (netip.Addr, bool) {
	type candidate struct {
		address   netip.Addr
		temporary bool
	}
	var candidates []candidate
	for _, item := range inventory.Addresses {
		if item.Interface != interfaceName || string(item.Family) != family || !usableAddress(item) {
			continue
		}
		address, err := netip.ParseAddr(item.Address)
		if err == nil {
			candidates = append(candidates, candidate{address: address, temporary: item.Temporary})
		}
	}
	if len(candidates) == 0 || !interfaceAvailable(inventory, interfaceName, family) {
		return netip.Addr{}, false
	}
	slices.SortFunc(candidates, func(a, b candidate) int {
		if a.temporary != b.temporary {
			if a.temporary {
				return 1
			}
			return -1
		}
		return strings.Compare(a.address.String(), b.address.String())
	})
	return candidates[0].address, true
}

func inventoryContainsAddress(inventory agentnetwork.Inventory, interfaceName, family string, address netip.Addr) bool {
	if (family == "ipv4") != address.Is4() || !interfaceAvailable(inventory, interfaceName, family) {
		return false
	}
	for _, item := range inventory.Addresses {
		if item.Interface == interfaceName && string(item.Family) == family && item.Address == address.String() && usableAddress(item) {
			return true
		}
	}
	return false
}

func interfaceAvailable(inventory agentnetwork.Inventory, interfaceName, family string) bool {
	for _, item := range inventory.Interfaces {
		if item.Name == interfaceName && item.Up && !item.Loopback {
			for _, route := range inventory.Routes {
				if route.Interface == interfaceName && string(route.Family) == family {
					return true
				}
			}
			return false
		}
	}
	return false
}

func usableAddress(address agentnetwork.Address) bool {
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
