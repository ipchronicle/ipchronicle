package nodes

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/center/database"
	"github.com/ipchronicle/ipchronicle/internal/center/database/configdb"
)

func TestNodeRecoveryCredentialLifecycle(t *testing.T) {
	ctx := context.Background()
	store, err := database.Open(ctx, database.PathsFromDataDirectory(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := NewService(store.Config, store.History, store.ConfigQueries, store.MasterKey, &testSyncConnections{})
	enrollment, err := service.RotateEnrollmentKey(ctx, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	registration, err := service.Register(ctx, enrollment.Key, testMetadata())
	if err != nil {
		t.Fatal(err)
	}

	credential, err := service.RecoveryCredential(ctx, registration.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	if credential.NodeID != registration.NodeID || !strings.HasPrefix(credential.Key, "ipc_recover_") {
		t.Fatalf("recovery credential = %#v", credential)
	}
	record, err := store.ConfigQueries.GetNodeRecoveryCredential(ctx, registration.NodeID.String())
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(credential.Key))
	if !bytes.Equal(record.KeyDigest, digest[:]) || bytes.Contains(record.KeyEncrypted, []byte(credential.Key)) {
		t.Fatal("recovery credential was not hashed and encrypted independently")
	}

	rotated, err := service.RotateRecoveryKey(ctx, registration.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.Key == credential.Key {
		t.Fatal("recovery credential did not rotate")
	}
	if _, err := service.Recover(ctx, credential.Key, testMetadata()); !errors.Is(err, ErrRecoveryKeyInvalid) {
		t.Fatalf("old recovery credential error = %v", err)
	}
	if recovered, err := service.Recover(ctx, rotated.Key, testMetadata()); err != nil || recovered.NodeID != registration.NodeID {
		t.Fatalf("rotated recovery credential result = %#v, %v", recovered, err)
	}
}

func TestEnsureRecoveryKeysBackfillsExistingNodes(t *testing.T) {
	ctx := context.Background()
	store, err := database.Open(ctx, database.PathsFromDataDirectory(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := NewService(store.Config, store.History, store.ConfigQueries, store.MasterKey, &testSyncConnections{})
	enrollment, err := service.RotateEnrollmentKey(ctx, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	registration, err := service.Register(ctx, enrollment.Key, testMetadata())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Config.ExecContext(ctx, `DELETE FROM node_recovery_credentials WHERE node_id = ?`, registration.NodeID.String()); err != nil {
		t.Fatal(err)
	}

	if err := service.EnsureRecoveryKeys(ctx); err != nil {
		t.Fatal(err)
	}
	first, err := service.RecoveryCredential(ctx, registration.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.EnsureRecoveryKeys(ctx); err != nil {
		t.Fatal(err)
	}
	second, err := service.RecoveryCredential(ctx, registration.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	if first.Key == "" || first.Key != second.Key {
		t.Fatalf("repeated backfill changed recovery credential: %#v, %#v", first, second)
	}
}

func TestRecoverPreservesDurableNodeStateAndClearsOldHostState(t *testing.T) {
	fixture := newProbeServiceFixture(t, 512*1024*1024)
	connections := &testSyncConnections{}
	fixture.service.sync = connections

	recovery, err := fixture.service.RecoveryCredential(fixture.ctx, fixture.registration.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	name := "retained-edge"
	logLevel := "debug"
	if _, err := fixture.service.Update(fixture.ctx, fixture.registration.NodeID, &name, nil, &logLevel); err != nil {
		t.Fatal(err)
	}
	probeState, err := fixture.service.Probe(fixture.ctx, fixture.registration.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.UpdateProbeSettings(fixture.ctx, fixture.registration.NodeID, ProbeSettingsUpdate{
		Schedule:          ProbeSchedule{Enabled: false, Cron: "0 15 3 * * *", Timezone: "Asia/Shanghai"},
		LowMemoryOverride: true, ProbeOnNewAddress: false,
	}); err != nil {
		t.Fatal(err)
	}
	password := "proxy-password"
	proxy, err := fixture.service.CreateNetworkProxy(fixture.ctx, fixture.registration.NodeID, NetworkProxyCreate{
		Name: "retained-proxy", Scheme: "socks5", Host: "proxy.example", Port: 1080, Password: &password,
	})
	if err != nil {
		t.Fatal(err)
	}
	task, err := fixture.service.CreateCompleteProbeTask(fixture.ctx, fixture.registration.NodeID, fixture.probeTargetIDs())
	if err != nil {
		t.Fatal(err)
	}
	metadataWithSync := fixture.metadata
	metadataWithSync.Capabilities = append(metadataWithSync.Capabilities, SyncWakeCapability)
	if _, err := fixture.service.Poll(
		fixture.ctx, fixture.registration.Credential, metadataWithSync,
		fixture.configuration.Revision, nil, nil, nil, nil,
		AddressUpload{ProbeStatus: &ProbeStatus{ActiveRunID: func() *uuid.UUID { id := uuid.New(); return &id }()}},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.StartSyncSession(fixture.ctx, fixture.registration.NodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.SetEnabled(fixture.ctx, fixture.registration.NodeID, false); err != nil {
		t.Fatal(err)
	}

	var addressLinksBefore, historyStatesBefore int
	if err := fixture.store.Config.QueryRowContext(
		fixture.ctx, `SELECT count(*) FROM public_address_nodes WHERE node_id = ?`, fixture.registration.NodeID.String(),
	).Scan(&addressLinksBefore); err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.History.QueryRowContext(
		fixture.ctx, `SELECT count(*) FROM address_states WHERE node_id = ?`, fixture.registration.NodeID.String(),
	).Scan(&historyStatesBefore); err != nil {
		t.Fatal(err)
	}
	if addressLinksBefore == 0 || historyStatesBefore == 0 {
		t.Fatalf("fixture lacks retained state: address links %d, history states %d", addressLinksBefore, historyStatesBefore)
	}

	replacementMetadata := Metadata{
		Hostname: "reinstalled.example", AgentVersion: "0.1.2", OperatingSystem: "linux", Architecture: "arm64",
		Capabilities: []string{AgentLogsCapability, "configuration-v10", "control-v1"}, PhysicalMemoryBytes: 1024 * 1024 * 1024,
	}
	recovered, err := fixture.service.Recover(fixture.ctx, recovery.Key, replacementMetadata)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.NodeID != fixture.registration.NodeID || recovered.Credential == fixture.registration.Credential {
		t.Fatalf("recovered registration = %#v", recovered)
	}
	if _, err := fixture.service.Configuration(fixture.ctx, fixture.registration.Credential); !errors.Is(err, ErrAgentRevoked) {
		t.Fatalf("old Agent credential error = %v", err)
	}
	configuration, err := fixture.service.Configuration(fixture.ctx, recovered.Credential)
	if err != nil {
		t.Fatal(err)
	}
	if configuration.Revision <= 1 || configuration.Enabled || configuration.ProbeSchedule.Enabled ||
		configuration.ProbeSchedule.Cron != "0 15 3 * * *" || configuration.ProbeSchedule.Timezone != "Asia/Shanghai" ||
		!configuration.ProbeLowMemoryOverride || configuration.LogLevel != logLevel {
		t.Fatalf("recovered configuration = %#v (prior state %#v)", configuration, probeState)
	}
	storedNode, err := fixture.store.ConfigQueries.GetNodeByID(fixture.ctx, fixture.registration.NodeID.String())
	if err != nil || storedNode.PhysicalMemoryBytes != nil || storedNode.LastSeenAt != nil {
		t.Fatalf("recovered host state = %#v, %v", storedNode, err)
	}
	poll, err := fixture.service.Poll(fixture.ctx, recovered.Credential, replacementMetadata, 0, nil, nil, nil, nil)
	if err != nil || poll.DesiredConfigurationRevision != configuration.Revision || poll.Enabled {
		t.Fatalf("first recovered poll = %#v, %v", poll, err)
	}

	node, err := fixture.service.Get(fixture.ctx, fixture.registration.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	if node.ID != fixture.registration.NodeID || node.Name != name || node.Hostname != replacementMetadata.Hostname || node.Enabled ||
		node.AppliedConfigurationRevision != 0 || node.ConfigurationError != nil ||
		!slices.Equal(node.Capabilities, replacementMetadata.Capabilities) {
		t.Fatalf("recovered node = %#v", node)
	}
	proxies, err := fixture.service.ListNetworkProxies(fixture.ctx, fixture.registration.NodeID)
	if err != nil || len(proxies) != 1 || proxies[0].ID != proxy.ID || !proxies[0].Enabled {
		t.Fatalf("recovered proxies = %#v, %v", proxies, err)
	}

	assertRecoveryCount(t, fixture.store.Config, `SELECT count(*) FROM node_network_inventories WHERE node_id = ?`, fixture.registration.NodeID.String(), 0)
	assertRecoveryCount(t, fixture.store.Config, `SELECT count(*) FROM public_address_paths WHERE node_id = ?`, fixture.registration.NodeID.String(), 0)
	assertRecoveryCount(t, fixture.store.Config, `SELECT count(*) FROM network_egresses WHERE node_id = ? AND kind != 'proxy'`, fixture.registration.NodeID.String(), 0)
	assertRecoveryCount(t, fixture.store.Config, `SELECT count(*) FROM network_egresses WHERE node_id = ? AND kind = 'proxy'`, fixture.registration.NodeID.String(), 2)
	assertRecoveryCount(t, fixture.store.Config, `SELECT count(*) FROM node_probe_status WHERE node_id = ?`, fixture.registration.NodeID.String(), 0)
	assertRecoveryCount(t, fixture.store.Config, `SELECT count(*) FROM public_address_nodes WHERE node_id = ?`, fixture.registration.NodeID.String(), addressLinksBefore)
	assertRecoveryCount(t, fixture.store.History, `SELECT count(*) FROM address_states WHERE node_id = ?`, fixture.registration.NodeID.String(), historyStatesBefore)

	storedTask, err := fixture.store.ConfigQueries.GetProbeTask(fixture.ctx, configdb.GetProbeTaskParams{
		ID: task.ID.String(), NodeID: fixture.registration.NodeID.String(),
	})
	if err != nil || storedTask.Status != "expired" || storedTask.CompletedAt == nil {
		t.Fatalf("expired task = %#v, %v", storedTask, err)
	}
	assertRecoveryCount(t, fixture.store.Config, `SELECT count(*) FROM node_sync_sessions WHERE node_id = ?`, fixture.registration.NodeID.String(), 0)
	if !slices.Contains(connections.disconnections, fixture.registration.NodeID.String()) {
		t.Fatalf("recovery did not disconnect old Agent: %#v", connections.disconnections)
	}
}

func TestRecoverRejectsRevokedAndDeletingNodes(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, *Service, Registration)
	}{
		{name: "revoked", mutate: func(t *testing.T, service *Service, registration Registration) {
			if _, err := service.Revoke(context.Background(), registration.NodeID); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "deletion pending", mutate: func(t *testing.T, service *Service, registration Registration) {
			if _, err := service.Delete(context.Background(), registration.NodeID); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			store, err := database.Open(ctx, database.PathsFromDataDirectory(t.TempDir()))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			service := NewService(store.Config, store.History, store.ConfigQueries, store.MasterKey, &testSyncConnections{})
			enrollment, err := service.RotateEnrollmentKey(ctx, "UTC")
			if err != nil {
				t.Fatal(err)
			}
			registration, err := service.Register(ctx, enrollment.Key, testMetadata())
			if err != nil {
				t.Fatal(err)
			}
			recovery, err := service.RecoveryCredential(ctx, registration.NodeID)
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(t, service, registration)
			if _, err := service.Recover(ctx, recovery.Key, testMetadata()); !errors.Is(err, ErrRecoveryKeyInvalid) {
				t.Fatalf("recovery error = %v", err)
			}
		})
	}
}

func assertRecoveryCount(t *testing.T, database *sql.DB, query, nodeID string, want int) {
	t.Helper()
	var got int
	if err := database.QueryRowContext(context.Background(), query, nodeID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("query %q count = %d, want %d", query, got, want)
	}
}
