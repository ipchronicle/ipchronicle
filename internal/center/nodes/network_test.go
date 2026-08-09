package nodes

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/center/database"
	"github.com/ipchronicle/ipchronicle/internal/center/database/configdb"
)

func TestInventoryCreatesDefaultsAndControlsDiscoveredEgresses(t *testing.T) {
	ctx := context.Background()
	store, err := database.Open(ctx, database.PathsFromDataDirectory(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	connections := &testSyncConnections{}
	service := NewService(store.Config, store.History, store.ConfigQueries, store.MasterKey, connections)
	now := time.Date(2026, 8, 9, 6, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	enrollment, err := service.RotateEnrollmentKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	registration, err := service.Register(ctx, enrollment.Key, testMetadata())
	if err != nil {
		t.Fatal(err)
	}

	inventory := testNetworkInventory(now)
	poll, err := service.Poll(ctx, registration.Credential, testMetadata(), 0, nil, nil, &inventory, nil)
	if err != nil {
		t.Fatal(err)
	}
	if poll.DesiredConfigurationRevision != 2 {
		t.Fatalf("desired revision = %d, want 2 after default egress creation", poll.DesiredConfigurationRevision)
	}
	state, err := service.Network(ctx, registration.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Inventory == nil || state.InventoryError != nil || len(state.Egresses) != 2 {
		t.Fatalf("network state after inventory = %#v", state)
	}
	for _, egress := range state.Egresses {
		if egress.Kind != "default" || !egress.Automatic || !egress.Enabled || !egress.Available {
			t.Fatalf("unexpected automatic egress: %#v", egress)
		}
	}
	var stable, temporary *NetworkEgressCandidate
	for index := range state.Candidates {
		candidate := &state.Candidates[index]
		if candidate.SourceAddress != nil && *candidate.SourceAddress == "10.0.0.5" {
			stable = candidate
		}
		if candidate.SourceAddress != nil && *candidate.SourceAddress == "2001:4860::99" {
			temporary = candidate
		}
	}
	if stable == nil || !stable.Eligible {
		t.Fatalf("stable private source candidate = %#v", stable)
	}
	if temporary == nil || temporary.Eligible || temporary.UnavailableReason == nil || *temporary.UnavailableReason != "temporary-address" {
		t.Fatalf("temporary IPv6 candidate = %#v", temporary)
	}

	address := "10.0.0.5"
	created, err := service.CreateEgress(ctx, registration.NodeID, NetworkEgressSelector{
		Kind: "source", Family: "ipv4", InterfaceName: "eth0", SourceAddress: &address,
	})
	if err != nil || !created.Enabled || !created.Available || created.Automatic {
		t.Fatalf("created source egress = %#v, %v", created, err)
	}
	if _, err := service.CreateEgress(ctx, registration.NodeID, NetworkEgressSelector{
		Kind: "source", Family: "ipv4", InterfaceName: "eth0", SourceAddress: &address,
	}); !errors.Is(err, ErrEgressAlreadyExists) {
		t.Fatalf("duplicate egress error = %v", err)
	}
	if _, err := service.CreateEgress(ctx, registration.NodeID, NetworkEgressSelector{
		Kind: "source", Family: "ipv6", InterfaceName: "eth0", SourceAddress: temporary.SourceAddress,
	}); !errors.Is(err, ErrInvalidEgressCandidate) {
		t.Fatalf("temporary source creation error = %v", err)
	}
	configuration, err := service.Configuration(ctx, registration.Credential)
	if err != nil || configuration.Revision != 3 || len(configuration.Egresses) != 3 {
		t.Fatalf("configuration after source creation = %#v, %v", configuration, err)
	}
	for index := 0; index < maxNodeEgresses-len(configuration.Egresses); index++ {
		interfaceName := fmt.Sprintf("test%d", index)
		if err := service.queries.CreateNodeEgress(ctx, configdb.CreateNodeEgressParams{
			ID: uuid.NewString(), NodeID: registration.NodeID.String(), Name: "test-" + interfaceName,
			Kind: "interface", Family: "ipv4", InterfaceName: &interfaceName,
			Enabled: 1, Available: 1, Automatic: 0, LightweightIntervalSeconds: 600,
			ProbeOnAddressChange: 1, CreatedAt: now.Unix(), UpdatedAt: now.Unix(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := service.CreateEgress(ctx, registration.NodeID, NetworkEgressSelector{
		Kind: "interface", Family: "ipv4", InterfaceName: "eth0",
	}); !errors.Is(err, ErrEgressLimitReached) {
		t.Fatalf("egress limit error = %v", err)
	}

	disabled, err := service.SetEgressEnabled(ctx, registration.NodeID, created.ID, false)
	if err != nil || disabled.Enabled {
		t.Fatalf("disabled egress = %#v, %v", disabled, err)
	}
	if err := service.DeleteEgress(ctx, registration.NodeID, state.Egresses[0].ID); err != nil {
		t.Fatalf("delete automatic egress: %v", err)
	}
	if err := service.DeleteEgress(ctx, registration.NodeID, created.ID); err != nil {
		t.Fatal(err)
	}
	if len(connections.wakes) != 4 {
		t.Fatalf("configuration wake count = %d, want 4", len(connections.wakes))
	}
}

func TestInventoryFailurePreservesLastValidSnapshotAndMissingSelector(t *testing.T) {
	ctx := context.Background()
	store, err := database.Open(ctx, database.PathsFromDataDirectory(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := NewService(store.Config, store.History, store.ConfigQueries, store.MasterKey, &testSyncConnections{})
	now := time.Date(2026, 8, 9, 7, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	enrollment, _ := service.RotateEnrollmentKey(ctx)
	registration, err := service.Register(ctx, enrollment.Key, testMetadata())
	if err != nil {
		t.Fatal(err)
	}
	inventory := testNetworkInventory(now)
	if _, err := service.Poll(ctx, registration.Credential, testMetadata(), 0, nil, nil, &inventory, nil); err != nil {
		t.Fatal(err)
	}
	address := "10.0.0.5"
	created, err := service.CreateEgress(ctx, registration.NodeID, NetworkEgressSelector{
		Kind: "source", Family: "ipv4", InterfaceName: "eth0", SourceAddress: &address,
	})
	if err != nil {
		t.Fatal(err)
	}
	message := "read /proc/net/route: input/output error"
	now = now.Add(time.Minute)
	if _, err := service.Poll(ctx, registration.Credential, testMetadata(), 0, nil, nil, nil, &message); err != nil {
		t.Fatal(err)
	}
	failed, err := service.Network(ctx, registration.NodeID)
	if err != nil || failed.Inventory == nil || failed.InventoryError == nil || *failed.InventoryError != message {
		t.Fatalf("state after inventory failure = %#v, %v", failed, err)
	}
	if _, err := service.CreateEgress(ctx, registration.NodeID, NetworkEgressSelector{
		Kind: "interface", Family: "ipv4", InterfaceName: "eth0",
	}); !errors.Is(err, ErrNetworkInventoryUnavailable) {
		t.Fatalf("create egress from stale inventory error = %v", err)
	}

	empty := NetworkInventory{CapturedAt: now.Add(time.Minute), Interfaces: []NetworkInterface{{Name: "eth0", Index: 2}}}
	if _, err := service.Poll(ctx, registration.Credential, testMetadata(), 0, nil, nil, &empty, nil); err != nil {
		t.Fatal(err)
	}
	missing, err := service.Network(ctx, registration.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	for _, egress := range missing.Egresses {
		if egress.ID == created.ID && egress.Available {
			t.Fatalf("missing source selector remained available: %#v", egress)
		}
	}
}

func testNetworkInventory(capturedAt time.Time) NetworkInventory {
	gateway4 := "10.0.0.1"
	gateway6 := "fe80::1"
	return NetworkInventory{
		CapturedAt: capturedAt,
		Interfaces: []NetworkInterface{{Name: "eth0", Index: 2, Up: true}},
		Addresses: []NetworkAddress{
			{InterfaceName: "eth0", Address: "10.0.0.5", PrefixLength: 24, Family: "ipv4", Scope: "private"},
			{InterfaceName: "eth0", Address: "fd00::5", PrefixLength: 64, Family: "ipv6", Scope: "unique-local"},
			{InterfaceName: "eth0", Address: "2001:4860::99", PrefixLength: 64, Family: "ipv6", Scope: "global", Temporary: true},
		},
		Routes: []NetworkRoute{
			{InterfaceName: "eth0", Family: "ipv4", Destination: "0.0.0.0/0", Gateway: &gateway4, Metric: 100, Default: true},
			{InterfaceName: "eth0", Family: "ipv4", Destination: "10.0.0.0/24", Metric: 100},
			{InterfaceName: "eth0", Family: "ipv6", Destination: "::/0", Gateway: &gateway6, Metric: 100, Default: true},
		},
	}
}
