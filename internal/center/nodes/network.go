package nodes

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/center/database/configdb"
	"github.com/ipchronicle/ipchronicle/internal/center/database/historydb"
)

const (
	defaultLightweightInterval = 10 * time.Minute
	maxNodeEgresses            = 64
)

var (
	ErrInvalidNetworkInventory    = errors.New("node network inventory is invalid")
	ErrEgressLimitReached         = errors.New("node network egress limit reached")
	ErrInvalidObservationSettings = errors.New("network observation settings are invalid")
	ErrPublicAddressNotFound      = errors.New("public address does not exist for node")
)

type NetworkInventory struct {
	CapturedAt time.Time          `json:"capturedAt"`
	Interfaces []NetworkInterface `json:"interfaces"`
	Addresses  []NetworkAddress   `json:"addresses"`
	Routes     []NetworkRoute     `json:"routes"`
}

type NetworkInterface struct {
	Name     string `json:"name"`
	Index    int    `json:"index"`
	Up       bool   `json:"up"`
	Loopback bool   `json:"loopback"`
}

type NetworkAddress struct {
	InterfaceName string `json:"interfaceName"`
	Address       string `json:"address"`
	PrefixLength  int    `json:"prefixLength"`
	Family        string `json:"family"`
	Scope         string `json:"scope"`
	Temporary     bool   `json:"temporary"`
	Tentative     bool   `json:"tentative"`
	Deprecated    bool   `json:"deprecated"`
	Duplicate     bool   `json:"duplicate"`
}

type NetworkRoute struct {
	InterfaceName string  `json:"interfaceName"`
	Family        string  `json:"family"`
	Destination   string  `json:"destination"`
	Gateway       *string `json:"gateway,omitempty"`
	Metric        int64   `json:"metric"`
	Default       bool    `json:"default"`
}

type NetworkEgress struct {
	ID                         uuid.UUID
	PathID                     *uuid.UUID
	PublicAddress              *string
	NodeID                     uuid.UUID
	Name                       string
	Kind                       string
	Family                     string
	InterfaceName              *string
	SourceAddress              *string
	ProxyID                    *uuid.UUID
	Enabled                    bool
	Available                  bool
	Automatic                  bool
	LightweightIntervalSeconds int64
}

type PublicAddress struct {
	ID                    uuid.UUID
	Address               string
	Family                string
	ProbeEnabled          bool
	Available             bool
	SelectedPathID        *uuid.UUID
	SelectedNodeID        *uuid.UUID
	SelectedNodeName      *string
	PathCount             int
	LikelyNAT             bool
	ProxyPath             bool
	FirstSeenAt           time.Time
	LastSeenAt            time.Time
	SelectedLastChecked   *time.Time
	SelectedLastSucceeded *time.Time
	LatestSnapshotID      *uuid.UUID
	LatestSnapshotAt      *time.Time
}

type PublicAddressUpdate struct {
	ProbeEnabled bool
}

type NetworkEgressCandidate struct {
	Kind               string
	Family             string
	InterfaceName      string
	SourceAddress      *string
	Scope              *string
	Temporary          bool
	Eligible           bool
	UnavailableReason  *string
	ConfiguredEgressID *uuid.UUID
}

type NodeNetworkState struct {
	AddressEvents   []AddressEvent
	AddressGaps     []AddressGap
	PublicAddresses []PublicAddress
	NetworkProxies  []NetworkProxy
}

type DiscoveryServices struct {
	IPv4      []string
	IPv6      []string
	UpdatedAt time.Time
}

