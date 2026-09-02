package nodes

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/ipchronicle/ipchronicle/internal/center/database"
	"github.com/ipchronicle/ipchronicle/internal/center/database/configdb"
)

func TestNetworkProxyCredentialsAndReferencedConfiguration(t *testing.T) {
	ctx := context.Background()
	paths := database.PathsFromDataDirectory(t.TempDir())
	store, err := database.Open(ctx, paths)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	connections := &testSyncConnections{}
	service := NewService(store.Config, store.History, store.ConfigQueries, store.MasterKey, connections)
	now := time.Date(2026, 8, 9, 8, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	enrollment, err := service.RotateEnrollmentKey(ctx, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Register(ctx, enrollment.Key, testMetadata())
	if err != nil {
		t.Fatal(err)
	}
	secondMetadata := testMetadata()
	secondMetadata.Hostname = "edge-two"
	second, err := service.Register(ctx, enrollment.Key, secondMetadata)
	if err != nil {
		t.Fatal(err)
	}

	password := "proxy-password-that-must-not-be-plaintext"
	username := "probe-user"
	proxy, err := service.CreateNetworkProxy(ctx, first.NodeID, NetworkProxyCreate{
		Name: "Primary proxy", Scheme: "socks5", Host: "proxy.example.test", Port: 1080,
		Username: &username, Password: &password,
	})
	if err != nil || proxy.NodeID != first.NodeID || proxy.Status != "checking" ||
		!proxy.PasswordConfigured || proxy.Username == nil || *proxy.Username != username {
		t.Fatalf("created proxy = %#v, %v", proxy, err)
	}
	record, err := store.ConfigQueries.GetNodeNetworkProxy(ctx, configdb.GetNodeNetworkProxyParams{
		NodeID: first.NodeID.String(), ID: proxy.ID.String(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(record.PasswordEncrypted) == 0 || bytes.Contains(record.PasswordEncrypted, []byte(password)) {
		t.Fatal("center proxy password was not protected at rest")
	}
	if _, err := decryptProxyPassword(store.MasterKey, proxy.ID.String(), record.PasswordEncrypted); err != nil {
		t.Fatalf("decrypt stored proxy password: %v", err)
	}
	if _, err := service.CreateNetworkProxy(ctx, first.NodeID, NetworkProxyCreate{
		Name: "primary PROXY", Scheme: "http", Host: "192.0.2.10", Port: 8080,
	}); !errors.Is(err, ErrNetworkProxyAlreadyExists) {
		t.Fatalf("duplicate proxy name error = %v", err)
	}
	secondProxy, err := service.CreateNetworkProxy(ctx, second.NodeID, NetworkProxyCreate{
		Name: "PRIMARY PROXY", Scheme: "https", Host: "2001:db8::10", Port: 8443,
	})
	if err != nil {
		t.Fatalf("same name on another node: %v", err)
	}
	configuration, err := service.Configuration(ctx, first.Credential)
	if err != nil {
		t.Fatal(err)
	}
	if configuration.SchemaVersion != 9 || configuration.Revision != 2 || len(configuration.DiscoveryPaths) != 2 || len(configuration.ProbeTargets) != 0 || len(configuration.Proxies) != 1 || len(configuration.DiscoveryServices.IPv4) < 2 {
		t.Fatalf("referenced proxy configuration = %#v", configuration)
	}
	if configuration.DiscoveryPaths[0].Family == configuration.DiscoveryPaths[1].Family {
		t.Fatalf("proxy discovery families = %#v", configuration.DiscoveryPaths)
	}
	if configuration.Proxies[0].ID != proxy.ID || configuration.Proxies[0].Password == nil || *configuration.Proxies[0].Password != password {
		t.Fatalf("delivered proxy credential = %#v", configuration.Proxies[0])
	}
	beforeToggleWakes := len(connections.wakes)
	now = now.Add(time.Minute)
	disabled, err := service.UpdateNetworkProxy(ctx, first.NodeID, proxy.ID, NetworkProxyUpdate{
		Name: proxy.Name, Scheme: proxy.Scheme, Host: proxy.Host, Port: proxy.Port,
		Username: proxy.Username, Enabled: false, PasswordAction: "keep",
	})
	if err != nil || disabled.Enabled || disabled.Status != "disabled" ||
		disabled.IPv4.Status != "disabled" || disabled.IPv6.Status != "disabled" {
		t.Fatalf("disabled proxy = %#v, %v", disabled, err)
	}
	disabledConfiguration, err := service.Configuration(ctx, first.Credential)
	if err != nil || disabledConfiguration.Revision != 3 || len(disabledConfiguration.DiscoveryPaths) != 0 || len(disabledConfiguration.Proxies) != 0 {
		t.Fatalf("configuration with disabled proxy = %#v, %v", disabledConfiguration, err)
	}
	if len(connections.wakes) != beforeToggleWakes+1 {
		t.Fatalf("disable wake count = %d, want %d", len(connections.wakes), beforeToggleWakes+1)
	}
	if _, err := service.UpdateNetworkProxy(ctx, first.NodeID, proxy.ID, NetworkProxyUpdate{
		Name: proxy.Name, Scheme: proxy.Scheme, Host: proxy.Host, Port: proxy.Port,
		Username: proxy.Username, Enabled: false, PasswordAction: "keep",
	}); err != nil {
		t.Fatal(err)
	}
	unchangedDisabledConfiguration, err := service.Configuration(ctx, first.Credential)
	if err != nil || unchangedDisabledConfiguration.Revision != 3 || len(connections.wakes) != beforeToggleWakes+1 {
		t.Fatalf("no-op proxy disable advanced configuration: %#v, wakes=%#v, err=%v", unchangedDisabledConfiguration, connections.wakes, err)
	}
	now = now.Add(time.Minute)
	reenabled, err := service.UpdateNetworkProxy(ctx, first.NodeID, proxy.ID, NetworkProxyUpdate{
		Name: proxy.Name, Scheme: proxy.Scheme, Host: proxy.Host, Port: proxy.Port,
		Username: proxy.Username, Enabled: true, PasswordAction: "keep",
	})
	if err != nil || !reenabled.Enabled || reenabled.Status != "checking" {
		t.Fatalf("re-enabled proxy = %#v, %v", reenabled, err)
	}
	reenabledConfiguration, err := service.Configuration(ctx, first.Credential)
	if err != nil || reenabledConfiguration.Revision != 4 || len(reenabledConfiguration.DiscoveryPaths) != 2 || len(reenabledConfiguration.Proxies) != 1 {
		t.Fatalf("configuration with re-enabled proxy = %#v, %v", reenabledConfiguration, err)
	}
	if len(connections.wakes) != beforeToggleWakes+2 {
		t.Fatalf("re-enable wake count = %d, want %d", len(connections.wakes), beforeToggleWakes+2)
	}
	secondConfiguration, err := service.Configuration(ctx, second.Credential)
	if err != nil || len(secondConfiguration.Proxies) != 1 || secondConfiguration.Proxies[0].ID != secondProxy.ID {
		t.Fatalf("second node proxy configuration = %#v, %v", secondConfiguration, err)
	}

	now = now.Add(time.Minute)
	systemState, err := store.ConfigQueries.GetSystemState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var ipv4Path, ipv6Path NetworkEgress
	for _, path := range configuration.DiscoveryPaths {
		if path.Family == "ipv4" {
			ipv4Path = path
		} else {
			ipv6Path = path
		}
	}
	failure := "no-valid-response"
	publicIPv4 := "8.8.8.8"
	confirmedAt := now
	if _, err := service.Poll(ctx, first.Credential, testMetadata(), 2, nil, nil, nil, nil, AddressUpload{States: []AddressState{
		{
			EgressID: ipv4Path.ID, HistoryGeneration: systemState.HistoryGeneration,
			Family: "ipv4", Status: "confirmed", Sequence: 1, PublicAddress: &publicIPv4,
			ProxyPath: true, LastCheckedAt: confirmedAt, LastSucceededAt: &confirmedAt, LastChangedAt: &confirmedAt,
		},
		{
			EgressID: ipv6Path.ID, HistoryGeneration: systemState.HistoryGeneration,
			Family: "ipv6", Status: "failed", Sequence: 1, ProxyPath: true,
			FailureReason: &failure, LastCheckedAt: now,
		},
	}}); err != nil {
		t.Fatal(err)
	}
	listed, err := service.ListNetworkProxies(ctx, first.NodeID)
	if err != nil || len(listed) != 1 || listed[0].Status != "ipv4-only" ||
		listed[0].IPv4.PublicAddress == nil || *listed[0].IPv4.PublicAddress != publicIPv4 || listed[0].IPv6.Status != "unavailable" {
		t.Fatalf("observed node proxy = %#v, %v", listed, err)
	}
	configuration, err = service.Configuration(ctx, first.Credential)
	if err != nil || len(configuration.ProbeTargets) != 1 {
		t.Fatalf("configuration with observed proxy target = %#v, %v", configuration, err)
	}
	network, err := service.Network(ctx, first.NodeID)
	if err != nil || !containsAvailablePublicAddress(network.PublicAddresses, publicIPv4) {
		t.Fatalf("network before proxy disable = %#v, %v", network, err)
	}
	now = now.Add(time.Minute)
	disabled, err = service.UpdateNetworkProxy(ctx, first.NodeID, proxy.ID, NetworkProxyUpdate{
		Name: proxy.Name, Scheme: proxy.Scheme, Host: proxy.Host, Port: proxy.Port,
		Username: proxy.Username, Enabled: false, PasswordAction: "keep",
	})
	if err != nil || disabled.Enabled {
		t.Fatalf("disable observed proxy = %#v, %v", disabled, err)
	}
	disabledConfiguration, err = service.Configuration(ctx, first.Credential)
	if err != nil || len(disabledConfiguration.DiscoveryPaths) != 0 || len(disabledConfiguration.ProbeTargets) != 0 || len(disabledConfiguration.Proxies) != 0 {
		t.Fatalf("configuration after observed proxy disable = %#v, %v", disabledConfiguration, err)
	}
	if _, err := service.Poll(
		ctx, first.Credential, testMetadata(), disabledConfiguration.Revision,
		nil, nil, nil, nil, AddressUpload{States: []AddressState{}},
	); err != nil {
		t.Fatal(err)
	}
	network, err = service.Network(ctx, first.NodeID)
	if err != nil || containsAvailablePublicAddress(network.PublicAddresses, publicIPv4) {
		t.Fatalf("network after proxy disable convergence = %#v, %v", network, err)
	}
	now = now.Add(time.Minute)
	reenabled, err = service.UpdateNetworkProxy(ctx, first.NodeID, proxy.ID, NetworkProxyUpdate{
		Name: proxy.Name, Scheme: proxy.Scheme, Host: proxy.Host, Port: proxy.Port,
		Username: proxy.Username, Enabled: true, PasswordAction: "keep",
	})
	if err != nil || !reenabled.Enabled {
		t.Fatalf("re-enable observed proxy = %#v, %v", reenabled, err)
	}
	configuration, err = service.Configuration(ctx, first.Credential)
	if err != nil || len(configuration.DiscoveryPaths) != 2 || len(configuration.ProbeTargets) != 0 || len(configuration.Proxies) != 1 {
		t.Fatalf("configuration after observed proxy re-enable = %#v, %v", configuration, err)
	}
	beforeUpdateRevision := configuration.Revision
	beforeUpdateWakes := len(connections.wakes)

	now = now.Add(time.Minute)
	updated, err := service.UpdateNetworkProxy(ctx, first.NodeID, proxy.ID, NetworkProxyUpdate{
		Name: proxy.Name, Scheme: proxy.Scheme, Host: "proxy-2.example.test", Port: proxy.Port,
		Username: proxy.Username, Enabled: true, PasswordAction: "clear",
	})
	if err != nil || updated.PasswordConfigured {
		t.Fatalf("updated proxy = %#v, %v", updated, err)
	}
	if updated.Status != "checking" {
		t.Fatalf("updated proxy status = %q, want checking", updated.Status)
	}
	configuration, err = service.Configuration(ctx, first.Credential)
	if err != nil || configuration.Revision != beforeUpdateRevision+1 || configuration.Proxies[0].Password != nil || configuration.Proxies[0].Host != "proxy-2.example.test" {
		t.Fatalf("configuration after proxy update = %#v, %v", configuration, err)
	}
	if len(connections.wakes) != beforeUpdateWakes+1 {
		t.Fatalf("configuration wake count = %d, want %d", len(connections.wakes), beforeUpdateWakes+1)
	}
	if _, err := service.UpdateNetworkProxy(ctx, first.NodeID, proxy.ID, NetworkProxyUpdate{
		Name: updated.Name, Scheme: updated.Scheme, Host: updated.Host, Port: updated.Port,
		Username: updated.Username, Enabled: true, PasswordAction: "clear",
	}); err != nil {
		t.Fatal(err)
	}
	unchanged, err := service.Configuration(ctx, first.Credential)
	if err != nil || unchanged.Revision != beforeUpdateRevision+1 || len(connections.wakes) != beforeUpdateWakes+1 {
		t.Fatalf("no-op proxy update advanced configuration: %#v, wakes=%#v, err=%v", unchanged, connections.wakes, err)
	}
	if _, err := service.UpdateNetworkProxy(ctx, second.NodeID, proxy.ID, NetworkProxyUpdate{
		Name: updated.Name, Scheme: updated.Scheme, Host: updated.Host, Port: updated.Port,
		Username: updated.Username, Enabled: true, PasswordAction: "keep",
	}); !errors.Is(err, ErrNetworkProxyNotFound) {
		t.Fatalf("cross-node proxy update error = %v", err)
	}
	deleting, err := service.DeleteNetworkProxy(ctx, first.NodeID, proxy.ID)
	if err != nil || deleting.DeletionStatus == nil || *deleting.DeletionStatus != "pending" {
		t.Fatalf("delete proxy = %#v, %v", deleting, err)
	}
	if _, err := service.UpdateNetworkProxy(ctx, first.NodeID, proxy.ID, NetworkProxyUpdate{
		Name: updated.Name, Scheme: updated.Scheme, Host: updated.Host, Port: updated.Port,
		Username: updated.Username, Enabled: true, PasswordAction: "keep",
	}); !errors.Is(err, ErrNetworkProxyDeletionPending) {
		t.Fatalf("update deleting proxy error = %v", err)
	}
	configuration, err = service.Configuration(ctx, first.Credential)
	if err != nil || configuration.Revision != beforeUpdateRevision+2 || len(configuration.DiscoveryPaths) != 0 || len(configuration.Proxies) != 0 {
		t.Fatalf("configuration during proxy deletion = %#v, %v", configuration, err)
	}
	if err := service.processDeletions(ctx, 16); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ConfigQueries.GetNodeNetworkProxy(ctx, configdb.GetNodeNetworkProxyParams{
		NodeID: first.NodeID.String(), ID: proxy.ID.String(),
	}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("deleted proxy remains: %v", err)
	}
}

func TestProxyPasswordEnvelopeIsBoundToProxyID(t *testing.T) {
	var key [32]byte
	key[0] = 7
	envelope, err := encryptProxyPassword(key, "proxy-one", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decryptProxyPassword(key, "proxy-two", envelope); err == nil {
		t.Fatal("proxy password envelope decrypted under a different proxy ID")
	}
}
