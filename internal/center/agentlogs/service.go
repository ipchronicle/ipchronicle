package agentlogs

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/center/database/configdb"
	"github.com/ipchronicle/ipchronicle/internal/center/database/logsdb"
)

const (
	cleanupBatch    = 250
	cleanupInterval = time.Hour
	maxResponseBody = 4 * 1024 * 1024
)

var (
	ErrInvalidEvent     = errors.New("Agent log event is invalid")
	ErrInvalidFilter    = errors.New("log filter is invalid")
	ErrInvalidRetention = errors.New("log retention settings are invalid")
)

type Service struct {
	database      *sql.DB
	queries       *logsdb.Queries
	configQueries *configdb.Queries
	now           func() time.Time
	retentionMu   sync.Mutex
}

type Event struct {
	ID                    uuid.UUID
	OccurredAt            time.Time
	Level                 string
	Component             string
	EventType             string
	Message               string
	PublicAddressID       *uuid.UUID
	PublicAddress         *string
	Family                *string
	TaskID                *uuid.UUID
	ProxyID               *uuid.UUID
	ConfigurationRevision *int64
	DiscoveryPath         *string
	FailureCategory       *string
	RequestMethod         *string
	RequestTarget         *string
	HTTPStatus            *int
	DurationMilliseconds  *int64
	ResponseContentType   *string
	RateLimitHeaders      map[string]string
	ResponseBody          []byte
	ResponseTruncated     bool
	DroppedCount          *int64
	DroppedFrom           *time.Time
	DroppedTo             *time.Time
}

type EventSummary struct {
	Event
	Source            string
	NodeID            *uuid.UUID
	NodeName          *string
	ReceivedAt        time.Time
	ResponseBodyBytes int64
}

type EventDetail struct {
	Summary EventSummary
}

type Filter struct {
	From, To      *time.Time
	NodeID        *uuid.UUID
	Level         *string
	Component     *string
	EventType     *string
	PublicAddress *string
	TaskID        *uuid.UUID
	ProxyID       *uuid.UUID
	Keyword       *string
	Cursor        *string
	PageSize      int
}

type Page struct {
	Items      []EventSummary
	NextCursor *string
}

type RetentionUpdate struct {
	Mode            string
	MaxAgeDays      *int64
	MaxLogicalBytes *int64
}

type RetentionState struct {
	RetentionUpdate
	UpdatedAt               time.Time
	LastCleanupAt           *time.Time
	LastCleanupDeletedItems int64
	LastCleanupError        *string
	LogicalBytes            int64
	RecordCount             int64
}

func NewService(database *sql.DB, queries *logsdb.Queries, configQueries *configdb.Queries) *Service {
	if database == nil || queries == nil || configQueries == nil {
		panic("Agent log service database dependencies must not be nil")
	}
	return &Service{database: database, queries: queries, configQueries: configQueries, now: time.Now}
}