func validateNetworkReport(inventory *NetworkInventory, reportError *string) error {
	if inventory != nil && reportError != nil {
		return ErrInvalidNetworkInventory
	}
	if reportError != nil {
		if !validBoundedText(*reportError, 1024) {
			return ErrInvalidNetworkInventory
		}
		return nil
	}
	if inventory == nil {
		return nil
	}
	if inventory.CapturedAt.IsZero() || len(inventory.Interfaces) > 64 || len(inventory.Addresses) > 128 || len(inventory.Routes) > 256 {
		return ErrInvalidNetworkInventory
	}
	interfaces := make(map[string]NetworkInterface, len(inventory.Interfaces))
	indexes := make(map[int]struct{}, len(inventory.Interfaces))
	for _, item := range inventory.Interfaces {
		if !validInterfaceName(item.Name) || item.Index < 1 {
			return ErrInvalidNetworkInventory
		}
		if _, exists := interfaces[item.Name]; exists {
			return ErrInvalidNetworkInventory
		}
		if _, exists := indexes[item.Index]; exists {
			return ErrInvalidNetworkInventory
		}
		interfaces[item.Name] = item
		indexes[item.Index] = struct{}{}
	}
	addresses := make(map[string]struct{}, len(inventory.Addresses))
	for _, item := range inventory.Addresses {
		if _, exists := interfaces[item.InterfaceName]; !exists {
			return ErrInvalidNetworkInventory
		}
		address, err := netip.ParseAddr(item.Address)
		if err != nil || address != address.Unmap() || familyOf(address) != item.Family ||
			item.PrefixLength < 0 || item.PrefixLength > address.BitLen() || addressScope(address) != item.Scope ||
			(item.Family == "ipv4" && (item.Temporary || item.Tentative || item.Deprecated || item.Duplicate)) {
			return ErrInvalidNetworkInventory
		}
		key := item.InterfaceName + "\x00" + item.Address
		if _, exists := addresses[key]; exists {
			return ErrInvalidNetworkInventory
		}
		addresses[key] = struct{}{}
	}
	routes := make(map[string]struct{}, len(inventory.Routes))
	for _, item := range inventory.Routes {
		if _, exists := interfaces[item.InterfaceName]; !exists || item.Metric < 0 {
			return ErrInvalidNetworkInventory
		}
		destination, err := netip.ParsePrefix(item.Destination)
		if err != nil || destination != destination.Masked() || familyOf(destination.Addr()) != item.Family ||
			item.Default != (destination.Bits() == 0 && destination.Addr().IsUnspecified()) {
			return ErrInvalidNetworkInventory
		}
		if item.Gateway != nil {
			gateway, err := netip.ParseAddr(*item.Gateway)
			if err != nil || gateway != gateway.Unmap() || familyOf(gateway) != item.Family || gateway.IsUnspecified() {
				return ErrInvalidNetworkInventory
			}
		}
		key := item.Family + "\x00" + item.InterfaceName + "\x00" + item.Destination + fmt.Sprint("\x00", item.Metric)
		if _, exists := routes[key]; exists {
			return ErrInvalidNetworkInventory
		}
		routes[key] = struct{}{}
	}
	return nil
}

