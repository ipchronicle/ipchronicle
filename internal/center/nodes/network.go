package nodes

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/center/database/configdb"
)

const (
	defaultLightweightInterval = 10 * time.Minute
	maxNodeEgresses            = 64
)

var (
	ErrNetworkInventoryUnavailable = errors.New("node network inventory is unavailable")
	ErrInvalidNetworkInventory     = errors.New("node network inventory is invalid")
	ErrInvalidEgressCandidate      = errors.New("network egress candidate is invalid or unavailable")
	ErrEgressAlreadyExists         = errors.New("network egress already exists")
	ErrEgressLimitReached          = errors.New("node network egress limit reached")
	ErrEgressNotFound              = errors.New("network egress does not exist")
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
	ProbeOnAddressChange       bool
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
	Inventory           *NetworkInventory
	InventoryError      *string
	InventoryReceivedAt *time.Time
	Egresses            []NetworkEgress
	Candidates          []NetworkEgressCandidate
}

type NetworkEgressSelector struct {
	Kind          string
	Family        string
	InterfaceName string
	SourceAddress *string
	ProxyID       *uuid.UUID
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
				ProbeOnAddressChange:       1, CreatedAt: now, UpdatedAt: now,
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
	record, err := s.queries.GetNodeNetworkInventory(ctx, id.String())
	if err == nil {
		state.InventoryError = record.LastError
		receivedAt := time.Unix(record.ReceivedAt, 0).UTC()
		state.InventoryReceivedAt = &receivedAt
		if record.Payload != nil {
			var inventory NetworkInventory
			if err := json.Unmarshal([]byte(*record.Payload), &inventory); err != nil {
				return NodeNetworkState{}, fmt.Errorf("decode stored node network inventory: %w", err)
			}
			if err := validateNetworkReport(&inventory, nil); err != nil {
				return NodeNetworkState{}, fmt.Errorf("validate stored node network inventory: %w", err)
			}
			state.Inventory = &inventory
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return NodeNetworkState{}, err
	}
	egressRecords, err := s.queries.ListNodeEgresses(ctx, id.String())
	if err != nil {
		return NodeNetworkState{}, err
	}
	state.Egresses = make([]NetworkEgress, 0, len(egressRecords))
	for _, item := range egressRecords {
		egress, err := egressFromRecord(item)
		if err != nil {
			return NodeNetworkState{}, err
		}
		state.Egresses = append(state.Egresses, egress)
	}
	if state.Inventory != nil {
		state.Candidates = inventoryCandidates(*state.Inventory, state.Egresses)
	}
	return state, nil
}

func (s *Service) CreateEgress(ctx context.Context, nodeID uuid.UUID, selector NetworkEgressSelector) (NetworkEgress, error) {
	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return NetworkEgress{}, err
	}
	defer transaction.Rollback()
	queries := s.queries.WithTx(transaction)
	if err := requireMutableNode(ctx, queries, nodeID); err != nil {
		return NetworkEgress{}, err
	}
	selector, err = normalizeSelector(selector)
	if err != nil {
		return NetworkEgress{}, err
	}
	name := ""
	var interfaceName *string
	if selector.Kind == "proxy" {
		proxy, proxyErr := queries.GetNetworkProxy(ctx, selector.ProxyID.String())
		if errors.Is(proxyErr, sql.ErrNoRows) {
			return NetworkEgress{}, ErrNetworkProxyNotFound
		}
		if proxyErr != nil {
			return NetworkEgress{}, proxyErr
		}
		proxyID := selector.ProxyID.String()
		_, err = queries.GetNodeEgressByProxy(ctx, configdb.GetNodeEgressByProxyParams{
			NodeID: nodeID.String(), Family: selector.Family, ProxyID: &proxyID,
		})
		name = "proxy:" + proxy.Name + ":" + selector.Family
	} else {
		inventoryRecord, inventoryErr := queries.GetNodeNetworkInventory(ctx, nodeID.String())
		if errors.Is(inventoryErr, sql.ErrNoRows) || inventoryRecord.Payload == nil || inventoryRecord.LastError != nil {
			return NetworkEgress{}, ErrNetworkInventoryUnavailable
		}
		if inventoryErr != nil {
			return NetworkEgress{}, inventoryErr
		}
		var inventory NetworkInventory
		if err := json.Unmarshal([]byte(*inventoryRecord.Payload), &inventory); err != nil {
			return NetworkEgress{}, fmt.Errorf("decode stored node network inventory: %w", err)
		}
		available := egressAvailability(inventory)
		if !available[selectorKey(selector.Kind, selector.Family, selector.InterfaceName, selector.SourceAddress)] {
			return NetworkEgress{}, ErrInvalidEgressCandidate
		}
		interfaceName = &selector.InterfaceName
		_, err = queries.GetNodeEgressBySelector(ctx, configdb.GetNodeEgressBySelectorParams{
			NodeID: nodeID.String(), Kind: selector.Kind, Family: selector.Family,
			InterfaceName: interfaceName, SourceAddress: selector.SourceAddress,
		})
		name = selector.Kind + ":" + selector.InterfaceName + ":" + selector.Family
		if selector.SourceAddress != nil {
			name = selector.Kind + ":" + *selector.SourceAddress
		}
	}
	if err == nil {
		return NetworkEgress{}, ErrEgressAlreadyExists
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return NetworkEgress{}, err
	}
	egresses, err := queries.ListNodeEgresses(ctx, nodeID.String())
	if err != nil {
		return NetworkEgress{}, err
	}
	if len(egresses) >= maxNodeEgresses {
		return NetworkEgress{}, ErrEgressLimitReached
	}
	now := s.now().UTC().Truncate(time.Second).Unix()
	id := uuid.New()
	var proxyID *string
	if selector.ProxyID != nil {
		value := selector.ProxyID.String()
		proxyID = &value
	}
	if err := queries.CreateNodeEgress(ctx, configdb.CreateNodeEgressParams{
		ID: id.String(), NodeID: nodeID.String(), Name: name, Kind: selector.Kind, Family: selector.Family,
		InterfaceName: interfaceName, SourceAddress: selector.SourceAddress, ProxyID: proxyID,
		Enabled: 1, Available: 1, Automatic: 0,
		LightweightIntervalSeconds: int64(defaultLightweightInterval / time.Second), ProbeOnAddressChange: 1,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		return NetworkEgress{}, err
	}
	if err := incrementNodeConfiguration(ctx, queries, nodeID.String()); err != nil {
		return NetworkEgress{}, err
	}
	created, err := queries.GetNodeEgress(ctx, configdb.GetNodeEgressParams{NodeID: nodeID.String(), ID: id.String()})
	if err != nil {
		return NetworkEgress{}, err
	}
	if err := transaction.Commit(); err != nil {
		return NetworkEgress{}, err
	}
	s.sync.Wake(nodeID.String())
	return egressFromRecord(created)
}

func (s *Service) SetEgressEnabled(ctx context.Context, nodeID, egressID uuid.UUID, enabled bool) (NetworkEgress, error) {
	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return NetworkEgress{}, err
	}
	defer transaction.Rollback()
	queries := s.queries.WithTx(transaction)
	if err := requireMutableNode(ctx, queries, nodeID); err != nil {
		return NetworkEgress{}, err
	}
	record, err := queries.GetNodeEgress(ctx, configdb.GetNodeEgressParams{NodeID: nodeID.String(), ID: egressID.String()})
	if errors.Is(err, sql.ErrNoRows) {
		return NetworkEgress{}, ErrEgressNotFound
	}
	if err != nil {
		return NetworkEgress{}, err
	}
	changed := (record.Enabled == 1) != enabled
	if changed {
		now := s.now().UTC().Truncate(time.Second).Unix()
		value := boolInteger(enabled)
		if _, err := queries.SetNodeEgressEnabled(ctx, configdb.SetNodeEgressEnabledParams{
			Enabled: value, UpdatedAt: now, ID: egressID.String(), NodeID: nodeID.String(), Enabled_2: value,
		}); err != nil {
			return NetworkEgress{}, err
		}
		if err := incrementNodeConfiguration(ctx, queries, nodeID.String()); err != nil {
			return NetworkEgress{}, err
		}
		record.Enabled = value
	}
	if err := transaction.Commit(); err != nil {
		return NetworkEgress{}, err
	}
	if changed {
		s.sync.Wake(nodeID.String())
	}
	return egressFromRecord(record)
}

