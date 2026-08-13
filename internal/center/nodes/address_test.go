package nodes

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/center/database"
	"github.com/ipchronicle/ipchronicle/internal/center/database/historydb"
)

func TestAddressUploadIsIdempotentAndRejectsConflictingOrder(t *testing.T) {
	ctx := context.Background()
	store, err := database.Open(ctx, database.PathsFromDataDirectory(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := NewService(store.Config, store.History, store.ConfigQueries, store.MasterKey, &testSyncConnections{})
	now := time.Date(2026, 8, 9, 11, 0, 0, 0, time.UTC)
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
	if _, err := service.Poll(ctx, registration.Credential, testMetadata(), 0, nil, nil, &inventory, nil); err != nil {
		t.Fatal(err)
	}
	network, err := service.Network(ctx, registration.NodeID)
	if err != nil || len(network.Egresses) != 4 {
		t.Fatalf("network egresses = %#v, %v", network.Egresses, err)
	}
	var egress NetworkEgress
	for _, item := range network.Egresses {
		if item.Family == "ipv4" && item.Kind == "default" {
			egress = item
		}
	}
	systemState, err := store.ConfigQueries.GetSystemState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	localInterface := "eth0"
	localAddress := "10.0.0.5"
	publicAddress := "203.0.113.10"
	succeededAt := now.Add(time.Minute)
	eventID := uuid.New()
	upload := AddressUpload{
		States: []AddressState{{
			EgressID: egress.ID, HistoryGeneration: systemState.HistoryGeneration,
			Family: "ipv4", Status: "confirmed", Sequence: 1, PublicAddress: &publicAddress,
			LocalInterface: &localInterface, LocalAddress: &localAddress, LikelyNAT: true,
			LastCheckedAt: succeededAt, LastSucceededAt: &succeededAt, LastChangedAt: &succeededAt,
		}},
		Events: []AddressEvent{{
			ID: eventID, EgressID: egress.ID, HistoryGeneration: systemState.HistoryGeneration,
			Sequence: 1, Kind: "first-observation", Family: "ipv4", PublicAddress: &publicAddress,
			LocalInterface: &localInterface, LocalAddress: &localAddress, LikelyNAT: true, ObservedAt: succeededAt,
		}},
	}
	for attempt := 0; attempt < 2; attempt++ {
		receipt, err := service.ingestAddressUpload(ctx, registration.NodeID.String(), upload, now.Unix())
		if err != nil || len(receipt.AcceptedEventIDs) != 1 || receipt.AcceptedEventIDs[0] != eventID {
			t.Fatalf("address receipt attempt %d = %#v, %v", attempt, receipt, err)
		}
	}
	state, err := service.Network(ctx, registration.NodeID)
	if err != nil || len(state.AddressStates) != 1 || len(state.AddressEvents) != 1 || !state.AddressStates[0].LikelyNAT {
		t.Fatalf("stored address state = %#v, %v", state, err)
	}

	conflict := upload
	conflict.Events = append([]AddressEvent(nil), upload.Events...)
	conflict.Events[0].ID = uuid.New()
	if _, err := service.ingestAddressUpload(ctx, registration.NodeID.String(), conflict, now.Unix()); !errors.Is(err, ErrInvalidMetadata) {
		t.Fatalf("conflicting ordered event error = %v", err)
	}

	deletion, err := service.DeleteEgress(ctx, registration.NodeID, egress.ID)
	if err != nil || deletion.Status != "pending" {
		t.Fatalf("queued egress deletion = %#v, %v", deletion, err)
	}
	configuration, err := service.Configuration(ctx, registration.Credential)
	if err != nil {
		t.Fatal(err)
	}
	for _, configured := range configuration.Egresses {
		if configured.ID == egress.ID {
			t.Fatalf("deleting egress remained in Agent configuration: %#v", configured)
		}
	}
	discarded, err := service.ingestAddressUpload(ctx, registration.NodeID.String(), upload, now.Unix())
	if err != nil || len(discarded.DiscardedEventIDs) != 1 || discarded.DiscardedEventIDs[0] != eventID {
		t.Fatalf("pending-deletion upload receipt = %#v, %v", discarded, err)
	}
	service.deleteEgressHistory = func(context.Context, string) error {
		return errors.New("history disk unavailable")
	}
	if err := service.processDeletions(ctx, 1); err != nil {
		t.Fatal(err)
	}
	failed, err := service.Network(ctx, registration.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	var failedEgress *NetworkEgress
	for index := range failed.Egresses {
		if failed.Egresses[index].ID == egress.ID {
			failedEgress = &failed.Egresses[index]
		}
	}
	if failedEgress == nil || failedEgress.DeletionStatus == nil || *failedEgress.DeletionStatus != "failed" || failedEgress.DeletionError == nil {
		t.Fatalf("failed egress deletion = %#v", failedEgress)
	}
	deletion, err = service.DeleteEgress(ctx, registration.NodeID, egress.ID)
	if err != nil || deletion.Status != "pending" {
		t.Fatalf("retried egress deletion = %#v, %v", deletion, err)
	}
	service.deleteEgressHistory = service.deleteNetworkEgressHistory
	if err := service.processDeletions(ctx, 1); err != nil {
		t.Fatal(err)
	}
	completed, err := service.Network(ctx, registration.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	for _, configured := range completed.Egresses {
		if configured.ID == egress.ID {
			t.Fatalf("completed deletion retained egress: %#v", configured)
		}
	}
	if len(completed.AddressStates) != 0 || len(completed.AddressEvents) != 1 ||
		completed.AddressEvents[0].EgressID != completed.PublicAddresses[0].ID {
		t.Fatalf("completed deletion did not preserve public-address history: %#v", completed)
	}
}

func TestObsoleteAndDeletedEgressAddressUploadsAreDiscarded(t *testing.T) {
	ctx := context.Background()
	store, err := database.Open(ctx, database.PathsFromDataDirectory(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := NewService(store.Config, store.History, store.ConfigQueries, store.MasterKey, &testSyncConnections{})
	nodeID := uuid.New()
	egressID := uuid.New()
	eventID := uuid.New()
	gapID := uuid.New()
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	upload := AddressUpload{
		Events: []AddressEvent{{
			ID: eventID, EgressID: egressID, HistoryGeneration: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Sequence: 1, Kind: "check-failure", Family: "ipv4", FailureReason: stringPointer("selector-unavailable"), ObservedAt: now,
		}},
		Gaps: []AddressGap{{
			ID: gapID, EgressID: egressID, HistoryGeneration: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			DroppedCount: 1, FirstSequence: 1, LastSequence: 1, FirstObservedAt: now, LastObservedAt: now,
		}},
	}
	receipt, err := service.ingestAddressUpload(ctx, nodeID.String(), upload, now.Unix())
	if err != nil || len(receipt.DiscardedEventIDs) != 1 || len(receipt.DiscardedGaps) != 1 {
		t.Fatalf("discard receipt = %#v, %v", receipt, err)
	}
}

func TestAddressUploadRejectsInvalidGenerationAndNonPublicAddress(t *testing.T) {
	egressID := uuid.New()
	egresses := map[uuid.UUID]NetworkEgress{
		egressID: {ID: egressID, Family: "ipv4", Kind: "default"},
	}
	now := time.Date(2026, 8, 9, 13, 0, 0, 0, time.UTC)
	publicAddress := "8.8.8.8"
	base := AddressState{
		EgressID:          egressID,
		HistoryGeneration: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Family:            "ipv4", Status: "confirmed", Sequence: 1, PublicAddress: &publicAddress,
		LastCheckedAt: now, LastSucceededAt: &now,
	}
	tests := []struct {
		name   string
		mutate func(*AddressState)
	}{
		{name: "uppercase generation", mutate: func(item *AddressState) {
			item.HistoryGeneration = "0123456789ABCDEF0123456789abcdef0123456789abcdef0123456789abcdef"
		}},
		{name: "private public address", mutate: func(item *AddressState) {
			value := "10.0.0.1"
			item.PublicAddress = &value
		}},
		{name: "shared public address", mutate: func(item *AddressState) {
			value := "100.64.0.1"
			item.PublicAddress = &value
		}},
		{name: "wrong address family", mutate: func(item *AddressState) {
			value := "2001:4860:4860::8888"
			item.PublicAddress = &value
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			item := base
			test.mutate(&item)
			if err := validateAddressUpload(AddressUpload{States: []AddressState{item}}, egresses); !errors.Is(err, ErrInvalidMetadata) {
				t.Fatalf("validation error = %v", err)
			}
		})
	}
}

func TestPublicAddressesAreGlobalAndPathsRemainExecutionDetails(t *testing.T) {
	ctx := context.Background()
	store, err := database.Open(ctx, database.PathsFromDataDirectory(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	connections := &testSyncConnections{}
	service := NewService(store.Config, store.History, store.ConfigQueries, store.MasterKey, connections)
	now := time.Date(2026, 8, 9, 14, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	enrollment, err := service.RotateEnrollmentKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	firstMetadata := testMetadata()
	firstMetadata.Hostname = "first.example"
	first, err := service.Register(ctx, enrollment.Key, firstMetadata)
	if err != nil {
		t.Fatal(err)
	}
	secondMetadata := testMetadata()
	secondMetadata.Hostname = "second.example"
	second, err := service.Register(ctx, enrollment.Key, secondMetadata)
	if err != nil {
		t.Fatal(err)
	}
	inventory := testNetworkInventory(now)
	if _, err := service.Poll(ctx, first.Credential, firstMetadata, 0, nil, nil, &inventory, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Poll(ctx, second.Credential, secondMetadata, 0, nil, nil, &inventory, nil); err != nil {
		t.Fatal(err)
	}
	firstNetwork, err := service.Network(ctx, first.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	secondNetwork, err := service.Network(ctx, second.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	firstPaths := publicAddressTestPaths(t, firstNetwork.Egresses)
	secondPaths := publicAddressTestPaths(t, secondNetwork.Egresses)
	systemState, err := store.ConfigQueries.GetSystemState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	publicAddress := "8.8.8.8"
	firstStates := []AddressState{
		confirmedAddressState(firstPaths[0].ID, systemState.HistoryGeneration, "ipv4", publicAddress, "eth0", "10.0.0.5", now),
		confirmedAddressState(firstPaths[1].ID, systemState.HistoryGeneration, "ipv4", publicAddress, "eth0", "10.0.0.5", now),
	}
	if _, err := service.Poll(ctx, first.Credential, firstMetadata, 0, nil, nil, nil, nil, AddressUpload{States: firstStates}); err != nil {
		t.Fatal(err)
	}
	firstNetwork, err = service.Network(ctx, first.NodeID)
	if err != nil || len(firstNetwork.PublicAddresses) != 1 || firstNetwork.PublicAddresses[0].PathCount != 2 {
		t.Fatalf("first node public addresses = %#v, %v", firstNetwork.PublicAddresses, err)
	}
	publicAddressID := firstNetwork.PublicAddresses[0].ID
	now = now.Add(time.Minute)
	secondState := confirmedAddressState(
		secondPaths[0].ID, systemState.HistoryGeneration, "ipv4", publicAddress, "eth0", "10.0.0.5", now,
	)
	if _, err := service.Poll(ctx, second.Credential, secondMetadata, 0, nil, nil, nil, nil, AddressUpload{States: []AddressState{secondState}}); err != nil {
		t.Fatal(err)
	}
	addresses, err := store.ConfigQueries.ListPublicAddresses(ctx)
	if err != nil || len(addresses) != 1 || addresses[0].ID != publicAddressID.String() {
		t.Fatalf("global public addresses = %#v, %v", addresses, err)
	}
	secondNetwork, err = service.Network(ctx, second.NodeID)
	if err != nil || len(secondNetwork.PublicAddresses) != 1 || secondNetwork.PublicAddresses[0].ID != publicAddressID || secondNetwork.PublicAddresses[0].PathCount != 3 {
		t.Fatalf("second node public addresses = %#v, %v", secondNetwork.PublicAddresses, err)
	}
	if _, err := service.UpdatePublicAddress(ctx, first.NodeID, publicAddressID, PublicAddressUpdate{
		ProbeEnabled: true, ProbeOnRediscovery: true,
	}); err != nil {
		t.Fatal(err)
	}
	listed, err := service.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range listed {
		if len(node.PublicAddresses) != 1 || node.PublicAddresses[0].ID != publicAddressID ||
			node.PublicAddresses[0].Address != publicAddress || !node.PublicAddresses[0].Available ||
			!node.PublicAddresses[0].ProbeEnabled {
			t.Fatalf("node public-address summary = %#v", node.PublicAddresses)
		}
	}
	configuration, err := service.Configuration(ctx, first.Credential)
	if err != nil || len(configuration.ProbeTargets) != 1 || configuration.ProbeTargets[0].ID != publicAddressID ||
		configuration.ProbeTargets[0].PathID == nil || configuration.ProbeTargets[0].PublicAddress == nil ||
		*configuration.ProbeTargets[0].PublicAddress != publicAddress {
		t.Fatalf("public-address probe target = %#v, %v", configuration.ProbeTargets, err)
	}

	connections.wakes = nil
	firstBefore, err := service.Get(ctx, first.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	secondBefore, err := service.Get(ctx, second.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Poll(
		ctx, first.Credential, firstMetadata, firstBefore.AppliedConfigurationRevision,
		nil, nil, nil, nil, AddressUpload{States: []AddressState{}},
	); err != nil {
		t.Fatal(err)
	}
	firstAfter, err := service.Get(ctx, first.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	secondAfter, err := service.Get(ctx, second.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	if firstAfter.DesiredConfigurationRevision != firstBefore.DesiredConfigurationRevision+1 ||
		secondAfter.DesiredConfigurationRevision != secondBefore.DesiredConfigurationRevision+1 ||
		!slices.Contains(connections.wakes, first.NodeID.String()) || !slices.Contains(connections.wakes, second.NodeID.String()) {
		t.Fatalf("failover revisions first=%d/%d second=%d/%d wakes=%#v",
			firstBefore.DesiredConfigurationRevision, firstAfter.DesiredConfigurationRevision,
			secondBefore.DesiredConfigurationRevision, secondAfter.DesiredConfigurationRevision, connections.wakes)
	}
}

func TestDynamicPublicAddressesKeepIndependentIdentityAndReuseOldAddress(t *testing.T) {
	ctx := context.Background()
	store, err := database.Open(ctx, database.PathsFromDataDirectory(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := NewService(store.Config, store.History, store.ConfigQueries, store.MasterKey, &testSyncConnections{})
	now := time.Date(2026, 8, 9, 15, 0, 0, 0, time.UTC)
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
	path := publicAddressTestPaths(t, network.Egresses)[0]
	systemState, err := store.ConfigQueries.GetSystemState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	observe := func(address string) PublicAddress {
		t.Helper()
		state := confirmedAddressState(path.ID, systemState.HistoryGeneration, "ipv4", address, "eth0", "10.0.0.5", now)
		if _, err := service.Poll(ctx, registration.Credential, testMetadata(), 0, nil, nil, nil, nil, AddressUpload{States: []AddressState{state}}); err != nil {
			t.Fatal(err)
		}
		current, err := service.Network(ctx, registration.NodeID)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range current.PublicAddresses {
			if item.Address == address {
				return item
			}
		}
		t.Fatalf("public address %s not found: %#v", address, current.PublicAddresses)
		return PublicAddress{}
	}
	first := observe("8.8.8.8")
	if _, err := service.UpdatePublicAddress(ctx, registration.NodeID, first.ID, PublicAddressUpdate{
		ProbeEnabled: true, ProbeOnRediscovery: false,
	}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	second := observe("1.1.1.1")
	if second.ID == first.ID || second.ProbeEnabled {
		t.Fatalf("dynamic replacement reused identity or settings: first=%#v second=%#v", first, second)
	}
	now = now.Add(time.Minute)
	reappeared := observe("8.8.8.8")
	if reappeared.ID != first.ID || !reappeared.ProbeEnabled || reappeared.ProbeOnRediscovery {
		t.Fatalf("reappeared address did not reuse identity and settings: %#v", reappeared)
	}
	addresses, err := store.ConfigQueries.ListPublicAddresses(ctx)
	if err != nil || len(addresses) != 2 {
		t.Fatalf("dynamic public address registry = %#v, %v", addresses, err)
	}
}

func TestIPv4MappedAddressNormalizesAndNotificationsUsePublicAddressIdentity(t *testing.T) {
	ctx := context.Background()
	store, err := database.Open(ctx, database.PathsFromDataDirectory(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := NewService(store.Config, store.History, store.ConfigQueries, store.MasterKey, &testSyncConnections{})
	now := time.Date(2026, 8, 9, 16, 0, 0, 0, time.UTC)
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
	path := publicAddressTestPaths(t, network.Egresses)[0]
	systemState, err := store.ConfigQueries.GetSystemState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	localInterface := "eth0"
	localAddress := "10.0.0.5"
	mapped := "::ffff:8.8.8.8"
	checkedAt := now.Add(time.Minute)
	firstEventID := uuid.New()
	initial := AddressUpload{
		States: []AddressState{{
			EgressID: path.ID, HistoryGeneration: systemState.HistoryGeneration,
			Family: "ipv4", Status: "confirmed", Sequence: 1, PublicAddress: &mapped,
			LocalInterface: &localInterface, LocalAddress: &localAddress, LikelyNAT: true,
			LastCheckedAt: checkedAt, LastSucceededAt: &checkedAt, LastChangedAt: &checkedAt,
		}},
		Events: []AddressEvent{{
			ID: firstEventID, EgressID: path.ID, HistoryGeneration: systemState.HistoryGeneration,
			Sequence: 1, Kind: "first-observation", Family: "ipv4", PublicAddress: &mapped,
			LocalInterface: &localInterface, LocalAddress: &localAddress, LikelyNAT: true, ObservedAt: checkedAt,
		}},
	}
	if _, err := service.ingestAddressUpload(ctx, registration.NodeID.String(), initial, now.Unix()); err != nil {
		t.Fatal(err)
	}
	canonical, err := store.ConfigQueries.GetPublicAddressByAddress(ctx, "8.8.8.8")
	if err != nil {
		t.Fatal(err)
	}
	addresses, err := store.ConfigQueries.ListPublicAddresses(ctx)
	if err != nil || len(addresses) != 1 {
		t.Fatalf("mapped address registry = %#v, %v", addresses, err)
	}

	nextAddress := "1.1.1.1"
	changeEventID := uuid.New()
	change := AddressUpload{
		States: []AddressState{confirmedAddressState(
			path.ID, systemState.HistoryGeneration, "ipv4", nextAddress, localInterface, localAddress, checkedAt.Add(time.Minute),
		)},
		Events: []AddressEvent{{
			ID: changeEventID, EgressID: path.ID, HistoryGeneration: systemState.HistoryGeneration,
			Sequence: 2, Kind: "address-change", Family: "ipv4", PreviousAddress: stringPointer("8.8.8.8"), PublicAddress: &nextAddress,
			LocalInterface: &localInterface, LocalAddress: &localAddress, LikelyNAT: true, ObservedAt: checkedAt.Add(time.Minute),
		}},
	}
	if _, err := service.ingestAddressUpload(ctx, registration.NodeID.String(), change, now.Unix()); err != nil {
		t.Fatal(err)
	}
	next, err := store.ConfigQueries.GetPublicAddressByAddress(ctx, nextAddress)
	if err != nil {
		t.Fatal(err)
	}
	changeNotification, err := notificationForSource(ctx, store, changeEventID)
	if err != nil || changeNotification.EgressID == nil || *changeNotification.EgressID != next.ID {
		t.Fatalf("address-change notification = %#v, %v", changeNotification, err)
	}

	failureEventID := uuid.New()
	failureReason := "no-valid-response"
	failure := AddressUpload{Events: []AddressEvent{{
		ID: failureEventID, EgressID: path.ID, HistoryGeneration: systemState.HistoryGeneration,
		Sequence: 3, Kind: "check-failure", Family: "ipv4", FailureReason: &failureReason,
		ObservedAt: checkedAt.Add(2 * time.Minute),
	}}}
	if _, err := service.ingestAddressUpload(ctx, registration.NodeID.String(), failure, now.Unix()); err != nil {
		t.Fatal(err)
	}
	failureNotification, err := notificationForSource(ctx, store, failureEventID)
	if err != nil || failureNotification.EgressID == nil || *failureNotification.EgressID != next.ID || *failureNotification.EgressID == canonical.ID {
		t.Fatalf("check-failure notification = %#v, %v", failureNotification, err)
	}

	gapID := uuid.New()
	gap := AddressUpload{Gaps: []AddressGap{{
		ID: gapID, EgressID: path.ID, HistoryGeneration: systemState.HistoryGeneration,
		DroppedCount: 1, FirstSequence: 4, LastSequence: 4,
		FirstObservedAt: checkedAt.Add(3 * time.Minute), LastObservedAt: checkedAt.Add(3 * time.Minute),
	}}}
	if _, err := service.ingestAddressUpload(ctx, registration.NodeID.String(), gap, now.Unix()); err != nil {
		t.Fatal(err)
	}
	gapNotification, err := notificationForSource(ctx, store, gapID)
	if err != nil || gapNotification.NodeID == nil || *gapNotification.NodeID != registration.NodeID.String() || gapNotification.EgressID != nil {
		t.Fatalf("address-gap notification = %#v, %v", gapNotification, err)
	}
}

func publicAddressTestPaths(t *testing.T, egresses []NetworkEgress) []NetworkEgress {
	t.Helper()
	paths := make([]NetworkEgress, 0, 2)
	for _, path := range egresses {
		if path.Family == "ipv4" && (path.Kind == "default" || path.Kind == "source") {
			paths = append(paths, path)
		}
	}
	if len(paths) < 2 {
		t.Fatalf("IPv4 discovery paths = %#v", egresses)
	}
	return paths
}

func notificationForSource(ctx context.Context, store *database.Store, sourceID uuid.UUID) (historydb.NotificationEvent, error) {
	events, err := store.HistoryQueries.ListPendingNotificationEvents(ctx, 100)
	if err != nil {
		return historydb.NotificationEvent{}, err
	}
	for _, event := range events {
		if event.SourceID == sourceID.String() {
			return event, nil
		}
	}
	return historydb.NotificationEvent{}, sql.ErrNoRows
}
