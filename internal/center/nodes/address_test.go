package nodes

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/center/database"
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
	if err != nil || len(network.Egresses) != 2 {
		t.Fatalf("network egresses = %#v, %v", network.Egresses, err)
	}
	var egress NetworkEgress
	for _, item := range network.Egresses {
		if item.Family == "ipv4" {
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
	if len(completed.AddressStates) != 0 || len(completed.AddressEvents) != 0 {
		t.Fatalf("completed deletion retained address history: %#v", completed)
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
