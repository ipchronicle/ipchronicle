// Derived in part from IPQuality at commit 0ee5f192fed70c04615852efba0e4b8bd43546c7.
// Attribution and modification details are retained in THIRD_PARTY_NOTICES.md.

package probe

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ipchronicle/ipchronicle/internal/agent/agentlogs"
)

const (
	providerRequestTimeout = 10 * time.Second
	providerResponseLimit  = 4 * 1024 * 1024
)

const (
	curlUserAgent    = "curl/8.0.0"
	browserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) " +
		"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36"
	openAIUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) " +
		"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36 Edg/119.0.0.0"
)

type probeHTTP struct {
	client            *http.Client
	events            agentlogs.Sink
	context           requestLogContext
	retryReadOnlyPost bool
	attempt           int
}

type probeHTTPResponse struct {
	Attempt              int
	Method               string
	StatusCode           int
	Body                 []byte
	FinalURL             string
	ContentType          string
	RateLimitHeaders     map[string]string
	DurationMilliseconds int64
	Truncated            bool
}

type requestLogContext struct {
	publicAddressID       *string
	publicAddress         *string
	family                *string
	taskID                *string
	proxyID               *string
	configurationRevision *int64
	discoveryPath         *string
}

func (client probeHTTP) get(ctx context.Context, target string, headers http.Header) (probeHTTPResponse, error) {
	return client.do(ctx, http.MethodGet, target, headers, nil)
}

func (client probeHTTP) doOnce(ctx context.Context, method, target string, headers http.Header, body []byte) (probeHTTPResponse, error) {
	startedAt := time.Now()
	requestContext, cancel := context.WithTimeout(ctx, providerRequestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestContext, method, target, bytes.NewReader(body))
	if err != nil {
		client.emitRequestFailure(method, target, probeHTTPResponse{DurationMilliseconds: time.Since(startedAt).Milliseconds()}, "internal")
		return probeHTTPResponse{}, err
	}
	if headers.Get("User-Agent") == "" {
		// The upstream probe uses curl's default UA except where it explicitly
		// selects a browser UA. check.place rejects browser and Go client UAs.
		request.Header.Set("User-Agent", curlUserAgent)
	}
	for name, values := range headers {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
	response, err := client.client.Do(request)
	if err != nil {
		client.emitRequestFailure(method, target, probeHTTPResponse{DurationMilliseconds: time.Since(startedAt).Milliseconds()}, agentlogs.ClassifyRequestError(err))
		return probeHTTPResponse{}, err
	}
	defer response.Body.Close()
	contents, err := io.ReadAll(io.LimitReader(response.Body, providerResponseLimit+1))
	result := probeHTTPResponse{
		Attempt: client.attempt,
		Method:  method, StatusCode: response.StatusCode, Body: contents, FinalURL: response.Request.URL.String(),
		ContentType: response.Header.Get("Content-Type"), RateLimitHeaders: agentlogs.RateLimitHeaders(response.Header),
		DurationMilliseconds: time.Since(startedAt).Milliseconds(),
	}
	if len(result.Body) > providerResponseLimit {
		result.Body = result.Body[:providerResponseLimit]
		result.Truncated = true
	}
	if err != nil {
		client.emitRequestFailure(method, result.FinalURL, result, agentlogs.ClassifyRequestError(err))
		return result, err
	}
	if result.Truncated {
		client.emitRequestFailure(method, result.FinalURL, result, "response-too-large")
		return result, errors.New("probe endpoint response exceeds 4 MiB")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		category := "http-status"
		if response.StatusCode == http.StatusTooManyRequests {
			category = "rate-limit"
		}
		client.emitRequestFailure(method, result.FinalURL, result, category)
	} else {
		client.emitRequestSuccess(method, result.FinalURL, result)
	}
	return result, nil
}

func headersWithUserAgent(userAgent string) http.Header {
	headers := make(http.Header)
	headers.Set("User-Agent", userAgent)
	return headers
}

func (client probeHTTP) json(
	ctx context.Context,
	method string,
	target string,
	headers http.Header,
	body []byte,
) map[string]any {
	document, _ := client.jsonWithResponse(ctx, method, target, headers, body)
	return document
}

func (client probeHTTP) jsonWithResponse(
	ctx context.Context,
	method string,
	target string,
	headers http.Header,
	body []byte,
) (map[string]any, probeHTTPResponse) {
	response, err := client.do(ctx, method, target, headers, body)
	if err != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, response
	}
	return client.decodeJSON(response), response
}

func (client probeHTTP) decodeJSON(response probeHTTPResponse) map[string]any {
	document := decodeJSONDocument(response.Body)
	if document == nil {
		client.emitInvalidResponse(response, "Probe provider returned invalid JSON")
	} else if documentReportsFailure(document) {
		client.emitInvalidResponse(response, "Probe provider reported an unsuccessful result")
		return nil
	}
	return document
}

func (client probeHTTP) emitInvalidResponse(response probeHTTPResponse, message string) {
	client.attempt = response.Attempt
	method := response.Method
	if method == "" {
		method = http.MethodGet
	}
	client.emit(agentlogs.Event{
		Level: "warn", Component: "ip-quality", EventType: "provider-response-invalid", Message: message,
		FailureCategory: stringPointer("invalid-response"), RequestMethod: &method,
		RequestTarget: agentlogs.SanitizeRequestTarget(response.FinalURL), HTTPStatus: &response.StatusCode,
		DurationMilliseconds: &response.DurationMilliseconds, ResponseContentType: optionalString(response.ContentType),
		RateLimitHeaders: response.RateLimitHeaders, ResponseBody: response.Body, ResponseTruncated: response.Truncated,
	})
}

