package center

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ipchronicle/ipchronicle/internal/generated/api"
)

func TestAgentRegistrationAndRecoveryShareRateLimit(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	limiter := newAgentAccessLimiter(2, time.Minute)
	handler := requestSecurityMiddleware(limitAgentAccess(limiter, func() time.Time { return now })(
		http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
			response.WriteHeader(http.StatusNoContent)
		}),
	))

	for _, path := range []string{"/api/v1/agent/enroll", "/api/v1/agent/recover"} {
		request := httptest.NewRequest(http.MethodPost, "http://center.example"+path, nil)
		request.RemoteAddr = "192.0.2.8:43100"
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent {
			t.Fatalf("%s status = %d", path, response.Code)
		}
	}

	request := httptest.NewRequest(http.MethodPost, "http://center.example/api/v1/agent/recover", nil)
	request.RemoteAddr = "192.0.2.8:43101"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	assertErrorCode(t, response, http.StatusTooManyRequests, api.RateLimited)
	if response.Header().Get("Retry-After") != "60" {
		t.Fatalf("Retry-After = %q", response.Header().Get("Retry-After"))
	}

	now = now.Add(time.Minute)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("request after window status = %d", response.Code)
	}
}