func (s *Service) applyNetworkReport(ctx context.Context, queries *configdb.Queries, nodeID string, inventory *NetworkInventory, reportError *string, now int64) (bool, error) {
	if inventory == nil && reportError == nil {
		return false, nil
	}
	if reportError != nil {
		message := *reportError
		return false, queries.RecordNodeNetworkInventoryError(ctx, configdb.RecordNodeNetworkInventoryErrorParams{
			NodeID: nodeID, ReceivedAt: now, LastError: &message,
		})
	}
	payload, err := json.Marshal(inventory)
	if err != nil {
		return false, fmt.Errorf("encode node network inventory: %w", err)
	}
	encoded := string(payload)
	capturedAt := inventory.CapturedAt.UTC().Unix()
	if err := queries.UpsertNodeNetworkInventory(ctx, configdb.UpsertNodeNetworkInventoryParams{
		NodeID: nodeID, Payload: &encoded, CapturedAt: &capturedAt, ReceivedAt: now,
	}); err != nil {
		return false, err
	}
	egresses, err := queries.ListNodeEgresses(ctx, nodeID)
	if err != nil {
		return false, err
	}
	availability := egressAvailability(*inventory)
	changed := false
	egressCount := len(egresses)
	for _, family := range []string{"ipv4", "ipv6"} {
		available := availability[selectorKey("default", family, "", nil)]
		var existing *configdb.NetworkEgress
		for index := range egresses {
			if egresses[index].Kind == "default" && egresses[index].Family == family {
				existing = &egresses[index]
				break
			}
		}
		if existing == nil && available && egressCount < maxNodeEgresses {
			id := uuid.New()
			if err := queries.CreateNodeEgress(ctx, configdb.CreateNodeEgressParams{
				ID: id.String(), NodeID: nodeID, Name: "default-" + family,
				Kind: "default", Family: family, Enabled: 1, Available: 1, Automatic: 1,
				LightweightIntervalSeconds: int64(defaultLightweightInterval / time.Second),
				CreatedAt:                  now, UpdatedAt: now,
			}); err != nil {
				return false, err
			}
			changed = true
			egressCount++
			continue
		}
		if existing != nil {
			value := boolInteger(available)
			if _, err := queries.SetNodeEgressAvailability(ctx, configdb.SetNodeEgressAvailabilityParams{
				Available: value, UpdatedAt: now, ID: existing.ID, NodeID: nodeID, Available_2: value,
			}); err != nil {
				return false, err
			}
		}
	}
	domainEgresses := make([]NetworkEgress, 0, len(egresses))
	for _, record := range egresses {
		domain, err := egressFromRecord(record)
		if err != nil {
			return false, err
		}
		domainEgresses = append(domainEgresses, domain)
	}
	for _, candidate := range inventoryCandidates(*inventory, domainEgresses) {
		if candidate.Kind != "source" || !candidate.Eligible || candidate.Temporary || candidate.SourceAddress == nil || candidate.ConfiguredEgressID != nil {
			continue
		}
		if egressCount >= maxNodeEgresses {
			break
		}
		id := uuid.New()
		if err := queries.CreateNodeEgress(ctx, configdb.CreateNodeEgressParams{
			ID: id.String(), NodeID: nodeID,
			Name: candidate.InterfaceName + "-" + *candidate.SourceAddress,
			Kind: "source", Family: candidate.Family,
			InterfaceName: &candidate.InterfaceName, SourceAddress: candidate.SourceAddress,
			Enabled: 1, Available: 1, Automatic: 1,
			LightweightIntervalSeconds: int64(defaultLightweightInterval / time.Second),
			CreatedAt:                  now, UpdatedAt: now,
		}); err != nil {
			return false, err
		}
		changed = true
		egressCount++
	}
	for _, item := range egresses {
		if item.Kind == "default" || item.Kind == "proxy" {
			continue
		}
		available := availability[selectorKey(item.Kind, item.Family, pointerValue(item.InterfaceName), item.SourceAddress)]
		value := boolInteger(available)
		if _, err := queries.SetNodeEgressAvailability(ctx, configdb.SetNodeEgressAvailabilityParams{
			Available: value, UpdatedAt: now, ID: item.ID, NodeID: nodeID, Available_2: value,
		}); err != nil {
			return false, err
		}
	}
	return changed, nil
}

