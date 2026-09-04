package center

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/ipchronicle/ipchronicle/internal/generated/api"
)

const (
	agentAccessAttemptsPerWindow = 120
	agentAccessWindow            = time.Minute
	maximumAgentAccessEntries    = 1024
)

type agentAccessEntry struct {
	windowStarted time.Time
	attempts      int
	lastSeen      time.Time
}

type agentAccessLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	entries map[string]agentAccessEntry
}

func newAgentAccessLimiter(limit int, window time.Duration) *agentAccessLimiter {
	if limit < 1 || window <= 0 {
		panic("Agent access limiter requires a positive limit and window")
	}
	return &agentAccessLimiter{limit: limit, window: window, entries: make(map[string]agentAccessEntry)}
}

func (l *agentAccessLimiter) Allow(address string, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry := l.entries[address]
	if entry.windowStarted.IsZero() || !now.Before(entry.windowStarted.Add(l.window)) {
		entry.windowStarted = now
		entry.attempts = 0
	}
	entry.lastSeen = now
	if entry.attempts >= l.limit {
		l.entries[address] = entry
		return false, entry.windowStarted.Add(l.window).Sub(now)
	}
	entry.attempts++
	l.entries[address] = entry
	l.evictOldest()
	return true, 0
}

func (l *agentAccessLimiter) evictOldest() {
	for len(l.entries) > maximumAgentAccessEntries {
		oldestKey := ""
		var oldest time.Time
		for key, entry := range l.entries {
			if oldestKey == "" || entry.lastSeen.Before(oldest) {
				oldestKey = key
				oldest = entry.lastSeen
			}
		}
		delete(l.entries, oldestKey)
	}
}

func limitAgentAccess(limiter *agentAccessLimiter, now func() time.Time) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			if request.Method != http.MethodPost ||
				(request.URL.Path != "/api/v1/agent/enroll" && request.URL.Path != "/api/v1/agent/recover") {
				next.ServeHTTP(response, request)
				return
			}
			security := requestSecurityFromContext(request.Context())
			allowed, retryAfter := limiter.Allow(security.ClientAddress, now().UTC())
			if allowed {
				next.ServeHTTP(response, request)
				return
			}
			seconds := max(1, int((retryAfter+time.Second-1)/time.Second))
			response.Header().Set("Retry-After", fmt.Sprint(seconds))
			logSecurityEvent("agent_access_rate_limited", security.ClientAddress, "", "rate_limited")
			writeError(response, http.StatusTooManyRequests, api.RateLimited, map[string]string{
				"retryAfterSeconds": fmt.Sprint(seconds),
			})
		})
	}
}

func logSecurityEvent(event, clientAddress, nodeID, result string) {
	log.Printf("security_event event=%q client=%q node=%q result=%q", event, clientAddress, nodeID, result)
}
