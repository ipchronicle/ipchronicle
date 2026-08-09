package notifications

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/center/database/configdb"
	"github.com/mattn/go-sqlite3"
	"golang.org/x/net/http/httpguts"
)

func (s *Service) CreateSender(ctx context.Context, input SenderCreate) (Sender, error) {
	input.Name = strings.TrimSpace(input.Name)
	if err := validateSender(input.Name, input.Kind, input.Configuration); err != nil {
		return Sender{}, err
	}
	id := uuid.New()
	encrypted, err := s.encryptConfiguration(id.String(), input.Configuration)
	if err != nil {
		return Sender{}, err
	}
	now := s.now().UTC().Unix()
	err = s.configQueries.CreateNotificationSender(ctx, configdb.CreateNotificationSenderParams{
		ID: id.String(), Name: input.Name, Kind: input.Kind, Enabled: boolInt(input.Enabled),
		ConfigurationEncrypted: encrypted, CreatedAt: now, UpdatedAt: now,
	})
	if isUniqueConstraint(err) {
		return Sender{}, ErrSenderNameInUse
	}
	if err != nil {
		return Sender{}, err
	}
	return s.Sender(ctx, id)
}

func (s *Service) Sender(ctx context.Context, id uuid.UUID) (Sender, error) {
	record, err := s.configQueries.GetNotificationSender(ctx, id.String())
	if errors.Is(err, sql.ErrNoRows) {
		return Sender{}, ErrSenderNotFound
	}
	if err != nil {
		return Sender{}, err
	}
	return s.senderFromRecord(record)
}

func (s *Service) Senders(ctx context.Context) ([]Sender, error) {
	records, err := s.configQueries.ListNotificationSenders(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]Sender, 0, len(records))
	for _, record := range records {
		sender, err := s.senderFromRecord(record)
		if err != nil {
			return nil, err
		}
		result = append(result, sender)
	}
	return result, nil
}

func (s *Service) UpdateSender(ctx context.Context, id uuid.UUID, input SenderUpdate) (Sender, error) {
	current, err := s.Sender(ctx, id)
	if err != nil {
		return Sender{}, err
	}
	input.Name = strings.TrimSpace(input.Name)
	configuration, err := mergeSenderConfiguration(current, input)
	if err != nil {
		return Sender{}, err
	}
	if err := validateSender(input.Name, current.Kind, configuration); err != nil {
		return Sender{}, err
	}
	encrypted, err := s.encryptConfiguration(id.String(), configuration)
	if err != nil {
		return Sender{}, err
	}
	now := s.now().UTC().Unix()
	changed, err := s.configQueries.UpdateNotificationSender(ctx, configdb.UpdateNotificationSenderParams{
		Name: input.Name, Enabled: boolInt(input.Enabled), ConfigurationEncrypted: encrypted,
		UpdatedAt: now, ID: id.String(), Kind: current.Kind,
	})
	if isUniqueConstraint(err) {
		return Sender{}, ErrSenderNameInUse
	}
	if err != nil {
		return Sender{}, err
	}
	if changed != 1 {
		return Sender{}, ErrSenderNotFound
	}
	s.wakeWorkers()
	return s.Sender(ctx, id)
}

func (s *Service) DeleteSender(ctx context.Context, id uuid.UUID) error {
	s.processMu.Lock()
	defer s.processMu.Unlock()
	if _, err := s.Sender(ctx, id); err != nil {
		return err
	}
	active, err := s.historyQueries.CountActiveNotificationDeliveriesForSender(ctx, id.String())
	if err != nil {
		return err
	}
	if active > 0 {
		return ErrSenderHasActiveWork
	}
	changed, err := s.configQueries.DeleteNotificationSender(ctx, id.String())
	if isConstraint(err) {
		return ErrSenderInUse
	}
	if err != nil {
		return err
	}
	if changed != 1 {
		return ErrSenderInUse
	}
	return nil
}

func (s *Service) CreateRule(ctx context.Context, input RuleCreate) (Rule, error) {
	input.Name = strings.TrimSpace(input.Name)
	if err := s.validateRule(ctx, input); err != nil {
		return Rule{}, err
	}
	id := uuid.New()
	now := s.now().UTC().Unix()
	err := s.configQueries.CreateNotificationRule(ctx, configdb.CreateNotificationRuleParams{
		ID: id.String(), Name: input.Name, Enabled: boolInt(input.Enabled), SenderID: input.SenderID.String(),
		EventType: input.EventType, FieldID: trimOptional(input.FieldID), NodeID: uuidString(input.NodeID),
		EgressID: uuidString(input.EgressID), CreatedAt: now, UpdatedAt: now,
	})
	if isUniqueConstraint(err) {
		return Rule{}, ErrRuleNameInUse
	}
	if err != nil {
		return Rule{}, err
	}
	return s.Rule(ctx, id)
}

