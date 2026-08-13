package notifications

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/center/database/configdb"
	"github.com/ipchronicle/ipchronicle/internal/center/database/historydb"
	"github.com/ipchronicle/ipchronicle/internal/center/systemsettings"
)

const (
	eventBatchSize = 64
	fixedWorkers   = 4
	workerPoll     = 500 * time.Millisecond
)

type JavaScriptRunner interface {
	Run(context.Context, JavaScriptRequest) DeliveryError
}

type ServiceOptions struct {
	ConfigDatabase  *sql.DB
	HistoryDatabase *sql.DB
	ConfigQueries   *configdb.Queries
	HistoryQueries  *historydb.Queries
	MasterKey       [32]byte
	SystemSettings  *systemsettings.Service
	Executable      string
	HTTPClient      *http.Client
}

type Service struct {
	configDatabase  *sql.DB
	historyDatabase *sql.DB
	configQueries   *configdb.Queries
	historyQueries  *historydb.Queries
	masterKey       [32]byte
	systemSettings  *systemsettings.Service
	httpClient      *http.Client
	javascript      JavaScriptRunner
	now             func() time.Time
	wake            chan struct{}
	processMu       sync.Mutex
}

func NewService(options ServiceOptions) *Service {
	if options.ConfigDatabase == nil || options.HistoryDatabase == nil || options.ConfigQueries == nil || options.HistoryQueries == nil || options.SystemSettings == nil {
		panic("notification service database dependencies must not be nil")
	}
	executable := options.Executable
	if executable == "" {
		var err error
		executable, err = os.Executable()
		if err != nil {
			panic("notification service executable path is unavailable")
		}
	}
	client := options.HTTPClient
	if client == nil {
		client = &http.Client{
			Timeout: 15 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	service := &Service{
		configDatabase: options.ConfigDatabase, historyDatabase: options.HistoryDatabase,
		configQueries: options.ConfigQueries, historyQueries: options.HistoryQueries,
		masterKey: options.MasterKey, systemSettings: options.SystemSettings, httpClient: client,
		javascript: ProcessJavaScriptRunner{Executable: executable}, now: time.Now,
		wake: make(chan struct{}, 1),
	}
	return service
}

func (s *Service) Run(ctx context.Context, logger *log.Logger) {
	if logger == nil {
		logger = log.Default()
	}
	for {
		now := s.now().UTC().Unix()
		err := s.historyQueries.RecoverRunningNotificationDeliveries(ctx, historydb.RecoverRunningNotificationDeliveriesParams{
			NextAttemptAt: &now, CompletedAt: &now, UpdatedAt: now,
		})
		if err == nil {
			break
		}
		if errors.Is(err, context.Canceled) {
			return
		}
		logger.Printf("notification delivery recovery failed: %v", err)
		timer := time.NewTimer(workerPoll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
	go s.eventLoop(ctx, logger)
	for range fixedWorkers {
		go s.deliveryLoop(ctx, logger, false)
	}
	go s.deliveryLoop(ctx, logger, true)
	<-ctx.Done()
}

func (s *Service) eventLoop(ctx context.Context, logger *log.Logger) {
	ticker := time.NewTicker(workerPoll)
	defer ticker.Stop()
	for {
		if err := s.processPendingEvents(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Printf("notification event processing failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-s.wake:
		}
	}
}

func (s *Service) deliveryLoop(ctx context.Context, logger *log.Logger, javascript bool) {
	ticker := time.NewTicker(workerPoll)
	defer ticker.Stop()
	for {
		worked, err := s.processOneDelivery(ctx, javascript)
		if err != nil && !errors.Is(err, context.Canceled) {
			logger.Printf("notification delivery coordination failed: %v", err)
		}
		if worked {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-s.wake:
		}
	}
}

func (s *Service) wakeWorkers() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Service) processPendingEvents(ctx context.Context) error {
	s.processMu.Lock()
	defer s.processMu.Unlock()
	events, err := s.historyQueries.ListPendingNotificationEvents(ctx, eventBatchSize)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return nil
	}
	rules, err := s.configQueries.ListEnabledNotificationRules(ctx)
	if err != nil {
		return err
	}
	administrator, err := s.configQueries.GetAdministrator(ctx)
	if err != nil {
		return err
	}
	for _, event := range events {
		if err := s.processEvent(ctx, event, rules, administrator.Locale); err != nil {
			return err
		}
	}
	s.wakeWorkers()
	return nil
}

func (s *Service) processEvent(
	ctx context.Context,
	event historydb.NotificationEvent,
	rules []configdb.ListEnabledNotificationRulesRow,
	locale string,
) error {
	matches := make(map[string][]string)
	senders := make(map[string]configdb.ListEnabledNotificationRulesRow)
	for _, rule := range rules {
		if ruleMatches(rule, event) {
			matches[rule.SenderID] = append(matches[rule.SenderID], rule.ID)
			senders[rule.SenderID] = rule
		}
	}
	envelope, title, body, err := s.buildDeliveryContent(ctx, event, locale)
	if err != nil {
		return err
	}
	now := s.now().UTC().Unix()
	transaction, err := s.historyDatabase.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	queries := s.historyQueries.WithTx(transaction)
	current, err := queries.GetNotificationEvent(ctx, event.ID)
	if err != nil {
		return err
	}
	if current.ProcessedAt != nil {
		return transaction.Commit()
	}
	senderIDs := make([]string, 0, len(matches))
	for senderID := range matches {
		senderIDs = append(senderIDs, senderID)
	}
	sort.Strings(senderIDs)
	for _, senderID := range senderIDs {
		ruleIDs := matches[senderID]
		sort.Strings(ruleIDs)
		matchedJSON, err := json.Marshal(ruleIDs)
		if err != nil {
			return err
		}
		status := "pending"
		nextAttemptAt := &now
		var completedAt *int64
		var errorCode *string
		active, err := queries.CountActiveNotificationDeliveriesForSender(ctx, senderID)
		if err != nil {
			return err
		}
		if active >= maximumActiveDeliveriesPerSender {
			status = "failed"
			nextAttemptAt = nil
			completedAt = &now
			code := "queue-full"
			errorCode = &code
		}
		sender := senders[senderID]
		if _, err := queries.CreateNotificationDelivery(ctx, historydb.CreateNotificationDeliveryParams{
			ID: stableID("notification-delivery", event.ID+":"+senderID), EventID: event.ID,
			SenderID: senderID, SenderName: sender.SenderName, SenderKind: sender.SenderKind,
			EventType: event.EventType, NodeID: event.NodeID, EgressID: event.EgressID,
			Status: status, NextAttemptAt: nextAttemptAt, CompletedAt: completedAt, ErrorCode: errorCode,
			MatchedRuleIdsJson: matchedJSON, EventJson: envelope, Title: title, Body: body,
			CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			return err
		}
	}
	changed, err := queries.MarkNotificationEventProcessed(ctx, historydb.MarkNotificationEventProcessedParams{
		ProcessedAt: &now, ID: event.ID,
	})
	if err != nil {
		return err
	}
	if changed != 1 {
		return errors.New("notification event processing lost its durable claim")
	}
	return transaction.Commit()
}

func ruleMatches(rule configdb.ListEnabledNotificationRulesRow, event historydb.NotificationEvent) bool {
	if rule.EventType != event.EventType || rule.NodeID != nil && !sameOptional(rule.NodeID, event.NodeID) ||
		rule.EgressID != nil && !sameOptional(rule.EgressID, event.EgressID) {
		return false
	}
	if rule.FieldID == nil {
		return true
	}
	var data ProbeChangeData
	if json.Unmarshal(event.PayloadJson, &data) != nil {
		return false
	}
	for _, change := range data.Changes {
		if change.FieldID == *rule.FieldID {
			return true
		}
	}
	return false
}

func sameOptional(left, right *string) bool {
	return left != nil && right != nil && *left == *right
}

type eventEnvelope struct {
	SchemaVersion int             `json:"schemaVersion"`
	ID            string          `json:"id"`
	Type          string          `json:"type"`
	ObservedAt    string          `json:"observedAt"`
	RecordedAt    string          `json:"recordedAt"`
	Node          *resourceRef    `json:"node,omitempty"`
	Egress        *egressRef      `json:"egress,omitempty"`
	Data          json.RawMessage `json:"data"`
	Link          *string         `json:"link,omitempty"`
}

type resourceRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type egressRef struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Family string `json:"family"`
}

func (s *Service) buildDeliveryContent(ctx context.Context, event historydb.NotificationEvent, locale string) ([]byte, string, string, error) {
	envelope := eventEnvelope{
		SchemaVersion: 1, ID: event.ID, Type: event.EventType,
		ObservedAt: unixTime(event.ObservedAt).Format(time.RFC3339),
		RecordedAt: unixTime(event.RecordedAt).Format(time.RFC3339), Data: event.PayloadJson,
	}
	if event.NodeID != nil {
		node, err := s.configQueries.GetNodeByID(ctx, *event.NodeID)
		if err == nil {
			envelope.Node = &resourceRef{ID: *event.NodeID, Name: node.Name}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return nil, "", "", err
		} else {
			envelope.Node = &resourceRef{ID: *event.NodeID, Name: *event.NodeID}
		}
	}
	if event.EgressID != nil {
		egress, err := s.configQueries.GetPublicAddressByID(ctx, *event.EgressID)
		if err == nil {
			envelope.Egress = &egressRef{ID: *event.EgressID, Name: egress.Address, Family: egress.Family}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return nil, "", "", err
		} else {
			envelope.Egress = &egressRef{ID: *event.EgressID, Name: *event.EgressID}
		}
	}
	link, err := s.eventLink(ctx, event)
	if err != nil {
		return nil, "", "", err
	}
	envelope.Link = link
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return nil, "", "", err
	}
	if len(encoded) > 1024*1024 {
		return nil, "", "", errors.New("notification event envelope exceeds its boundary")
	}
	title, body := renderEvent(locale, envelope)
	return encoded, title, body, nil
}

func (s *Service) eventLink(ctx context.Context, event historydb.NotificationEvent) (*string, error) {
	settings, err := s.systemSettings.Get(ctx)
	if err != nil {
		return nil, err
	}
	if settings.ExternalOrigin == "" {
		return nil, nil
	}
	externalOrigin, err := url.Parse(settings.ExternalOrigin)
	if err != nil {
		return nil, err
	}
	path := "/history"
	if event.EventType == EventProbeFieldChange || strings.HasPrefix(event.EventType, "format-") {
		var payload struct {
			SnapshotID string `json:"snapshotId"`
		}
		if json.Unmarshal(event.PayloadJson, &payload) == nil && payload.SnapshotID != "" {
			path = "/probe-snapshots/" + url.PathEscape(payload.SnapshotID)
		}
	}
	result := externalOrigin.ResolveReference(&url.URL{Path: path}).String()
	return &result, nil
}

func renderEvent(locale string, event eventEnvelope) (string, string) {
	zh := locale == "zh-CN"
	titles := map[string][2]string{
		EventProbeFieldChange:    {"IP quality changed", "IP 质量发生变化"},
		EventAddressChange:       {"Public address changed", "公网地址发生变化"},
		EventAddressCheckFailure: {"Address check failed", "地址检查失败"},
		EventAddressCheckRecover: {"Address check recovered", "地址检查已恢复"},
		EventProbeFailure:        {"Complete probe failed", "完整探测失败"},
		EventProbeRecovery:       {"Complete probe recovered", "完整探测已恢复"},
		EventAddressGap:          {"Address history gap recorded", "已记录地址历史缺口"},
		EventProbeGap:            {"Probe history gap recorded", "已记录探测历史缺口"},
		EventFormatMismatch:      {"Upstream report format mismatch", "上游报告格式不匹配"},
		EventFormatChanged:       {"Upstream format mismatch changed", "上游格式异常发生变化"},
		EventFormatRecovery:      {"Upstream report format recovered", "上游报告格式已恢复"},
		EventTest:                {"IPChronicle test notification", "IPChronicle 测试通知"},
	}
	pair := titles[event.Type]
	title := pair[0]
	if zh {
		title = pair[1]
	}
	lines := []string{title}
	if event.Node != nil {
		if zh {
			lines = append(lines, "节点："+event.Node.Name)
		} else {
			lines = append(lines, "Node: "+event.Node.Name)
		}
	}
	if event.Egress != nil {
		if zh {
			lines = append(lines, "网络出口："+event.Egress.Name)
		} else {
			lines = append(lines, "Egress: "+event.Egress.Name)
		}
	}
	if event.Type == EventProbeFieldChange {
		var data ProbeChangeData
		if json.Unmarshal(event.Data, &data) == nil {
			for _, change := range data.Changes {
				lines = append(lines, fmt.Sprintf("- %s: %s -> %s", change.Path, change.Before, change.After))
			}
		}
	}
	if event.Link != nil {
		lines = append(lines, *event.Link)
	}
	return truncateUTF8(title, 8192), truncateUTF8(strings.Join(lines, "\n"), 65536)
}

func (s *Service) CreateTestDelivery(ctx context.Context, senderID uuid.UUID) (Delivery, error) {
	s.processMu.Lock()
	defer s.processMu.Unlock()
	sender, err := s.Sender(ctx, senderID)
	if err != nil {
		return Delivery{}, err
	}
	administrator, err := s.configQueries.GetAdministrator(ctx)
	if err != nil {
		return Delivery{}, err
	}
	now := s.now().UTC().Unix()
	eventID := uuid.New()
	payload := []byte(`{"message":"IPChronicle notification delivery test"}`)
	envelope, title, body, err := s.testDeliveryContent(eventID.String(), payload, now, administrator.Locale)
	if err != nil {
		return Delivery{}, err
	}
	transaction, err := s.historyDatabase.BeginTx(ctx, nil)
	if err != nil {
		return Delivery{}, err
	}
	defer transaction.Rollback()
	queries := s.historyQueries.WithTx(transaction)
	if _, err := queries.CreateNotificationEvent(ctx, historydb.CreateNotificationEventParams{
		ID: eventID.String(), EventType: EventTest, SourceKind: "test", SourceID: eventID.String(),
		PayloadJson: payload, ObservedAt: now, RecordedAt: now, ProcessedAt: &now,
	}); err != nil {
		return Delivery{}, err
	}
	status := "pending"
	nextAttempt := &now
	var completedAt *int64
	var errorCode *string
	active, err := queries.CountActiveNotificationDeliveriesForSender(ctx, senderID.String())
	if err != nil {
		return Delivery{}, err
	}
	if active >= maximumActiveDeliveriesPerSender {
		status = "failed"
		nextAttempt = nil
		completedAt = &now
		code := "queue-full"
		errorCode = &code
	}
	deliveryID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("notification-test-delivery:"+eventID.String()+":"+senderID.String()))
	if _, err := queries.CreateNotificationDelivery(ctx, historydb.CreateNotificationDeliveryParams{
		ID: deliveryID.String(), EventID: eventID.String(), SenderID: senderID.String(),
		SenderName: sender.Name, SenderKind: sender.Kind, EventType: EventTest, IsTest: 1,
		Status: status, NextAttemptAt: nextAttempt, CompletedAt: completedAt, ErrorCode: errorCode,
		MatchedRuleIdsJson: []byte("[]"), EventJson: envelope, Title: title, Body: body,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		return Delivery{}, err
	}
	if err := transaction.Commit(); err != nil {
		return Delivery{}, err
	}
	s.wakeWorkers()
	return s.Delivery(ctx, deliveryID)
}

func (s *Service) testDeliveryContent(id string, payload []byte, now int64, locale string) ([]byte, string, string, error) {
	envelope := eventEnvelope{
		SchemaVersion: 1, ID: id, Type: EventTest,
		ObservedAt: unixTime(now).Format(time.RFC3339), RecordedAt: unixTime(now).Format(time.RFC3339),
		Data: payload,
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return nil, "", "", err
	}
	title, body := renderEvent(locale, envelope)
	return encoded, title, body, nil
}

func (s *Service) Delivery(ctx context.Context, id uuid.UUID) (Delivery, error) {
	record, err := s.historyQueries.GetNotificationDelivery(ctx, id.String())
	if errors.Is(err, sql.ErrNoRows) {
		return Delivery{}, ErrDeliveryNotFound
	}
	if err != nil {
		return Delivery{}, err
	}
	return deliveryFromRecord(record)
}

func (s *Service) Deliveries(ctx context.Context, filter DeliveryFilter) (DeliveryPage, error) {
	if filter.Page < 1 || filter.PageSize < 1 || filter.PageSize > 100 || !validDeliveryStatus(filter.Status) {
		return DeliveryPage{}, ErrInvalidDeliveryQuery
	}
	senderID := ""
	if filter.SenderID != nil {
		senderID = filter.SenderID.String()
	}
	records, err := s.historyQueries.ListNotificationDeliveries(ctx, historydb.ListNotificationDeliveriesParams{
		SenderID: senderID, DeliveryStatus: filter.Status,
		PageOffset: int64((filter.Page - 1) * filter.PageSize), PageSize: int64(filter.PageSize),
	})
	if err != nil {
		return DeliveryPage{}, err
	}
	total, err := s.historyQueries.CountNotificationDeliveries(ctx, historydb.CountNotificationDeliveriesParams{
		SenderID: senderID, DeliveryStatus: filter.Status,
	})
	if err != nil {
		return DeliveryPage{}, err
	}
	result := DeliveryPage{Items: make([]Delivery, 0, len(records)), Page: filter.Page, PageSize: filter.PageSize, TotalItems: total}
	if total > 0 {
		result.TotalPages = int(math.Ceil(float64(total) / float64(filter.PageSize)))
	}
	for _, record := range records {
		delivery, err := deliveryFromRecord(record)
		if err != nil {
			return DeliveryPage{}, err
		}
		result.Items = append(result.Items, delivery)
	}
	return result, nil
}

func deliveryFromRecord(record historydb.NotificationDelivery) (Delivery, error) {
	id, err := uuid.Parse(record.ID)
	if err != nil {
		return Delivery{}, err
	}
	eventID, err := uuid.Parse(record.EventID)
	if err != nil {
		return Delivery{}, err
	}
	senderID, err := uuid.Parse(record.SenderID)
	if err != nil {
		return Delivery{}, err
	}
	nodeID, err := parseOptionalUUID(record.NodeID)
	if err != nil {
		return Delivery{}, err
	}
	egressID, err := parseOptionalUUID(record.EgressID)
	if err != nil {
		return Delivery{}, err
	}
	var matched []string
	if err := json.Unmarshal(record.MatchedRuleIdsJson, &matched); err != nil {
		return Delivery{}, err
	}
	matchedIDs := make([]uuid.UUID, 0, len(matched))
	for _, value := range matched {
		parsed, err := uuid.Parse(value)
		if err != nil {
			return Delivery{}, err
		}
		matchedIDs = append(matchedIDs, parsed)
	}
	return Delivery{
		ID: id, EventID: eventID, SenderID: senderID, SenderName: record.SenderName,
		SenderKind: record.SenderKind, EventType: record.EventType, NodeID: nodeID, EgressID: egressID,
		Test: record.IsTest == 1, Status: record.Status, AttemptCount: record.AttemptCount,
		NextAttemptAt: timePointer(record.NextAttemptAt), LastAttemptAt: timePointer(record.LastAttemptAt),
		CompletedAt: timePointer(record.CompletedAt), ErrorCode: record.ErrorCode, MatchedRuleIDs: matchedIDs,
		Event: append(json.RawMessage(nil), record.EventJson...), Title: record.Title, Body: record.Body,
		CreatedAt: unixTime(record.CreatedAt), UpdatedAt: unixTime(record.UpdatedAt),
	}, nil
}

func validDeliveryStatus(value string) bool {
	switch value {
	case "", "pending", "running", "retrying", "succeeded", "failed":
		return true
	default:
		return false
	}
}

func timePointer(value *int64) *time.Time {
	if value == nil {
		return nil
	}
	result := unixTime(*value)
	return &result
}

func truncateUTF8(value string, maximum int) string {
	if len(value) <= maximum {
		return value
	}
	const suffix = "\n[truncated]"
	limit := maximum - len(suffix)
	for limit > 0 && value[limit]&0xc0 == 0x80 {
		limit--
	}
	return value[:limit] + suffix
}