func (s *Service) Network(ctx context.Context, id uuid.UUID) (NodeNetworkState, error) {
	if _, err := s.queries.GetNodeByID(ctx, id.String()); errors.Is(err, sql.ErrNoRows) {
		return NodeNetworkState{}, ErrNodeNotFound
	} else if err != nil {
		return NodeNetworkState{}, err
	}
	state := NodeNetworkState{}
	publicAddressRecords, err := s.queries.ListNodePublicAddresses(ctx, id.String())
	if err != nil {
		return NodeNetworkState{}, err
	}
	state.PublicAddresses = make([]PublicAddress, 0, len(publicAddressRecords))
	currentSnapshots, err := s.historyQueries.ListCurrentProbeSnapshots(ctx)
	if err != nil {
		return NodeNetworkState{}, err
	}
	latestSnapshots := make(map[string]historydb.ListCurrentProbeSnapshotsRow, len(currentSnapshots))
	for _, snapshot := range currentSnapshots {
		latestSnapshots[snapshot.EgressID] = snapshot
	}
	for _, record := range publicAddressRecords {
		address, err := s.publicAddressFromRecord(ctx, record)
		if err != nil {
			return NodeNetworkState{}, err
		}
		if snapshot, ok := latestSnapshots[address.ID.String()]; ok {
			snapshotID, err := uuid.Parse(snapshot.SnapshotID)
			if err != nil {
				return NodeNetworkState{}, err
			}
			observedAt := time.Unix(snapshot.ObservedAt, 0).UTC()
			address.LatestSnapshotID = &snapshotID
			address.LatestSnapshotAt = &observedAt
		}
		state.PublicAddresses = append(state.PublicAddresses, address)
	}
	addressEvents, err := s.historyQueries.ListNodeAddressEvents(ctx, historydb.ListNodeAddressEventsParams{NodeID: id.String(), Limit: 100})
	if err != nil {
		return NodeNetworkState{}, err
	}
	state.AddressEvents = make([]AddressEvent, 0, len(addressEvents))
	for _, record := range addressEvents {
		item, err := addressEventFromRecord(record)
		if err != nil {
			return NodeNetworkState{}, err
		}
		state.AddressEvents = append(state.AddressEvents, item)
	}
	addressGaps, err := s.historyQueries.ListNodeAddressGaps(ctx, historydb.ListNodeAddressGapsParams{NodeID: id.String(), Limit: 100})
	if err != nil {
		return NodeNetworkState{}, err
	}
	state.AddressGaps = make([]AddressGap, 0, len(addressGaps))
	for _, record := range addressGaps {
		item, err := addressGapFromRecord(record)
		if err != nil {
			return NodeNetworkState{}, err
		}
		state.AddressGaps = append(state.AddressGaps, item)
	}
	proxyRecords, err := s.queries.ListNodeNetworkProxies(ctx, id.String())
	if err != nil {
		return NodeNetworkState{}, err
	}
	state.NetworkProxies, err = s.networkProxiesFromRecords(ctx, id, proxyRecords)
	if err != nil {
		return NodeNetworkState{}, err
	}
	return state, nil
}

func (s *Service) UpdatePublicAddress(ctx context.Context, nodeID, addressID uuid.UUID, update PublicAddressUpdate) (PublicAddress, error) {
	if _, err := s.queries.GetNodeByID(ctx, nodeID.String()); errors.Is(err, sql.ErrNoRows) {
		return PublicAddress{}, ErrNodeNotFound
	} else if err != nil {
		return PublicAddress{}, err
	}
	count, err := s.queries.PublicAddressBelongsToNode(ctx, configdb.PublicAddressBelongsToNodeParams{
		PublicAddressID: addressID.String(), NodeID: nodeID.String(),
	})
	if err != nil {
		return PublicAddress{}, err
	}
	if count == 0 {
		return PublicAddress{}, ErrPublicAddressNotFound
	}
	record, err := s.queries.GetPublicAddressByID(ctx, addressID.String())
	if err != nil {
		return PublicAddress{}, err
	}
	value := boolInteger(update.ProbeEnabled)
	changed, err := s.queries.SetPublicAddressProbeSettings(ctx, configdb.SetPublicAddressProbeSettingsParams{
		ProbeEnabled: value, UpdatedAt: s.now().UTC().Unix(), ID: addressID.String(), ProbeEnabled_2: value,
	})
	if err != nil {
		return PublicAddress{}, err
	}
	if changed > 0 && record.SelectedPathID != nil {
		path, err := s.queries.GetPublicAddressPathByPathID(ctx, *record.SelectedPathID)
		if err != nil {
			return PublicAddress{}, err
		}
		if err := incrementNodeConfiguration(ctx, s.queries, path.NodeID); err != nil {
			return PublicAddress{}, err
		}
		s.sync.Wake(path.NodeID)
	}
	if err := s.queries.DeletePendingPublicAddressProbeByAddress(ctx, addressID.String()); err != nil {
		return PublicAddress{}, err
	}
	record.ProbeEnabled = value
	return s.publicAddressFromRecord(ctx, record)
}

