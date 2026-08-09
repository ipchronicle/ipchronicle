package nodes

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ipchronicle/ipchronicle/internal/center/database"
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

	password := "proxy-password-that-must-not-be-plaintext"
	username := "probe-user"
	proxy, err := service.CreateNetworkProxy(ctx, NetworkProxyCreate{
		Name: "Primary proxy", Scheme: "socks5", Host: "proxy.example.test", Port: 1080,
		Username: &username, Password: &password,
	})
	if err != nil || !proxy.PasswordConfigured || proxy.Username == nil || *proxy.Username != username {
		t.Fatalf("created proxy = %#v, %v", proxy, err)
	}
	record, err := store.ConfigQueries.GetNetworkProxy(ctx, proxy.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(record.PasswordEncrypted) == 0 || bytes.Contains(record.PasswordEncrypted, []byte(password)) {
		t.Fatal("center proxy password was not protected at rest")
	}
	if _, err := decryptProxyPassword(store.MasterKey, proxy.ID.String(), record.PasswordEncrypted); err != nil {
		t.Fatalf("decrypt stored proxy password: %v", err)
	}
	if _, err := service.CreateNetworkProxy(ctx, NetworkProxyCreate{
		Name: "primary PROXY", Scheme: "http", Host: "192.0.2.10", Port: 8080,
	}); !errors.Is(err, ErrNetworkProxyAlreadyExists) {
		t.Fatalf("duplicate proxy name error = %v", err)
	}
	unused, err := service.CreateNetworkProxy(ctx, NetworkProxyCreate{
		Name: "Unused proxy", Scheme: "https", Host: "2001:db8::10", Port: 8443,
	})
	if err != nil {
		t.Fatal(err)
	}

	enrollment, err := service.RotateEnrollmentKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	registration, err := service.Register(ctx, enrollment.Key, testMetadata())
	if err != nil {
		t.Fatal(err)
	}
	egress, err := service.CreateEgress(ctx, registration.NodeID, NetworkEgressSelector{
		Kind: "proxy", Family: "ipv4", ProxyID: &proxy.ID,
	})
	if err != nil || egress.ProxyID == nil || *egress.ProxyID != proxy.ID || !egress.Available {
		t.Fatalf("created proxy egress = %#v, %v", egress, err)
	}
	if _, err := service.CreateEgress(ctx, registration.NodeID, NetworkEgressSelector{
		Kind: "proxy", Family: "ipv4", ProxyID: &proxy.ID,
	}); !errors.Is(err, ErrEgressAlreadyExists) {
		t.Fatalf("duplicate proxy egress error = %v", err)
	}
	configuration, err := service.Configuration(ctx, registration.Credential)
	if err != nil {
		t.Fatal(err)
	}
	if configuration.SchemaVersion != 5 || configuration.Revision != 2 || len(configuration.Egresses) != 1 || len(configuration.Proxies) != 1 || len(configuration.DiscoveryServices.IPv4) < 2 {
		t.Fatalf("referenced proxy configuration = %#v", configuration)
	}
	if configuration.Proxies[0].ID != proxy.ID || configuration.Proxies[0].Password == nil || *configuration.Proxies[0].Password != password {
		t.Fatalf("delivered proxy credential = %#v", configuration.Proxies[0])
	}
	if configuration.Proxies[0].ID == unused.ID {
		t.Fatal("unreferenced proxy was delivered to the Agent")
	}

	now = now.Add(time.Minute)
	updated, err := service.UpdateNetworkProxy(ctx, proxy.ID, NetworkProxyUpdate{
		Name: proxy.Name, Scheme: proxy.Scheme, Host: "proxy-2.example.test", Port: proxy.Port,
		Username: proxy.Username, PasswordAction: "clear",
	})
	if err != nil || updated.PasswordConfigured {
		t.Fatalf("updated proxy = %#v, %v", updated, err)
	}
	configuration, err = service.Configuration(ctx, registration.Credential)
	if err != nil || configuration.Revision != 3 || configuration.Proxies[0].Password != nil || configuration.Proxies[0].Host != "proxy-2.example.test" {
		t.Fatalf("configuration after proxy update = %#v, %v", configuration, err)
	}
	if len(connections.wakes) != 2 {
		t.Fatalf("configuration wake count = %d, want 2", len(connections.wakes))
	}
	if _, err := service.UpdateNetworkProxy(ctx, proxy.ID, NetworkProxyUpdate{
		Name: updated.Name, Scheme: updated.Scheme, Host: updated.Host, Port: updated.Port,
		Username: updated.Username, PasswordAction: "clear",
	}); err != nil {
		t.Fatal(err)
	}
	unchanged, err := service.Configuration(ctx, registration.Credential)
	if err != nil || unchanged.Revision != 3 || len(connections.wakes) != 2 {
		t.Fatalf("no-op proxy update advanced configuration: %#v, wakes=%#v, err=%v", unchanged, connections.wakes, err)
	}
	if err := service.DeleteNetworkProxy(ctx, proxy.ID); !errors.Is(err, ErrNetworkProxyInUse) {
		t.Fatalf("delete referenced proxy error = %v", err)
	}
	if _, err := service.DeleteEgress(ctx, registration.NodeID, egress.ID); err != nil {
		t.Fatal(err)
	}
	if err := service.processDeletions(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteNetworkProxy(ctx, proxy.ID); err != nil {
		t.Fatal(err)
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
