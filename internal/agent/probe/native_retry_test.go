package probe

import (
	"context"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"testing"
	"testing/synctest"
	"time"
)

func TestProviderRetryClassification(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		err    error
		want   bool
	}{
		{"rate-limit", 429, nil, true}, {"timeout", 408, nil, true},
		{"server", 500, nil, true}, {"gateway", 502, nil, true},
		{"unavailable", 503, nil, true}, {"gateway-timeout", 504, nil, true},
		{"auth", 401, nil, false}, {"blocked", 403, nil, false},
		{"missing", 404, nil, false}, {"unsupported", 501, nil, false},
		{"success", 200, nil, false},
		{"EOF", 0, io.EOF, true}, {"short-body", 200, io.ErrUnexpectedEOF, true},
		{"reset", 0, &net.OpError{Op: "read", Err: syscall.ECONNRESET}, true},
		{"refused", 0, syscall.ECONNREFUSED, true},
		{"timeout-error", 0, context.DeadlineExceeded, true},
		{"certificate", 0, &url.Error{Op: "Get", Err: x509.UnknownAuthorityError{}}, false},
		{"hostname", 0, x509.HostnameError{}, false},
		{"NXDOMAIN", 0, &net.DNSError{IsNotFound: true}, false},
		{"DNS-timeout", 0, &net.DNSError{IsTimeout: true}, true},
		{"DNS-temporary", 0, &net.DNSError{IsTemporary: true}, true},
		{"unknown", 0, errors.New("invalid request"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := retryableProviderFailure(context.Background(), probeHTTPResponse{StatusCode: tc.status}, tc.err); got != tc.want {
				t.Fatalf("retry=%v want=%v", got, tc.want)
			}
		})
	}
	if retryableProviderFailure(context.Background(), probeHTTPResponse{StatusCode: 503, Truncated: true}, io.ErrUnexpectedEOF) {
		t.Fatal("oversized response retried")
	}
}

func TestProviderRetryRequests(t *testing.T) {
	for _, tc := range []struct {
		name, method, body string
		statuses           []int
		safePost           bool
		want               int
	}{
		{"GET-recovery", "GET", `{}`, []int{503, 200}, false, 2},
		{"GET-bound", "GET", `{}`, []int{503, 503, 503}, false, 3},
		{"POST-query", "POST", `{}`, []int{503, 200}, true, 2},
		{"POST-session", "POST", `{}`, []int{503}, false, 1},
		{"rate-limit", "GET", `{}`, []int{429, 200}, false, 2},
		{"blocked", "GET", `{}`, []int{403}, false, 1},
		{"null-field", "GET", `{"score":null}`, []int{200}, false, 1},
		{"bad-json", "GET", `not-json`, []int{200}, false, 1},
		{"quota", "GET", `{"success":false,"message":"Daily quota exceeded"}`, []int{200}, false, 1},
		{"quota-429", "GET", `{"error":"Daily request limit reached"}`, []int{429}, false, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				requests := 0
				logs := &capturedProbeLogs{}
				client := probeHTTP{retryReadOnlyPost: tc.safePost, events: logs, client: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					if requests >= len(tc.statuses) {
						t.Fatal("extra request")
					}
					body, _ := io.ReadAll(r.Body)
					if string(body) != "query-body" || r.UserAgent() != curlUserAgent || r.Header.Get("X-Test") != "value" {
						t.Fatal("request changed on replay")
					}
					status := tc.statuses[requests]
					requests++
					return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header), Request: r}, nil
				})}}
				started := time.Now()
				client.json(context.Background(), tc.method, "https://provider.example/?key=test-secret", http.Header{"X-Test": []string{"value"}}, []byte("query-body"))
				if requests != tc.want {
					t.Fatalf("requests=%d want=%d", requests, tc.want)
				}
				elapsed := time.Since(started)
				if requests == 3 && (elapsed < 4*time.Second || elapsed >= 5*time.Second) {
					t.Fatalf("elapsed=%s", elapsed)
				}
				for _, event := range logs.events {
					if strings.Contains(event.Message, "test-secret") || (event.RequestTarget != nil && strings.Contains(*event.RequestTarget, "test-secret")) {
						t.Fatal("credential in log")
					}
					if !strings.Contains(event.Message, "attempt ") {
						t.Fatalf("attempt missing: %s", event.Message)
					}
				}
			})
		})
	}
}

func TestProviderRetryAfterAndCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		now := time.Now()
		for _, value := range []string{"60", now.Add(time.Minute).UTC().Format(http.TimeFormat)} {
			if got := providerRetryDelay(probeHTTPResponse{RateLimitHeaders: map[string]string{"Retry-After": value}}, 1, now); got < time.Minute || got >= time.Minute+500*time.Millisecond {
				t.Fatalf("delay=%s", got)
			}
		}
		for _, value := range []string{"invalid", "-1", "0"} {
			if got := providerRetryDelay(probeHTTPResponse{RateLimitHeaders: map[string]string{"Retry-After": value}}, 1, now); got < time.Second || got >= 1500*time.Millisecond {
				t.Fatalf("delay=%s", got)
			}
		}
		for _, value := range []string{"18446744073709551615", "184467440737095516150"} {
			if got := providerRetryDelay(probeHTTPResponse{RateLimitHeaders: map[string]string{"Retry-After": value}}, 1, now); got != time.Duration(1<<63-1) {
				t.Fatalf("overflow delay=%s", got)
			}
		}
		ctx, cancel := context.WithCancel(context.Background())
		requests := 0
		logs := &capturedProbeLogs{}
		client := probeHTTP{events: logs, client: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			requests++
			return &http.Response{StatusCode: 429, Body: io.NopCloser(strings.NewReader(`{}`)), Header: http.Header{"Retry-After": []string{"60"}}, Request: r}, nil
		})}}
		time.AfterFunc(time.Second, cancel)
		_, err := client.get(ctx, "https://provider.example", nil)
		if !errors.Is(err, context.Canceled) || requests != 1 || time.Since(now) != time.Second {
			t.Fatalf("requests=%d err=%v elapsed=%s", requests, err, time.Since(now))
		}
		if logs.events[len(logs.events)-1].EventType != "provider-request-stopped" {
			t.Fatal("missing cancellation log")
		}
		deadlineCtx, done := context.WithTimeout(context.Background(), time.Second)
		defer done()
		client.get(deadlineCtx, "https://provider.example", nil)
		if requests != 2 || time.Since(now) != time.Second {
			t.Fatal("waited past task budget")
		}
	})
}

func TestIPAPIRetriesReadOnlyPOST(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		requests := 0
		engine := nativeEngine{input: nativeProbeInput{Target: "203.0.113.10", IPAPIAPIKey: "test-only"}}
		engine.explicitLookupHTTP = probeHTTP{client: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			body, _ := io.ReadAll(r.Body)
			if r.Method != "POST" || !strings.Contains(string(body), `"key":"test-only"`) || !strings.Contains(string(body), `"q":"203.0.113.10"`) {
				t.Fatal("incorrect lookup")
			}
			requests++
			status := 503
			if requests == 2 {
				status = 200
			}
			return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(`{"is_proxy":false}`)), Header: make(http.Header), Request: r}, nil
		})}}
		finding := engine.probeIPAPI(context.Background())
		if requests != 2 || finding.Proxy == nil || *finding.Proxy {
			t.Fatalf("requests=%d finding=%#v", requests, finding)
		}
	})
}
