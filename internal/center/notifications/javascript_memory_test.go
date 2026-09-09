package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

func TestJavaScriptWorkerRepeatedSmallHTTPRequests(t *testing.T) {
	if raceInstrumentationEnabled {
		t.Skip("race workers do not install the production memory limit")
	}
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer receiver.Close()
	request, err := json.Marshal(workerRequest{
		Version: javaScriptWorkerProtocolVersion, TimeoutMilliseconds: 3000,
		Script: `var r = ipchronicle.http.request({method: "POST", url: ` + quoteJavaScript(receiver.URL) +
			`, body: "{}"}); if (r.status !== 204) throw new Error("unexpected status");`,
		Event: json.RawMessage(`{"id":"memory-boundary-test"}`), Title: "Test", Body: "Test",
	})
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 100; attempt++ {
		t.Run(strconv.Itoa(attempt), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			readyReader, readyWriter, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer readyReader.Close()
			defer readyWriter.Close()
			command := exec.CommandContext(ctx, os.Args[0], "notification-worker")
			command.Env = environmentWithValue(os.Environ(), "GOMAXPROCS", "1")
			command.Env = environmentWithValue(command.Env, workerReadyEnvironment, "3")
			command.Env = environmentWithValue(command.Env, "IPCHRONICLE_TEST_WORKER_MEMORY_DIAGNOSTICS", "1")
			command.Stdin = bytes.NewReader(request)
			command.ExtraFiles = []*os.File{readyWriter}
			var output, diagnostics bytes.Buffer
			command.Stdout = &output
			command.Stderr = &diagnostics
			if err := command.Run(); err != nil {
				t.Fatalf("small HTTP worker failed: %v\n%s", err, diagnostics.String())
			}
			var response workerResponse
			if err := json.Unmarshal(output.Bytes(), &response); err != nil || !response.OK {
				t.Fatalf("small HTTP worker response: %s, %v\n%s", output.String(), err, diagnostics.String())
			}
		})
		if t.Failed() {
			break
		}
	}
}
