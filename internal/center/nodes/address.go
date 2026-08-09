package nodes

import (
	"context"
	"database/sql"
	"errors"
	"net/netip"
	"reflect"
	"time"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/center/database/historydb"
)

var sharedIPv4Prefix = netip.MustParsePrefix("100.64.0.0/10")

type AddressState struct {
	EgressID          uuid.UUID
	HistoryGeneration string
	Family            string
	Status            string
	Sequence          int64
	PublicAddress     *string
	LocalInterface    *string
	LocalAddress      *string
	ProxyPath         bool
	LikelyNAT         bool
	Temporary         bool
	FailureReason     *string
	LastCheckedAt     time.Time
	LastSucceededAt   *time.Time
	LastChangedAt     *time.Time
}

type AddressEvent struct {
	ID                uuid.UUID
	EgressID          uuid.UUID
	HistoryGeneration string
	Sequence          int64
	Kind              string
	Family            string
	PreviousAddress   *string
	PublicAddress     *string
	LocalInterface    *string
	LocalAddress      *string
	ProxyPath         bool
	LikelyNAT         bool
	Temporary         bool
	FailureReason     *string
	ObservedAt        time.Time
}

type AddressGap struct {
	ID                uuid.UUID
	EgressID          uuid.UUID
	HistoryGeneration string
	DroppedCount      int64
	FirstSequence     int64
	LastSequence      int64
	FirstObservedAt   time.Time
	LastObservedAt    time.Time
}

type AddressUpload struct {
	States      []AddressState
	Events      []AddressEvent
	Gaps        []AddressGap
	ProbeStatus *ProbeStatus
	TaskReport  *TaskReport
}

type AddressGapReceipt struct {
	ID           uuid.UUID
	LastSequence int64
}

type AddressUploadReceipt struct {
	AcceptedEventIDs  []uuid.UUID
	DiscardedEventIDs []uuid.UUID
	AcceptedGaps      []AddressGapReceipt
	DiscardedGaps     []AddressGapReceipt
}

func (s *Service) ingestAddressUpload(ctx context.Context, nodeID string, upload AddressUpload, receivedAt int64) (AddressUploadReceipt, error) {
	receipt := AddressUploadReceipt{
		AcceptedEventIDs: make([]uuid.UUID, 0), DiscardedEventIDs: make([]uuid.UUID, 0),
		AcceptedGaps: make([]AddressGapReceipt, 0), DiscardedGaps: make([]AddressGapReceipt, 0),
	}
	if len(upload.States) > 64 || len(upload.Events) > 64 || len(upload.Gaps) > 64 {
		return receipt, ErrInvalidMetadata
	}
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	systemState, err := s.queries.GetSystemState(ctx)
	if err != nil {
		return receipt, err
	}
	egressRecords, err := s.queries.ListActiveNodeEgresses(ctx, nodeID)
	if err != nil {
		return receipt, err
	}
	egresses := make(map[uuid.UUID]NetworkEgress, len(egressRecords))
	for _, record := range egressRecords {
		egress, err := egressFromRecord(record)
		if err != nil {
			return receipt, err
		}
		egresses[egress.ID] = egress
	}
	if err := validateAddressUpload(upload, egresses); err != nil {
		return receipt, ErrInvalidMetadata
	}
	transaction, err := s.history.BeginTx(ctx, nil)
	if err != nil {
		return receipt, err
	}
	defer transaction.Rollback()
	queries := s.historyQueries.WithTx(transaction)
	for _, item := range upload.States {
		if item.HistoryGeneration != systemState.HistoryGeneration {
			continue
		}
		if _, exists := egresses[item.EgressID]; !exists {
			continue
		}
		if err := queries.UpsertAddressState(ctx, addressStateParams(nodeID, item, receivedAt)); err != nil {
			return receipt, err
		}
	}
	for _, item := range upload.Events {
		if item.HistoryGeneration != systemState.HistoryGeneration {
			receipt.DiscardedEventIDs = append(receipt.DiscardedEventIDs, item.ID)
			continue
		}
		if _, exists := egresses[item.EgressID]; !exists {
			receipt.DiscardedEventIDs = append(receipt.DiscardedEventIDs, item.ID)
			continue
		}
		accepted, err := ingestAddressEvent(ctx, queries, nodeID, item, receivedAt)
		if err != nil {
			return receipt, err
		}
		if !accepted {
			return receipt, ErrInvalidMetadata
		}
		receipt.AcceptedEventIDs = append(receipt.AcceptedEventIDs, item.ID)
	}
	for _, item := range upload.Gaps {
		acknowledgement := AddressGapReceipt{ID: item.ID, LastSequence: item.LastSequence}
		if item.HistoryGeneration != systemState.HistoryGeneration {
			receipt.DiscardedGaps = append(receipt.DiscardedGaps, acknowledgement)
			continue
		}
		if _, exists := egresses[item.EgressID]; !exists {
			receipt.DiscardedGaps = append(receipt.DiscardedGaps, acknowledgement)
			continue
		}
		rows, err := queries.UpsertAddressGap(ctx, historydb.UpsertAddressGapParams{
			ID: item.ID.String(), EgressID: item.EgressID.String(), NodeID: nodeID,
			HistoryGeneration: item.HistoryGeneration, DroppedCount: item.DroppedCount,
			FirstSequence: item.FirstSequence, LastSequence: item.LastSequence,
			FirstObservedAt: item.FirstObservedAt.UTC().Unix(), LastObservedAt: item.LastObservedAt.UTC().Unix(),
			ReceivedAt: receivedAt,
		})
		if err != nil {
			return receipt, err
		}
		if rows != 1 {
			return receipt, ErrInvalidMetadata
		}
		receipt.AcceptedGaps = append(receipt.AcceptedGaps, acknowledgement)
	}
	if err := transaction.Commit(); err != nil {
		return receipt, err
	}
	return receipt, nil
}

