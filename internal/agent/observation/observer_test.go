package observation

import (
	"context"
	"errors"
	"io"
	"log"
	"path/filepath"
	"testing"
	"time"

	"github.com/ipchronicle/ipchronicle/internal/agent/state"
)

func TestReconcileScheduleRunsChangedEgressesImmediately(t *testing.T) {
	now := time.Date(2026, 8, 9, 15, 0, 0, 0, time.UTC)
	configuration := observerTestConfiguration()
	configuration.Egresses = append(configuration.Egresses, state.Egress{
		ID: "11111111-1111-4111-8111-111111111111", Kind: "default", Family: "ipv6",
		Enabled: true, LightweightIntervalSeconds: 900, ProbeOnAddressChange: true,
	})
	schedule := make(map[string]scheduleEntry)
	reconcileSchedule(schedule, configuration, now)
	if len(schedule) != 2 {
		t.Fatalf("initial schedule = %#v", schedule)
	}
	first, found := nextDueEgress(configuration, schedule, now)
	if !found || first.ID != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("first due egress = %#v, %v", first, found)
	}
	later := now.Add(time.Hour)
	entry := schedule[first.ID]
	entry.next = later
	schedule[first.ID] = entry
	reconcileSchedule(schedule, configuration, now.Add(time.Minute))
	if !schedule[first.ID].next.Equal(later) {
		t.Fatalf("unchanged configuration reset schedule = %#v", schedule[first.ID])
	}
	configuration.DiscoveryServices.IPv6[0] = "https://changed-six-one.example/ip"
	reconcileSchedule(schedule, configuration, now.Add(2*time.Minute))
	if !schedule[first.ID].next.Equal(now.Add(2 * time.Minute)) {
		t.Fatalf("discovery-service change did not become immediately due: %#v", schedule[first.ID])
	}
	configuration.Enabled = false
	reconcileSchedule(schedule, configuration, now.Add(3*time.Minute))
	if len(schedule) != 0 {
		t.Fatalf("disabled node retained address schedule: %#v", schedule)
	}
}

func TestObserverCancellationDoesNotRecordFailure(t *testing.T) {
	store := openObserverTestStore(t)
	entered := make(chan struct{})
	observer := NewObserver(store, log.New(io.Discard, "", 0))
	observer.checker = checkerFunc(func(ctx context.Context, configuration state.Configuration, egress state.Egress, _ *state.AddressState, checkedAt time.Time) state.AddressObservation {
		close(entered)
		<-ctx.Done()
		return state.AddressObservation{
			EgressID: egress.ID, ConfigurationRevision: configuration.Revision,
			HistoryGeneration: configuration.HistoryGeneration, Family: egress.Family,
			FailureReason: "no-valid-response", CheckedAt: checkedAt,
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- observer.Run(ctx) }()
	<-entered
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddressState(observerTestConfiguration().Egresses[0].ID); !errors.Is(err, state.ErrNoAddressState) {
		t.Fatalf("shutdown recorded an address state: %v", err)
	}
}

func TestObserverSurfacesInvalidCheckerOutputForCurrentConfiguration(t *testing.T) {
	store := openObserverTestStore(t)
	observer := NewObserver(store, log.New(io.Discard, "", 0))
	observer.checker = checkerFunc(func(_ context.Context, configuration state.Configuration, egress state.Egress, _ *state.AddressState, checkedAt time.Time) state.AddressObservation {
		return state.AddressObservation{
			EgressID: egress.ID, ConfigurationRevision: configuration.Revision,
			HistoryGeneration: configuration.HistoryGeneration, Family: egress.Family,
			FailureReason: "unknown-failure", CheckedAt: checkedAt,
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := observer.Run(ctx); !errors.Is(err, state.ErrInvalidAddressObservation) {
		t.Fatalf("observer error = %v", err)
	}
}

type checkerFunc func(context.Context, state.Configuration, state.Egress, *state.AddressState, time.Time) state.AddressObservation

func (function checkerFunc) Check(ctx context.Context, configuration state.Configuration, egress state.Egress, previous *state.AddressState, checkedAt time.Time) state.AddressObservation {
	return function(ctx, configuration, egress, previous, checkedAt)
}

func openObserverTestStore(t *testing.T) *state.Store {
	t.Helper()
	store, err := state.Open(filepath.Join(t.TempDir(), "agent"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.SaveIdentity(state.Identity{
		CenterURL: "https://center.example", NodeID: "7289cfa3-a75d-4a3f-ac06-8f1074446a85",
		Credential: "ipc_agent_observer-test",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.ApplyConfiguration(observerTestConfiguration()); err != nil {
		t.Fatal(err)
	}
	return store
}

func observerTestConfiguration() state.Configuration {
	return state.Configuration{
		SchemaVersion: 4, Revision: 1, Enabled: true,
		HistoryGeneration: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		DiscoveryServices: state.DiscoveryServices{
			IPv4: []string{"https://one.example/ip", "https://two.example/ip"},
			IPv6: []string{"https://six-one.example/ip", "https://six-two.example/ip"},
		},
		Egresses: []state.Egress{{
			ID: "d099bad9-e7c4-42a9-bd19-ad85408321c5", Kind: "default", Family: "ipv4",
			Enabled: true, LightweightIntervalSeconds: 600, ProbeOnAddressChange: true,
		}},
	}
}
