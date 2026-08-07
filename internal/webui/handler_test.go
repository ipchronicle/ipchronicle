package webui

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestHandlerServesAssetsAndSPAFallback(t *testing.T) {
	handler := NewHandler(fstest.MapFS{
		"index.html":    {Data: []byte("<main>IPChronicle</main>")},
		"assets/app.js": {Data: []byte("console.log('ok')")},
	})

	for _, route := range []string{"/", "/system/status"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, route, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want %d", route, response.Code, http.StatusOK)
		}
		if response.Body.String() != "<main>IPChronicle</main>" {
			t.Fatalf("%s served unexpected body %q", route, response.Body.String())
		}
	}

	asset := httptest.NewRecorder()
	handler.ServeHTTP(asset, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	if asset.Code != http.StatusOK || asset.Body.String() != "console.log('ok')" {
		t.Fatalf("asset response = %d %q", asset.Code, asset.Body.String())
	}
}

func TestHandlerDoesNotCaptureReservedOrMissingAssetPaths(t *testing.T) {
	handler := NewHandler(fstest.MapFS{"index.html": {Data: []byte("index")}})
	for _, route := range []string{"/api/v1/missing", "/agent/poll", "/healthz/extra", "/ws/node", "/assets/missing.js"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, route, nil))
		if response.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want %d", route, response.Code, http.StatusNotFound)
		}
	}
}

func TestHandlerReportsMissingBuild(t *testing.T) {
	handler := NewHandler(fstest.MapFS{})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}
