package agent

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConfigurationRejectsUnknownFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/agent/configuration" {
			http.NotFound(response, request)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(response, `{"schemaVersion":1,"revision":1,"enabled":true,"historyGeneration":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","unexpected":true}`)
	}))
	defer server.Close()

	client, err := NewControlClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.configuration(context.Background(), "credential", 1); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown configuration field error = %v", err)
	}
}

func TestAgentAPIResponseBodyIsBounded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(response, strings.Repeat("x", maxAgentAPIResponseSize*2))
	}))
	defer server.Close()

	client := &http.Client{
		Transport: boundedResponseTransport{base: http.DefaultTransport, maxBytes: maxAgentAPIResponseSize},
	}
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if len(body) != maxAgentAPIResponseSize+1 {
		t.Fatalf("bounded response length = %d", len(body))
	}
}

func TestParsePhysicalMemory(t *testing.T) {
	memory, err := parsePhysicalMemory(strings.NewReader("SwapTotal: 0 kB\nMemTotal:       262144 kB\n"))
	if err != nil || memory != 256*1024*1024 {
		t.Fatalf("physical memory = %d, %v", memory, err)
	}
	for _, input := range []string{
		"SwapTotal: 0 kB\n",
		"MemTotal: unknown kB\n",
		"MemTotal: 0 kB\n",
		"MemTotal: 1024 MB\n",
	} {
		if _, err := parsePhysicalMemory(strings.NewReader(input)); err == nil {
			t.Fatalf("invalid meminfo %q unexpectedly succeeded", input)
		}
	}
}
