package agent

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/generated/agentapi"
)

func TestSyncWebSocketURLIsBoundToCenterOrigin(t *testing.T) {
	sessionID := uuid.New()
	session := agentapi.AgentSyncSession{
		Id: sessionID, ExpiresAt: time.Now().Add(time.Minute),
		WebsocketPath: "/api/v1/agent/sync/" + sessionID.String(),
	}
	result, err := syncWebSocketURL("https://center.example:8443", session)
	if err != nil {
		t.Fatal(err)
	}
	want := "wss://center.example:8443" + session.WebsocketPath
	if result != want {
		t.Fatalf("sync WebSocket URL = %q, want %q", result, want)
	}

	session.WebsocketPath = "wss://attacker.example/api/v1/agent/sync/" + sessionID.String()
	if _, err := syncWebSocketURL("https://center.example", session); err == nil {
		t.Fatal("absolute WebSocket URL was accepted")
	}
	session.WebsocketPath = "/api/v1/agent/sync/" + uuid.NewString()
	if _, err := syncWebSocketURL("https://center.example", session); err == nil {
		t.Fatal("WebSocket path for another session was accepted")
	}
}

func TestSyncManagerReceivesAuthenticatedWake(t *testing.T) {
	sessionID := uuid.New()
	accepted := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/agent/sync/"+sessionID.String() || request.Header.Get("Authorization") != "Bearer agent-secret" {
			http.Error(response, "unauthorized", http.StatusUnauthorized)
			return
		}
		connection, err := websocket.Accept(response, request, nil)
		if err != nil {
			return
		}
		defer connection.CloseNow()
		accepted <- struct{}{}
		ctx, cancel := context.WithTimeout(request.Context(), time.Second)
		defer cancel()
		if err := connection.Write(ctx, websocket.MessageText, expectedWakeMessage); err != nil {
			return
		}
		_, _, _ = connection.Read(request.Context())
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	manager := newSyncManager(ctx, server.URL, "agent-secret", log.New(io.Discard, "", 0))
	manager.Update(&agentapi.AgentSyncSession{
		Id: sessionID, ExpiresAt: time.Now().Add(time.Minute),
		WebsocketPath: "/api/v1/agent/sync/" + sessionID.String(),
	})
	select {
	case <-accepted:
	case <-time.After(time.Second):
		t.Fatal("Agent did not authenticate its sync WebSocket")
	}
	select {
	case <-manager.Wake():
	case <-time.After(time.Second):
		t.Fatal("Agent did not surface the WebSocket wake-up")
	}
	cancel()
	manager.Close()
}

func TestSyncManagerRejectsInvalidWakeMessage(t *testing.T) {
	sessionID := uuid.New()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		connection, err := websocket.Accept(response, request, nil)
		if err != nil {
			return
		}
		defer connection.CloseNow()
		_ = connection.Write(request.Context(), websocket.MessageText, []byte(`{"type":"configuration"}`))
	}))
	defer server.Close()
	session := agentapi.AgentSyncSession{
		Id: sessionID, ExpiresAt: time.Now().Add(time.Minute),
		WebsocketPath: "/api/v1/agent/sync/" + sessionID.String(),
	}
	manager := newSyncManager(context.Background(), server.URL, "credential", log.New(io.Discard, "", 0))
	err := manager.connectAndRead(context.Background(), session)
	manager.Close()
	if err == nil || !strings.Contains(err.Error(), "invalid wake-up message") {
		t.Fatalf("invalid wake error = %v", err)
	}
}

func TestReconnectDelayIsBoundedAndIncreases(t *testing.T) {
	for attempt, ceiling := range []time.Duration{
		time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second,
		16 * time.Second, 30 * time.Second, 30 * time.Second,
	} {
		delay := reconnectDelay(attempt)
		floor := max(ceiling/2, time.Second)
		if delay < floor || delay > ceiling {
			t.Fatalf("attempt %d delay = %s, want [%s, %s]", attempt, delay, floor, ceiling)
		}
	}
}
