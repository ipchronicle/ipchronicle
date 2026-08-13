package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"time"

	"github.com/google/uuid"
	bolt "go.etcd.io/bbolt"
)

const maxPendingAddressEventsPerEgress = 30

var (
	ErrInvalidAddressObservation = errors.New("invalid address observation")
	ErrNoAddressState            = errors.New("address state does not exist")
)

type AddressState struct {
	EgressID          string     `json:"egressId"`
	HistoryGeneration string     `json:"historyGeneration"`
	Family            string     `json:"family"`
	Status            string     `json:"status"`
	Sequence          int64      `json:"sequence"`
	PublicAddress     *string    `json:"publicAddress,omitempty"`
	LocalInterface    *string    `json:"localInterface,omitempty"`
	LocalAddress      *string    `json:"localAddress,omitempty"`
	ProxyPath         bool       `json:"proxyPath"`
	LikelyNAT         bool       `json:"likelyNat"`
	Temporary         bool       `json:"temporary"`
	FailureReason     *string    `json:"failureReason,omitempty"`
	LastCheckedAt     time.Time  `json:"lastCheckedAt"`
	LastSucceededAt   *time.Time `json:"lastSucceededAt,omitempty"`
	LastChangedAt     *time.Time `json:"lastChangedAt,omitempty"`
}

type AddressEvent struct {
	ID                string    `json:"id"`
	EgressID          string    `json:"egressId"`
	HistoryGeneration string    `json:"historyGeneration"`
	Sequence          int64     `json:"sequence"`
	Kind              string    `json:"kind"`
	Family            string    `json:"family"`
	PreviousAddress   *string   `json:"previousAddress,omitempty"`
	PublicAddress     *string   `json:"publicAddress,omitempty"`
	LocalInterface    *string   `json:"localInterface,omitempty"`
	LocalAddress      *string   `json:"localAddress,omitempty"`
	ProxyPath         bool      `json:"proxyPath"`
	LikelyNAT         bool      `json:"likelyNat"`
	Temporary         bool      `json:"temporary"`
	FailureReason     *string   `json:"failureReason,omitempty"`
	ObservedAt        time.Time `json:"observedAt"`
}

type AddressGap struct {
	ID                string    `json:"id"`
	EgressID          string    `json:"egressId"`
	HistoryGeneration string    `json:"historyGeneration"`
	DroppedCount      int64     `json:"droppedCount"`
	FirstSequence     int64     `json:"firstSequence"`
	LastSequence      int64     `json:"lastSequence"`
	FirstObservedAt   time.Time `json:"firstObservedAt"`
	LastObservedAt    time.Time `json:"lastObservedAt"`
}

type AddressObservation struct {
	EgressID              string
	ConfigurationRevision int64
	HistoryGeneration     string
	Family                string
	Confirmed             bool
	PublicAddress         string
	LocalInterface        *string
	LocalAddress          *string
	ProxyPath             bool
	LikelyNAT             bool
	Temporary             bool
	FailureReason         string
	CheckedAt             time.Time
}

type AddressUpload struct {
	States []AddressState
	Events []AddressEvent
	Gaps   []AddressGap
}

type AddressGapReceipt struct {
	ID           string
	LastSequence int64
}

type AddressUploadReceipt struct {
	AcceptedEventIDs  []string
	DiscardedEventIDs []string
	AcceptedGaps      []AddressGapReceipt
	DiscardedGaps     []AddressGapReceipt
}

func (s *Store) AddressState(egressID string) (AddressState, error) {
	var result AddressState
	err := s.database.View(func(transaction *bolt.Tx) error {
		encoded := transaction.Bucket(addressCurrentBucket).Get([]byte(egressID))
		if encoded == nil {
			return ErrNoAddressState
		}
		return decodeAddressState(encoded, &result)
	})
	return result, err
}

func (s *Store) RecordAddressObservation(observation AddressObservation) (bool, error) {
	if err := validateAddressObservation(observation); err != nil {
		return false, err
	}
	changed := false
	err := s.database.Update(func(transaction *bolt.Tx) error {
		configuration, err := configurationFromTransaction(s.masterKey, transaction)
		if err != nil {
			return err
		}
		if configuration.Revision != observation.ConfigurationRevision || configuration.HistoryGeneration != observation.HistoryGeneration ||
			!configurationContainsEgress(configuration, observation.EgressID, observation.Family) {
			return ErrInvalidAddressObservation
		}
		currentBucket := transaction.Bucket(addressCurrentBucket)
		var previous *AddressState
		if encoded := currentBucket.Get([]byte(observation.EgressID)); encoded != nil {
			var value AddressState
			if err := decodeAddressState(encoded, &value); err != nil {
				return err
			}
			previous = &value
		}
		state, events, didChange := transitionAddressState(previous, observation)
		changed = didChange
		encoded, err := json.Marshal(state)
		if err != nil {
			return err
		}
		if err := currentBucket.Put([]byte(state.EgressID), encoded); err != nil {
			return err
		}
		for _, event := range events {
			if err := enqueueAddressEvent(transaction, event); err != nil {
				return err
			}
		}
		return nil
	})
	return changed, err
}