func (s *Service) DeleteEgress(ctx context.Context, nodeID, egressID uuid.UUID) error {
	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	queries := s.queries.WithTx(transaction)
	if err := requireMutableNode(ctx, queries, nodeID); err != nil {
		return err
	}
	changed, err := queries.DeleteNodeEgress(ctx, configdb.DeleteNodeEgressParams{ID: egressID.String(), NodeID: nodeID.String()})
	if err != nil {
		return err
	}
	if changed != 1 {
		return ErrEgressNotFound
	}
	if err := incrementNodeConfiguration(ctx, queries, nodeID.String()); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return err
	}
	s.sync.Wake(nodeID.String())
	return nil
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

func normalizeSelector(selector NetworkEgressSelector) (NetworkEgressSelector, error) {
	if (selector.Family != "ipv4" && selector.Family != "ipv6") ||
		(selector.Kind != "interface" && selector.Kind != "source" && selector.Kind != "proxy") {
		return NetworkEgressSelector{}, ErrInvalidEgressCandidate
	}
	if selector.Kind == "proxy" {
		if selector.ProxyID == nil || selector.InterfaceName != "" || selector.SourceAddress != nil {
			return NetworkEgressSelector{}, ErrInvalidEgressCandidate
		}
		return selector, nil
	}
	if selector.ProxyID != nil || !validInterfaceName(selector.InterfaceName) ||
		(selector.Kind == "interface" && selector.SourceAddress != nil) || (selector.Kind == "source" && selector.SourceAddress == nil) {
		return NetworkEgressSelector{}, ErrInvalidEgressCandidate
	}
	if selector.SourceAddress != nil {
		address, err := netip.ParseAddr(*selector.SourceAddress)
		if err != nil || address != address.Unmap() || familyOf(address) != selector.Family {
			return NetworkEgressSelector{}, ErrInvalidEgressCandidate
		}
		canonical := address.String()
		selector.SourceAddress = &canonical
	}
	return selector, nil
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
		ProbeOnAddressChange:       record.ProbeOnAddressChange == 1,
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
