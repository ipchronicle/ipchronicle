package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/dop251/goja"
	"golang.org/x/net/http/httpguts"
	"golang.org/x/sys/unix"
)

const (
	javaScriptWorkerProtocolVersion = 1
	javaScriptWorkerTimeout         = 30 * time.Second
	javaScriptWorkerStartupTimeout  = 10 * time.Second
	javaScriptWorkerShutdownGrace   = 2 * time.Second
	javaScriptRequestTimeout        = 10 * time.Second
	javaScriptDataLimit             = 128 * 1024 * 1024
	maximumJavaScriptRequests       = 10
	maximumJavaScriptBodyBytes      = 1024 * 1024
	maximumWorkerOutputBytes        = 16 * 1024
	maximumWorkerDiagnosticBytes    = 16 * 1024
	maximumWorkerInputBytes         = 2 * 1024 * 1024
	workerReadyEnvironment          = "IPCHRONICLE_NOTIFICATION_WORKER_READY_FD"
	workerReadyDescriptor           = 3
	workerReadyByte                 = 1
)

type JavaScriptRequest struct {
	Script string
	Event  json.RawMessage
	Title  string
	Body   string
}

type workerRequest struct {
	Version             int             `json:"version"`
	TimeoutMilliseconds int64           `json:"timeoutMilliseconds"`
	Script              string          `json:"script"`
	Event               json.RawMessage `json:"event"`
	Title               string          `json:"title"`
	Body                string          `json:"body"`
}

type workerResponse struct {
	OK   bool   `json:"ok"`
	Code string `json:"code,omitempty"`
}

type ProcessJavaScriptRunner struct {
	Executable string
	Timeout    time.Duration
}

func (r ProcessJavaScriptRunner) Run(ctx context.Context, request JavaScriptRequest) DeliveryError {
	if strings.TrimSpace(r.Executable) == "" || len(request.Script) == 0 || len(request.Script) > 256*1024 ||
		len(request.Event) < 2 || len(request.Event) > 1024*1024 || !json.Valid(request.Event) {
		return DeliveryError{Code: "worker-input-invalid"}
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = javaScriptWorkerTimeout
	}
	if timeout > javaScriptWorkerTimeout {
		timeout = javaScriptWorkerTimeout
	}
	encoded, err := json.Marshal(workerRequest{
		Version: javaScriptWorkerProtocolVersion, TimeoutMilliseconds: timeout.Milliseconds(),
		Script: request.Script, Event: request.Event, Title: request.Title, Body: request.Body,
	})
	if err != nil || len(encoded) > maximumWorkerInputBytes {
		return DeliveryError{Code: "worker-input-invalid"}
	}
	inputFile, err := workerMemoryFile("ipchronicle-notification-input")
	if err != nil {
		return DeliveryError{Code: "worker-failed"}
	}
	defer inputFile.Close()
	if _, err := inputFile.Write(encoded); err != nil {
		return DeliveryError{Code: "worker-failed"}
	}
	if _, err := inputFile.Seek(0, io.SeekStart); err != nil {
		return DeliveryError{Code: "worker-failed"}
	}
	outputFile, err := workerMemoryFile("ipchronicle-notification-output")
	if err != nil {
		return DeliveryError{Code: "worker-failed"}
	}
	defer outputFile.Close()
	readyReader, readyWriter, err := os.Pipe()
	if err != nil {
		return DeliveryError{Code: "worker-failed"}
	}
	defer readyReader.Close()
	command := exec.Command(r.Executable, "notification-worker")
	command.Env = environmentWithValue(os.Environ(), "GOMAXPROCS", "1")
	command.Env = environmentWithValue(command.Env, workerReadyEnvironment, strconv.Itoa(workerReadyDescriptor))
	command.Stdin = inputFile
	command.Stdout = outputFile
	diagnostics := &boundedWorkerDiagnostics{limit: maximumWorkerDiagnosticBytes}
	command.Stderr = diagnostics
	command.ExtraFiles = []*os.File{readyWriter}
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	if err := command.Start(); err != nil {
		_ = readyWriter.Close()
		return DeliveryError{Code: "worker-failed"}
	}
	_ = readyWriter.Close()
	completed := make(chan struct{})
	ready := make(chan struct{}, 1)
	go awaitWorkerReady(readyReader, ready)
	var canceled atomic.Bool
	go superviseWorker(ctx, command.Process, timeout, ready, completed, &canceled)
	err = command.Wait()
	close(completed)
	if err != nil {
		return classifyWorkerExit(err, diagnostics.Bytes(), canceled.Load())
	}
	if _, err := outputFile.Seek(0, io.SeekStart); err != nil {
		return DeliveryError{Code: "worker-failed"}
	}
	output, err := io.ReadAll(io.LimitReader(outputFile, maximumWorkerOutputBytes+1))
	if err != nil || len(output) > maximumWorkerOutputBytes {
		return DeliveryError{Code: "worker-failed"}
	}
	decoder := json.NewDecoder(bytes.NewReader(output))
	decoder.DisallowUnknownFields()
	var response workerResponse
	if err := decoder.Decode(&response); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return DeliveryError{Code: "worker-output-invalid"}
	}
	if response.OK {
		if response.Code != "" {
			return DeliveryError{Code: "worker-output-invalid"}
		}
		return DeliveryError{}
	}
	code := safeWorkerCode(response.Code)
	retryable := code == "http-failed" || code == "http-timeout" || code == "worker-timeout"
	return DeliveryError{Code: code, Retryable: retryable}
}

