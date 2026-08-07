package admin

import (
	"strings"
	"sync"
	"time"
)

const maximumThrottleEntries = 1024

type throttleEntry struct {
	failures    uint
	nextAllowed time.Time
	lastSeen    time.Time
}

type loginThrottle struct {
	mu      sync.Mutex
	entries map[string]throttleEntry
}

func newLoginThrottle() *loginThrottle {
	return &loginThrottle{entries: make(map[string]throttleEntry)}
}

func (t *loginThrottle) RetryAfter(username, address string, now time.Time) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	var wait time.Duration
	for _, key := range throttleKeys(username, address) {
		entry := t.entries[key]
		if remaining := entry.nextAllowed.Sub(now); remaining > wait {
			wait = remaining
		}
	}
	return wait
}

func (t *loginThrottle) Failed(username, address string, now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, key := range throttleKeys(username, address) {
		entry := t.entries[key]
		entry.failures++
		entry.lastSeen = now
		if entry.failures >= 2 {
			delay := time.Second << min(entry.failures-2, 5)
			entry.nextAllowed = now.Add(delay)
		}
		t.entries[key] = entry
	}
	for len(t.entries) > maximumThrottleEntries {
		var oldestKey string
		var oldest time.Time
		for key, entry := range t.entries {
			if oldestKey == "" || entry.lastSeen.Before(oldest) {
				oldestKey = key
				oldest = entry.lastSeen
			}
		}
		delete(t.entries, oldestKey)
	}
}

func (t *loginThrottle) Succeeded(username, address string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, key := range throttleKeys(username, address) {
		delete(t.entries, key)
	}
}

func throttleKeys(username, address string) []string {
	return []string{"username:" + strings.ToLower(strings.TrimSpace(username)), "address:" + address}
}