func (s *Service) Upload(ctx context.Context, nodeID uuid.UUID, events []Event) (accepted, discarded []uuid.UUID, err error) {
	if nodeID == uuid.Nil || len(events) == 0 || len(events) > 64 {
		return nil, nil, ErrInvalidEvent
	}
	receivedAt := s.now().UTC()
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()
	queries := s.queries.WithTx(tx)
	for _, event := range events {
		if validateEvent(event) != nil {
			discarded = append(discarded, event.ID)
			continue
		}
		params := eventParams("agent", &nodeID, event, receivedAt)
		if _, err := queries.InsertLogEvent(ctx, params); err != nil {
			return nil, nil, err
		}
		accepted = append(accepted, event.ID)
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	return accepted, discarded, nil
}

func (s *Service) List(ctx context.Context, filter Filter) (Page, error) {
	if filter.PageSize == 0 {
		filter.PageSize = 100
	}
	if filter.PageSize < 1 || filter.PageSize > 200 || !validFilter(filter) {
		return Page{}, ErrInvalidFilter
	}
	var cursorTime any
	var cursorID *string
	if filter.Cursor != nil {
		parsedTime, parsedID, err := decodeCursor(*filter.Cursor)
		if err != nil {
			return Page{}, ErrInvalidFilter
		}
		cursorTime, cursorID = parsedTime.UnixMilli(), &parsedID
	}
	rows, err := s.queries.ListLogEvents(ctx, logsdb.ListLogEventsParams{
		FromTime: nullableMillis(filter.From), ToTime: nullableMillis(filter.To),
		NodeID: nullableUUID(filter.NodeID), Level: nullableString(filter.Level),
		Component: nullableString(filter.Component), EventType: nullableString(filter.EventType),
		PublicAddress: nullableString(filter.PublicAddress), TaskID: nullableUUID(filter.TaskID),
		ProxyID: nullableUUID(filter.ProxyID), Keyword: likeKeyword(filter.Keyword),
		CursorTime: cursorTime, CursorID: cursorID, PageSize: int64(filter.PageSize + 1),
	})
	if err != nil {
		return Page{}, err
	}
	names, err := s.nodeNames(ctx)
	if err != nil {
		return Page{}, err
	}
	hasMore := len(rows) > filter.PageSize
	if hasMore {
		rows = rows[:filter.PageSize]
	}
	items := make([]EventSummary, 0, len(rows))
	for _, row := range rows {
		item, err := summaryFromRow(row, names)
		if err != nil {
			return Page{}, err
		}
		items = append(items, item)
	}
	page := Page{Items: items}
	if hasMore && len(items) > 0 {
		cursor := encodeCursor(items[len(items)-1].OccurredAt, items[len(items)-1].ID)
		page.NextCursor = &cursor
	}
	return page, nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (EventDetail, error) {
	record, err := s.queries.GetLogEvent(ctx, id.String())
	if err != nil {
		return EventDetail{}, err
	}
	names, err := s.nodeNames(ctx)
	if err != nil {
		return EventDetail{}, err
	}
	summary, err := summaryFromRecord(record, names)
	if err != nil {
		return EventDetail{}, err
	}
	summary.ResponseBody = append([]byte(nil), record.ResponseBody...)
	summary.DiscoveryPath = record.DiscoveryPath
	return EventDetail{Summary: summary}, nil
}

func (s *Service) Retention(ctx context.Context) (RetentionState, error) {
	record, err := s.configQueries.GetLogRetentionSettings(ctx)
	if err != nil {
		return RetentionState{}, err
	}
	logicalBytes, err := s.queries.SumLogLogicalBytes(ctx)
	if err != nil {
		return RetentionState{}, err
	}
	count, err := s.queries.CountLogEvents(ctx)
	if err != nil {
		return RetentionState{}, err
	}
	state := RetentionState{
		RetentionUpdate: RetentionUpdate{Mode: record.Mode, MaxAgeDays: record.MaxAgeDays, MaxLogicalBytes: record.MaxLogicalBytes},
		UpdatedAt:       time.Unix(record.UpdatedAt, 0).UTC(), LastCleanupDeletedItems: record.LastCleanupDeletedItems,
		LastCleanupError: record.LastCleanupError, LogicalBytes: logicalBytes, RecordCount: count,
	}
	if record.LastCleanupAt != nil {
		value := time.Unix(*record.LastCleanupAt, 0).UTC()
		state.LastCleanupAt = &value
	}
	return state, nil
}

func (s *Service) UpdateRetention(ctx context.Context, update RetentionUpdate) (RetentionState, error) {
	if validateRetention(update) != nil {
		return RetentionState{}, ErrInvalidRetention
	}
	now := s.now().UTC().Truncate(time.Second)
	changed, err := s.configQueries.UpdateLogRetentionSettings(ctx, configdb.UpdateLogRetentionSettingsParams{
		Mode: update.Mode, MaxAgeDays: update.MaxAgeDays,
		MaxLogicalBytes: update.MaxLogicalBytes, UpdatedAt: now.Unix(),
	})
	if err != nil {
		return RetentionState{}, err
	}
	if changed != 1 {
		return RetentionState{}, errors.New("log retention settings were not updated")
	}
	return s.Cleanup(ctx)
}

func (s *Service) Cleanup(ctx context.Context) (RetentionState, error) {
	s.retentionMu.Lock()
	defer s.retentionMu.Unlock()
	settings, err := s.Retention(ctx)
	if err != nil {
		return RetentionState{}, err
	}
	var deleted int64
	cleanupErr := error(nil)
	switch settings.Mode {
	case "indefinite":
	case "age":
		cutoff := s.now().UTC().AddDate(0, 0, -int(*settings.MaxAgeDays)).UnixMilli()
		for {
			removed, err := s.queries.DeleteLogEventsOlderThan(ctx, logsdb.DeleteLogEventsOlderThanParams{OccurredAt: cutoff, Limit: cleanupBatch})
			if err != nil {
				cleanupErr = err
				break
			}
			deleted += removed
			if removed < cleanupBatch {
				break
			}
		}
	case "size":
		for settings.LogicalBytes > *settings.MaxLogicalBytes {
			sizes, err := s.queries.ListOldestLogEventSizes(ctx, cleanupBatch)
			if err != nil {
				cleanupErr = err
				break
			}
			if len(sizes) == 0 {
				break
			}
			overage := settings.LogicalBytes - *settings.MaxLogicalBytes
			var reclaimed int64
			removeCount := 0
			for removeCount < len(sizes) && reclaimed < overage {
				reclaimed += sizes[removeCount]
				removeCount++
			}
			removed, err := s.queries.DeleteOldestLogEvents(ctx, int64(removeCount))
			if err != nil {
				cleanupErr = err
				break
			}
			deleted += removed
			settings.LogicalBytes -= reclaimed
		}
	default:
		cleanupErr = fmt.Errorf("unknown log retention mode %q", settings.Mode)
	}
	var message *string
	if cleanupErr != nil {
		value := truncate(cleanupErr.Error(), 4096)
		message = &value
	}
	now := s.now().UTC().Truncate(time.Second)
	cleanupAt := now.Unix()
	if err := s.configQueries.RecordLogRetentionCleanup(ctx, configdb.RecordLogRetentionCleanupParams{
		LastCleanupAt: &cleanupAt, LastCleanupDeletedItems: deleted, LastCleanupError: message,
	}); err != nil {
		return RetentionState{}, err
	}
	if cleanupErr != nil {
		return RetentionState{}, cleanupErr
	}
	return s.Retention(ctx)
}

func (s *Service) RunRetentionWorker(ctx context.Context, logger *log.Logger) {
	if logger == nil {
		panic("log retention worker logger must not be nil")
	}
	if _, err := s.Cleanup(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Printf("log retention cleanup failed: %v", err)
	}
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := s.Cleanup(ctx); err != nil && !errors.Is(err, context.Canceled) {
				logger.Printf("log retention cleanup failed: %v", err)
			}
		}
	}
}

