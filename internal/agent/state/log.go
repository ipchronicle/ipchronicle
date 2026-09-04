package state

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	bolt "go.etcd.io/bbolt"
)

const (
	MaxAgentLogEvents       = 10_000
	MaxAgentLogQueueBytes   = 64 * 1024 * 1024
	MaxAgentLogResponseBody = 4 * 1024 * 1024

	pendingLogGapKey = "pending-gap"
	activeLogGapKey  = "active-gap"
	logCountKey      = "count"
	logBytesKey      = "bytes"
)

var ErrInvalidAgentLog = errors.New("invalid Agent log event")

type LogEvent struct {
	ID                    string            `json:"id"`
	OccurredAt            time.Time         `json:"occurredAt"`
	Level                 string            `json:"level"`
	Component             string            `json:"component"`
	EventType             string            `json:"eventType"`
	Message               string            `json:"message"`
	PublicAddressID       *string           `json:"publicAddressId,omitempty"`
	PublicAddress         *string           `json:"publicAddress,omitempty"`
	Family                *string           `json:"family,omitempty"`
	TaskID                *string           `json:"taskId,omitempty"`
	ProxyID               *string           `json:"proxyId,omitempty"`
	ConfigurationRevision *int64            `json:"configurationRevision,omitempty"`
	DiscoveryPath         *string           `json:"discoveryPath,omitempty"`
	FailureCategory       *string           `json:"failureCategory,omitempty"`
	RequestMethod         *string           `json:"requestMethod,omitempty"`
	RequestTarget         *string           `json:"requestTarget,omitempty"`
	HTTPStatus            *int              `json:"httpStatus,omitempty"`
	DurationMilliseconds  *int64            `json:"durationMilliseconds,omitempty"`
	ResponseContentType   *string           `json:"responseContentType,omitempty"`
	RateLimitHeaders      map[string]string `json:"rateLimitHeaders,omitempty"`
	ResponseBody          []byte            `json:"responseBody,omitempty"`
	ResponseTruncated     bool              `json:"responseTruncated,omitempty"`
	DroppedCount          *int64            `json:"droppedCount,omitempty"`
	DroppedFrom           *time.Time        `json:"droppedFrom,omitempty"`
	DroppedTo             *time.Time        `json:"droppedTo,omitempty"`
}

type logGap struct {
	ID    string    `json:"id"`
	Count int64     `json:"count"`
	From  time.Time `json:"from"`
	To    time.Time `json:"to"`
}

