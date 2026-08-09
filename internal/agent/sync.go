package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/ipchronicle/ipchronicle/internal/generated/agentapi"
)

const (
	syncWakeCapability   = "sync-wakeup-v1"
	syncHandshakeTimeout = 15 * time.Second
	syncMaxMessageSize   = 128
	syncMaxBackoff       = 30 * time.Second
)

var expectedWakeMessage = []byte(`{"type":"wake"}`)

type syncManager struct {
	ctx        context.Context
	cancel     context.CancelFunc
	centerURL  string
	credential string
	logger     *log.Logger
	wake       chan struct{}
	mu         sync.Mutex
	sessionID  string
	expiresAt  time.Time
	sessionEnd context.CancelFunc
}

func newSyncManager(ctx context.Context, centerURL, credential string, logger *log.Logger) *syncManager {
	if logger == nil {
		logger = log.Default()
	}
	managerContext, cancel := context.WithCancel(ctx)
	return &syncManager{
		ctx: managerContext, cancel: cancel, centerURL: centerURL, credential: credential,
		logger: logger, wake: make(chan struct{}, 1),
	}
}

func (m *syncManager) Wake() <-chan struct{} {
	return m.wake
}

func (m *syncManager) Update(session *agentapi.AgentSyncSession) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if session == nil || !session.ExpiresAt.After(time.Now()) {
		m.stopLocked()
		return
	}
	sessionID := session.Id.String()
	if m.sessionID == sessionID && m.expiresAt.Equal(session.ExpiresAt) {
		return
	}
	m.stopLocked()
	sessionContext, cancel := context.WithDeadline(m.ctx, session.ExpiresAt)
	m.sessionID = sessionID
	m.expiresAt = session.ExpiresAt
	m.sessionEnd = cancel
	copy := *session
	go m.maintain(sessionContext, copy)
}

func (m *syncManager) Close() {
	m.mu.Lock()
	m.stopLocked()
	m.mu.Unlock()
	m.cancel()
}

func (m *syncManager) stopLocked() {
	if m.sessionEnd != nil {
		m.sessionEnd()
	}
	m.sessionID = ""
	m.expiresAt = time.Time{}
	m.sessionEnd = nil
}

func (m *syncManager) maintain(ctx context.Context, session agentapi.AgentSyncSession) {
	attempt := 0
	for ctx.Err() == nil {
		startedAt := time.Now()
		err := m.connectAndRead(ctx, session)
		if ctx.Err() != nil {
			return
		}
		if time.Since(startedAt) >= time.Minute {
			attempt = 0
		}
		delay := reconnectDelay(attempt)
		attempt++
		m.logger.Printf("temporary sync connection failed; retrying in %s: %v", delay.Round(time.Millisecond), err)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return
		case <-timer.C:
		}
	}
}

func (m *syncManager) connectAndRead(ctx context.Context, session agentapi.AgentSyncSession) error {
	websocketURL, err := syncWebSocketURL(m.centerURL, session)
	if err != nil {
		return err
	}
	handshakeContext, cancel := context.WithTimeout(ctx, syncHandshakeTimeout)
	header := http.Header{"Authorization": []string{"Bearer " + m.credential}}
	connection, response, err := websocket.Dial(handshakeContext, websocketURL, &websocket.DialOptions{
		HTTPHeader: header, CompressionMode: websocket.CompressionDisabled,
	})
	cancel()
	if err != nil {
		if response != nil {
			return fmt.Errorf("open temporary sync WebSocket: HTTP %d: %w", response.StatusCode, err)
		}
		return fmt.Errorf("open temporary sync WebSocket: %w", err)
	}
	defer connection.CloseNow()
	connection.SetReadLimit(syncMaxMessageSize)
	m.logger.Printf("temporary sync connection established until %s", session.ExpiresAt.UTC().Format(time.RFC3339))
	readContext, stopRead := context.WithCancel(context.Background())
	defer stopRead()
	readResult := make(chan error, 1)
	go func() {
		readResult <- m.readWakeMessages(readContext, connection)
	}()
	select {
	case <-ctx.Done():
		_ = connection.Close(websocket.StatusNormalClosure, "sync session ended")
		stopRead()
		return nil
	case err := <-readResult:
		return err
	}
}

func (m *syncManager) readWakeMessages(ctx context.Context, connection *websocket.Conn) error {
	for {
		messageType, message, err := connection.Read(ctx)
		if err != nil {
			return fmt.Errorf("read temporary sync WebSocket: %w", err)
		}
		if messageType != websocket.MessageText || !bytes.Equal(message, expectedWakeMessage) {
			return errors.New("temporary sync WebSocket returned an invalid wake-up message")
		}
		select {
		case m.wake <- struct{}{}:
		default:
		}
	}
}

func syncWebSocketURL(centerURL string, session agentapi.AgentSyncSession) (string, error) {
	expectedPath := "/api/v1/agent/sync/" + session.Id.String()
	if session.WebsocketPath != expectedPath {
		return "", errors.New("temporary sync WebSocket path does not match its session")
	}
	parsed, err := url.Parse(centerURL)
	if err != nil {
		return "", fmt.Errorf("parse center URL for temporary sync: %w", err)
	}
	if parsed.Scheme == "https" {
		parsed.Scheme = "wss"
	} else if parsed.Scheme == "http" {
		parsed.Scheme = "ws"
	} else {
		return "", errors.New("temporary sync requires an HTTP or HTTPS center origin")
	}
	parsed.Path = expectedPath
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func reconnectDelay(attempt int) time.Duration {
	ceiling := time.Second
	for range min(attempt, 5) {
		ceiling *= 2
	}
	if ceiling > syncMaxBackoff {
		ceiling = syncMaxBackoff
	}
	floor := max(ceiling/2, time.Second)
	return floor + time.Duration(rand.Int64N(int64(ceiling-floor)+1))
}
