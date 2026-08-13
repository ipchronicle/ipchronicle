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
	if state.Inventory == nil || state.InventoryError != nil || len(state.Egresses) != 4 {
		t.Fatalf("network state after inventory = %#v", state)
	}
	stablePathFound := false
	for _, egress := range state.Egresses {
		if !egress.Automatic || !egress.Enabled || !egress.Available {
			t.Fatalf("unexpected automatic egress: %#v", egress)
		}
		stablePathFound = stablePathFound || egress.Kind == "source" && egress.SourceAddress != nil && *egress.SourceAddress == "10.0.0.5"
	}
	if !stablePathFound {
		t.Fatalf("stable source path was not configured: %#v", state.Egresses)
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
	if err != nil || configuration.Revision != 2 || len(configuration.DiscoveryPaths) != 4 || len(configuration.ProbeTargets) != 0 {
		t.Fatalf("configuration after automatic discovery paths = %#v, %v", configuration, err)
	}
	for index := 0; index < maxNodeEgresses-len(configuration.DiscoveryPaths); index++ {
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

	firstDeletion, err := service.DeleteEgress(ctx, registration.NodeID, state.Egresses[0].ID)
	if err != nil || firstDeletion.Status != "pending" {
		t.Fatalf("delete automatic egress: %v", err)
	}
	pending, err := service.Network(ctx, registration.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	for _, egress := range pending.Egresses {
		if egress.ID == state.Egresses[0].ID &&
			(egress.DeletionStatus == nil || *egress.DeletionStatus != "pending") {
			t.Fatalf("queued egress deletion is not visible: %#v", egress)
		}
	}
	configuration, err = service.Configuration(ctx, registration.Credential)
	if err != nil {
		t.Fatal(err)
	}
	for _, egress := range configuration.Egresses {
		if egress.ID == state.Egresses[0].ID {
			t.Fatalf("deleting egress remained in Agent configuration: %#v", egress)
		}
	}
	if err := service.processDeletions(ctx, 16); err != nil {
		t.Fatal(err)
	}
	completed, err := service.Network(ctx, registration.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	for _, egress := range completed.Egresses {
		if egress.ID == state.Egresses[0].ID {
			t.Fatalf("completed egress deletion retained configuration: %#v", egress)
		}
	}
	if len(connections.wakes) != 1 {
		t.Fatalf("configuration wake count = %d, want 1", len(connections.wakes))
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
	network, err := service.Network(ctx, registration.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	var created NetworkEgress
	for _, path := range network.Egresses {
		if path.Kind == "source" && path.SourceAddress != nil && *path.SourceAddress == "10.0.0.5" {
			created = path
			break
		}
	}
	if created.ID == uuid.Nil {
		t.Fatalf("automatic source path not found: %#v", network.Egresses)
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
