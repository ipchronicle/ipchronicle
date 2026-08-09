package state

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

const testHistoryGeneration = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestAddressTransitionsAndBoundedGapSurviveRestart(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "agent")
	store, egressID := openAddressTestStore(t, directory)
	start := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	local := "10.0.0.5"
	interfaceName := "eth0"

	record := func(index int, confirmed bool, publicAddress, failureReason string) bool {
		changed, err := store.RecordAddressObservation(AddressObservation{
			EgressID: egressID, ConfigurationRevision: 1, HistoryGeneration: testHistoryGeneration,
			Family: "ipv4", Confirmed: confirmed, PublicAddress: publicAddress,
			LocalInterface: &interfaceName, LocalAddress: &local, LikelyNAT: confirmed,
			FailureReason: failureReason, CheckedAt: start.Add(time.Duration(index) * time.Minute),
		})
		if err != nil {
			t.Fatal(err)
		}
		return changed
	}

	if record(0, false, "", "no-valid-response") {
		t.Fatal("failure unexpectedly reported an address change")
	}
	record(1, false, "", "no-valid-response")
	if record(2, true, "203.0.113.10", "") {
		t.Fatal("first confirmed address unexpectedly reported a change")
	}
	record(3, true, "203.0.113.10", "")
	if !record(4, true, "203.0.113.11", "") {
		t.Fatal("confirmed address transition was not reported as a change")
	}
	upload, err := store.AddressUpload(64)
	if err != nil {
		t.Fatal(err)
	}
	if len(upload.States) != 1 || len(upload.Events) != 4 || len(upload.Gaps) != 0 {
		t.Fatalf("initial address upload = %#v", upload)
	}
	wantKinds := []string{"check-failure", "recovery", "first-observation", "address-change"}
	for index, event := range upload.Events {
		if event.Kind != wantKinds[index] || event.Sequence != int64(index+1) {
			t.Fatalf("event %d = %#v", index, event)
		}
	}

	for index := 5; index < 40; index++ {
		if index%2 == 1 {
			record(index, false, "", "confirmation-unavailable")
		} else {
			record(index, true, "203.0.113.11", "")
		}
	}
	upload, err = store.AddressUpload(64)
	if err != nil {
		t.Fatal(err)
	}
	if len(upload.Events) != 30 || len(upload.Gaps) != 1 || upload.Gaps[0].DroppedCount != 9 || upload.States[0].Sequence != 39 {
		t.Fatalf("bounded address upload = %#v", upload)
	}
	oldGap := upload.Gaps[0]
	record(40, true, "203.0.113.11", "")
	record(41, false, "", "no-valid-response")
	if err := store.AcknowledgeAddressUpload(AddressUploadReceipt{
		AcceptedGaps: []AddressGapReceipt{{ID: oldGap.ID, LastSequence: oldGap.LastSequence}},
	}); err != nil {
		t.Fatal(err)
	}
	upload, err = store.AddressUpload(64)
	if err != nil {
		t.Fatal(err)
	}
	if len(upload.Gaps) != 1 || upload.Gaps[0].LastSequence <= oldGap.LastSequence {
		t.Fatalf("extended gap was removed by stale acknowledgement: %#v", upload.Gaps)
	}

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	upload, err = restarted.AddressUpload(64)
	if err != nil || len(upload.States) != 1 || len(upload.Events) != 30 || len(upload.Gaps) != 1 {
		t.Fatalf("restarted address upload = %#v, %v", upload, err)
	}
}

func TestHistoryGenerationChangeClearsAddressStateAndQueue(t *testing.T) {
	store, egressID := openAddressTestStore(t, filepath.Join(t.TempDir(), "agent"))
	defer store.Close()
	if _, err := store.RecordAddressObservation(AddressObservation{
		EgressID: egressID, ConfigurationRevision: 1, HistoryGeneration: testHistoryGeneration,
		Family: "ipv4", FailureReason: "selector-unavailable", CheckedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	configuration, err := store.Configuration()
	if err != nil {
		t.Fatal(err)
	}
	configuration.Revision = 2
	configuration.HistoryGeneration = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	if err := store.ApplyConfiguration(configuration); err != nil {
		t.Fatal(err)
	}
	upload, err := store.AddressUpload(64)
	if err != nil || len(upload.States)+len(upload.Events)+len(upload.Gaps) != 0 {
		t.Fatalf("address data survived history generation change: %#v, %v", upload, err)
	}
	if _, err := store.AddressState(egressID); !errors.Is(err, ErrNoAddressState) {
		t.Fatalf("address state after generation change error = %v", err)
	}
}

func openAddressTestStore(t *testing.T, directory string) (*Store, string) {
	t.Helper()
	store, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveIdentity(Identity{
		CenterURL: "https://center.example", NodeID: "7289cfa3-a75d-4a3f-ac06-8f1074446a85",
		Credential: "ipc_agent_secret-address-test",
	}); err != nil {
		t.Fatal(err)
	}
	egressID := "c7b5eeac-903d-4b99-961d-190a8a4e5d2e"
	if err := store.ApplyConfiguration(Configuration{
		SchemaVersion: 5, Revision: 1, Enabled: true, HistoryGeneration: testHistoryGeneration,
		ProbeSchedule:     ProbeSchedule{Enabled: true, Cron: "0 0 0 * * *", Timezone: "agent-local"},
		DiscoveryServices: testDiscoveryServices(),
		Egresses: []Egress{{
			ID: egressID, Kind: "default", Family: "ipv4", Enabled: true,
			LightweightIntervalSeconds: 600, ProbeOnAddressChange: true,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	return store, egressID
}