type boundedWorkerDiagnostics struct {
	value []byte
	limit int
}

func (b *boundedWorkerDiagnostics) Write(value []byte) (int, error) {
	written := len(value)
	if remaining := b.limit - len(b.value); remaining > 0 {
		b.value = append(b.value, value[:min(len(value), remaining)]...)
	}
	return written, nil
}

func (b *boundedWorkerDiagnostics) Bytes() []byte {
	return b.value
}

func classifyWorkerExit(err error, diagnostics []byte, canceled bool) DeliveryError {
	if workerExitedWithSignal(err, syscall.SIGKILL) {
		if canceled {
			return DeliveryError{Code: "worker-canceled", Retryable: true}
		}
		return DeliveryError{Code: "worker-timeout", Retryable: true}
	}
	if workerExceededMemoryLimit(diagnostics) {
		return DeliveryError{Code: "worker-memory-limit"}
	}
	return DeliveryError{Code: "worker-failed"}
}

func workerExceededMemoryLimit(diagnostics []byte) bool {
	for _, marker := range [][]byte{
		[]byte("fatal error: out of memory"),
		[]byte("runtime: out of memory"),
		[]byte("runtime: cannot allocate memory"),
	} {
		if bytes.Contains(diagnostics, marker) {
			return true
		}
	}
	return bytes.HasPrefix(diagnostics, []byte("SIGSEGV: segmentation violation")) &&
		bytes.Contains(diagnostics, []byte("runtime/mgcmark_greenteagc.go"))
}

func RunJavaScriptWorker(input io.Reader, output io.Writer) error {
	readyFile, err := javaScriptWorkerReadyFile()
	if err != nil {
		return err
	}
	if readyFile != nil {
		defer readyFile.Close()
	}
	response := executeJavaScriptWorker(input, readyFile)
	return json.NewEncoder(output).Encode(response)
}

