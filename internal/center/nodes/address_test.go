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
	enrollment, err := service.RotateEnrollmentKey(ctx, "UTC")
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
	configuration, err := service.Configuration(ctx, registration.Credential)
	if err != nil || len(configuration.DiscoveryPaths) != 4 {
		t.Fatalf("discovery paths = %#v, %v", configuration.DiscoveryPaths, err)
	}
	var egress NetworkEgress
	for _, item := range configuration.DiscoveryPaths {
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
	if err != nil || len(state.AddressEvents) != 1 {
		t.Fatalf("stored address state = %#v, %v", state, err)
	}
	addressStates, err := store.HistoryQueries.ListNodeAddressStates(ctx, registration.NodeID.String())
	if err != nil || len(addressStates) != 1 || addressStates[0].LikelyNat != 1 {
		t.Fatalf("stored address state records = %#v, %v", addressStates, err)
	}
	if len(state.PublicAddresses) != 1 || !state.PublicAddresses[0].ProbeEnabled {
		t.Fatalf("new public address defaults = %#v", state.PublicAddresses)
	}
	publicAddressID := state.PublicAddresses[0].ID.String()
	runID := uuid.NewString()
	executionID := uuid.NewString()
	snapshotID := uuid.NewString()
	startedAt := succeededAt.Unix()
	completedAt := succeededAt.Add(time.Minute).Unix()
	if _, err := store.HistoryQueries.CreateProbeRun(ctx, historydb.CreateProbeRunParams{
		ID: runID, NodeID: registration.NodeID.String(), HistoryGeneration: systemState.HistoryGeneration,
		ConfigurationRevision: 2, Trigger: "manual", Status: "succeeded", ExpectedExecutions: 1,
		StartedAt: succeededAt.Unix(), CompletedAt: &completedAt, ReceivedAt: completedAt,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.HistoryQueries.CreateProbeExecution(ctx, historydb.CreateProbeExecutionParams{
		ID: executionID, RunID: runID, EgressID: publicAddressID, Ordinal: 0, Sequence: 1,
		Status: "succeeded", StartedAt: &startedAt, CompletedAt: &completedAt, ReceivedAt: completedAt,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.HistoryQueries.CreateProbeSnapshot(ctx, historydb.CreateProbeSnapshotParams{
		ID: snapshotID, ExecutionID: executionID, EgressID: publicAddressID, Sequence: 1,
		ObservedAt: completedAt, RawResult: []byte(`{}`), EncodedSize: 2, ReceivedAt: completedAt,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.HistoryQueries.UpsertCurrentProbeSnapshot(ctx, historydb.UpsertCurrentProbeSnapshotParams{
		EgressID: publicAddressID, ExecutionID: executionID, SnapshotID: snapshotID,
		Sequence: 1, ObservedAt: completedAt, ReceivedAt: completedAt,
	}); err != nil {
		t.Fatal(err)
	}
	state, err = service.Network(ctx, registration.NodeID)
	if err != nil || state.PublicAddresses[0].LatestSnapshotID == nil ||
		state.PublicAddresses[0].LatestSnapshotID.String() != snapshotID ||
		state.PublicAddresses[0].LatestSnapshotAt == nil || state.PublicAddresses[0].LatestSnapshotAt.Unix() != completedAt {
		t.Fatalf("public address latest snapshot = %#v, %v", state.PublicAddresses[0], err)
	}
	var pendingProbeCount int
	if err := store.Config.QueryRowContext(ctx, `SELECT count(*) FROM pending_public_address_probes`).Scan(&pendingProbeCount); err != nil {
		t.Fatal(err)
	}
	if pendingProbeCount != 0 {
		t.Fatalf("first observation queued %d complete probes", pendingProbeCount)
	}

	conflict := upload
	conflict.Events = append([]AddressEvent(nil), upload.Events...)
	conflict.Events[0].ID = uuid.New()
	if _, err := service.ingestAddressUpload(ctx, registration.NodeID.String(), conflict, now.Unix()); !errors.Is(err, ErrInvalidMetadata) {
		t.Fatalf("conflicting ordered event error = %v", err)
	}

	if state.AddressEvents[0].EgressID != state.PublicAddresses[0].ID {
		t.Fatalf("public-address history was not rebound to the public address: %#v", state)
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
	enrollment, err := service.RotateEnrollmentKey(ctx, "UTC")
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
	firstConfiguration, err := service.Configuration(ctx, first.Credential)
	if err != nil {
		t.Fatal(err)
	}
	secondConfiguration, err := service.Configuration(ctx, second.Credential)
	if err != nil {
		t.Fatal(err)
	}
	firstPaths := publicAddressTestPaths(t, firstConfiguration.DiscoveryPaths)
	secondPaths := publicAddressTestPaths(t, secondConfiguration.DiscoveryPaths)
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
	firstNetwork, err := service.Network(ctx, first.NodeID)
	if err != nil || len(firstNetwork.PublicAddresses) != 1 || firstNetwork.PublicAddresses[0].PathCount != 2 {
		t.Fatalf("first node public addresses = %#v, %v", firstNetwork.PublicAddresses, err)
	}
	if !firstNetwork.PublicAddresses[0].ProbeEnabled {
		t.Fatalf("first public-address defaults = %#v", firstNetwork.PublicAddresses[0])
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
	secondNetwork, err := service.Network(ctx, second.NodeID)
	if err != nil || len(secondNetwork.PublicAddresses) != 1 || secondNetwork.PublicAddresses[0].ID != publicAddressID || secondNetwork.PublicAddresses[0].PathCount != 3 {
		t.Fatalf("second node public addresses = %#v, %v", secondNetwork.PublicAddresses, err)
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
	path := publicAddressTestPaths(t, configuration.DiscoveryPaths)[0]
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
		ProbeEnabled: false,
	}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	second := observe("1.1.1.1")
	if second.ID == first.ID || !second.ProbeEnabled {
		t.Fatalf("dynamic replacement reused identity or settings: first=%#v second=%#v", first, second)
	}
	current, err := service.Network(ctx, registration.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	if len(current.PublicAddresses) != 2 || current.PublicAddresses[0].ID != second.ID || !current.PublicAddresses[0].Available ||
		current.PublicAddresses[1].ID != first.ID || current.PublicAddresses[1].Available {
		t.Fatalf("current and historical public addresses = %#v", current.PublicAddresses)
	}
	now = now.Add(time.Minute)
	reappeared := observe("8.8.8.8")
	if reappeared.ID != first.ID || reappeared.ProbeEnabled {
		t.Fatalf("reappeared address did not reuse identity and settings: %#v", reappeared)
	}
	addresses, err := store.ConfigQueries.ListPublicAddresses(ctx)
	if err != nil || len(addresses) != 2 {
		t.Fatalf("dynamic public address registry = %#v, %v", addresses, err)
	}
}

func TestFailedAddressCheckKeepsConfirmedBaselineCurrent(t *testing.T) {
	fixture := newProbeServiceFixture(t, 512*1024*1024)
	var ipv4Path, ipv6Path NetworkEgress
	for _, path := range fixture.configuration.DiscoveryPaths {
		if path.Kind != "default" {
			continue
		}
		if path.Family == "ipv4" {
			ipv4Path = path
		} else if path.Family == "ipv6" {
			ipv6Path = path
		}
	}
	checkedAt := fixture.now.Add(time.Minute)
	lastSucceededAt := fixture.now
	failureReason := "no-valid-response"
	publicIPv4 := "8.8.8.8"
	localInterface := "eth0"
	localIPv4 := "10.0.0.5"
	failed := AddressState{
		EgressID: ipv4Path.ID, HistoryGeneration: fixture.configuration.HistoryGeneration,
		Family: "ipv4", Status: "failed", PublicAddress: &publicIPv4,
		LocalInterface: &localInterface, LocalAddress: &localIPv4, LikelyNAT: true,
		FailureReason: &failureReason, LastCheckedAt: checkedAt, LastSucceededAt: &lastSucceededAt,
	}
	confirmedIPv6 := confirmedAddressState(
		ipv6Path.ID, fixture.configuration.HistoryGeneration, "ipv6",
		"2001:4860:4860::8888", "eth0", "fd00::5", checkedAt,
	)
	poll, err := fixture.service.Poll(
		fixture.ctx, fixture.registration.Credential, fixture.metadata,
		fixture.configuration.Revision, nil, nil, nil, nil,
		AddressUpload{States: []AddressState{failed, confirmedIPv6}},
	)
	if err != nil || poll.Task != nil {
		t.Fatalf("failed-check poll = %#v, %v", poll, err)
	}
	network, err := fixture.service.Network(fixture.ctx, fixture.registration.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	if !containsAvailablePublicAddress(network.PublicAddresses, publicIPv4) {
		t.Fatalf("confirmed baseline disappeared after a failed check: %#v", network.PublicAddresses)
	}

	checkedAt = checkedAt.Add(time.Minute)
	recoveredIPv4 := confirmedAddressState(
		ipv4Path.ID, fixture.configuration.HistoryGeneration, "ipv4",
		publicIPv4, localInterface, localIPv4, checkedAt,
	)
	confirmedIPv6 = confirmedAddressState(
		ipv6Path.ID, fixture.configuration.HistoryGeneration, "ipv6",
		"2001:4860:4860::8888", "eth0", "fd00::5", checkedAt,
	)
	poll, err = fixture.service.Poll(
		fixture.ctx, fixture.registration.Credential, fixture.metadata,
		fixture.configuration.Revision, nil, nil, nil, nil,
		AddressUpload{States: []AddressState{recoveredIPv4, confirmedIPv6}},
	)
	if err != nil || poll.Task != nil {
		t.Fatalf("same-address recovery poll = %#v, %v", poll, err)
	}
	var pending int
	if err := fixture.store.Config.QueryRowContext(
		fixture.ctx, `SELECT count(*) FROM pending_public_address_probes`,
	).Scan(&pending); err != nil || pending != 0 {
		t.Fatalf("pending probes after failure and recovery = %d, %v", pending, err)
	}
}

func TestNewAddressProbeTaskContainsOnlyNewlyCurrentAddresses(t *testing.T) {
	fixture := newProbeServiceFixture(t, 512*1024*1024)
	var defaultIPv4, defaultIPv6 NetworkEgress
	for _, path := range fixture.configuration.DiscoveryPaths {
		switch {
		case path.Kind == "default" && path.Family == "ipv4":
			defaultIPv4 = path
		case path.Kind == "default" && path.Family == "ipv6":
			defaultIPv6 = path
		}
	}
	if defaultIPv4.ID == uuid.Nil || defaultIPv6.ID == uuid.Nil {
		t.Fatalf("required discovery paths = %#v", fixture.configuration.DiscoveryPaths)
	}
	states := func(at time.Time, ipv4, ipv6 string) []AddressState {
		return []AddressState{
			confirmedAddressState(defaultIPv4.ID, fixture.configuration.HistoryGeneration, "ipv4", ipv4, "eth0", "10.0.0.5", at),
			confirmedAddressState(defaultIPv6.ID, fixture.configuration.HistoryGeneration, "ipv6", ipv6, "eth0", "fd00::5", at),
		}
	}

	fixture.now = fixture.now.Add(time.Minute)
	baselinePoll, err := fixture.service.Poll(
		fixture.ctx, fixture.registration.Credential, fixture.metadata,
		fixture.configuration.Revision, nil, nil, nil, nil,
		AddressUpload{States: states(fixture.now, "8.8.8.8", "2001:4860:4860::8888")},
	)
	if err != nil || baselinePoll.Task != nil {
		t.Fatalf("new-path baseline poll = %#v, %v", baselinePoll, err)
	}
	baselineConfiguration, err := fixture.service.Configuration(fixture.ctx, fixture.registration.Credential)
	if err != nil {
		t.Fatal(err)
	}

	fixture.now = fixture.now.Add(time.Minute)
	changePoll, err := fixture.service.Poll(
		fixture.ctx, fixture.registration.Credential, fixture.metadata,
		baselineConfiguration.Revision, nil, nil, nil, nil,
		AddressUpload{States: states(fixture.now, "1.1.1.1", "2606:4700:4700::1111")},
	)
	if err != nil || changePoll.Task != nil {
		t.Fatalf("new-address configuration poll = %#v, %v", changePoll, err)
	}
	changedConfiguration, err := fixture.service.Configuration(fixture.ctx, fixture.registration.Credential)
	if err != nil {
		t.Fatal(err)
	}
	network, err := fixture.service.Network(fixture.ctx, fixture.registration.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	want := make(map[uuid.UUID]struct{}, 2)
	for _, address := range network.PublicAddresses {
		if address.Address == "1.1.1.1" || address.Address == "2606:4700:4700::1111" {
			want[address.ID] = struct{}{}
		}
	}
	if len(want) != 2 {
		t.Fatalf("current public addresses = %#v", network.PublicAddresses)
	}

	fixture.now = fixture.now.Add(time.Second)
	delivery, err := fixture.service.Poll(
		fixture.ctx, fixture.registration.Credential, fixture.metadata,
		changedConfiguration.Revision, nil, nil, nil, nil,
	)
	if err != nil || delivery.Task == nil || delivery.Task.Trigger != "new-address" || len(delivery.Task.PublicAddressIDs) != 2 {
		t.Fatalf("new-address task delivery = %#v, %v", delivery.Task, err)
	}
	for _, id := range delivery.Task.PublicAddressIDs {
		if _, exists := want[id]; !exists {
			t.Fatalf("unrelated public address %s included in task %#v", id, delivery.Task.PublicAddressIDs)
		}
		delete(want, id)
	}
	if len(want) != 0 {
		t.Fatalf("new public addresses missing from task: %#v", want)
	}
}

func TestFirstProxyObservationProbesNewAddressAfterNodeBaseline(t *testing.T) {
	fixture := newProbeServiceFixture(t, 512*1024*1024)
	proxy, err := fixture.service.CreateNetworkProxy(fixture.ctx, fixture.registration.NodeID, NetworkProxyCreate{
		Name: "New exit", Scheme: "socks5", Host: "proxy.example.test", Port: 1080,
	})
	if err != nil {
		t.Fatal(err)
	}
	configuration, err := fixture.service.Configuration(fixture.ctx, fixture.registration.Credential)
	if err != nil {
		t.Fatal(err)
	}
	var directIPv4, directIPv6, proxyIPv4 NetworkEgress
	for _, path := range configuration.DiscoveryPaths {
		switch {
		case path.Kind == "default" && path.Family == "ipv4":
			directIPv4 = path
		case path.Kind == "default" && path.Family == "ipv6":
			directIPv6 = path
		case path.ProxyID != nil && *path.ProxyID == proxy.ID && path.Family == "ipv4":
			proxyIPv4 = path
		}
	}
	if directIPv4.ID == uuid.Nil || directIPv6.ID == uuid.Nil || proxyIPv4.ID == uuid.Nil {
		t.Fatalf("required discovery paths = %#v", configuration.DiscoveryPaths)
	}

	fixture.now = fixture.now.Add(time.Minute)
	proxyAddress := "1.1.1.1"
	proxyCheckedAt := fixture.now
	proxyState := AddressState{
		EgressID: proxyIPv4.ID, HistoryGeneration: configuration.HistoryGeneration,
		Family: "ipv4", Status: "confirmed", PublicAddress: &proxyAddress, ProxyPath: true,
		LastCheckedAt: proxyCheckedAt, LastSucceededAt: &proxyCheckedAt, LastChangedAt: &proxyCheckedAt,
	}
	poll, err := fixture.service.Poll(
		fixture.ctx, fixture.registration.Credential, fixture.metadata,
		configuration.Revision, nil, nil, nil, nil,
		AddressUpload{States: []AddressState{
			confirmedAddressState(directIPv4.ID, configuration.HistoryGeneration, "ipv4", "8.8.8.8", "eth0", "10.0.0.5", fixture.now),
			confirmedAddressState(directIPv6.ID, configuration.HistoryGeneration, "ipv6", "2001:4860:4860::8888", "eth0", "fd00::5", fixture.now),
			proxyState,
		}},
	)
	if err != nil || poll.Task != nil {
		t.Fatalf("first proxy observation poll = %#v, %v", poll, err)
	}
	changedConfiguration, err := fixture.service.Configuration(fixture.ctx, fixture.registration.Credential)
	if err != nil {
		t.Fatal(err)
	}
	if changedConfiguration.Revision <= configuration.Revision {
		t.Fatalf("new proxy address did not advance configuration: before=%d after=%d", configuration.Revision, changedConfiguration.Revision)
	}

	fixture.now = fixture.now.Add(time.Second)
	delivery, err := fixture.service.Poll(
		fixture.ctx, fixture.registration.Credential, fixture.metadata,
		changedConfiguration.Revision, nil, nil, nil, nil,
	)
	if err != nil || delivery.Task == nil || delivery.Task.Trigger != "new-address" || len(delivery.Task.PublicAddressIDs) != 1 {
		t.Fatalf("new proxy address task = %#v, %v", delivery.Task, err)
	}
	network, err := fixture.service.Network(fixture.ctx, fixture.registration.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	for _, address := range network.PublicAddresses {
		if address.Address == proxyAddress {
			if delivery.Task.PublicAddressIDs[0] != address.ID {
				t.Fatalf("task target = %s, want %s", delivery.Task.PublicAddressIDs[0], address.ID)
			}
			return
		}
	}
	t.Fatalf("proxy address missing from network state: %#v", network.PublicAddresses)
}

func containsAvailablePublicAddress(addresses []PublicAddress, value string) bool {
	for _, address := range addresses {
		if address.Address == value && address.Available {
			return true
		}
	}
	return false
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
	path := publicAddressTestPaths(t, configuration.DiscoveryPaths)[0]
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
	removedEventID := uuid.New()
	changeEventID := uuid.New()
	change := AddressUpload{
		States: []AddressState{confirmedAddressState(
			path.ID, systemState.HistoryGeneration, "ipv4", nextAddress, localInterface, localAddress, checkedAt.Add(time.Minute),
		)},
		Events: []AddressEvent{
			{
				ID: removedEventID, EgressID: path.ID, HistoryGeneration: systemState.HistoryGeneration,
				Sequence: 2, Kind: "address-removed", Family: "ipv4", PublicAddress: stringPointer("8.8.8.8"),
				LocalInterface: &localInterface, LocalAddress: &localAddress, LikelyNAT: true, ObservedAt: checkedAt.Add(time.Minute),
			},
			{
				ID: changeEventID, EgressID: path.ID, HistoryGeneration: systemState.HistoryGeneration,
				Sequence: 3, Kind: "address-added", Family: "ipv4", PublicAddress: &nextAddress,
				LocalInterface: &localInterface, LocalAddress: &localAddress, LikelyNAT: true, ObservedAt: checkedAt.Add(time.Minute),
			},
		},
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
	removedNotification, err := notificationForSource(ctx, store, removedEventID)
	if err != nil || removedNotification.EgressID == nil || *removedNotification.EgressID != canonical.ID {
		t.Fatalf("address-removed notification = %#v, %v", removedNotification, err)
	}

	failureEventID := uuid.New()
	failureReason := "no-valid-response"
	failure := AddressUpload{Events: []AddressEvent{{
		ID: failureEventID, EgressID: path.ID, HistoryGeneration: systemState.HistoryGeneration,
		Sequence: 4, Kind: "check-failure", Family: "ipv4", FailureReason: &failureReason,
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
		DroppedCount: 1, FirstSequence: 5, LastSequence: 5,
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
