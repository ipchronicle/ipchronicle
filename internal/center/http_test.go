package center

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ipchronicle/ipchronicle/internal/generated/api"
)

func TestHealth(t *testing.T) {
	handler := NewHTTPHandler("test", http.NotFoundHandler())
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if response.Body.String() != "ok\n" {
		t.Fatalf("body = %q, want %q", response.Body.String(), "ok\n")
	}
}

func TestSystemStatus(t *testing.T) {
	handler := NewHTTPHandler("0.0.0-test", http.NotFoundHandler())
	request := httptest.NewRequest(http.MethodGet, "/api/v1/system/status", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	var status api.SystemStatus
	if err := json.NewDecoder(response.Body).Decode(&status); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if status.Service != api.IpchronicleCenter || status.Status != api.Ok || status.Version != "0.0.0-test" {
		t.Fatalf("unexpected status response: %#v", status)
	}
}