func executeJavaScriptWorker(input io.Reader, ready io.Writer) (response workerResponse) {
	response = workerResponse{Code: "worker-failed"}
	defer func() {
		if recover() != nil {
			response = workerResponse{Code: "worker-panic"}
		}
	}()
	if !raceInstrumentationEnabled {
		if err := unix.Setrlimit(unix.RLIMIT_DATA, &unix.Rlimit{Cur: javaScriptDataLimit, Max: javaScriptDataLimit}); err != nil {
			return workerResponse{Code: "resource-limit-unavailable"}
		}
	}
	if err := unix.Setrlimit(unix.RLIMIT_FSIZE, &unix.Rlimit{Cur: maximumWorkerOutputBytes, Max: maximumWorkerOutputBytes}); err != nil {
		return workerResponse{Code: "resource-limit-unavailable"}
	}
	decoder := json.NewDecoder(io.LimitReader(input, maximumWorkerInputBytes+1))
	decoder.DisallowUnknownFields()
	var request workerRequest
	if err := decoder.Decode(&request); err != nil || decoder.Decode(&struct{}{}) != io.EOF ||
		request.Version != javaScriptWorkerProtocolVersion || request.Script == "" || len(request.Script) > 256*1024 ||
		len(request.Event) < 2 || len(request.Event) > 1024*1024 || !json.Valid(request.Event) ||
		len(request.Title) > 8192 || len(request.Body) > 65536 || request.TimeoutMilliseconds < 100 ||
		request.TimeoutMilliseconds > javaScriptWorkerTimeout.Milliseconds() {
		return workerResponse{Code: "worker-input-invalid"}
	}
	var event any
	eventDecoder := json.NewDecoder(bytes.NewReader(request.Event))
	eventDecoder.UseNumber()
	if err := eventDecoder.Decode(&event); err != nil {
		return workerResponse{Code: "worker-input-invalid"}
	}
	if err := armWorkerKillTimer(time.Duration(request.TimeoutMilliseconds) * time.Millisecond); err != nil {
		return workerResponse{Code: "resource-limit-unavailable"}
	}
	if ready != nil {
		if _, err := ready.Write([]byte{workerReadyByte}); err != nil {
			return workerResponse{Code: "resource-limit-unavailable"}
		}
	}
	runtime := goja.New()
	deadline := time.Now().Add(time.Duration(request.TimeoutMilliseconds) * time.Millisecond)
	host := &javaScriptHTTPHost{runtime: runtime, client: isolatedWorkerHTTPClient(), deadline: deadline}
	httpObject := runtime.NewObject()
	if err := httpObject.Set("request", host.request); err != nil {
		return workerResponse{Code: "worker-failed"}
	}
	root := runtime.NewObject()
	if err := root.Set("apiVersion", javaScriptWorkerProtocolVersion); err != nil {
		return workerResponse{Code: "worker-failed"}
	}
	if err := root.Set("event", event); err != nil {
		return workerResponse{Code: "worker-failed"}
	}
	if err := root.Set("title", request.Title); err != nil {
		return workerResponse{Code: "worker-failed"}
	}
	if err := root.Set("body", request.Body); err != nil {
		return workerResponse{Code: "worker-failed"}
	}
	if err := root.Set("http", httpObject); err != nil {
		return workerResponse{Code: "worker-failed"}
	}
	if err := runtime.Set("ipchronicle", root); err != nil {
		return workerResponse{Code: "worker-failed"}
	}
	if _, err := runtime.RunString(request.Script); err != nil {
		if host.lastError != "" {
			return workerResponse{Code: host.lastError}
		}
		return workerResponse{Code: "script-error"}
	}
	return workerResponse{OK: true}
}

func environmentWithValue(input []string, name string, value string) []string {
	prefix := name + "="
	result := make([]string, 0, len(input)+1)
	for _, entry := range input {
		if !strings.HasPrefix(entry, prefix) {
			result = append(result, entry)
		}
	}
	return append(result, prefix+value)
}

func workerExitedWithSignal(err error, signal syscall.Signal) bool {
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) {
		return false
	}
	waitStatus, ok := exitError.Sys().(syscall.WaitStatus)
	return ok && waitStatus.Signaled() && waitStatus.Signal() == signal
}

func javaScriptWorkerReadyFile() (*os.File, error) {
	value := os.Getenv(workerReadyEnvironment)
	if value == "" {
		return nil, nil
	}
	descriptor, err := strconv.Atoi(value)
	if err != nil || descriptor != workerReadyDescriptor {
		return nil, errors.New("invalid notification worker ready descriptor")
	}
	return os.NewFile(uintptr(descriptor), "ipchronicle-notification-ready"), nil
}

func awaitWorkerReady(reader io.Reader, ready chan<- struct{}) {
	var signal [1]byte
	if _, err := io.ReadFull(reader, signal[:]); err == nil && signal[0] == workerReadyByte {
		ready <- struct{}{}
	}
}

