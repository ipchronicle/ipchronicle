package center

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/ipchronicle/ipchronicle/internal/center/admin"
	"github.com/ipchronicle/ipchronicle/internal/center/database"
	"github.com/ipchronicle/ipchronicle/internal/generated/api"
)

func TestHealthDoesNotRequireAuthentication(t *testing.T) {
	handler := newTestHTTPHandler(t, nil)
	response := performRequest(handler, http.MethodGet, "/healthz", nil, "", nil)
	if response.Code != http.StatusOK || response.Body.String() != "ok\n" {
		t.Fatalf("health response = %d %q", response.Code, response.Body.String())
	}
}

func TestAdministratorLoginStatusAndLogout(t *testing.T) {
	handler := newTestHTTPHandler(t, nil)
	unauthenticated := performRequest(handler, http.MethodGet, "/api/v1/system/status", nil, "", nil)
	assertErrorCode(t, unauthenticated, http.StatusUnauthorized, api.Unauthenticated)

	missingOrigin := performRequest(handler, http.MethodPost, "/api/v1/auth/login", loginBody(), "", nil)
	assertErrorCode(t, missingOrigin, http.StatusForbidden, api.OriginNotAllowed)

	login := performRequest(handler, http.MethodPost, "/api/v1/auth/login", loginBody(), "http://example.test", nil)
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", login.Code, login.Body.String())
	}
	var session api.AuthenticatedSession
	if err := json.NewDecoder(login.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	if session.Account.Username != "admin" || !session.Account.UsesDefaultCredentials || session.CsrfToken == "" {
		t.Fatalf("unexpected login response: %#v", session)
	}
	cookies := login.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("session cookies = %#v, want exactly one", cookies)
	}
	if cookies[0].Name != administratorSessionCookie || !cookies[0].HttpOnly || cookies[0].Secure {
		t.Fatalf("unexpected session cookie: %#v", cookies[0])
	}
	cookie := cookies[0]

	statusResponse := performRequest(handler, http.MethodGet, "/api/v1/system/status", nil, "", cookie)
	if statusResponse.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", statusResponse.Code, statusResponse.Body.String())
	}
	var status api.SystemStatus
	if err := json.NewDecoder(statusResponse.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if status.Service != api.IpchronicleCenter || status.Status != api.Ok || !status.TransportWarning || status.ConfigSchemaVersion != 1 || status.HistorySchemaVersion != 1 {
		t.Fatalf("unexpected status response: %#v", status)
	}

	missingCSRF := performRequest(handler, http.MethodPost, "/api/v1/auth/logout", nil, "http://example.test", cookie)
	assertErrorCode(t, missingCSRF, http.StatusForbidden, api.CsrfFailed)
	wrongOrigin := performRequestWithCSRF(handler, http.MethodPost, "/api/v1/auth/logout", nil, "https://wrong.example", cookie, session.CsrfToken)
	assertErrorCode(t, wrongOrigin, http.StatusForbidden, api.OriginNotAllowed)
	logout := performRequestWithCSRF(handler, http.MethodPost, "/api/v1/auth/logout", nil, "http://example.test", cookie, session.CsrfToken)
	if logout.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d, body = %s", logout.Code, logout.Body.String())
	}
	revoked := performRequest(handler, http.MethodGet, "/api/v1/auth/session", nil, "", cookie)
	assertErrorCode(t, revoked, http.StatusUnauthorized, api.Unauthenticated)
}

func TestMalformedJSONUsesStructuredError(t *testing.T) {
	handler := newTestHTTPHandler(t, nil)
	response := performRequest(handler, http.MethodPost, "/api/v1/auth/login", []byte("{"), "http://example.test", nil)
	assertErrorCode(t, response, http.StatusBadRequest, api.InvalidRequest)
}

func TestTrustedProxyControlsForwardedHTTPS(t *testing.T) {
	prefix := netip.MustParsePrefix("10.0.0.0/8")
	handler := newTestHTTPHandler(t, []netip.Prefix{prefix})
	request := httptest.NewRequest(http.MethodPost, "http://example.test/api/v1/auth/login", bytes.NewReader(loginBody()))
	request.RemoteAddr = "10.0.0.10:1234"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://example.test")
	request.Header.Set("X-Forwarded-Proto", "https")
	request.Header.Set("X-Forwarded-For", "198.51.100.20")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("proxied login status = %d, body = %s", response.Code, response.Body.String())
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].Secure {
		t.Fatalf("trusted HTTPS proxy did not produce a Secure cookie: %#v", cookies)
	}
}

func TestUntrustedClientCannotSupplyForwardedHTTPS(t *testing.T) {
	handler := newTestHTTPHandler(t, nil)
	request := httptest.NewRequest(http.MethodPost, "http://example.test/api/v1/auth/login", bytes.NewReader(loginBody()))
	request.RemoteAddr = "192.0.2.10:1234"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://example.test")
	request.Header.Set("X-Forwarded-Proto", "https")
	request.Header.Set("X-Forwarded-For", "198.51.100.20")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	assertErrorCode(t, response, http.StatusForbidden, api.OriginNotAllowed)
}

func newTestHTTPHandler(t *testing.T, trustedProxies []netip.Prefix) http.Handler {
	t.Helper()
	store, err := database.Open(context.Background(), database.PathsFromDataDirectory(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	administrator := admin.NewService(store.Config, store.ConfigQueries, store.MasterKey)
	if err := administrator.Bootstrap(context.Background(), "admin", "admin"); err != nil {
		t.Fatal(err)
	}
	return NewHTTPHandler(HTTPOptions{
		Version: "0.0.0-test", Web: http.NotFoundHandler(),
		Administrator: administrator, Store: store, TrustedProxies: trustedProxies,
	})
}

func loginBody() []byte {
	return []byte(`{"username":"admin","password":"admin"}`)
}

func performRequest(handler http.Handler, method, path string, body []byte, origin string, cookie *http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "http://example.test"+path, bytes.NewReader(body))
	request.RemoteAddr = "192.0.2.10:1234"
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func performRequestWithCSRF(handler http.Handler, method, path string, body []byte, origin string, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "http://example.test"+path, bytes.NewReader(body))
	request.RemoteAddr = "192.0.2.10:1234"
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("Origin", origin)
	request.Header.Set("X-CSRF-Token", csrf)
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func assertErrorCode(t *testing.T, response *httptest.ResponseRecorder, status int, code api.ErrorCode) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, status, response.Body.String())
	}
	var body api.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Code != code {
		t.Fatalf("error code = %s, want %s", body.Code, code)
	}
}
