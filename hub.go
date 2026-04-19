package main

import (
	"context"
	"encoding/json"
	"log"
	"sync"

	"github.com/coder/websocket"
)

// wsMessage is a JSON envelope sent over WebSocket to the client.
// The client JS uses Target and Action to determine where and how to insert HTML.
type wsMessage struct {
	Target string `json:"target"` // DOM element ID to update
	Action string `json:"action"` // "append" or "replace"
	HTML   string `json:"html"`   // HTML fragment
}

// hub manages a single WebSocket client connection.
// Only one client is supported at a time (single-session model).
type hub struct {
	mu   sync.Mutex
	conn *websocket.Conn
}

// setConn stores a new WebSocket connection, closing any existing one.
func (h *hub) setConn(c *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.conn != nil {
		h.conn.CloseNow()
	}
	h.conn = c
}

// send writes a wsMessage to the connected client as JSON.
// If no client is connected, it is a no-op.
func (h *hub) send(ctx context.Context, msg wsMessage) error {
	h.mu.Lock()
	c := h.conn
	h.mu.Unlock()

	if c == nil {
		return nil
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	err = c.Write(ctx, websocket.MessageText, data)
	if err != nil {
		log.Printf("hub: write error: %v", err)
		h.clearConn(c)
		return err
	}
	return nil
}

// connected reports whether a client is currently connected.
func (h *hub) connected() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.conn != nil
}

// clearConn removes the connection only if it matches the given pointer.
// This avoids clearing a newer connection that replaced the old one.
func (h *hub) clearConn(c *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.conn == c {
		h.conn = nil
	}
}

// close shuts down any active connection.
func (h *hub) close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.conn != nil {
		h.conn.Close(websocket.StatusNormalClosure, "server shutdown")
		h.conn = nil
	}
}
