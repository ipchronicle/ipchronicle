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
)

const (
	providerRequestTimeout = 10 * time.Second
	providerResponseLimit  = 4 * 1024 * 1024
)

const browserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) " +
	"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/151.0.0.0 Safari/537.36"

type probeHTTP struct {
	client *http.Client
}

type probeHTTPResponse struct {
	StatusCode int
	Body       []byte
	FinalURL   string
}

func (client probeHTTP) get(ctx context.Context, target string, headers http.Header) (probeHTTPResponse, error) {
	return client.do(ctx, http.MethodGet, target, headers, nil)
}

func (client probeHTTP) do(
	ctx context.Context,
	method string,
	target string,
	headers http.Header,
	body []byte,
) (probeHTTPResponse, error) {
	requestContext, cancel := context.WithTimeout(ctx, providerRequestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestContext, method, target, bytes.NewReader(body))
	if err != nil {
		return probeHTTPResponse{}, err
	}
	request.Header.Set("User-Agent", browserUserAgent)
	for name, values := range headers {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
	response, err := client.client.Do(request)
	if err != nil {
		return probeHTTPResponse{}, err
	}
	defer response.Body.Close()
	contents, err := io.ReadAll(io.LimitReader(response.Body, providerResponseLimit+1))
	if err != nil {
		return probeHTTPResponse{}, err
	}
	if len(contents) > providerResponseLimit {
		return probeHTTPResponse{}, errors.New("probe endpoint response exceeds 4 MiB")
	}
	return probeHTTPResponse{
		StatusCode: response.StatusCode, Body: contents, FinalURL: response.Request.URL.String(),
	}, nil
}

func (client probeHTTP) json(
	ctx context.Context,
	method string,
	target string,
	headers http.Header,
	body []byte,
) map[string]any {
	response, err := client.do(ctx, method, target, headers, body)
	if err != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(response.Body))
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
