package syncws

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const (
	defaultPingInterval = 20 * time.Second
	defaultPingTimeout  = 45 * time.Second
	writeTimeout        = 5 * time.Second
	maxMessageSize      = 128
)

var wakeMessage = []byte(`{"type":"wake"}`)

type Hub struct {
	mu           sync.Mutex
	connections  map[string]*Connection
	closed       bool
	pingInterval time.Duration
	pingTimeout  time.Duration
}

type Connection struct {
	hub       *Hub
	nodeID    string
	sessionID string
	expiresAt time.Time
	conn      *websocket.Conn
	wake      chan struct{}
	ctx       context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once
}

func NewHub() *Hub {
	return &Hub{
		connections:  make(map[string]*Connection),
		pingInterval: defaultPingInterval,
		pingTimeout:  defaultPingTimeout,
	}
}

func (h *Hub) Connected(nodeID, sessionID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	connection := h.connections[nodeID]
	return connection != nil && connection.sessionID == sessionID && connection.ctx.Err() == nil
}

func (h *Hub) Wake(nodeID string) {
	h.mu.Lock()
	connection := h.connections[nodeID]
	h.mu.Unlock()
	if connection == nil {
		return
	}
	select {
	case connection.wake <- struct{}{}:
	default:
	}
}

func (h *Hub) Disconnect(nodeID string) {
	h.mu.Lock()
	connection := h.connections[nodeID]
	h.mu.Unlock()
	if connection != nil {
		connection.Close()
	}
}

func (h *Hub) CloseAll() {
	h.mu.Lock()
	h.closed = true
	connections := make([]*Connection, 0, len(h.connections))
	for _, connection := range h.connections {
		connections = append(connections, connection)
	}
	h.mu.Unlock()
	var wait sync.WaitGroup
	wait.Add(len(connections))
	for _, connection := range connections {
		go func() {
			defer wait.Done()
			connection.Close()
		}()
	}
	wait.Wait()
}

func (h *Hub) Attach(nodeID, sessionID string, expiresAt time.Time, conn *websocket.Conn) (*Connection, bool) {
	ctx, cancel := context.WithCancel(context.Background())
	connection := &Connection{
		hub: h, nodeID: nodeID, sessionID: sessionID, expiresAt: expiresAt,
		conn: conn, wake: make(chan struct{}, 1), ctx: ctx, cancel: cancel,
	}
	h.mu.Lock()
	previous := h.connections[nodeID]
	if h.closed || (previous != nil && previous.sessionID != sessionID) {
		h.mu.Unlock()
		cancel()
		_ = conn.Close(websocket.StatusNormalClosure, "sync session superseded")
		return nil, false
	}
	h.connections[nodeID] = connection
	h.mu.Unlock()
	if previous != nil {
		go previous.Close()
	}
	return connection, true
}

func (c *Connection) Run() error {
	defer c.Close()
	c.conn.SetReadLimit(maxMessageSize)
	readResult := make(chan error, 1)
	go func() {
		for {
			_, _, err := c.conn.Read(c.ctx)
			if err != nil {
				readResult <- err
				return
			}
			readResult <- errors.New("Agent sent application data on wake-up WebSocket")
			return
		}
	}()

	ticker := time.NewTicker(c.hub.pingInterval)
	defer ticker.Stop()
	leaseTimer := time.NewTimer(max(time.Until(c.expiresAt), 0))
	defer leaseTimer.Stop()
	if err := c.writeWake(); err != nil {
		return err
	}
	for {
		select {
		case <-c.ctx.Done():
			return c.ctx.Err()
		case <-leaseTimer.C:
			return nil
		case err := <-readResult:
			if websocket.CloseStatus(err) == websocket.StatusNormalClosure {
				return nil
			}
			return err
		case <-c.wake:
			if err := c.writeWake(); err != nil {
				return err
			}
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(c.ctx, c.hub.pingTimeout)
			err := c.conn.Ping(ctx)
			cancel()
			if err != nil {
				return fmt.Errorf("ping Agent sync connection: %w", err)
			}
		}
	}
}

func (c *Connection) Close() {
	c.closeOnce.Do(func() {
		c.hub.mu.Lock()
		if c.hub.connections[c.nodeID] == c {
			delete(c.hub.connections, c.nodeID)
		}
		c.hub.mu.Unlock()
		_ = c.conn.Close(websocket.StatusNormalClosure, "sync session ended")
		c.cancel()
	})
}

func (c *Connection) writeWake() error {
	ctx, cancel := context.WithTimeout(c.ctx, writeTimeout)
	defer cancel()
	if err := c.conn.Write(ctx, websocket.MessageText, wakeMessage); err != nil {
		return fmt.Errorf("write Agent sync wake-up: %w", err)
	}
	return nil
}
