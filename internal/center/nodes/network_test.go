package nodes

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/center/database"
)

func TestInventoryCreatesAutomaticDiscoveryPaths(t *testing.T) {
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
	enrollment, err := service.RotateEnrollmentKey(ctx, "UTC")
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
	configuration, err := service.Configuration(ctx, registration.Credential)
	if err != nil || configuration.Revision != 2 || len(configuration.DiscoveryPaths) != 4 || len(configuration.ProbeTargets) != 0 {
		t.Fatalf("configuration after automatic discovery paths = %#v, %v", configuration, err)
	}
	stablePathFound := false
	for _, egress := range configuration.DiscoveryPaths {
		if !egress.Automatic || !egress.Enabled || !egress.Available {
			t.Fatalf("unexpected automatic egress: %#v", egress)
		}
		stablePathFound = stablePathFound || egress.Kind == "source" && egress.SourceAddress != nil && *egress.SourceAddress == "10.0.0.5"
	}
	if !stablePathFound {
		t.Fatalf("stable source path was not configured: %#v", configuration.DiscoveryPaths)
	}
	var stable, temporary *NetworkEgressCandidate
	candidates := inventoryCandidates(inventory, configuration.DiscoveryPaths)
	for index := range candidates {
		candidate := &candidates[index]
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
	for _, path := range configuration.DiscoveryPaths {
		if path.SourceAddress != nil && *path.SourceAddress == "2001:4860::99" {
			t.Fatalf("temporary IPv6 address created a discovery path: %#v", path)
		}
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
	enrollment, _ := service.RotateEnrollmentKey(ctx, "UTC")
	registration, err := service.Register(ctx, enrollment.Key, testMetadata())
	if err != nil {
		t.Fatal(err)
	}
	inventory := testNetworkInventory(now)
	if _, err := service.Poll(ctx, registration.Credential, testMetadata(), 0, nil, nil, &inventory, nil); err != nil {
		t.Fatal(err)
	}
	configuration, err := service.Configuration(ctx, registration.Credential)
	if err != nil {
		t.Fatal(err)
	}
	var created NetworkEgress
	for _, path := range configuration.DiscoveryPaths {
		if path.Kind == "source" && path.SourceAddress != nil && *path.SourceAddress == "10.0.0.5" {
			created = path
			break
		}
	}
	if created.ID == uuid.Nil {
		t.Fatalf("automatic source path not found: %#v", configuration.DiscoveryPaths)
	}
	message := "read /proc/net/route: input/output error"
	now = now.Add(time.Minute)
	if _, err := service.Poll(ctx, registration.Credential, testMetadata(), 0, nil, nil, nil, &message); err != nil {
		t.Fatal(err)
	}
	record, err := service.queries.GetNodeNetworkInventory(ctx, registration.NodeID.String())
	if err != nil || record.Payload == nil || record.LastError == nil || *record.LastError != message {
		t.Fatalf("stored inventory after failure = %#v, %v", record, err)
	}
	var preserved NetworkInventory
	if err := json.Unmarshal([]byte(*record.Payload), &preserved); err != nil {
		t.Fatal(err)
	}
	if preserved.CapturedAt != inventory.CapturedAt || len(preserved.Addresses) != len(inventory.Addresses) {
		t.Fatalf("last valid inventory was not preserved: %#v", preserved)
	}

	empty := NetworkInventory{CapturedAt: now.Add(time.Minute), Interfaces: []NetworkInterface{{Name: "eth0", Index: 2}}}
	if _, err := service.Poll(ctx, registration.Credential, testMetadata(), 0, nil, nil, &empty, nil); err != nil {
		t.Fatal(err)
	}
	configuration, err = service.Configuration(ctx, registration.Credential)
	if err != nil {
		t.Fatal(err)
	}
	for _, egress := range configuration.DiscoveryPaths {
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
