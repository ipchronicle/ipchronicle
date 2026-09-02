package notifications

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

const (
	EventAll                 = "all"
	EventProbeFieldChange    = "probe-field-change"
	EventAddressChange       = "address-change"
	EventAddressCheckFailure = "address-check-failure"
	EventAddressCheckRecover = "address-check-recovery"
	EventProbeFailure        = "probe-failure"
	EventProbeRecovery       = "probe-recovery"
	EventAddressGap          = "address-gap"
	EventProbeGap            = "probe-gap"
	EventFormatMismatch      = "format-mismatch"
	EventFormatChanged       = "format-changed"
	EventFormatRecovery      = "format-recovery"
	EventTest                = "test"

	SenderTelegram   = "telegram"
	SenderWebhook    = "webhook"
	SenderJavaScript = "javascript"
)

const (
	maximumActiveDeliveriesPerSender = 1024
	maximumAttempts                  = 4
)

var (
	ErrSenderNotFound       = errors.New("notification sender does not exist")
	ErrSenderNameInUse      = errors.New("notification sender name is already in use")
	ErrSenderInUse          = errors.New("notification sender is referenced by a rule")
	ErrSenderHasActiveWork  = errors.New("notification sender has active deliveries")
	ErrInvalidSender        = errors.New("notification sender is invalid")
	ErrSenderTestFailed     = errors.New("notification sender test failed")
	ErrRuleNotFound         = errors.New("notification rule does not exist")
	ErrRuleNameInUse        = errors.New("notification rule name is already in use")
	ErrInvalidRule          = errors.New("notification rule is invalid")
	ErrDeliveryNotFound     = errors.New("notification delivery does not exist")
	ErrInvalidDeliveryQuery = errors.New("notification delivery query is invalid")
)

type SenderTestFailure struct {
	Code string
}

func (failure SenderTestFailure) Error() string {
	return ErrSenderTestFailed.Error() + ": " + failure.Code
}

func (failure SenderTestFailure) Unwrap() error {
	return ErrSenderTestFailed
}

type TelegramConfiguration struct {
	ChatID  string `json:"chatId"`
	Token   string `json:"token"`
	TopicID *int64 `json:"topicId,omitempty"`
}

type WebhookConfiguration struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

type JavaScriptConfiguration struct {
	Source string `json:"source"`
}

type SenderConfiguration struct {
	Telegram   *TelegramConfiguration   `json:"telegram,omitempty"`
	Webhook    *WebhookConfiguration    `json:"webhook,omitempty"`
	JavaScript *JavaScriptConfiguration `json:"javascript,omitempty"`
}

type Sender struct {
	ID            uuid.UUID
	Name          string
	Kind          string
	Enabled       bool
	Configuration SenderConfiguration
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type SenderCreate struct {
	Name          string
	Kind          string
	Enabled       bool
	Configuration SenderConfiguration
}

type TelegramUpdate struct {
	ChatID  string
	Token   *string
	TopicID *int64
}

type WebhookUpdate struct {
	URL     string
	Headers *map[string]string
}

type SenderUpdate struct {
	Name       string
	Enabled    bool
	Telegram   *TelegramUpdate
	Webhook    *WebhookUpdate
	JavaScript *JavaScriptConfiguration
}

type Rule struct {
	ID            uuid.UUID
	Name          string
	Enabled       bool
	SenderID      uuid.UUID
	EventType     string
	FieldID       *string
	NodeID        *uuid.UUID
	EgressID      *uuid.UUID
	PublicAddress *string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type RuleCreate struct {
	Name      string
	Enabled   bool
	SenderID  uuid.UUID
	EventType string
	FieldID   *string
	NodeID    *uuid.UUID
	EgressID  *uuid.UUID
}

type Delivery struct {
	ID             uuid.UUID
	EventID        uuid.UUID
	SenderID       uuid.UUID
	SenderName     string
	SenderKind     string
	EventType      string
	NodeID         *uuid.UUID
	EgressID       *uuid.UUID
	Test           bool
	Status         string
	AttemptCount   int64
	NextAttemptAt  *time.Time
	LastAttemptAt  *time.Time
	CompletedAt    *time.Time
	ErrorCode      *string
	MatchedRuleIDs []uuid.UUID
	Event          json.RawMessage
	Title          string
	Body           string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type DeliveryPage struct {
	Items      []Delivery
	Page       int
	PageSize   int
	TotalItems int64
	TotalPages int
}

type DeliveryFilter struct {
	SenderID *uuid.UUID
	Status   string
	Page     int
	PageSize int
}

type EventInput struct {
	Type       string
	SourceKind string
	SourceID   string
	NodeID     *string
	EgressID   *string
	Payload    any
	ObservedAt int64
	RecordedAt int64
}

type FieldChange struct {
	FieldID   string `json:"fieldId"`
	Group     string `json:"group"`
	Path      string `json:"path"`
	ValueType string `json:"valueType"`
	Before    string `json:"before"`
	After     string `json:"after"`
}

type ProbeChangeData struct {
	ExecutionID        string        `json:"executionId"`
	SnapshotID         string        `json:"snapshotId"`
	PreviousSnapshotID *string       `json:"previousSnapshotId,omitempty"`
	Sequence           int64         `json:"sequence"`
	Changes            []FieldChange `json:"changes"`
}

type ProbeOutcomeData struct {
	ExecutionID  string  `json:"executionId"`
	Sequence     int64   `json:"sequence"`
	Status       string  `json:"status"`
	FailureStage *string `json:"failureStage,omitempty"`
}

type FormatData struct {
	ExecutionID string `json:"executionId"`
	SnapshotID  string `json:"snapshotId"`
	Sequence    int64  `json:"sequence"`
	Kind        string `json:"kind"`
	IssueCount  int64  `json:"issueCount"`
}

type AddressData struct {
	Sequence       int64   `json:"sequence"`
	Kind           string  `json:"kind"`
	Family         string  `json:"family"`
	PublicAddress  *string `json:"publicAddress,omitempty"`
	LocalInterface *string `json:"localInterface,omitempty"`
	LocalAddress   *string `json:"localAddress,omitempty"`
	ProxyPath      bool    `json:"proxyPath"`
	LikelyNAT      bool    `json:"likelyNat"`
	Temporary      bool    `json:"temporary"`
	FailureReason  *string `json:"failureReason,omitempty"`
}

type GapData struct {
	Kind            string `json:"kind"`
	DroppedCount    int64  `json:"droppedCount"`
	FirstSequence   int64  `json:"firstSequence"`
	LastSequence    int64  `json:"lastSequence"`
	FirstObservedAt int64  `json:"firstObservedAt"`
	LastObservedAt  int64  `json:"lastObservedAt"`
}