func validateAddressUpload(upload AddressUpload, egresses map[uuid.UUID]NetworkEgress) error {
	states := make(map[uuid.UUID]struct{}, len(upload.States))
	events := make(map[uuid.UUID]struct{}, len(upload.Events))
	gaps := make(map[uuid.UUID]struct{}, len(upload.Gaps))
	for _, item := range upload.States {
		if _, exists := states[item.EgressID]; exists {
			return ErrInvalidMetadata
		}
		states[item.EgressID] = struct{}{}
		if err := validateAddressState(item, validationEgress(egresses, item.EgressID, item.Family, item.ProxyPath)); err != nil {
			return err
		}
	}
	for _, item := range upload.Events {
		if _, exists := events[item.ID]; exists {
			return ErrInvalidMetadata
		}
		events[item.ID] = struct{}{}
		if err := validateAddressEvent(item, validationEgress(egresses, item.EgressID, item.Family, item.ProxyPath)); err != nil {
			return err
		}
	}
	for _, item := range upload.Gaps {
		if _, exists := gaps[item.ID]; exists {
			return ErrInvalidMetadata
		}
		gaps[item.ID] = struct{}{}
		if item.ID == uuid.Nil || item.EgressID == uuid.Nil || !validHistoryGeneration(item.HistoryGeneration) ||
			item.DroppedCount < 1 || item.FirstSequence < 1 || item.LastSequence < item.FirstSequence ||
			item.DroppedCount != item.LastSequence-item.FirstSequence+1 || item.FirstObservedAt.IsZero() ||
			item.LastObservedAt.Before(item.FirstObservedAt) {
			return ErrInvalidMetadata
		}
	}
	return nil
}

func validationEgress(egresses map[uuid.UUID]NetworkEgress, id uuid.UUID, family string, proxyPath bool) NetworkEgress {
	if egress, exists := egresses[id]; exists {
		return egress
	}
	kind := "default"
	if proxyPath {
		kind = "proxy"
	}
	return NetworkEgress{ID: id, Family: family, Kind: kind}
}

func validateAddressState(item AddressState, egress NetworkEgress) error {
	if item.EgressID == uuid.Nil || !validHistoryGeneration(item.HistoryGeneration) || item.Sequence < 0 ||
		item.LastCheckedAt.IsZero() || (item.Status != "confirmed" && item.Status != "failed") ||
		!addressPathMatches(item.Family, item.PublicAddress, item.LocalInterface, item.LocalAddress, item.ProxyPath, item.LikelyNAT, item.Temporary, egress) {
		return ErrInvalidMetadata
	}
	if item.Status == "confirmed" {
		if item.PublicAddress == nil || item.FailureReason != nil || item.LastSucceededAt == nil || item.LastSucceededAt.After(item.LastCheckedAt) {
			return ErrInvalidMetadata
		}
	} else if item.FailureReason == nil || !validAddressFailureReason(*item.FailureReason) {
		return ErrInvalidMetadata
	}
	if item.LastChangedAt != nil && item.LastChangedAt.After(item.LastCheckedAt) {
		return ErrInvalidMetadata
	}
	return nil
}