func (s *Service) Rule(ctx context.Context, id uuid.UUID) (Rule, error) {
	record, err := s.configQueries.GetNotificationRule(ctx, id.String())
	if errors.Is(err, sql.ErrNoRows) {
		return Rule{}, ErrRuleNotFound
	}
	if err != nil {
		return Rule{}, err
	}
	return ruleFromRecord(record)
}

func (s *Service) Rules(ctx context.Context) ([]Rule, error) {
	records, err := s.configQueries.ListNotificationRules(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]Rule, 0, len(records))
	for _, record := range records {
		rule, err := ruleFromRecord(record)
		if err != nil {
			return nil, err
		}
		result = append(result, rule)
	}
	return result, nil
}

func (s *Service) UpdateRule(ctx context.Context, id uuid.UUID, input RuleCreate) (Rule, error) {
	if _, err := s.Rule(ctx, id); err != nil {
		return Rule{}, err
	}
	input.Name = strings.TrimSpace(input.Name)
	if err := s.validateRule(ctx, input); err != nil {
		return Rule{}, err
	}
	changed, err := s.configQueries.UpdateNotificationRule(ctx, configdb.UpdateNotificationRuleParams{
		Name: input.Name, Enabled: boolInt(input.Enabled), SenderID: input.SenderID.String(),
		EventType: input.EventType, FieldID: trimOptional(input.FieldID), NodeID: uuidString(input.NodeID),
		EgressID: uuidString(input.EgressID), UpdatedAt: s.now().UTC().Unix(), ID: id.String(),
	})
	if isUniqueConstraint(err) {
		return Rule{}, ErrRuleNameInUse
	}
	if err != nil {
		return Rule{}, err
	}
	if changed != 1 {
		return Rule{}, ErrRuleNotFound
	}
	return s.Rule(ctx, id)
}

func (s *Service) DeleteRule(ctx context.Context, id uuid.UUID) error {
	changed, err := s.configQueries.DeleteNotificationRule(ctx, id.String())
	if err != nil {
		return err
	}
	if changed != 1 {
		return ErrRuleNotFound
	}
	return nil
}