func superviseWorker(
	ctx context.Context,
	process *os.Process,
	timeout time.Duration,
	ready <-chan struct{},
	completed <-chan struct{},
	canceled *atomic.Bool,
) {
	startupTimer := time.NewTimer(javaScriptWorkerStartupTimeout)
	select {
	case <-ctx.Done():
		startupTimer.Stop()
		canceled.Store(true)
	case <-completed:
		startupTimer.Stop()
		return
	case <-ready:
		startupTimer.Stop()
		executionTimer := time.NewTimer(timeout + javaScriptWorkerShutdownGrace)
		defer executionTimer.Stop()
		select {
		case <-ctx.Done():
			canceled.Store(true)
		case <-executionTimer.C:
		case <-completed:
			return
		}
	case <-startupTimer.C:
	}
	_ = unix.Kill(-process.Pid, unix.SIGKILL)
	_ = process.Kill()
}

type javaScriptHTTPRequest struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

type javaScriptHTTPHost struct {
	runtime   *goja.Runtime
	client    *http.Client
	requests  int
	lastError string
	deadline  time.Time
}

func (h *javaScriptHTTPHost) request(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) != 1 {
		h.fail("request-invalid")
	}
	encoded, err := json.Marshal(call.Argument(0).Export())
	if err != nil {
		h.fail("request-invalid")
	}
	var input javaScriptHTTPRequest
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		h.fail("request-invalid")
	}
	h.requests++
	if h.requests > maximumJavaScriptRequests || !validJavaScriptHTTPRequest(input) {
		h.fail("request-invalid")
	}
	method := strings.ToUpper(strings.TrimSpace(input.Method))
	if method == "" {
		method = http.MethodGet
	}
	requestTimeout := min(javaScriptRequestTimeout, time.Until(h.deadline))
	if requestTimeout <= 0 {
		h.fail("http-timeout")
	}
	requestContext, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestContext, method, input.URL, strings.NewReader(input.Body))
	if err != nil {
		h.fail("request-invalid")
	}
	for name, value := range input.Headers {
		request.Header.Set(name, value)
	}
	response, err := h.client.Do(request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			h.fail("http-timeout")
		}
		h.fail("http-failed")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maximumJavaScriptBodyBytes+1))
	if err != nil {
		h.fail("http-failed")
	}
	if len(body) > maximumJavaScriptBodyBytes {
		h.fail("response-too-large")
	}
	return h.runtime.ToValue(map[string]any{
		"status": response.StatusCode, "headers": cloneHeaders(response.Header), "body": string(body),
	})
}

func (h *javaScriptHTTPHost) fail(code string) {
	h.lastError = code
	panic(h.runtime.NewGoError(errors.New(code)))
}

func validJavaScriptHTTPRequest(request javaScriptHTTPRequest) bool {
	parsed, err := url.Parse(request.URL)
	if err != nil || len(request.URL) > 4096 || parsed.Host == "" || parsed.Fragment != "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") || len(request.Body) > maximumJavaScriptBodyBytes ||
		len(request.Headers) > 32 {
		return false
	}
	method := strings.ToUpper(strings.TrimSpace(request.Method))
	if method == "" {
		method = http.MethodGet
	}
	if !validHTTPMethod(method) {
		return false
	}
	for name, value := range request.Headers {
		canonical := strings.ToLower(name)
		if !httpguts.ValidHeaderFieldName(name) || !httpguts.ValidHeaderFieldValue(value) || len(name)+len(value) > 8192 ||
			canonical == "host" || canonical == "content-length" || canonical == "connection" || canonical == "transfer-encoding" {
			return false
		}
	}
	return true
}

func validHTTPMethod(value string) bool {
	if value == "" || len(value) > 32 {
		return false
	}
	for _, character := range value {
		if character <= 0x20 || character >= 0x7f || strings.ContainsRune("()<>@,;:\\\"/[]?={}", character) {
			return false
		}
	}
	return true
}

func isolatedWorkerHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = (&net.Dialer{Timeout: javaScriptRequestTimeout, KeepAlive: 30 * time.Second}).DialContext
	transport.ResponseHeaderTimeout = javaScriptRequestTimeout
	transport.TLSHandshakeTimeout = javaScriptRequestTimeout
	return &http.Client{
		Transport: transport, Timeout: javaScriptRequestTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func cloneHeaders(input http.Header) map[string][]string {
	result := make(map[string][]string, len(input))
	for name, values := range input {
		result[name] = append([]string(nil), values...)
	}
	return result
}