func validateAddressEvent(item AddressEvent, egress NetworkEgress) error {
	if item.ID == uuid.Nil || item.EgressID == uuid.Nil || !validHistoryGeneration(item.HistoryGeneration) || item.Sequence < 1 ||
		item.ObservedAt.IsZero() || !addressPathMatches(item.Family, item.PublicAddress, item.LocalInterface, item.LocalAddress, item.ProxyPath, item.LikelyNAT, item.Temporary, egress) {
		return ErrInvalidMetadata
	}
	switch item.Kind {
	case "first-observation":
		if item.PreviousAddress != nil || item.PublicAddress == nil || item.FailureReason != nil {
			return ErrInvalidMetadata
		}
	case "address-change":
		if item.PreviousAddress == nil || item.PublicAddress == nil || *item.PreviousAddress == *item.PublicAddress || item.FailureReason != nil {
			return ErrInvalidMetadata
		}
	case "check-failure":
		if item.FailureReason == nil || !validAddressFailureReason(*item.FailureReason) {
			return ErrInvalidMetadata
		}
	case "recovery":
		if item.PublicAddress == nil || item.FailureReason != nil {
			return ErrInvalidMetadata
		}
	default:
		return ErrInvalidMetadata
	}
	if item.PreviousAddress != nil && !validPublicAddress(*item.PreviousAddress, item.Family) {
		return ErrInvalidMetadata
	}
	return nil
}

func addressPathMatches(family string, publicAddress, localInterface, localAddress *string, proxyPath, likelyNAT, temporary bool, egress NetworkEgress) bool {
	if (family != "ipv4" && family != "ipv6") || egress.ID == uuid.Nil || egress.Family != family || proxyPath != (egress.Kind == "proxy") {
		return false
	}
	if publicAddress != nil && !validPublicAddress(*publicAddress, family) {
		return false
	}
	if (localInterface == nil) != (localAddress == nil) || (localInterface != nil && !validInterfaceName(*localInterface)) ||
		(localAddress != nil && !validCanonicalAddress(*localAddress, family)) || (temporary && family != "ipv6") {
		return false
	}
	if proxyPath {
		return !likelyNAT && !temporary
	}
	if publicAddress != nil && localAddress != nil {
		local, _ := netip.ParseAddr(*localAddress)
		public, _ := netip.ParseAddr(*publicAddress)
		expectedNAT := local != public || addressScope(local) != "global"
		return likelyNAT == expectedNAT
	}
	return !likelyNAT
}

func validCanonicalAddress(value, family string) bool {
	address, err := netip.ParseAddr(value)
	return err == nil && address == address.Unmap() && address.String() == value && familyOf(address) == family
}

func validPublicAddress(value, family string) bool {
	address, err := netip.ParseAddr(value)
	return err == nil && address == address.Unmap() && address.String() == value && familyOf(address) == family &&
		address.IsGlobalUnicast() && !address.IsPrivate() && !address.IsLoopback() &&
		!address.IsLinkLocalUnicast() && !sharedIPv4Prefix.Contains(address)
}