func validateEvent(event Event) error {
	if event.ID == uuid.Nil || event.OccurredAt.IsZero() || !validLevel(event.Level) ||
		!validText(event.Component, 64) || !validText(event.EventType, 96) || !validText(event.Message, 4096) ||
		len(event.ResponseBody) > maxResponseBody {
		return ErrInvalidEvent
	}
	if event.PublicAddress != nil && net.ParseIP(*event.PublicAddress) == nil {
		return ErrInvalidEvent
	}
	if event.Family != nil && *event.Family != "ipv4" && *event.Family != "ipv6" {
		return ErrInvalidEvent
	}
	if event.FailureCategory != nil && !validFailure(*event.FailureCategory) {
		return ErrInvalidEvent
	}
	if event.DiscoveryPath != nil && !validText(*event.DiscoveryPath, 2048) {
		return ErrInvalidEvent
	}
	if event.RequestMethod != nil && !validText(*event.RequestMethod, 16) {
		return ErrInvalidEvent
	}
	if event.RequestTarget != nil {
		parsed, err := url.Parse(*event.RequestTarget)
		if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
			parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
			!validText(*event.RequestTarget, 2048) {
			return ErrInvalidEvent
		}
	}
	if event.HTTPStatus != nil && (*event.HTTPStatus < 100 || *event.HTTPStatus > 599) {
		return ErrInvalidEvent
	}
	if event.DurationMilliseconds != nil && *event.DurationMilliseconds < 0 {
		return ErrInvalidEvent
	}
	if event.ResponseContentType != nil && !validText(*event.ResponseContentType, 256) {
		return ErrInvalidEvent
	}
	if !validRateLimitHeaders(event.RateLimitHeaders) {
		return ErrInvalidEvent
	}
	if (event.DroppedCount == nil) != (event.DroppedFrom == nil) || (event.DroppedCount == nil) != (event.DroppedTo == nil) {
		return ErrInvalidEvent
	}
	if event.DroppedCount != nil && (*event.DroppedCount < 1 || event.DroppedFrom.After(*event.DroppedTo)) {
		return ErrInvalidEvent
	}
	return nil
}

