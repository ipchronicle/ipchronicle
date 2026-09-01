package nodes

import (
	"context"
	"database/sql"
	"errors"
	"net/netip"
	"reflect"
	"time"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/center/database/configdb"
	"github.com/ipchronicle/ipchronicle/internal/center/database/historydb"
	"github.com/ipchronicle/ipchronicle/internal/center/notifications"
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
	nodeName := ""
	if len(egresses) > 0 && (len(upload.States) > 0 || len(upload.Events) > 0 || len(upload.Gaps) > 0) {
		node, err := s.queries.GetNodeByID(ctx, nodeID)
		if err != nil {
			return receipt, err
		}
		nodeName = node.Name
	}
	upload, err = normalizeAddressUpload(upload)
	if err != nil {
		return receipt, ErrInvalidMetadata
	}
	if err := validateAddressUpload(upload, egresses); err != nil {
		return receipt, ErrInvalidMetadata
	}
	if upload.States != nil {
		if err := s.reconcilePublicAddresses(ctx, nodeID, upload.States, systemState.HistoryGeneration, receivedAt); err != nil {
			return receipt, err
		}
	}
	transaction, err := s.history.BeginTx(ctx, nil)
	if err != nil {
		return receipt, err
	}
	defer transaction.Rollback()
	queries := s.historyQueries.WithTx(transaction)
	if nodeName != "" {
		if err := recordHistoryNode(ctx, queries, nodeID, nodeName, receivedAt); err != nil {
			return receipt, err
		}
	}
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
		publicAddressID, err := s.publicAddressIDForEvent(ctx, item)
		if err != nil {
			return receipt, err
		}
		if publicAddressID == "" {
			receipt.AcceptedEventIDs = append(receipt.AcceptedEventIDs, item.ID)
			continue
		}
		accepted, err := ingestAddressEvent(ctx, queries, nodeID, publicAddressID, item, receivedAt)
		if err != nil {
			return receipt, err
		}
		if !accepted {
			return receipt, ErrInvalidMetadata
		}
		if err := s.createAddressNotificationEvent(ctx, queries, nodeID, publicAddressID, item, receivedAt); err != nil {
			return receipt, err
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
		if err := notifications.CreateEvent(ctx, queries, notifications.EventInput{
			Type: notifications.EventAddressGap, SourceKind: "address-gap", SourceID: item.ID.String(),
			NodeID: &nodeID,
			Payload: notifications.GapData{
				Kind: "address", DroppedCount: item.DroppedCount,
				FirstSequence: item.FirstSequence, LastSequence: item.LastSequence,
				FirstObservedAt: item.FirstObservedAt.UTC().Unix(), LastObservedAt: item.LastObservedAt.UTC().Unix(),
			},
			ObservedAt: item.LastObservedAt.UTC().Unix(), RecordedAt: receivedAt,
		}); err != nil {
			return receipt, err
		}
		receipt.AcceptedGaps = append(receipt.AcceptedGaps, acknowledgement)
	}
	if err := transaction.Commit(); err != nil {
		return receipt, err
	}
	return receipt, nil
}

