package agentlogs

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/agent/state"
)

const recorderBuffer = 256

type Event = state.LogEvent

type Sink interface {
	Emit(Event)
}

type Recorder struct {
	store  *state.Store
	output *log.Logger
	level  atomic.Int32
	events chan Event
	wake   chan struct{}
	dropMu sync.Mutex
	drops  droppedEvents
}

type droppedEvents struct {
	count int64
	from  time.Time
	to    time.Time
}

func NewRecorder(store *state.Store, output *log.Logger) *Recorder {
	if store == nil {
		panic("Agent log recorder store must not be nil")
	}
	if output == nil {
		output = log.Default()
	}
	recorder := &Recorder{
		store: store, output: output, events: make(chan Event, recorderBuffer), wake: make(chan struct{}, 1),
	}
	recorder.level.Store(levelValue("info"))
	return recorder
}

func (r *Recorder) SetLevel(level string) error {
	value := levelValue(level)
	if value < 0 {
		return fmt.Errorf("invalid Agent log level %q", level)
	}
	r.level.Store(value)
	return nil
}

func (r *Recorder) Emit(event Event) {
	if levelValue(event.Level) > r.level.Load() {
		return
	}
	if event.ID == "" {
		event.ID = uuid.NewString()
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}
	event.Message = truncateUTF8(event.Message, 4096)
	event.ResponseBody = append([]byte(nil), event.ResponseBody...)
	if len(event.ResponseBody) > state.MaxAgentLogResponseBody {
		event.ResponseBody = event.ResponseBody[:state.MaxAgentLogResponseBody]
		event.ResponseTruncated = true
	}
	r.output.Printf("level=%s component=%s event=%s message=%q%s", event.Level, event.Component, event.EventType,
		event.Message, localFields(event))
	select {
	case r.events <- event:
	default:
		r.recordMemoryDrop(event.OccurredAt)
	}
}

func (r *Recorder) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return r.flush()
		case event := <-r.events:
			if err := r.persist(event); err != nil {
				return err
			}
		}
	}
}

func (r *Recorder) UploadWake() <-chan struct{} {
	return r.wake
}

func (r *Recorder) persist(event Event) error {
	if err := r.persistMemoryDrops(); err != nil {
		return err
	}
	if err := r.store.EnqueueAgentLog(event); err != nil {
		return fmt.Errorf("persist Agent log event: %w", err)
	}
	select {
	case r.wake <- struct{}{}:
	default:
	}
	return nil
}

func (r *Recorder) flush() error {
	var result error
	for {
		select {
		case event := <-r.events:
			result = errors.Join(result, r.persist(event))
		default:
			return errors.Join(result, r.persistMemoryDrops())
		}
	}
}

func (r *Recorder) recordMemoryDrop(at time.Time) {
	r.dropMu.Lock()
	defer r.dropMu.Unlock()
	r.drops.count++
	if r.drops.from.IsZero() || at.Before(r.drops.from) {
		r.drops.from = at
	}
	if at.After(r.drops.to) {
		r.drops.to = at
	}
}

func (r *Recorder) persistMemoryDrops() error {
	r.dropMu.Lock()
	drops := r.drops
	r.drops = droppedEvents{}
	r.dropMu.Unlock()
	if drops.count == 0 {
		return nil
	}
	if err := r.store.RecordAgentLogGap(drops.count, drops.from, drops.to); err != nil {
		r.dropMu.Lock()
		r.drops.count += drops.count
		if r.drops.from.IsZero() || drops.from.Before(r.drops.from) {
			r.drops.from = drops.from
		}
		if drops.to.After(r.drops.to) {
			r.drops.to = drops.to
		}
		r.dropMu.Unlock()
		return fmt.Errorf("persist Agent log queue gap: %w", err)
	}
	return nil
}

func levelValue(level string) int32 {
	switch level {
	case "error":
		return 0
	case "warn":
		return 1
	case "info":
		return 2
	case "debug":
		return 3
	default:
		return -1
	}
}

func SanitizeRequestTarget(value string) *string {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	result := parsed.String()
	if len(result) > 2048 {
		return nil
	}
	return &result
}

func RateLimitHeaders(headers http.Header) map[string]string {
	result := make(map[string]string)
	for name, values := range headers {
		lower := strings.ToLower(name)
		if lower != "retry-after" && !strings.HasPrefix(lower, "ratelimit-") &&
			!strings.HasPrefix(lower, "x-ratelimit-") && !strings.HasPrefix(lower, "x-rate-limit-") {
			continue
		}
		value := strings.Join(values, ", ")
		value = strings.ReplaceAll(strings.ReplaceAll(value, "\r", " "), "\n", " ")
		value = truncateUTF8(strings.TrimSpace(value), 1024)
		if value != "" && len(result) < 16 {
			result[http.CanonicalHeaderKey(name)] = value
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func ClassifyRequestError(err error) string {
	var dnsError *net.DNSError
	if errors.As(err, &dnsError) {
		return "dns"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return "timeout"
	}
	var recordError tls.RecordHeaderError
	var authorityError x509.UnknownAuthorityError
	var certificateError x509.CertificateInvalidError
	if errors.As(err, &recordError) || errors.As(err, &authorityError) || errors.As(err, &certificateError) {
		return "tls"
	}
	var operationError *net.OpError
	if errors.As(err, &operationError) && operationError.Op == "dial" {
		return "connect"
	}
	return "internal"
}

func localFields(event Event) string {
	parts := make([]string, 0, 8)
	fields := []struct {
		name  string
		value *string
	}{
		{name: "public_address", value: event.PublicAddress},
		{name: "task_id", value: event.TaskID},
		{name: "proxy_id", value: event.ProxyID},
		{name: "failure", value: event.FailureCategory},
		{name: "request", value: event.RequestTarget},
	}
	for _, field := range fields {
		if field.value != nil {
			parts = append(parts, field.name+"="+fmt.Sprintf("%q", *field.value))
		}
	}
	if event.HTTPStatus != nil {
		parts = append(parts, fmt.Sprintf("http_status=%d", *event.HTTPStatus))
	}
	if len(event.ResponseBody) > 0 {
		parts = append(parts, fmt.Sprintf("response_bytes=%d", len(event.ResponseBody)))
	}
	if len(parts) == 0 {
		return ""
	}
	return " " + strings.Join(parts, " ")
}

func truncateUTF8(value string, maximum int) string {
	if len(value) <= maximum {
		return value
	}
	value = value[:maximum]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