func (s *Service) validateRule(ctx context.Context, input RuleCreate) error {
	if input.Name == "" || len(input.Name) > 128 || input.SenderID == uuid.Nil ||
		!validEventType(input.EventType, false) {
		return ErrInvalidRule
	}
	if input.FieldID != nil {
		value := strings.TrimSpace(*input.FieldID)
		if input.EventType != EventProbeFieldChange || value == "" || len(value) > 256 {
			return ErrInvalidRule
		}
	}
	if _, err := s.configQueries.GetNotificationSender(ctx, input.SenderID.String()); errors.Is(err, sql.ErrNoRows) {
		return ErrInvalidRule
	} else if err != nil {
		return err
	}
	if input.NodeID != nil {
		if _, err := s.configQueries.GetNodeByID(ctx, input.NodeID.String()); errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidRule
		} else if err != nil {
			return err
		}
	}
	if input.EgressID != nil {
		egress, err := s.configQueries.GetNodeEgress(ctx, configdb.GetNodeEgressParams{
			NodeID: ownerNodeID(input.NodeID), ID: input.EgressID.String(),
		})
		if input.NodeID == nil {
			egress, err = s.getEgressByID(ctx, input.EgressID.String())
		}
		if errors.Is(err, sql.ErrNoRows) || err == nil && input.NodeID != nil && egress.NodeID != input.NodeID.String() {
			return ErrInvalidRule
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) getEgressByID(ctx context.Context, egressID string) (configdb.NetworkEgress, error) {
	return s.configQueries.GetNetworkEgressByID(ctx, egressID)
}

func (s *Service) encryptConfiguration(id string, configuration SenderConfiguration) ([]byte, error) {
	plaintext, err := json.Marshal(configuration)
	if err != nil {
		return nil, err
	}
	return encryptSenderConfiguration(s.masterKey, id, plaintext)
}

func (s *Service) senderFromRecord(record configdb.NotificationSender) (Sender, error) {
	id, err := uuid.Parse(record.ID)
	if err != nil {
		return Sender{}, err
	}
	plaintext, err := decryptSenderConfiguration(s.masterKey, record.ID, record.ConfigurationEncrypted)
	if err != nil {
		return Sender{}, err
	}
	var configuration SenderConfiguration
	if err := json.Unmarshal(plaintext, &configuration); err != nil {
		return Sender{}, errors.New("decode notification sender configuration")
	}
	if err := validateSender(record.Name, record.Kind, configuration); err != nil {
		return Sender{}, errors.New("stored notification sender configuration is invalid")
	}
	return Sender{
		ID: id, Name: record.Name, Kind: record.Kind, Enabled: record.Enabled == 1,
		Configuration: configuration, CreatedAt: unixTime(record.CreatedAt), UpdatedAt: unixTime(record.UpdatedAt),
	}, nil
}

func ruleFromRecord(record configdb.NotificationRule) (Rule, error) {
	id, err := uuid.Parse(record.ID)
	if err != nil {
		return Rule{}, err
	}
	senderID, err := uuid.Parse(record.SenderID)
	if err != nil {
		return Rule{}, err
	}
	nodeID, err := parseOptionalUUID(record.NodeID)
	if err != nil {
		return Rule{}, err
	}
	egressID, err := parseOptionalUUID(record.EgressID)
	if err != nil {
		return Rule{}, err
	}
	return Rule{
		ID: id, Name: record.Name, Enabled: record.Enabled == 1, SenderID: senderID,
		EventType: record.EventType, FieldID: record.FieldID, NodeID: nodeID, EgressID: egressID,
		CreatedAt: unixTime(record.CreatedAt), UpdatedAt: unixTime(record.UpdatedAt),
	}, nil
}

func mergeSenderConfiguration(current Sender, input SenderUpdate) (SenderConfiguration, error) {
	switch current.Kind {
	case SenderTelegram:
		if input.Telegram == nil || input.Webhook != nil || input.JavaScript != nil {
			return SenderConfiguration{}, ErrInvalidSender
		}
		configuration := *current.Configuration.Telegram
		configuration.ChatID = input.Telegram.ChatID
		if input.Telegram.Token != nil {
			configuration.Token = *input.Telegram.Token
		}
		return SenderConfiguration{Telegram: &configuration}, nil
	case SenderWebhook:
		if input.Webhook == nil || input.Telegram != nil || input.JavaScript != nil {
			return SenderConfiguration{}, ErrInvalidSender
		}
		configuration := *current.Configuration.Webhook
		configuration.URL = input.Webhook.URL
		if input.Webhook.Headers != nil {
			configuration.Headers = cloneMap(*input.Webhook.Headers)
		}
		return SenderConfiguration{Webhook: &configuration}, nil
	case SenderJavaScript:
		if input.JavaScript == nil || input.Telegram != nil || input.Webhook != nil {
			return SenderConfiguration{}, ErrInvalidSender
		}
		configuration := *input.JavaScript
		return SenderConfiguration{JavaScript: &configuration}, nil
	default:
		return SenderConfiguration{}, ErrInvalidSender
	}
}

func validateSender(name, kind string, configuration SenderConfiguration) error {
	if name == "" || len(name) > 128 {
		return ErrInvalidSender
	}
	switch kind {
	case SenderTelegram:
		if configuration.Telegram == nil || configuration.Webhook != nil || configuration.JavaScript != nil ||
			strings.TrimSpace(configuration.Telegram.ChatID) == "" || len(configuration.Telegram.ChatID) > 128 ||
			strings.TrimSpace(configuration.Telegram.Token) == "" || len(configuration.Telegram.Token) > 512 {
			return ErrInvalidSender
		}
	case SenderWebhook:
		if configuration.Webhook == nil || configuration.Telegram != nil || configuration.JavaScript != nil ||
			!validWebhook(configuration.Webhook) {
			return ErrInvalidSender
		}
	case SenderJavaScript:
		if configuration.JavaScript == nil || configuration.Telegram != nil || configuration.Webhook != nil ||
			configuration.JavaScript.Source == "" || len(configuration.JavaScript.Source) > 256*1024 ||
			!utf8.ValidString(configuration.JavaScript.Source) {
			return ErrInvalidSender
		}
	default:
		return ErrInvalidSender
	}
	return nil
}

func validWebhook(configuration *WebhookConfiguration) bool {
	parsed, err := url.Parse(configuration.URL)
	if err != nil || len(configuration.URL) > 4096 || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") || len(configuration.Headers) > 32 {
		return false
	}
	for name, value := range configuration.Headers {
		canonical := strings.ToLower(name)
		if !httpguts.ValidHeaderFieldName(name) || !httpguts.ValidHeaderFieldValue(value) || len(name)+len(value) > 8192 ||
			canonical == "host" || canonical == "content-length" || canonical == "connection" || canonical == "transfer-encoding" {
			return false
		}
	}
	return true
}

func HeaderNames(configuration WebhookConfiguration) []string {
	result := make([]string, 0, len(configuration.Headers))
	for name := range configuration.Headers {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func trimOptional(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	return &trimmed
}

func parseOptionalUUID(value *string) (*uuid.UUID, error) {
	if value == nil {
		return nil, nil
	}
	parsed, err := uuid.Parse(*value)
	return &parsed, err
}

func uuidString(value *uuid.UUID) *string {
	if value == nil {
		return nil
	}
	result := value.String()
	return &result
}

func ownerNodeID(value *uuid.UUID) string {
	if value == nil {
		return ""
	}
	return value.String()
}

func boolInt(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

func unixTime(value int64) time.Time {
	return time.Unix(value, 0).UTC()
}

func cloneMap(input map[string]string) map[string]string {
	result := make(map[string]string, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}

func isConstraint(err error) bool {
	var sqliteError sqlite3.Error
	return errors.As(err, &sqliteError) && sqliteError.Code == sqlite3.ErrConstraint
}

func isUniqueConstraint(err error) bool {
	var sqliteError sqlite3.Error
	return errors.As(err, &sqliteError) && sqliteError.ExtendedCode == sqlite3.ErrConstraintUnique
}