func (s *Store) AddressUpload(maxEvents int) (AddressUpload, error) {
	if maxEvents < 1 || maxEvents > 64 {
		return AddressUpload{}, errors.New("address upload event limit must be between 1 and 64")
	}
	result := AddressUpload{}
	err := s.database.View(func(transaction *bolt.Tx) error {
		if err := transaction.Bucket(addressCurrentBucket).ForEach(func(_, encoded []byte) error {
			var item AddressState
			if err := decodeAddressState(encoded, &item); err != nil {
				return err
			}
			result.States = append(result.States, item)
			return nil
		}); err != nil {
			return err
		}
		if err := transaction.Bucket(addressEventsBucket).ForEach(func(_, encoded []byte) error {
			var item AddressEvent
			if err := decodeAddressEvent(encoded, &item); err != nil {
				return err
			}
			result.Events = append(result.Events, item)
			return nil
		}); err != nil {
			return err
		}
		if err := transaction.Bucket(addressGapsBucket).ForEach(func(_, encoded []byte) error {
			var item AddressGap
			if err := decodeAddressGap(encoded, &item); err != nil {
				return err
			}
			result.Gaps = append(result.Gaps, item)
			return nil
		}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return AddressUpload{}, err
	}
	slices.SortFunc(result.States, func(a, b AddressState) int { return bytes.Compare([]byte(a.EgressID), []byte(b.EgressID)) })
	slices.SortFunc(result.Events, func(a, b AddressEvent) int {
		if order := a.ObservedAt.Compare(b.ObservedAt); order != 0 {
			return order
		}
		if order := bytes.Compare([]byte(a.EgressID), []byte(b.EgressID)); order != 0 {
			return order
		}
		return int(a.Sequence - b.Sequence)
	})
	if len(result.Events) > maxEvents {
		result.Events = result.Events[:maxEvents]
	}
	slices.SortFunc(result.Gaps, func(a, b AddressGap) int { return bytes.Compare([]byte(a.EgressID), []byte(b.EgressID)) })
	return result, nil
}

func (s *Store) AcknowledgeAddressUpload(receipt AddressUploadReceipt) error {
	return s.database.Update(func(transaction *bolt.Tx) error {
		events := transaction.Bucket(addressEventsBucket)
		for _, id := range append(slices.Clone(receipt.AcceptedEventIDs), receipt.DiscardedEventIDs...) {
			if _, err := uuid.Parse(id); err != nil {
				return errors.New("address upload receipt contains an invalid event ID")
			}
			if err := events.Delete([]byte(id)); err != nil {
				return err
			}
		}
		gaps := transaction.Bucket(addressGapsBucket)
		for _, acknowledged := range append(slices.Clone(receipt.AcceptedGaps), receipt.DiscardedGaps...) {
			if _, err := uuid.Parse(acknowledged.ID); err != nil || acknowledged.LastSequence < 1 {
				return errors.New("address upload receipt contains an invalid gap")
			}
			var storedKey []byte
			var gap AddressGap
			if err := gaps.ForEach(func(key, encoded []byte) error {
				var item AddressGap
				if err := decodeAddressGap(encoded, &item); err != nil {
					return err
				}
				if item.ID == acknowledged.ID {
					storedKey = append([]byte(nil), key...)
					gap = item
				}
				return nil
			}); err != nil {
				return err
			}
			if storedKey == nil {
				continue
			}
			if gap.LastSequence <= acknowledged.LastSequence {
				if err := gaps.Delete(storedKey); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func transitionAddressState(previous *AddressState, observation AddressObservation) (AddressState, []AddressEvent, bool) {
	state := AddressState{
		EgressID: observation.EgressID, HistoryGeneration: observation.HistoryGeneration,
		Family: observation.Family, LastCheckedAt: observation.CheckedAt.UTC(),
		LocalInterface: cloneString(observation.LocalInterface), LocalAddress: cloneString(observation.LocalAddress),
		ProxyPath: observation.ProxyPath, LikelyNAT: observation.LikelyNAT, Temporary: observation.Temporary,
	}
	if previous != nil {
		state.Sequence = previous.Sequence
		state.PublicAddress = cloneString(previous.PublicAddress)
		state.LastSucceededAt = cloneTime(previous.LastSucceededAt)
		state.LastChangedAt = cloneTime(previous.LastChangedAt)
		if state.LocalInterface == nil {
			state.LocalInterface = cloneString(previous.LocalInterface)
		}
		if state.LocalAddress == nil {
			state.LocalAddress = cloneString(previous.LocalAddress)
		}
		if !observation.Confirmed {
			state.ProxyPath = previous.ProxyPath
			state.LikelyNAT = previous.LikelyNAT
			state.Temporary = previous.Temporary
		}
	}
	var events []AddressEvent
	appendEvent := func(kind string, previousAddress *string) {
		state.Sequence++
		events = append(events, AddressEvent{
			ID: uuid.NewString(), EgressID: state.EgressID, HistoryGeneration: state.HistoryGeneration,
			Sequence: state.Sequence, Kind: kind, Family: state.Family,
			PreviousAddress: cloneString(previousAddress), PublicAddress: cloneString(state.PublicAddress),
			LocalInterface: cloneString(state.LocalInterface), LocalAddress: cloneString(state.LocalAddress),
			ProxyPath: state.ProxyPath, LikelyNAT: state.LikelyNAT, Temporary: state.Temporary,
			FailureReason: cloneString(state.FailureReason), ObservedAt: state.LastCheckedAt,
		})
	}
	if !observation.Confirmed {
		state.Status = "failed"
		failureReason := observation.FailureReason
		state.FailureReason = &failureReason
		if previous == nil || previous.Status != "failed" {
			appendEvent("check-failure", state.PublicAddress)
		}
		return state, events, false
	}
	publicAddress := observation.PublicAddress
	oldAddress := cloneString(state.PublicAddress)
	state.Status = "confirmed"
	state.FailureReason = nil
	state.PublicAddress = &publicAddress
	succeededAt := state.LastCheckedAt
	state.LastSucceededAt = &succeededAt
	if previous != nil && previous.Status == "failed" {
		appendEvent("recovery", oldAddress)
	}
	if oldAddress == nil {
		changedAt := state.LastCheckedAt
		state.LastChangedAt = &changedAt
		appendEvent("first-observation", nil)
		return state, events, false
	}
	if *oldAddress != publicAddress {
		changedAt := state.LastCheckedAt
		state.LastChangedAt = &changedAt
		appendEvent("address-change", oldAddress)
		return state, events, true
	}
	return state, events, false
}

func enqueueAddressEvent(transaction *bolt.Tx, event AddressEvent) error {
	encoded, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if err := transaction.Bucket(addressEventsBucket).Put([]byte(event.ID), encoded); err != nil {
		return err
	}
	type queued struct {
		key   []byte
		event AddressEvent
	}
	var matching []queued
	if err := transaction.Bucket(addressEventsBucket).ForEach(func(key, value []byte) error {
		var item AddressEvent
		if err := decodeAddressEvent(value, &item); err != nil {
			return err
		}
		if item.EgressID == event.EgressID {
			matching = append(matching, queued{key: append([]byte(nil), key...), event: item})
		}
		return nil
	}); err != nil {
		return err
	}
	slices.SortFunc(matching, func(a, b queued) int { return int(a.event.Sequence - b.event.Sequence) })
	for len(matching) > maxPendingAddressEventsPerEgress {
		dropped := matching[0]
		matching = matching[1:]
		if err := transaction.Bucket(addressEventsBucket).Delete(dropped.key); err != nil {
			return err
		}
		if err := extendAddressGap(transaction, dropped.event); err != nil {
			return err
		}
	}
	return nil
}

func extendAddressGap(transaction *bolt.Tx, dropped AddressEvent) error {
	bucket := transaction.Bucket(addressGapsBucket)
	var gap AddressGap
	key := []byte(dropped.EgressID)
	if encoded := bucket.Get(key); encoded == nil {
		gap = AddressGap{
			ID: uuid.NewString(), EgressID: dropped.EgressID, HistoryGeneration: dropped.HistoryGeneration,
			DroppedCount: 1, FirstSequence: dropped.Sequence, LastSequence: dropped.Sequence,
			FirstObservedAt: dropped.ObservedAt, LastObservedAt: dropped.ObservedAt,
		}
	} else {
		if err := decodeAddressGap(encoded, &gap); err != nil {
			return err
		}
		gap.DroppedCount++
		gap.LastSequence = dropped.Sequence
		gap.LastObservedAt = dropped.ObservedAt
	}
	encoded, err := json.Marshal(gap)
	if err != nil {
		return err
	}
	return bucket.Put(key, encoded)
}

func reconcileAddressBuckets(transaction *bolt.Tx, configuration Configuration, previousGeneration string) (int64, error) {
	var discarded int64
	if previousGeneration != "" && previousGeneration != configuration.HistoryGeneration {
		for _, name := range [][]byte{addressCurrentBucket, addressEventsBucket, addressGapsBucket} {
			count := transaction.Bucket(name).Stats().KeyN
			discarded += int64(count)
			if err := clearBucket(transaction.Bucket(name)); err != nil {
				return 0, err
			}
		}
		return discarded, nil
	}
	paths := configuration.discoveryPaths()
	retained := make(map[string]struct{}, len(paths))
	for _, egress := range paths {
		retained[egress.ID] = struct{}{}
	}
	for _, name := range [][]byte{addressCurrentBucket, addressEventsBucket, addressGapsBucket} {
		bucket := transaction.Bucket(name)
		var keys [][]byte
		if err := bucket.ForEach(func(key, encoded []byte) error {
			var header struct {
				EgressID string `json:"egressId"`
			}
			if err := json.Unmarshal(encoded, &header); err != nil {
				return err
			}
			if _, exists := retained[header.EgressID]; !exists {
				keys = append(keys, append([]byte(nil), key...))
			}
			return nil
		}); err != nil {
			return 0, err
		}
		for _, key := range keys {
			if err := bucket.Delete(key); err != nil {
				return 0, err
			}
		}
	}
	return 0, nil
}

func clearBucket(bucket *bolt.Bucket) error {
	var keys [][]byte
	if err := bucket.ForEach(func(key, _ []byte) error {
		keys = append(keys, append([]byte(nil), key...))
		return nil
	}); err != nil {
		return err
	}
	for _, key := range keys {
		if err := bucket.Delete(key); err != nil {
			return err
		}
	}
	return nil
}

func configurationFromTransaction(masterKey [masterKeySize]byte, transaction *bolt.Tx) (Configuration, error) {
	encoded := transaction.Bucket(configurationBucket).Get(configurationKey)
	if encoded == nil {
		return Configuration{}, ErrNoConfiguration
	}
	configuration, err := decodeStoredConfiguration(masterKey, encoded)
	if err != nil {
		return Configuration{}, err
	}
	return configuration, validateConfiguration(configuration)
}

func configurationContainsEgress(configuration Configuration, egressID, family string) bool {
	for _, egress := range configuration.discoveryPaths() {
		if egress.ID == egressID && egress.Family == family && egress.Enabled && configuration.Enabled {
			return true
		}
	}
	return false
}

func validateAddressObservation(observation AddressObservation) error {
	if _, err := uuid.Parse(observation.EgressID); err != nil || observation.ConfigurationRevision < 1 || len(observation.HistoryGeneration) != 64 ||
		(observation.Family != "ipv4" && observation.Family != "ipv6") || observation.CheckedAt.IsZero() {
		return ErrInvalidAddressObservation
	}
	if observation.Confirmed {
		address, err := netip.ParseAddr(observation.PublicAddress)
		if err != nil || address != address.Unmap() || (address.Is4() && observation.Family != "ipv4") || (address.Is6() && observation.Family != "ipv6") ||
			observation.FailureReason != "" {
			return ErrInvalidAddressObservation
		}
	} else if !validAddressFailureReason(observation.FailureReason) || observation.PublicAddress != "" {
		return ErrInvalidAddressObservation
	}
	if observation.LocalAddress != nil {
		address, err := netip.ParseAddr(*observation.LocalAddress)
		if err != nil || address != address.Unmap() || (address.Is4() && observation.Family != "ipv4") || (address.Is6() && observation.Family != "ipv6") {
			return ErrInvalidAddressObservation
		}
	}
	if (observation.LocalInterface == nil) != (observation.LocalAddress == nil) && !observation.ProxyPath {
		return ErrInvalidAddressObservation
	}
	return nil
}

func validAddressFailureReason(value string) bool {
	switch value {
	case "selector-unavailable", "no-valid-response", "confirmation-unavailable", "conflicting-responses":
		return true
	default:
		return false
	}
}

func decodeAddressState(encoded []byte, target *AddressState) error {
	if err := json.Unmarshal(encoded, target); err != nil {
		return fmt.Errorf("decode retained address state: %w", err)
	}
	return nil
}

func decodeAddressEvent(encoded []byte, target *AddressEvent) error {
	if err := json.Unmarshal(encoded, target); err != nil {
		return fmt.Errorf("decode queued address event: %w", err)
	}
	return nil
}

func decodeAddressGap(encoded []byte, target *AddressGap) error {
	if err := json.Unmarshal(encoded, target); err != nil {
		return fmt.Errorf("decode queued address gap: %w", err)
	}
	return nil
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}