func validHistoryGeneration(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range []byte(value) {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func validAddressFailureReason(value string) bool {
	switch value {
	case "selector-unavailable", "no-valid-response", "confirmation-unavailable", "conflicting-responses":
		return true
	default:
		return false
	}
}

func addressStateParams(nodeID string, item AddressState, receivedAt int64) historydb.UpsertAddressStateParams {
	return historydb.UpsertAddressStateParams{
		EgressID: item.EgressID.String(), NodeID: nodeID, HistoryGeneration: item.HistoryGeneration,
		Family: item.Family, Status: item.Status, Sequence: item.Sequence,
		PublicAddress: item.PublicAddress, LocalInterface: item.LocalInterface, LocalAddress: item.LocalAddress,
		ProxyPath: boolInteger(item.ProxyPath), LikelyNat: boolInteger(item.LikelyNAT), Temporary: boolInteger(item.Temporary),
		FailureReason: item.FailureReason, LastCheckedAt: item.LastCheckedAt.UTC().Unix(),
		LastSucceededAt: unixPointer(item.LastSucceededAt), LastChangedAt: unixPointer(item.LastChangedAt), ReceivedAt: receivedAt,
	}
}

func ingestAddressEvent(ctx context.Context, queries *historydb.Queries, nodeID string, item AddressEvent, receivedAt int64) (bool, error) {
	parameters := addressEventParams(nodeID, item, receivedAt)
	existing, err := queries.GetAddressEventBySequence(ctx, historydb.GetAddressEventBySequenceParams{
		EgressID: item.EgressID.String(), HistoryGeneration: item.HistoryGeneration, Sequence: item.Sequence,
	})
	if err == nil {
		return addressEventMatches(existing, parameters), nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	rows, err := queries.CreateAddressEvent(ctx, parameters)
	if err != nil {
		return false, err
	}
	if rows == 1 {
		return true, nil
	}
	existing, err = queries.GetAddressEvent(ctx, item.ID.String())
	if err != nil {
		return false, err
	}
	return addressEventMatches(existing, parameters), nil
}

func addressEventParams(nodeID string, item AddressEvent, receivedAt int64) historydb.CreateAddressEventParams {
	return historydb.CreateAddressEventParams{
		ID: item.ID.String(), EgressID: item.EgressID.String(), NodeID: nodeID,
		HistoryGeneration: item.HistoryGeneration, Sequence: item.Sequence, Kind: item.Kind, Family: item.Family,
		PreviousAddress: item.PreviousAddress, PublicAddress: item.PublicAddress,
		LocalInterface: item.LocalInterface, LocalAddress: item.LocalAddress,
		ProxyPath: boolInteger(item.ProxyPath), LikelyNat: boolInteger(item.LikelyNAT), Temporary: boolInteger(item.Temporary),
		FailureReason: item.FailureReason, ObservedAt: item.ObservedAt.UTC().Unix(), ReceivedAt: receivedAt,
	}
}

func addressEventMatches(existing historydb.AddressEvent, expected historydb.CreateAddressEventParams) bool {
	return existing.ID == expected.ID && existing.EgressID == expected.EgressID && existing.NodeID == expected.NodeID &&
		existing.HistoryGeneration == expected.HistoryGeneration && existing.Sequence == expected.Sequence &&
		existing.Kind == expected.Kind && existing.Family == expected.Family &&
		reflect.DeepEqual(existing.PreviousAddress, expected.PreviousAddress) && reflect.DeepEqual(existing.PublicAddress, expected.PublicAddress) &&
		reflect.DeepEqual(existing.LocalInterface, expected.LocalInterface) && reflect.DeepEqual(existing.LocalAddress, expected.LocalAddress) &&
		existing.ProxyPath == expected.ProxyPath && existing.LikelyNat == expected.LikelyNat && existing.Temporary == expected.Temporary &&
		reflect.DeepEqual(existing.FailureReason, expected.FailureReason) && existing.ObservedAt == expected.ObservedAt
}

func unixPointer(value *time.Time) *int64 {
	if value == nil {
		return nil
	}
	result := value.UTC().Unix()
	return &result
}

func addressStateFromRecord(record historydb.AddressState) (AddressState, error) {
	egressID, err := uuid.Parse(record.EgressID)
	if err != nil {
		return AddressState{}, err
	}
	return AddressState{
		EgressID: egressID, HistoryGeneration: record.HistoryGeneration, Family: record.Family,
		Status: record.Status, Sequence: record.Sequence, PublicAddress: record.PublicAddress,
		LocalInterface: record.LocalInterface, LocalAddress: record.LocalAddress,
		ProxyPath: record.ProxyPath == 1, LikelyNAT: record.LikelyNat == 1, Temporary: record.Temporary == 1,
		FailureReason: record.FailureReason, LastCheckedAt: time.Unix(record.LastCheckedAt, 0).UTC(),
		LastSucceededAt: timeFromUnixPointer(record.LastSucceededAt), LastChangedAt: timeFromUnixPointer(record.LastChangedAt),
	}, nil
}

func addressEventFromRecord(record historydb.AddressEvent) (AddressEvent, error) {
	id, err := uuid.Parse(record.ID)
	if err != nil {
		return AddressEvent{}, err
	}
	egressID, err := uuid.Parse(record.EgressID)
	if err != nil {
		return AddressEvent{}, err
	}
	return AddressEvent{
		ID: id, EgressID: egressID, HistoryGeneration: record.HistoryGeneration,
		Sequence: record.Sequence, Kind: record.Kind, Family: record.Family,
		PreviousAddress: record.PreviousAddress, PublicAddress: record.PublicAddress,
		LocalInterface: record.LocalInterface, LocalAddress: record.LocalAddress,
		ProxyPath: record.ProxyPath == 1, LikelyNAT: record.LikelyNat == 1, Temporary: record.Temporary == 1,
		FailureReason: record.FailureReason, ObservedAt: time.Unix(record.ObservedAt, 0).UTC(),
	}, nil
}

func addressGapFromRecord(record historydb.HistoryGap) (AddressGap, error) {
	id, err := uuid.Parse(record.ID)
	if err != nil {
		return AddressGap{}, err
	}
	egressID, err := uuid.Parse(record.EgressID)
	if err != nil {
		return AddressGap{}, err
	}
	return AddressGap{
		ID: id, EgressID: egressID, HistoryGeneration: record.HistoryGeneration,
		DroppedCount: record.DroppedCount, FirstSequence: record.FirstSequence, LastSequence: record.LastSequence,
		FirstObservedAt: time.Unix(record.FirstObservedAt, 0).UTC(), LastObservedAt: time.Unix(record.LastObservedAt, 0).UTC(),
	}, nil
}

func timeFromUnixPointer(value *int64) *time.Time {
	if value == nil {
		return nil
	}
	result := time.Unix(*value, 0).UTC()
	return &result
}