func (s *Service) publicAddressFromRecord(ctx context.Context, record configdb.PublicAddress) (PublicAddress, error) {
	id, err := uuid.Parse(record.ID)
	if err != nil {
		return PublicAddress{}, err
	}
	result := PublicAddress{
		ID: id, Address: record.Address, Family: record.Family,
		ProbeEnabled: record.ProbeEnabled == 1,
		FirstSeenAt:  time.Unix(record.FirstSeenAt, 0).UTC(), LastSeenAt: time.Unix(record.LastSeenAt, 0).UTC(),
	}
	paths, err := s.queries.ListPublicAddressPaths(ctx, record.ID)
	if err != nil {
		return PublicAddress{}, err
	}
	result.PathCount = len(paths)
	for _, path := range paths {
		if path.Available == 1 {
			result.Available = true
		}
		if record.SelectedPathID == nil || path.PathID != *record.SelectedPathID {
			continue
		}
		pathID, err := uuid.Parse(path.PathID)
		if err != nil {
			return PublicAddress{}, err
		}
		nodeID, err := uuid.Parse(path.NodeID)
		if err != nil {
			return PublicAddress{}, err
		}
		result.SelectedPathID = &pathID
		result.SelectedNodeID = &nodeID
		result.LikelyNAT = path.LikelyNat == 1
		result.ProxyPath = path.ProxyPath == 1
		checked := time.Unix(path.LastCheckedAt, 0).UTC()
		result.SelectedLastChecked = &checked
		if path.LastSucceededAt != nil {
			value := time.Unix(*path.LastSucceededAt, 0).UTC()
			result.SelectedLastSucceeded = &value
		}
		if node, err := s.queries.GetNodeByID(ctx, path.NodeID); err == nil {
			result.SelectedNodeName = &node.Name
		} else if !errors.Is(err, sql.ErrNoRows) {
			return PublicAddress{}, err
		}
	}
	return result, nil
}

func (s *Service) ObservationSettings(ctx context.Context) (DiscoveryServices, error) {
	record, err := s.queries.GetNetworkObservationSettings(ctx)
	if err != nil {
		return DiscoveryServices{}, err
	}
	settings, err := decodeDiscoveryServices(record.Ipv4Services, record.Ipv6Services)
	if err != nil {
		return DiscoveryServices{}, fmt.Errorf("decode stored network observation settings: %w", err)
	}
	settings.UpdatedAt = time.Unix(record.UpdatedAt, 0).UTC()
	return settings, nil
}