func (s *Service) reconcilePublicAddresses(
	ctx context.Context,
	nodeID string,
	states []AddressState,
	historyGeneration string,
	receivedAt int64,
) error {
	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	queries := s.queries.WithTx(transaction)
	selectedBefore, err := selectedPublicAddressPaths(ctx, queries)
	if err != nil {
		return err
	}
	priorPaths, err := queries.ListNodePublicAddressPaths(ctx, nodeID)
	if err != nil {
		return err
	}
	priorAddresses := make(map[string]struct{}, len(priorPaths))
	for _, path := range priorPaths {
		if path.Available == 1 {
			priorAddresses[path.PublicAddressID] = struct{}{}
		}
	}
	hadConfirmedBaseline := len(priorAddresses) > 0
	newAddressCandidates := make(map[string]struct{})
	if err := queries.MarkNodePublicAddressPathsUnavailable(ctx, nodeID); err != nil {
		return err
	}
	for _, state := range states {
		if state.HistoryGeneration != historyGeneration || state.PublicAddress == nil {
			continue
		}
		parsed, err := netip.ParseAddr(*state.PublicAddress)
		if err != nil {
			return ErrInvalidMetadata
		}
		address := parsed.Unmap().String()
		observedAt := state.LastSucceededAt.UTC().Unix()
		updatedAt := max(receivedAt, state.LastCheckedAt.UTC().Unix())
		addressID := uuid.NewString()
		if err := queries.UpsertPublicAddress(ctx, configdb.UpsertPublicAddressParams{
			ID: addressID, Address: address, Family: state.Family,
			FirstSeenAt: observedAt, LastSeenAt: observedAt, UpdatedAt: updatedAt,
		}); err != nil {
			return err
		}
		publicAddress, err := queries.GetPublicAddressByAddress(ctx, address)
		if err != nil {
			return err
		}
		if err := queries.UpsertPublicAddressNode(ctx, configdb.UpsertPublicAddressNodeParams{
			PublicAddressID: publicAddress.ID, NodeID: nodeID,
			FirstSeenAt: observedAt, LastSeenAt: observedAt,
		}); err != nil {
			return err
		}
		var lastSucceededAt *int64
		if state.LastSucceededAt != nil {
			value := state.LastSucceededAt.UTC().Unix()
			lastSucceededAt = &value
		}
		if err := queries.UpsertPublicAddressPath(ctx, configdb.UpsertPublicAddressPathParams{
			PublicAddressID: publicAddress.ID, PathID: state.EgressID.String(), NodeID: nodeID,
			LocalInterface: state.LocalInterface, LocalAddress: state.LocalAddress,
			ProxyPath: boolInteger(state.ProxyPath), LikelyNat: boolInteger(state.LikelyNAT),
			Temporary: boolInteger(state.Temporary), Available: 1,
			LastCheckedAt: state.LastCheckedAt.UTC().Unix(), LastSucceededAt: lastSucceededAt,
		}); err != nil {
			return err
		}
		_, alreadyCurrent := priorAddresses[publicAddress.ID]
		if hadConfirmedBaseline && !alreadyCurrent {
			newAddressCandidates[publicAddress.ID] = struct{}{}
		}
	}
	cleared, err := queries.ClearUnavailablePublicAddressSelections(ctx)
	if err != nil {
		return err
	}
	_ = cleared
	addresses, err := queries.ListPublicAddressesWithoutSelectedPath(ctx)
	if err != nil {
		return err
	}
	for _, address := range addresses {
		path, err := queries.GetPreferredPublicAddressPath(ctx, address.ID)
		if err != nil {
			return err
		}
		pathID := path.PathID
		changed, err := queries.SelectPublicAddressPath(ctx, configdb.SelectPublicAddressPathParams{
			SelectedPathID: &pathID, ID: address.ID, SelectedPathID_2: &pathID,
		})
		if err != nil {
			return err
		}
		_ = changed
	}
	selectedAfter, err := selectedPublicAddressPaths(ctx, queries)
	if err != nil {
		return err
	}
	affectedNodes := changedSelectionNodes(selectedBefore, selectedAfter)
	for affectedNodeID := range affectedNodes {
		if err := incrementNodeConfiguration(ctx, queries, affectedNodeID); err != nil {
			return err
		}
	}
	cleanupNodes := make(map[string]struct{}, len(affectedNodes)+1)
	cleanupNodes[nodeID] = struct{}{}
	for affectedNodeID := range affectedNodes {
		cleanupNodes[affectedNodeID] = struct{}{}
	}
	for cleanupNodeID := range cleanupNodes {
		if err := queries.DeleteUnavailablePendingPublicAddressProbes(ctx, cleanupNodeID); err != nil {
			return err
		}
	}
	node, err := queries.GetNodeProbeSettings(ctx, nodeID)
	if err != nil {
		return err
	}
	if node.ProbeOnNewAddress == 1 {
		for publicAddressID := range newAddressCandidates {
			address, err := queries.GetPublicAddressByID(ctx, publicAddressID)
			if err != nil {
				return err
			}
			if address.ProbeEnabled != 1 || address.SelectedPathID == nil {
				continue
			}
			path, err := queries.GetPublicAddressPathByPathID(ctx, *address.SelectedPathID)
			if err != nil {
				return err
			}
			if path.NodeID != nodeID || path.PublicAddressID != publicAddressID || path.Available != 1 {
				continue
			}
			if err := queries.UpsertPendingPublicAddressProbe(ctx, configdb.UpsertPendingPublicAddressProbeParams{
				PublicAddressID: publicAddressID, NodeID: nodeID,
				RequiredConfigurationRevision: node.DesiredConfigurationRevision,
				CreatedAt:                     receivedAt,
			}); err != nil {
				return err
			}
		}
	}
	if err := transaction.Commit(); err != nil {
		return err
	}
	for affectedNodeID := range affectedNodes {
		s.sync.Wake(affectedNodeID)
	}
	return nil
}