func documentReportsFailure(document map[string]any) bool {
	if success := documentBool(document, "success"); success != nil && !*success {
		return true
	}
	value, exists := document["error"]
	if !exists || value == nil || value == false || value == "" {
		return false
	}
	return true
}

func (client probeHTTP) emitRequestFailure(method, target string, response probeHTTPResponse, category string) {
	event := agentlogs.Event{
		Level: "warn", Component: "ip-quality", EventType: "provider-request-failed",
		Message: "IP quality provider request failed", FailureCategory: &category,
		RequestMethod: &method, RequestTarget: agentlogs.SanitizeRequestTarget(target),
		DurationMilliseconds: &response.DurationMilliseconds,
	}
	if response.StatusCode != 0 {
		event.HTTPStatus = &response.StatusCode
	}
	event.ResponseContentType = optionalString(response.ContentType)
	event.RateLimitHeaders = response.RateLimitHeaders
	event.ResponseBody = response.Body
	event.ResponseTruncated = response.Truncated
	client.emit(event)
}

func (client probeHTTP) emitRequestSuccess(method, target string, response probeHTTPResponse) {
	client.emit(agentlogs.Event{
		Level: "debug", Component: "ip-quality", EventType: "provider-request-succeeded",
		Message: "IP quality provider request succeeded", RequestMethod: &method,
		RequestTarget: agentlogs.SanitizeRequestTarget(target), HTTPStatus: &response.StatusCode,
		DurationMilliseconds: &response.DurationMilliseconds,
	})
}

func (client probeHTTP) emit(event agentlogs.Event) {
	if client.events == nil {
		return
	}
	if client.attempt > 0 {
		event.Message = fmt.Sprintf("%s (attempt %d)", event.Message, client.attempt)
	}
	event.PublicAddressID = client.context.publicAddressID
	event.PublicAddress = client.context.publicAddress
	event.Family = client.context.family
	event.TaskID = client.context.taskID
	event.ProxyID = client.context.proxyID
	event.ConfigurationRevision = client.context.configurationRevision
	event.DiscoveryPath = client.context.discoveryPath
	client.events.Emit(event)
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func stringPointer(value string) *string {
	return &value
}

func decodeJSONDocument(contents []byte) map[string]any {
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.UseNumber()
	var document map[string]any
	if err := decoder.Decode(&document); err != nil {
		return nil
	}
	return document
}

func documentValue(document map[string]any, path ...string) any {
	var current any = document
	for _, name := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = object[name]
	}
	return current
}

func documentString(document map[string]any, path ...string) string {
	value := documentValue(document, path...)
	switch typed := value.(type) {
	case string:
		if strings.EqualFold(typed, "null") {
			return ""
		}
		return strings.TrimSpace(typed)
	case json.Number:
		return typed.String()
	case float64:
		return fmt.Sprintf("%v", typed)
	default:
		return ""
	}
}

func documentBool(document map[string]any, path ...string) *bool {
	value := documentValue(document, path...)
	switch typed := value.(type) {
	case bool:
		result := typed
		return &result
	case string:
		if strings.EqualFold(typed, "true") || strings.EqualFold(typed, "false") {
			result := strings.EqualFold(typed, "true")
			return &result
		}
	}
	return nil
}

func combinedBool(values ...*bool) *bool {
	complete := true
	for _, value := range values {
		if value == nil {
			complete = false
			continue
		}
		if *value {
			result := true
			return &result
		}
	}
	if !complete {
		return nil
	}
	result := false
	return &result
}

func queryEscapeAddress(address string) string {
	return url.QueryEscape(address)
}

func (engine *nativeEngine) dialEndpoint(ctx context.Context, address string) (net.Conn, error) {
	if engine.input.ProxyAdapterURL == "" {
		return engine.input.DialContext(ctx, "tcp", address)
	}
	proxyURL, err := url.Parse(engine.input.ProxyAdapterURL)
	if err != nil {
		return nil, err
	}
	dialer := &net.Dialer{Timeout: providerRequestTimeout, KeepAlive: -1}
	connection, err := dialer.DialContext(ctx, "tcp4", proxyURL.Host)
	if err != nil {
		return nil, err
	}
	request := &http.Request{
		Method: http.MethodConnect, URL: &url.URL{Opaque: address}, Host: address,
		Header: make(http.Header),
	}
	if err := request.Write(connection); err != nil {
		_ = connection.Close()
		return nil, err
	}
	reader := bufio.NewReader(connection)
	response, err := http.ReadResponse(reader, request)
	if err != nil {
		_ = connection.Close()
		return nil, err
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_ = connection.Close()
		return nil, fmt.Errorf("proxy CONNECT returned HTTP %d", response.StatusCode)
	}
	if reader.Buffered() == 0 {
		return connection, nil
	}
	return &bufferedConn{Conn: connection, reader: reader}, nil
}

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (connection *bufferedConn) Read(value []byte) (int, error) {
	return connection.reader.Read(value)
}