func (s *Service) UpdateObservationSettings(ctx context.Context, settings DiscoveryServices) (DiscoveryServices, error) {
	normalized, err := normalizeDiscoveryServices(settings)
	if err != nil {
		return DiscoveryServices{}, err
	}
	current, err := s.ObservationSettings(ctx)
	if err != nil {
		return DiscoveryServices{}, err
	}
	if slices.Equal(current.IPv4, normalized.IPv4) && slices.Equal(current.IPv6, normalized.IPv6) {
		return current, nil
	}
	ipv4, err := json.Marshal(normalized.IPv4)
	if err != nil {
		return DiscoveryServices{}, err
	}
	ipv6, err := json.Marshal(normalized.IPv6)
	if err != nil {
		return DiscoveryServices{}, err
	}
	now := s.now().UTC().Truncate(time.Second)
	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return DiscoveryServices{}, err
	}
	defer transaction.Rollback()
	queries := s.queries.WithTx(transaction)
	if err := queries.UpdateNetworkObservationSettings(ctx, configdb.UpdateNetworkObservationSettingsParams{
		Ipv4Services: string(ipv4), Ipv6Services: string(ipv6), UpdatedAt: now.Unix(),
	}); err != nil {
		return DiscoveryServices{}, err
	}
	if err := queries.IncrementAllNodeDesiredConfigurationRevisions(ctx); err != nil {
		return DiscoveryServices{}, err
	}
	nodes, err := queries.ListNodes(ctx)
	if err != nil {
		return DiscoveryServices{}, err
	}
	if err := transaction.Commit(); err != nil {
		return DiscoveryServices{}, err
	}
	for _, node := range nodes {
		if node.RevokedAt == nil {
			s.sync.Wake(node.ID)
		}
	}
	normalized.UpdatedAt = now
	return normalized, nil
}

func decodeDiscoveryServices(ipv4, ipv6 string) (DiscoveryServices, error) {
	settings := DiscoveryServices{}
	if err := json.Unmarshal([]byte(ipv4), &settings.IPv4); err != nil {
		return DiscoveryServices{}, err
	}
	if err := json.Unmarshal([]byte(ipv6), &settings.IPv6); err != nil {
		return DiscoveryServices{}, err
	}
	return normalizeDiscoveryServices(settings)
}

func normalizeDiscoveryServices(settings DiscoveryServices) (DiscoveryServices, error) {
	ipv4, err := normalizeDiscoveryServiceList(settings.IPv4)
	if err != nil {
		return DiscoveryServices{}, err
	}
	ipv6, err := normalizeDiscoveryServiceList(settings.IPv6)
	if err != nil {
		return DiscoveryServices{}, err
	}
	return DiscoveryServices{IPv4: ipv4, IPv6: ipv6}, nil
}

func normalizeDiscoveryServiceList(values []string) ([]string, error) {
	if len(values) < 2 || len(values) > 8 {
		return nil, ErrInvalidObservationSettings
	}
	result := make([]string, 0, len(values))
	hosts := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value != strings.TrimSpace(value) || len(value) < 8 || len(value) > 2048 {
			return nil, ErrInvalidObservationSettings
		}
		parsed, err := url.Parse(value)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
			parsed.User != nil || parsed.Fragment != "" || parsed.Hostname() == "" {
			return nil, ErrInvalidObservationSettings
		}
		parsed.Scheme = strings.ToLower(parsed.Scheme)
		parsed.Host = strings.ToLower(parsed.Host)
		host := strings.ToLower(parsed.Hostname())
		if _, exists := hosts[host]; exists {
			return nil, ErrInvalidObservationSettings
		}
		hosts[host] = struct{}{}
		result = append(result, parsed.String())
	}
	return result, nil
}

