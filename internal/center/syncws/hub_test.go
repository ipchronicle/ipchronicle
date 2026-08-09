package syncws

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestHubWakesAndReplacesNodeConnection(t *testing.T) {
	hub := NewHub()
	t.Cleanup(hub.CloseAll)
	server := newHubTestServer(t, hub, time.Now().Add(5*time.Second))

	first := dialHubTestConnection(t, server.URL)
	t.Cleanup(func() { first.CloseNow() })
	assertWakeMessage(t, first)
	waitForConnection(t, hub, "node-1", "session-1", true)

	firstClosed := make(chan error, 1)
	go func() {
		_, _, err := first.Read(context.Background())
		firstClosed <- err
	}()
	second := dialHubTestConnection(t, server.URL)
	t.Cleanup(func() { second.CloseNow() })
	assertWakeMessage(t, second)
	select {
	case err := <-firstClosed:
		if websocket.CloseStatus(err) != websocket.StatusNormalClosure {
			t.Fatalf("replaced connection close = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("replaced connection remained open")
	}

	hub.Wake("node-1")
	assertWakeMessage(t, second)
	secondClosed := make(chan error, 1)
	go func() {
		_, _, err := second.Read(context.Background())
		secondClosed <- err
	}()
	hub.Disconnect("node-1")
	waitForConnection(t, hub, "node-1", "session-1", false)
	select {
	case err := <-secondClosed:
		if websocket.CloseStatus(err) != websocket.StatusNormalClosure {
			t.Fatalf("disconnected connection close = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("disconnected connection remained open")
	}
}

func TestHubRejectsConnectionFromDifferentSession(t *testing.T) {
	hub := NewHub()
	t.Cleanup(hub.CloseAll)
	server := newHubTestServer(t, hub, time.Now().Add(5*time.Second))

	current := dialHubTestConnection(t, server.URL+"?session=current-session")
	t.Cleanup(func() { current.CloseNow() })
	assertWakeMessage(t, current)
	waitForConnection(t, hub, "node-1", "current-session", true)

	stale := dialHubTestConnection(t, server.URL+"?session=stale-session")
	t.Cleanup(func() { stale.CloseNow() })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, _, err := stale.Read(ctx)
	if websocket.CloseStatus(err) != websocket.StatusNormalClosure {
		t.Fatalf("stale session close = %v", err)
	}
	if !hub.Connected("node-1", "current-session") {
		t.Fatal("stale session replaced the current connection")
	}
	hub.Wake("node-1")
	assertWakeMessage(t, current)
}

func TestHubExpiresConnectionAndMaintainsPingPong(t *testing.T) {
	hub := NewHub()
	hub.pingInterval = 20 * time.Millisecond
	hub.pingTimeout = 100 * time.Millisecond
	t.Cleanup(hub.CloseAll)
	server := newHubTestServer(t, hub, time.Now().Add(300*time.Millisecond))
	connection := dialHubTestConnection(t, server.URL)
	t.Cleanup(func() { connection.CloseNow() })
	assertWakeMessage(t, connection)

	readContext, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, _, err := connection.Read(readContext)
	if websocket.CloseStatus(err) != websocket.StatusNormalClosure {
		t.Fatalf("expired connection close = %v", err)
	}
	waitForConnection(t, hub, "node-1", "session-1", false)
}

func newHubTestServer(t *testing.T, hub *Hub, expiresAt time.Time) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		connection, err := websocket.Accept(response, request, &websocket.AcceptOptions{
			CompressionMode: websocket.CompressionDisabled,
		})
		if err != nil {
			return
		}
		sessionID := request.URL.Query().Get("session")
		if sessionID == "" {
			sessionID = "session-1"
		}
		handle, attached := hub.Attach("node-1", sessionID, expiresAt, connection)
		if !attached {
			return
		}
		_ = handle.Run()
	}))
	t.Cleanup(server.Close)
	return server
}

func dialHubTestConnection(t *testing.T, serverURL string) *websocket.Conn {
	t.Helper()
	connection, response, err := websocket.Dial(context.Background(), "ws"+strings.TrimPrefix(serverURL, "http"), nil)
	if err != nil {
		if response != nil {
			t.Fatalf("dial test WebSocket: HTTP %d: %v", response.StatusCode, err)
		}
		t.Fatal(err)
	}
	return connection
}

func assertWakeMessage(t *testing.T, connection *websocket.Conn) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	messageType, message, err := connection.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if messageType != websocket.MessageText || string(message) != string(wakeMessage) {
		t.Fatalf("wake message = %s %q", messageType, message)
	}
}

func waitForConnection(t *testing.T, hub *Hub, nodeID, sessionID string, connected bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for hub.Connected(nodeID, sessionID) != connected {
		if time.Now().After(deadline) {
			t.Fatalf("connection state did not become %t", connected)
		}
		time.Sleep(time.Millisecond)
	}
}