func validFilter(filter Filter) bool {
	if filter.From != nil && filter.To != nil && filter.From.After(*filter.To) {
		return false
	}
	if filter.Level != nil && !validLevel(*filter.Level) {
		return false
	}
	for _, value := range []*string{filter.Component, filter.EventType, filter.Keyword} {
		if value != nil && !utf8.ValidString(*value) {
			return false
		}
	}
	if filter.Component != nil && (!validText(*filter.Component, 64)) ||
		filter.EventType != nil && (!validText(*filter.EventType, 96)) ||
		filter.Keyword != nil && (!validText(*filter.Keyword, 128)) {
		return false
	}
	return filter.PublicAddress == nil || net.ParseIP(*filter.PublicAddress) != nil
}

func validateRetention(update RetentionUpdate) error {
	switch update.Mode {
	case "indefinite":
		if update.MaxAgeDays != nil || update.MaxLogicalBytes != nil {
			return ErrInvalidRetention
		}
	case "age":
		if update.MaxAgeDays == nil || *update.MaxAgeDays < 1 || *update.MaxAgeDays > 36500 || update.MaxLogicalBytes != nil {
			return ErrInvalidRetention
		}
	case "size":
		if update.MaxLogicalBytes == nil || *update.MaxLogicalBytes < 1024*1024 || *update.MaxLogicalBytes > 1<<40 || update.MaxAgeDays != nil {
			return ErrInvalidRetention
		}
	default:
		return ErrInvalidRetention
	}
	return nil
}

func validLevel(value string) bool {
	return value == "error" || value == "warn" || value == "info" || value == "debug"
}
func validFailure(value string) bool {
	switch value {
	case "dns", "connect", "tls", "timeout", "rate-limit", "http-status", "response-too-large", "invalid-response", "internal":
		return true
	}
	return false
}

func validText(value string, maximum int) bool {
	return value != "" && utf8.ValidString(value) && len(value) <= maximum && !strings.ContainsRune(value, '\x00')
}

func validRateLimitHeaders(headers map[string]string) bool {
	if len(headers) > 16 {
		return false
	}
	for name, value := range headers {
		lower := strings.ToLower(name)
		if !(lower == "retry-after" || strings.HasPrefix(lower, "ratelimit-") ||
			strings.HasPrefix(lower, "x-ratelimit-") || strings.HasPrefix(lower, "x-rate-limit-")) ||
			!validText(name, 64) || !validText(value, 1024) || strings.ContainsAny(value, "\r\n") {
			return false
		}
	}
	return true
}