func inventoryCandidates(inventory NetworkInventory, egresses []NetworkEgress) []NetworkEgressCandidate {
	configured := make(map[string]uuid.UUID, len(egresses))
	for _, item := range egresses {
		configured[selectorKey(item.Kind, item.Family, pointerValue(item.InterfaceName), item.SourceAddress)] = item.ID
	}
	interfaces := make(map[string]NetworkInterface, len(inventory.Interfaces))
	for _, item := range inventory.Interfaces {
		interfaces[item.Name] = item
	}
	hasRoute := make(map[string]bool)
	for _, route := range inventory.Routes {
		hasRoute[route.InterfaceName+"\x00"+route.Family] = true
	}
	hasAddress := make(map[string]bool)
	for _, address := range inventory.Addresses {
		if usableAddress(address, true) {
			hasAddress[address.InterfaceName+"\x00"+address.Family] = true
		}
	}
	var candidates []NetworkEgressCandidate
	for _, item := range inventory.Interfaces {
		if item.Loopback {
			continue
		}
		for _, family := range []string{"ipv4", "ipv6"} {
			candidate := NetworkEgressCandidate{Kind: "interface", Family: family, InterfaceName: item.Name}
			candidate.Eligible, candidate.UnavailableReason = interfaceEligibility(item, family, hasRoute, hasAddress)
			if id, exists := configured[selectorKey(candidate.Kind, family, item.Name, nil)]; exists {
				candidate.ConfiguredEgressID = &id
			}
			if hasRoute[item.Name+"\x00"+family] || hasAddress[item.Name+"\x00"+family] {
				candidates = append(candidates, candidate)
			}
		}
	}
	for _, item := range inventory.Addresses {
		if item.Scope == "loopback" || item.Scope == "link-local" || item.Scope == "multicast" || item.Scope == "unspecified" {
			continue
		}
		address := item.Address
		scope := item.Scope
		candidate := NetworkEgressCandidate{
			Kind: "source", Family: item.Family, InterfaceName: item.InterfaceName,
			SourceAddress: &address, Scope: &scope, Temporary: item.Temporary,
		}
		interfaceItem := interfaces[item.InterfaceName]
		candidate.Eligible, candidate.UnavailableReason = sourceEligibility(item, interfaceItem, hasRoute)
		if id, exists := configured[selectorKey(candidate.Kind, candidate.Family, candidate.InterfaceName, candidate.SourceAddress)]; exists {
			candidate.ConfiguredEgressID = &id
		}
		candidates = append(candidates, candidate)
	}
	slices.SortFunc(candidates, func(a, b NetworkEgressCandidate) int {
		return strings.Compare(a.Family+"\x00"+a.InterfaceName+"\x00"+a.Kind+"\x00"+pointerValue(a.SourceAddress), b.Family+"\x00"+b.InterfaceName+"\x00"+b.Kind+"\x00"+pointerValue(b.SourceAddress))
	})
	return candidates
}

func egressAvailability(inventory NetworkInventory) map[string]bool {
	result := make(map[string]bool)
	interfaces := make(map[string]NetworkInterface, len(inventory.Interfaces))
	for _, item := range inventory.Interfaces {
		interfaces[item.Name] = item
	}
	hasRoute := make(map[string]bool)
	hasDefault := make(map[string]bool)
	for _, route := range inventory.Routes {
		item, found := interfaces[route.InterfaceName]
		if !found || !item.Up || item.Loopback {
			continue
		}
		hasRoute[route.InterfaceName+"\x00"+route.Family] = true
		if route.Default {
			hasDefault[route.InterfaceName+"\x00"+route.Family] = true
		}
	}
	hasAddress := make(map[string]bool)
	for _, address := range inventory.Addresses {
		item := interfaces[address.InterfaceName]
		if item.Up && !item.Loopback && usableAddress(address, true) {
			hasAddress[address.InterfaceName+"\x00"+address.Family] = true
			if hasRoute[address.InterfaceName+"\x00"+address.Family] && usableAddress(address, false) {
				value := address.Address
				result[selectorKey("source", address.Family, address.InterfaceName, &value)] = true
			}
		}
	}
	for _, item := range inventory.Interfaces {
		for _, family := range []string{"ipv4", "ipv6"} {
			key := item.Name + "\x00" + family
			available := item.Up && !item.Loopback && hasRoute[key] && hasAddress[key]
			result[selectorKey("interface", family, item.Name, nil)] = available
			if available && hasDefault[key] {
				result[selectorKey("default", family, "", nil)] = true
			}
		}
	}
	return result
}

func interfaceEligibility(item NetworkInterface, family string, routes, addresses map[string]bool) (bool, *string) {
	if !item.Up {
		return false, stringPointer("interface-down")
	}
	key := item.Name + "\x00" + family
	if !routes[key] || !addresses[key] {
		return false, stringPointer("no-usable-route")
	}
	return true, nil
}

