package observation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/ipchronicle/ipchronicle/internal/agent/state"
)

const schedulerResolution = 500 * time.Millisecond

type scheduleEntry struct {
	fingerprint string
	next        time.Time
}

type checker interface {
	Check(context.Context, state.Configuration, state.Egress, *state.AddressState, time.Time) state.AddressObservation
}

type Observer struct {
	store             *state.Store
	checker           checker
	logger            *log.Logger
	now               func() time.Time
	onConfirmedChange func(context.Context, string, time.Time) error
}

func NewObserver(store *state.Store, logger *log.Logger) *Observer {
	return NewObserverWithChangeHandler(store, logger, nil)
}

func NewObserverWithChangeHandler(
	store *state.Store,
	logger *log.Logger,
	onConfirmedChange func(context.Context, string, time.Time) error,
) *Observer {
	if store == nil {
		panic("address observer store must not be nil")
	}
	if logger == nil {
		logger = log.Default()
	}
	return &Observer{
		store: store, checker: NewChecker(), logger: logger, now: time.Now,
		onConfirmedChange: onConfirmedChange,
	}
}

func (o *Observer) Run(ctx context.Context) error {
	schedule := make(map[string]scheduleEntry)
	for {
		configuration, err := o.store.Configuration()
		if errors.Is(err, state.ErrNoConfiguration) {
			if err := waitForObservationCycle(ctx); err != nil {
				return nil
			}
			continue
		}
		if err != nil {
			return err
		}
		now := o.now().UTC()
		reconcileSchedule(schedule, configuration, now)
		egress, found := nextDueEgress(configuration, schedule, now)
		if !found {
			if err := waitForObservationCycle(ctx); err != nil {
				return nil
			}
			continue
		}
		var previous *state.AddressState
		retained, err := o.store.AddressState(egress.ID)
		if err == nil {
			previous = &retained
		} else if !errors.Is(err, state.ErrNoAddressState) {
			return err
		}
		observation := o.checker.Check(ctx, configuration, egress, previous, now)
		if ctx.Err() != nil {
			return nil
		}
		changed, err := o.store.RecordAddressObservation(observation)
		if errors.Is(err, state.ErrInvalidAddressObservation) {
			current, currentErr := o.store.Configuration()
			if currentErr != nil {
				return currentErr
			}
			if current.Revision == configuration.Revision && current.HistoryGeneration == configuration.HistoryGeneration {
				return fmt.Errorf("address checker returned an invalid observation for configuration revision %d: %w", configuration.Revision, err)
			}
			continue
		}
		if err != nil {
			return err
		}
		if changed && egress.ProbeOnAddressChange && o.onConfirmedChange != nil {
			if err := o.onConfirmedChange(ctx, egress.ID, observation.CheckedAt); err != nil {
				return fmt.Errorf("trigger complete probe after address change: %w", err)
			}
		}
		if !observation.Confirmed {
			o.logger.Printf("lightweight address check for egress %s failed: %s", egress.ID, observation.FailureReason)
		}
		entry := schedule[egress.ID]
		entry.next = o.now().UTC().Add(time.Duration(egress.LightweightIntervalSeconds) * time.Second)
		schedule[egress.ID] = entry
	}
}

func reconcileSchedule(schedule map[string]scheduleEntry, configuration state.Configuration, now time.Time) {
	paths := configuration.DiscoveryPathList()
	retained := make(map[string]struct{}, len(paths))
	if configuration.Enabled {
		for _, egress := range paths {
			if !egress.Enabled {
				continue
			}
			retained[egress.ID] = struct{}{}
			fingerprint := egressFingerprint(configuration, egress)
			entry, exists := schedule[egress.ID]
			if !exists || entry.fingerprint != fingerprint {
				schedule[egress.ID] = scheduleEntry{fingerprint: fingerprint, next: now}
			}
		}
	}
	for id := range schedule {
		if _, exists := retained[id]; !exists {
			delete(schedule, id)
		}
	}
}

func nextDueEgress(configuration state.Configuration, schedule map[string]scheduleEntry, now time.Time) (state.Egress, bool) {
	var selected state.Egress
	var selectedAt time.Time
	found := false
	for _, egress := range configuration.DiscoveryPathList() {
		entry, exists := schedule[egress.ID]
		if !exists || entry.next.After(now) {
			continue
		}
		if !found || entry.next.Before(selectedAt) || (entry.next.Equal(selectedAt) && egress.ID < selected.ID) {
			selected = egress
			selectedAt = entry.next
			found = true
		}
	}
	return selected, found
}

func egressFingerprint(configuration state.Configuration, egress state.Egress) string {
	value := struct {
		HistoryGeneration string
		Egress            state.Egress
		Proxy             *state.Proxy
		Services          []string
	}{HistoryGeneration: configuration.HistoryGeneration, Egress: egress}
	if egress.Family == "ipv4" {
		value.Services = configuration.DiscoveryServices.IPv4
	} else {
		value.Services = configuration.DiscoveryServices.IPv6
	}
	if egress.ProxyID != nil {
		for index := range configuration.Proxies {
			if configuration.Proxies[index].ID == *egress.ProxyID {
				value.Proxy = &configuration.Proxies[index]
				break
			}
		}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func waitForObservationCycle(ctx context.Context) error {
	timer := time.NewTimer(schedulerResolution)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
