package probe

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ipchronicle/ipchronicle/internal/agent/agentlogs"
)

const providerAttempts = 3

func (client probeHTTP) do(ctx context.Context, method, target string, headers http.Header, body []byte) (probeHTTPResponse, error) {
	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return probeHTTPResponse{}, err
		}
		client.attempt = attempt
		response, err := client.doOnce(ctx, method, target, headers, body)
		safe := method == http.MethodGet || method == http.MethodHead || (method == http.MethodPost && client.retryReadOnlyPost)
		if !safe || !retryableProviderFailure(ctx, response, err) {
			return response, err
		}
		if attempt == providerAttempts {
			client.emitRetryState(method, target, response, err, "provider-request-stopped", "Request retry limit reached")
			return response, err
		}
		delay := providerRetryDelay(response, attempt, time.Now())
		if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) <= delay {
			client.emitRetryState(method, target, response, err, "provider-request-stopped", "Retry wait exceeds remaining task time")
			return response, err
		}
		client.emitRetryState(method, target, response, err, "provider-request-retry",
			fmt.Sprintf("Retrying request with attempt %d/%d after %d ms", attempt+1, providerAttempts, delay.Milliseconds()))
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			client.emitRetryState(method, target, response, err, "provider-request-stopped", "Task canceled while waiting to retry")
			return response, ctx.Err()
		case <-timer.C:
		}
	}
}

func retryableProviderFailure(ctx context.Context, response probeHTTPResponse, err error) bool {
	if ctx.Err() != nil || response.Truncated {
		return false
	}
	if err != nil {
		var dns *net.DNSError
		if errors.As(err, &dns) {
			return !dns.IsNotFound && (dns.IsTimeout || dns.IsTemporary)
		}
		var network net.Error
		return (errors.As(err, &network) && network.Timeout()) ||
			errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
			errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNREFUSED) ||
			errors.Is(err, syscall.EPIPE)
	}
	switch response.StatusCode {
	case http.StatusTooManyRequests:
		return !providerQuotaExhausted(response.Body)
	case http.StatusRequestTimeout, http.StatusInternalServerError,
		http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func providerQuotaExhausted(body []byte) bool {
	document := decodeJSONDocument(body)
	if !documentReportsFailure(document) {
		return false
	}
	// Only explicit structured error messages distinguish exhausted allowance
	// from an otherwise transient 429. Unknown responses retain bounded retries.
	for _, field := range []string{"error", "message"} {
		message, ok := document[field].(string)
		if !ok {
			continue
		}
		message = strings.ToLower(message)
		for _, marker := range []string{"quota exceeded", "quota exhausted", "exceeded your request quota", "daily request limit", "monthly request limit", "insufficient credits"} {
			if strings.Contains(message, marker) {
				return true
			}
		}
	}
	return false
}

func providerRetryDelay(response probeHTTPResponse, attempt int, now time.Time) time.Duration {
	const maxDelay = time.Duration(1<<63 - 1)
	delay := time.Duration(2*attempt-1) * time.Second
	value := response.RateLimitHeaders["Retry-After"]
	if seconds, err := strconv.ParseUint(value, 10, 64); err == nil {
		if seconds > uint64(maxDelay/time.Second) {
			return maxDelay
		}
		delay = max(delay, time.Duration(seconds)*time.Second)
	} else if errors.Is(err, strconv.ErrRange) {
		return maxDelay
	} else if date, err := http.ParseTime(value); err == nil {
		delay = max(delay, date.Sub(now))
	}
	jitter := time.Duration(rand.Int64N(int64(500 * time.Millisecond)))
	return delay + min(jitter, maxDelay-delay)
}

func (client probeHTTP) emitRetryState(method, target string, response probeHTTPResponse, err error, eventType, message string) {
	category := "http-status"
	if err != nil {
		category = agentlogs.ClassifyRequestError(err)
	} else if response.StatusCode == http.StatusTooManyRequests {
		category = "rate-limit"
	}
	event := agentlogs.Event{
		Level: "warn", Component: "ip-quality", EventType: eventType, Message: message,
		FailureCategory: &category, RequestMethod: &method,
		RequestTarget: agentlogs.SanitizeRequestTarget(target),
	}
	if response.StatusCode != 0 {
		event.HTTPStatus = &response.StatusCode
	}
	client.emit(event)
}