func sourceEligibility(address NetworkAddress, item NetworkInterface, routes map[string]bool) (bool, *string) {
	if address.Temporary {
		return false, stringPointer("temporary-address")
	}
	if !usableAddress(address, false) {
		return false, stringPointer("unusable-address")
	}
	if !item.Up {
		return false, stringPointer("interface-down")
	}
	if !routes[address.InterfaceName+"\x00"+address.Family] {
		return false, stringPointer("no-usable-route")
	}
	return true, nil
}

func usableAddress(address NetworkAddress, allowTemporary bool) bool {
	if address.Tentative || address.Deprecated || address.Duplicate || (!allowTemporary && address.Temporary) {
		return false
	}
	switch address.Scope {
	case "global", "private", "shared", "unique-local":
		return true
	default:
		return false
	}
}

func requireMutableNode(ctx context.Context, queries *configdb.Queries, nodeID uuid.UUID) error {
	record, err := queries.GetNodeByID(ctx, nodeID.String())
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNodeNotFound
	}
	if err != nil {
		return err
	}
	if record.RevokedAt != nil {
		return ErrNodeRevoked
	}
	if deletion, err := queries.GetNodeDeletion(ctx, nodeID.String()); err == nil && deletion.Status != "completed" {
		return ErrNodeDeletionPending
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	return nil
}

func incrementNodeConfiguration(ctx context.Context, queries *configdb.Queries, nodeID string) error {
	changed, err := queries.IncrementNodeDesiredConfigurationRevision(ctx, nodeID)
	if err != nil {
		return err
	}
	if changed != 1 {
		return ErrNodeRevoked
	}
	return nil
}

func egressFromRecord(record configdb.NetworkEgress) (NetworkEgress, error) {
	id, err := uuid.Parse(record.ID)
	if err != nil {
		return NetworkEgress{}, fmt.Errorf("parse stored egress ID %q: %w", record.ID, err)
	}
	nodeID, err := uuid.Parse(record.NodeID)
	if err != nil {
		return NetworkEgress{}, fmt.Errorf("parse stored egress node ID %q: %w", record.NodeID, err)
	}
	var proxyID *uuid.UUID
	if record.ProxyID != nil {
		value, err := uuid.Parse(*record.ProxyID)
		if err != nil {
			return NetworkEgress{}, fmt.Errorf("parse stored egress proxy ID %q: %w", *record.ProxyID, err)
		}
		proxyID = &value
	}
	return NetworkEgress{
		ID: id, NodeID: nodeID, Name: record.Name, Kind: record.Kind, Family: record.Family,
		InterfaceName: record.InterfaceName, SourceAddress: record.SourceAddress, ProxyID: proxyID,
		Enabled: record.Enabled == 1, Available: record.Available == 1, Automatic: record.Automatic == 1,
		LightweightIntervalSeconds: record.LightweightIntervalSeconds,
	}, nil
}

func validInterfaceName(value string) bool {
	return value == strings.TrimSpace(value) && utf8.RuneCountInString(value) >= 1 && utf8.RuneCountInString(value) <= 64 &&
		!strings.ContainsAny(value, "\x00\r\n\t")
}

func validBoundedText(value string, limit int) bool {
	count := utf8.RuneCountInString(value)
	return value == strings.TrimSpace(value) && count >= 1 && count <= limit && !strings.ContainsRune(value, '\x00')
}

func familyOf(address netip.Addr) string {
	if address.Is4() {
		return "ipv4"
	}
	return "ipv6"
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

func selectorKey(kind, family, interfaceName string, sourceAddress *string) string {
	return kind + "\x00" + family + "\x00" + interfaceName + "\x00" + pointerValue(sourceAddress)
}

func pointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func stringPointer(value string) *string { return &value }