type selectedPublicAddressPath struct {
	pathID string
	nodeID string
}

func selectedPublicAddressPaths(ctx context.Context, queries *configdb.Queries) (map[string]selectedPublicAddressPath, error) {
	addresses, err := queries.ListPublicAddresses(ctx)
	if err != nil {
		return nil, err
	}
	selected := make(map[string]selectedPublicAddressPath, len(addresses))
	for _, address := range addresses {
		if address.SelectedPathID == nil {
			continue
		}
		path, err := queries.GetPublicAddressPathByPathID(ctx, *address.SelectedPathID)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		selected[address.ID] = selectedPublicAddressPath{pathID: path.PathID, nodeID: path.NodeID}
	}
	return selected, nil
}

func changedSelectionNodes(before, after map[string]selectedPublicAddressPath) map[string]struct{} {
	affected := make(map[string]struct{})
	addressIDs := make(map[string]struct{}, len(before)+len(after))
	for addressID := range before {
		addressIDs[addressID] = struct{}{}
	}
	for addressID := range after {
		addressIDs[addressID] = struct{}{}
	}
	for addressID := range addressIDs {
		oldPath, hadOldPath := before[addressID]
		newPath, hasNewPath := after[addressID]
		if hadOldPath == hasNewPath && oldPath.pathID == newPath.pathID {
			continue
		}
		if hadOldPath {
			affected[oldPath.nodeID] = struct{}{}
		}
		if hasNewPath {
			affected[newPath.nodeID] = struct{}{}
		}
	}
	return affected
}

func (s *Service) createAddressNotificationEvent(
	ctx context.Context,
	queries *historydb.Queries,
	nodeID string,
	publicAddressID string,
	item AddressEvent,
	recordedAt int64,
) error {
	eventType := ""
	switch item.Kind {
	case "address-added", "address-removed":
		eventType = notifications.EventAddressChange
	case "check-failure":
		eventType = notifications.EventAddressCheckFailure
	case "recovery":
		eventType = notifications.EventAddressCheckRecover
	default:
		return nil
	}
	return notifications.CreateEvent(ctx, queries, notifications.EventInput{
		Type: eventType, SourceKind: "address-event", SourceID: item.ID.String(),
		NodeID: &nodeID, EgressID: &publicAddressID,
		Payload: notifications.AddressData{
			Sequence: item.Sequence, Kind: item.Kind, Family: item.Family,
			PublicAddress:  item.PublicAddress,
			LocalInterface: item.LocalInterface, LocalAddress: item.LocalAddress,
			ProxyPath: item.ProxyPath, LikelyNAT: item.LikelyNAT,
			Temporary: item.Temporary, FailureReason: item.FailureReason,
		},
		ObservedAt: item.ObservedAt.UTC().Unix(), RecordedAt: recordedAt,
	})
}

func (s *Service) publicAddressIDForEvent(ctx context.Context, item AddressEvent) (string, error) {
	if item.PublicAddress != nil {
		address, err := s.queries.GetPublicAddressByAddress(ctx, *item.PublicAddress)
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		if err != nil {
			return "", err
		}
		return address.ID, nil
	}
	path, err := s.queries.GetPublicAddressPathByPathID(ctx, item.EgressID.String())
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return path.PublicAddressID, nil
}

