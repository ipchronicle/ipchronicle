package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/center/database/historydb"
)

func CreateEvent(ctx context.Context, queries *historydb.Queries, input EventInput) error {
	if queries == nil || !validEventType(input.Type, false) || !validSourceKind(input.SourceKind) ||
		input.SourceID == "" || len(input.SourceID) > 64 || input.NodeID == nil || input.EgressID == nil ||
		input.ObservedAt <= 0 || input.RecordedAt <= 0 {
		return errors.New("invalid durable notification event")
	}
	payload, err := json.Marshal(input.Payload)
	if err != nil {
		return fmt.Errorf("encode durable notification event: %w", err)
	}
	if len(payload) < 2 || len(payload) > 1024*1024 {
		return errors.New("durable notification event payload exceeds its boundary")
	}
	_, err = queries.CreateNotificationEvent(ctx, historydb.CreateNotificationEventParams{
		ID:        stableID("notification-event", input.SourceKind+":"+input.SourceID+":"+input.Type),
		EventType: input.Type, SourceKind: input.SourceKind, SourceID: input.SourceID,
		NodeID: input.NodeID, EgressID: input.EgressID, PayloadJson: payload,
		ObservedAt: input.ObservedAt, RecordedAt: input.RecordedAt,
	})
	return err
}

func stableID(kind, source string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(kind+":"+source)).String()
}

func validEventType(value string, allowTest bool) bool {
	switch value {
	case EventProbeFieldChange, EventAddressChange, EventAddressCheckFailure,
		EventAddressCheckRecover, EventProbeFailure, EventProbeRecovery,
		EventAddressGap, EventProbeGap, EventFormatMismatch, EventFormatChanged,
		EventFormatRecovery:
		return true
	case EventTest:
		return allowTest
	default:
		return false
	}
}

func validSourceKind(value string) bool {
	switch value {
	case "probe-change-set", "address-event", "probe-execution", "address-gap", "probe-gap", "format-event":
		return true
	default:
		return false
	}
}