func (s *Store) EnqueueAgentLog(event LogEvent) error {
	if err := validateAgentLog(event); err != nil {
		return err
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return s.database.Update(func(tx *bolt.Tx) error {
		queue := tx.Bucket(agentLogsBucket)
		index := tx.Bucket(agentLogIndexBucket)
		metadata := tx.Bucket(agentLogMetadataBucket)
		if index.Get([]byte(event.ID)) != nil {
			return nil
		}
		sequence, err := queue.NextSequence()
		if err != nil {
			return err
		}
		key := make([]byte, 8)
		binary.BigEndian.PutUint64(key, sequence)
		if err := queue.Put(key, encoded); err != nil {
			return err
		}
		if err := index.Put([]byte(event.ID), key); err != nil {
			return err
		}
		count := metadataUint64(metadata, logCountKey) + 1
		logicalBytes := metadataUint64(metadata, logBytesKey) + uint64(len(key)+len(encoded))
		for overAgentLogLimit(count, logicalBytes) {
			oldestKey, oldestEncoded := queue.Cursor().First()
			if oldestKey == nil {
				return errors.New("Agent log queue accounting is inconsistent")
			}
			var dropped LogEvent
			if err := decodeJSON(oldestEncoded, &dropped, "queued Agent log"); err != nil {
				return err
			}
			if err := queue.Delete(oldestKey); err != nil {
				return err
			}
			if err := index.Delete([]byte(dropped.ID)); err != nil {
				return err
			}
			count--
			logicalBytes -= uint64(len(oldestKey) + len(oldestEncoded))
			if err := extendLogGap(metadata, 1, dropped.OccurredAt, dropped.OccurredAt); err != nil {
				return err
			}
		}
		if err := putMetadataUint64(metadata, logCountKey, count); err != nil {
			return err
		}
		return putMetadataUint64(metadata, logBytesKey, logicalBytes)
	})
}

func (s *Store) RecordAgentLogGap(count int64, from, to time.Time) error {
	if count < 1 || from.IsZero() || to.Before(from) {
		return ErrInvalidAgentLog
	}
	return s.database.Update(func(tx *bolt.Tx) error {
		return extendLogGap(tx.Bucket(agentLogMetadataBucket), count, from, to)
	})
}

func (s *Store) PendingAgentLogs(maxCount, maxEncodedBytes int) ([]LogEvent, error) {
	if maxCount < 1 || maxCount > 64 || maxEncodedBytes < 1 {
		return nil, ErrInvalidAgentLog
	}
	var result []LogEvent
	err := s.database.Update(func(tx *bolt.Tx) error {
		metadata := tx.Bucket(agentLogMetadataBucket)
		active, err := activateLogGap(metadata)
		if err != nil {
			return err
		}
		used := 0
		if active != nil {
			event := logGapEvent(*active)
			encoded, err := json.Marshal(event)
			if err != nil {
				return err
			}
			if len(encoded) > maxEncodedBytes {
				return errors.New("Agent log gap exceeds upload batch size")
			}
			result = append(result, event)
			used += len(encoded)
		}
		cursor := tx.Bucket(agentLogsBucket).Cursor()
		for _, encoded := cursor.First(); encoded != nil && len(result) < maxCount; _, encoded = cursor.Next() {
			if used+len(encoded) > maxEncodedBytes && len(result) > 0 {
				break
			}
			if len(encoded) > maxEncodedBytes {
				return errors.New("queued Agent log exceeds upload batch size")
			}
			var event LogEvent
			if err := decodeJSON(encoded, &event, "queued Agent log"); err != nil {
				return err
			}
			result = append(result, event)
			used += len(encoded)
		}
		return nil
	})
	return result, err
}

func (s *Store) AcknowledgeAgentLogs(ids []string) error {
	if len(ids) > 64 {
		return ErrInvalidAgentLog
	}
	if len(ids) == 0 {
		return nil
	}
	return s.database.Update(func(tx *bolt.Tx) error {
		queue := tx.Bucket(agentLogsBucket)
		index := tx.Bucket(agentLogIndexBucket)
		metadata := tx.Bucket(agentLogMetadataBucket)
		count := metadataUint64(metadata, logCountKey)
		logicalBytes := metadataUint64(metadata, logBytesKey)
		active, err := readLogGap(metadata, activeLogGapKey)
		if err != nil {
			return err
		}
		seen := make(map[string]struct{}, len(ids))
		for _, id := range ids {
			if _, err := uuid.Parse(id); err != nil {
				return ErrInvalidAgentLog
			}
			if _, duplicate := seen[id]; duplicate {
				continue
			}
			seen[id] = struct{}{}
			if active != nil && active.ID == id {
				if err := metadata.Delete([]byte(activeLogGapKey)); err != nil {
					return err
				}
				active = nil
				continue
			}
			key := index.Get([]byte(id))
			if key == nil {
				continue
			}
			keyCopy := append([]byte(nil), key...)
			encoded := queue.Get(keyCopy)
			itemBytes := uint64(len(keyCopy) + len(encoded))
			if encoded == nil || count == 0 || logicalBytes < itemBytes {
				return errors.New("Agent log queue accounting is inconsistent")
			}
			if err := queue.Delete(keyCopy); err != nil {
				return err
			}
			if err := index.Delete([]byte(id)); err != nil {
				return err
			}
			count--
			logicalBytes -= itemBytes
		}
		if err := putMetadataUint64(metadata, logCountKey, count); err != nil {
			return err
		}
		return putMetadataUint64(metadata, logBytesKey, logicalBytes)
	})
}

func activateLogGap(metadata *bolt.Bucket) (*logGap, error) {
	active, err := readLogGap(metadata, activeLogGapKey)
	if err != nil || active != nil {
		return active, err
	}
	pending, err := readLogGap(metadata, pendingLogGapKey)
	if err != nil || pending == nil {
		return pending, err
	}
	if err := putJSON(metadata, []byte(activeLogGapKey), pending); err != nil {
		return nil, err
	}
	if err := metadata.Delete([]byte(pendingLogGapKey)); err != nil {
		return nil, err
	}
	return pending, nil
}

func extendLogGap(metadata *bolt.Bucket, count int64, from, to time.Time) error {
	gap, err := readLogGap(metadata, pendingLogGapKey)
	if err != nil {
		return err
	}
	if gap == nil {
		gap = &logGap{ID: uuid.NewString(), From: from.UTC(), To: to.UTC()}
	}
	gap.Count += count
	if from.Before(gap.From) {
		gap.From = from.UTC()
	}
	if to.After(gap.To) {
		gap.To = to.UTC()
	}
	return putJSON(metadata, []byte(pendingLogGapKey), gap)
}

func readLogGap(metadata *bolt.Bucket, key string) (*logGap, error) {
	encoded := metadata.Get([]byte(key))
	if encoded == nil {
		return nil, nil
	}
	var gap logGap
	if err := decodeJSON(encoded, &gap, "Agent log gap"); err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(gap.ID); err != nil || gap.Count < 1 || gap.From.IsZero() || gap.To.Before(gap.From) {
		return nil, errors.New("stored Agent log gap is invalid")
	}
	return &gap, nil
}

func logGapEvent(gap logGap) LogEvent {
	count := gap.Count
	from, to := gap.From, gap.To
	return LogEvent{
		ID: gap.ID, OccurredAt: gap.To, Level: "warn", Component: "log-queue",
		EventType: "events-dropped", Message: fmt.Sprintf("%d Agent log events were dropped before upload", gap.Count),
		DroppedCount: &count, DroppedFrom: &from, DroppedTo: &to,
	}
}

func validateAgentLog(event LogEvent) error {
	if _, err := uuid.Parse(event.ID); err != nil || event.OccurredAt.IsZero() ||
		!validAgentLogLevel(event.Level) || !validLogText(event.Component, 64) ||
		!validLogText(event.EventType, 96) || !validLogText(event.Message, 4096) ||
		len(event.ResponseBody) > MaxAgentLogResponseBody {
		return ErrInvalidAgentLog
	}
	for _, id := range []*string{event.PublicAddressID, event.TaskID, event.ProxyID} {
		if id != nil {
			if _, err := uuid.Parse(*id); err != nil {
				return ErrInvalidAgentLog
			}
		}
	}
	if event.PublicAddress != nil && net.ParseIP(*event.PublicAddress) == nil {
		return ErrInvalidAgentLog
	}
	if event.Family != nil && *event.Family != "ipv4" && *event.Family != "ipv6" {
		return ErrInvalidAgentLog
	}
	if event.DiscoveryPath != nil && !validLogText(*event.DiscoveryPath, 2048) ||
		event.RequestMethod != nil && (len(*event.RequestMethod) < 3 || !validLogText(*event.RequestMethod, 16)) ||
		event.ResponseContentType != nil && !validLogText(*event.ResponseContentType, 256) {
		return ErrInvalidAgentLog
	}
	if !validRateLimitHeaders(event.RateLimitHeaders) {
		return ErrInvalidAgentLog
	}
	if event.RequestTarget != nil {
		parsed, err := url.Parse(*event.RequestTarget)
		if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
			parsed.Host == "" || !slices.Contains([]string{"http", "https"}, parsed.Scheme) ||
			!validLogText(*event.RequestTarget, 2048) {
			return ErrInvalidAgentLog
		}
	}
	if event.FailureCategory != nil && !slices.Contains([]string{
		"dns", "connect", "tls", "timeout", "rate-limit", "http-status",
		"response-too-large", "invalid-response", "internal",
	}, *event.FailureCategory) {
		return ErrInvalidAgentLog
	}
	if event.HTTPStatus != nil && (*event.HTTPStatus < 100 || *event.HTTPStatus > 599) ||
		event.DurationMilliseconds != nil && *event.DurationMilliseconds < 0 {
		return ErrInvalidAgentLog
	}
	if (event.DroppedCount == nil) != (event.DroppedFrom == nil) ||
		(event.DroppedCount == nil) != (event.DroppedTo == nil) ||
		event.DroppedCount != nil && (*event.DroppedCount < 1 || event.DroppedFrom.After(*event.DroppedTo)) {
		return ErrInvalidAgentLog
	}
	return nil
}

func validAgentLogLevel(value string) bool {
	return slices.Contains([]string{"error", "warn", "info", "debug"}, value)
}

func validLogText(value string, maximum int) bool {
	return value != "" && utf8.ValidString(value) && len(value) <= maximum &&
		!strings.ContainsRune(value, '\x00')
}

func validRateLimitHeaders(headers map[string]string) bool {
	if len(headers) > 16 {
		return false
	}
	for name, value := range headers {
		lower := strings.ToLower(name)
		if !(lower == "retry-after" || strings.HasPrefix(lower, "ratelimit-") ||
			strings.HasPrefix(lower, "x-ratelimit-") || strings.HasPrefix(lower, "x-rate-limit-")) ||
			!validLogText(name, 64) || !validLogText(value, 1024) || strings.ContainsAny(value, "\r\n") {
			return false
		}
	}
	return true
}

func metadataUint64(bucket *bolt.Bucket, key string) uint64 {
	value := bucket.Get([]byte(key))
	if len(value) != 8 {
		return 0
	}
	return binary.BigEndian.Uint64(value)
}

func putMetadataUint64(bucket *bolt.Bucket, key string, value uint64) error {
	encoded := make([]byte, 8)
	binary.BigEndian.PutUint64(encoded, value)
	return bucket.Put([]byte(key), encoded)
}

func overAgentLogLimit(count, logicalBytes uint64) bool {
	return count > MaxAgentLogEvents || logicalBytes > MaxAgentLogQueueBytes
}