func eventParams(source string, nodeID *uuid.UUID, event Event, receivedAt time.Time) logsdb.InsertLogEventParams {
	encodedRateLimits, _ := json.Marshal(event.RateLimitHeaders)
	if len(event.RateLimitHeaders) == 0 {
		encodedRateLimits = nil
	}
	var rateLimitHeaders *string
	if len(encodedRateLimits) > 0 {
		value := string(encodedRateLimits)
		rateLimitHeaders = &value
	}
	logical := int64(len(event.Message) + len(event.Component) + len(event.EventType) + len(encodedRateLimits) + len(event.ResponseBody) + 256)
	return logsdb.InsertLogEventParams{
		ID: event.ID.String(), Source: source, NodeID: uuidString(nodeID), OccurredAt: event.OccurredAt.UTC().UnixMilli(),
		ReceivedAt: receivedAt.UTC().UnixMilli(), Level: event.Level, Component: event.Component, EventType: event.EventType,
		Message: event.Message, PublicAddressID: uuidString(event.PublicAddressID), PublicAddress: event.PublicAddress,
		Family: event.Family, TaskID: uuidString(event.TaskID), ProxyID: uuidString(event.ProxyID),
		ConfigurationRevision: event.ConfigurationRevision, DiscoveryPath: event.DiscoveryPath,
		FailureCategory: event.FailureCategory, RequestMethod: event.RequestMethod, RequestTarget: event.RequestTarget,
		HttpStatus: intPointer64(event.HTTPStatus), DurationMilliseconds: event.DurationMilliseconds,
		ResponseContentType: event.ResponseContentType, RateLimitHeaders: rateLimitHeaders, ResponseBody: event.ResponseBody,
		ResponseTruncated: boolInt(event.ResponseTruncated), DroppedCount: event.DroppedCount,
		DroppedFrom: timeMillis(event.DroppedFrom), DroppedTo: timeMillis(event.DroppedTo), LogicalBytes: logical,
	}
}

func summaryFromRow(row logsdb.ListLogEventsRow, names map[string]string) (EventSummary, error) {
	record := logsdb.LogEvent{ID: row.ID, Source: row.Source, NodeID: row.NodeID, OccurredAt: row.OccurredAt,
		ReceivedAt: row.ReceivedAt, Level: row.Level, Component: row.Component, EventType: row.EventType,
		Message: row.Message, PublicAddressID: row.PublicAddressID, PublicAddress: row.PublicAddress,
		Family: row.Family, TaskID: row.TaskID, ProxyID: row.ProxyID, ConfigurationRevision: row.ConfigurationRevision,
		FailureCategory: row.FailureCategory, RequestMethod: row.RequestMethod, RequestTarget: row.RequestTarget,
		HttpStatus: row.HttpStatus, DurationMilliseconds: row.DurationMilliseconds, ResponseContentType: row.ResponseContentType,
		ResponseTruncated: row.ResponseTruncated, DroppedCount: row.DroppedCount, DroppedFrom: row.DroppedFrom,
		DroppedTo: row.DroppedTo, LogicalBytes: row.LogicalBytes}
	summary, err := summaryFromRecord(record, names)
	if err != nil {
		return EventSummary{}, err
	}
	summary.ResponseBodyBytes = row.ResponseBodyBytes
	return summary, nil
}