func normalizeAddressUpload(upload AddressUpload) (AddressUpload, error) {
	if upload.States != nil {
		states := make([]AddressState, len(upload.States))
		copy(states, upload.States)
		upload.States = states
	}
	if upload.Events != nil {
		events := make([]AddressEvent, len(upload.Events))
		copy(events, upload.Events)
		upload.Events = events
	}
	for index := range upload.States {
		value, err := canonicalPublicAddress(upload.States[index].PublicAddress, upload.States[index].Family)
		if err != nil {
			return AddressUpload{}, err
		}
		upload.States[index].PublicAddress = value
	}
	for index := range upload.Events {
		value, err := canonicalPublicAddress(upload.Events[index].PublicAddress, upload.Events[index].Family)
		if err != nil {
			return AddressUpload{}, err
		}
		upload.Events[index].PublicAddress = value
	}
	return upload, nil
}

func canonicalPublicAddress(value *string, family string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	parsed, err := netip.ParseAddr(*value)
	if err != nil {
		return nil, err
	}
	parsed = parsed.Unmap()
	if familyOf(parsed) != family {
		return nil, ErrInvalidMetadata
	}
	canonical := parsed.String()
	return &canonical, nil
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
	if item.LastSucceededAt != nil && item.LastSucceededAt.After(item.LastCheckedAt) {
		return ErrInvalidMetadata
	}
	if item.Status == "confirmed" {
		if item.PublicAddress == nil || item.FailureReason != nil || item.LastSucceededAt == nil || item.LastSucceededAt.After(item.LastCheckedAt) {
			return ErrInvalidMetadata
		}
	} else if item.FailureReason == nil || !validAddressFailureReason(*item.FailureReason) ||
		(item.PublicAddress != nil && item.LastSucceededAt == nil) {
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
		if item.PublicAddress == nil || item.FailureReason != nil {
			return ErrInvalidMetadata
		}
	case "address-added", "address-removed":
		if item.PublicAddress == nil || item.FailureReason != nil {
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

func ingestAddressEvent(ctx context.Context, queries *historydb.Queries, nodeID, publicAddressID string, item AddressEvent, receivedAt int64) (bool, error) {
	parameters := addressEventParams(nodeID, publicAddressID, item, receivedAt)
	existing, err := queries.GetAddressEventBySequence(ctx, historydb.GetAddressEventBySequenceParams{
		SourcePathID: item.EgressID.String(), HistoryGeneration: item.HistoryGeneration, Sequence: item.Sequence,
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

func addressEventParams(nodeID, publicAddressID string, item AddressEvent, receivedAt int64) historydb.CreateAddressEventParams {
	return historydb.CreateAddressEventParams{
		ID: item.ID.String(), PublicAddressID: publicAddressID, SourcePathID: item.EgressID.String(), NodeID: nodeID,
		HistoryGeneration: item.HistoryGeneration, Sequence: item.Sequence, Kind: item.Kind, Family: item.Family,
		PublicAddress:  item.PublicAddress,
		LocalInterface: item.LocalInterface, LocalAddress: item.LocalAddress,
		ProxyPath: boolInteger(item.ProxyPath), LikelyNat: boolInteger(item.LikelyNAT), Temporary: boolInteger(item.Temporary),
		FailureReason: item.FailureReason, ObservedAt: item.ObservedAt.UTC().Unix(), ReceivedAt: receivedAt,
	}
}

func addressEventMatches(existing historydb.AddressEvent, expected historydb.CreateAddressEventParams) bool {
	return existing.ID == expected.ID && existing.PublicAddressID == expected.PublicAddressID &&
		existing.SourcePathID == expected.SourcePathID && existing.NodeID == expected.NodeID &&
		existing.HistoryGeneration == expected.HistoryGeneration && existing.Sequence == expected.Sequence &&
		existing.Kind == expected.Kind && existing.Family == expected.Family &&
		reflect.DeepEqual(existing.PublicAddress, expected.PublicAddress) &&
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
	egressID, err := uuid.Parse(record.PublicAddressID)
	if err != nil {
		return AddressEvent{}, err
	}
	return AddressEvent{
		ID: id, EgressID: egressID, HistoryGeneration: record.HistoryGeneration,
		Sequence: record.Sequence, Kind: record.Kind, Family: record.Family,
		PublicAddress:  record.PublicAddress,
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