func summaryFromRecord(record logsdb.LogEvent, names map[string]string) (EventSummary, error) {
	id, err := uuid.Parse(record.ID)
	if err != nil {
		return EventSummary{}, err
	}
	event := Event{ID: id, OccurredAt: time.UnixMilli(record.OccurredAt).UTC(), Level: record.Level,
		Component: record.Component, EventType: record.EventType, Message: record.Message,
		PublicAddress: record.PublicAddress, Family: record.Family, ConfigurationRevision: record.ConfigurationRevision,
		FailureCategory: record.FailureCategory, RequestMethod: record.RequestMethod, RequestTarget: record.RequestTarget,
		HTTPStatus: intPointer(record.HttpStatus), DurationMilliseconds: record.DurationMilliseconds,
		ResponseContentType: record.ResponseContentType, ResponseTruncated: record.ResponseTruncated == 1,
		DroppedCount: record.DroppedCount, DroppedFrom: millisTime(record.DroppedFrom), DroppedTo: millisTime(record.DroppedTo)}
	if record.RateLimitHeaders != nil {
		if err := json.Unmarshal([]byte(*record.RateLimitHeaders), &event.RateLimitHeaders); err != nil || !validRateLimitHeaders(event.RateLimitHeaders) {
			return EventSummary{}, errors.New("stored Agent log rate-limit headers are invalid")
		}
	}
	if event.PublicAddressID, err = parseUUID(record.PublicAddressID); err != nil {
		return EventSummary{}, err
	}
	if event.TaskID, err = parseUUID(record.TaskID); err != nil {
		return EventSummary{}, err
	}
	if event.ProxyID, err = parseUUID(record.ProxyID); err != nil {
		return EventSummary{}, err
	}
	nodeID, err := parseUUID(record.NodeID)
	if err != nil {
		return EventSummary{}, err
	}
	var nodeName *string
	if record.NodeID != nil {
		if value, ok := names[*record.NodeID]; ok {
			nodeName = &value
		}
	}
	return EventSummary{Event: event, Source: record.Source, NodeID: nodeID, NodeName: nodeName,
		ReceivedAt: time.UnixMilli(record.ReceivedAt).UTC(), ResponseBodyBytes: int64(len(record.ResponseBody))}, nil
}

func (s *Service) nodeNames(ctx context.Context) (map[string]string, error) {
	records, err := s.configQueries.ListNodes(ctx)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(records))
	for _, record := range records {
		result[record.ID] = record.Name
	}
	return result, nil
}

func encodeCursor(at time.Time, id uuid.UUID) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(at.UnixMilli(), 10) + ":" + id.String()))
}
func decodeCursor(value string) (time.Time, string, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return time.Time{}, "", err
	}
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return time.Time{}, "", ErrInvalidFilter
	}
	millis, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, "", err
	}
	if _, err := uuid.Parse(parts[1]); err != nil {
		return time.Time{}, "", err
	}
	return time.UnixMilli(millis).UTC(), parts[1], nil
}

func nullableMillis(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().UnixMilli()
}
func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}
func nullableUUID(value *uuid.UUID) any {
	if value == nil {
		return nil
	}
	return value.String()
}
func likeKeyword(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}
func uuidString(value *uuid.UUID) *string {
	if value == nil {
		return nil
	}
	encoded := value.String()
	return &encoded
}
func intPointer64(value *int) *int64 {
	if value == nil {
		return nil
	}
	encoded := int64(*value)
	return &encoded
}
func intPointer(value *int64) *int {
	if value == nil {
		return nil
	}
	encoded := int(*value)
	return &encoded
}
func timeMillis(value *time.Time) *int64 {
	if value == nil {
		return nil
	}
	encoded := value.UTC().UnixMilli()
	return &encoded
}
func millisTime(value *int64) *time.Time {
	if value == nil {
		return nil
	}
	encoded := time.UnixMilli(*value).UTC()
	return &encoded
}
func parseUUID(value *string) (*uuid.UUID, error) {
	if value == nil {
		return nil, nil
	}
	parsed, err := uuid.Parse(*value)
	return &parsed, err
}
func boolInt(value bool) int64 {
	if value {
		return 1
	}
	return 0
}
func truncate(value string, maximum int) string {
	if len(value) <= maximum {
		return value
	}
	return value[:maximum]
}
