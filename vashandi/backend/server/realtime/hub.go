package realtime

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// LiveEvent is the envelope structure that wraps all WebSocket events.
// It mirrors the TypeScript LiveEvent interface from @paperclipai/shared.
type LiveEvent struct {
	ID        int64           `json:"id"`
	CompanyID string          `json:"companyId"`
	Type      string          `json:"type"`
	CreatedAt string          `json:"createdAt"`
	Payload   json.RawMessage `json:"payload"`
}

// Hub maintains per-company sets of active WebSocket clients and SSE subscribers
// and provides a Publish method for broadcasting live events to all of them.
// It also supports global broadcasting via PublishGlobal for events that should
// reach all connected clients regardless of company.
type Hub struct {
	mu                   sync.RWMutex
	clients              map[string]map[*Client]bool     // companyID → set of WS clients
	sseSubscribers       map[string]map[chan []byte]bool // companyID → set of SSE channels
	globalClients        map[*Client]bool                // set of WS clients subscribed to global events
	globalSSESubscribers map[chan []byte]bool            // set of SSE channels subscribed to global events
	nextEventID          int64                           // atomic counter for event IDs
}

// NewHub creates a ready-to-use Hub. No background goroutine is needed; all
// operations are protected by a read-write mutex.
func NewHub() *Hub {
	return &Hub{
		clients:              make(map[string]map[*Client]bool),
		sseSubscribers:       make(map[string]map[chan []byte]bool),
		globalClients:        make(map[*Client]bool),
		globalSSESubscribers: make(map[chan []byte]bool),
	}
}

// Subscribe registers an SSE listener for events published to companyID.
// It returns a receive-only channel and a cancel function. The caller must
// invoke cancel (e.g. via defer) to avoid leaking the subscription.
func (h *Hub) Subscribe(companyID string) (<-chan []byte, func()) {
	ch := make(chan []byte, 64)
	h.mu.Lock()
	if h.sseSubscribers[companyID] == nil {
		h.sseSubscribers[companyID] = make(map[chan []byte]bool)
	}
	h.sseSubscribers[companyID][ch] = true
	h.mu.Unlock()

	cancel := func() {
		h.mu.Lock()
		delete(h.sseSubscribers[companyID], ch)
		if len(h.sseSubscribers[companyID]) == 0 {
			delete(h.sseSubscribers, companyID)
		}
		h.mu.Unlock()
	}
	return ch, cancel
}

// SubscribeGlobal registers an SSE listener for global events (published via
// PublishGlobal). It returns a receive-only channel and a cancel function.
// The caller must invoke cancel (e.g. via defer) to avoid leaking the subscription.
func (h *Hub) SubscribeGlobal() (<-chan []byte, func()) {
	ch := make(chan []byte, 64)
	h.mu.Lock()
	h.globalSSESubscribers[ch] = true
	h.mu.Unlock()

	cancel := func() {
		h.mu.Lock()
		delete(h.globalSSESubscribers, ch)
		h.mu.Unlock()
	}
	return ch, cancel
}

// nextID returns an auto-incrementing event ID.
func (h *Hub) nextID() int64 {
	return atomic.AddInt64(&h.nextEventID, 1)
}

// wrapEvent wraps raw payload data in a LiveEvent envelope.
func (h *Hub) wrapEvent(companyID, eventType string, payload json.RawMessage) ([]byte, error) {
	event := LiveEvent{
		ID:        h.nextID(),
		CompanyID: companyID,
		Type:      eventType,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Payload:   payload,
	}
	return json.Marshal(event)
}

// PublishEvent sends a wrapped LiveEvent to every WebSocket client and SSE
// subscriber for companyID. The eventType and payload are wrapped in the
// standard LiveEvent envelope before sending.
func (h *Hub) PublishEvent(companyID, eventType string, payload json.RawMessage) {
	data, err := h.wrapEvent(companyID, eventType, payload)
	if err != nil {
		slog.Warn("realtime: failed to wrap event", "company", companyID, "type", eventType, "error", err)
		return
	}
	h.publishRaw(companyID, data)
}

// Publish sends data to every WebSocket client and SSE subscriber for companyID.
// Slow WebSocket clients whose send buffer is full are disconnected.
// Slow SSE subscribers are skipped (their buffered channel will eventually drain).
// It is safe to call Publish concurrently with other Hub operations.
//
// Note: This method sends raw bytes without wrapping in a LiveEvent envelope.
// For wrapped events, use PublishEvent instead.
func (h *Hub) Publish(companyID string, data []byte) {
	h.publishRaw(companyID, data)
}

// publishRaw is the internal method that delivers data to company-scoped clients.
func (h *Hub) publishRaw(companyID string, data []byte) {
	h.mu.RLock()
	company := h.clients[companyID]
	wsSnapshot := make([]*Client, 0, len(company))
	for c := range company {
		wsSnapshot = append(wsSnapshot, c)
	}
	sseSnapshot := make([]chan []byte, 0, len(h.sseSubscribers[companyID]))
	for ch := range h.sseSubscribers[companyID] {
		sseSnapshot = append(sseSnapshot, ch)
	}
	h.mu.RUnlock()

	for _, c := range wsSnapshot {
		select {
		case c.send <- data:
			// delivered
		case <-c.done:
			// client is being disconnected; skip
		default:
			// Buffer full — disconnect the slow client.
			h.unregister(c)
		}
	}

	for _, ch := range sseSnapshot {
		select {
		case ch <- data:
			// delivered
		default:
			// Slow SSE subscriber — skip this event rather than blocking.
		}
	}
}

// PublishGlobal broadcasts data to all connected WebSocket clients and SSE
// subscribers that are subscribed to global events (companyID = "*").
// This mirrors the Node.js publishGlobalLiveEvent() function.
// Slow WebSocket clients whose send buffer is full are disconnected.
// Slow SSE subscribers are skipped (their buffered channel will eventually drain).
func (h *Hub) PublishGlobal(data []byte) {
	h.mu.RLock()
	wsSnapshot := make([]*Client, 0, len(h.globalClients))
	for c := range h.globalClients {
		wsSnapshot = append(wsSnapshot, c)
	}
	sseSnapshot := make([]chan []byte, 0, len(h.globalSSESubscribers))
	for ch := range h.globalSSESubscribers {
		sseSnapshot = append(sseSnapshot, ch)
	}
	h.mu.RUnlock()

	for _, c := range wsSnapshot {
		select {
		case c.send <- data:
			// delivered
		case <-c.done:
			// client is being disconnected; skip
		default:
			// Buffer full — disconnect the slow client.
			h.unregisterGlobal(c)
		}
	}

	for _, ch := range sseSnapshot {
		select {
		case ch <- data:
			// delivered
		default:
			// Slow SSE subscriber — skip this event rather than blocking.
		}
	}
}

// PublishGlobalEvent sends a wrapped LiveEvent to all global subscribers.
// The eventType and payload are wrapped in the standard LiveEvent envelope
// with companyID set to "*".
func (h *Hub) PublishGlobalEvent(eventType string, payload json.RawMessage) {
	data, err := h.wrapEvent("*", eventType, payload)
	if err != nil {
		slog.Warn("realtime: failed to wrap global event", "type", eventType, "error", err)
		return
	}
	h.PublishGlobal(data)
}

func (h *Hub) register(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if c.companyID == "*" {
		// Global subscription
		h.globalClients[c] = true
	} else {
		// Company-scoped subscription
		if h.clients[c.companyID] == nil {
			h.clients[c.companyID] = make(map[*Client]bool)
		}
		h.clients[c.companyID][c] = true
	}
}

// unregister removes the client from the hub and signals it to disconnect.
// Calling unregister more than once for the same client is safe.
func (h *Hub) unregister(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if c.companyID == "*" {
		// Global client
		if _, exists := h.globalClients[c]; !exists {
			return
		}
		delete(h.globalClients, c)
		c.disconnect()
	} else {
		// Company-scoped client
		company, ok := h.clients[c.companyID]
		if !ok {
			return
		}
		if _, exists := company[c]; !exists {
			return
		}
		delete(company, c)
		c.disconnect()
		if len(company) == 0 {
			delete(h.clients, c.companyID)
		}
	}
}

// unregisterGlobal removes a global client from the hub and signals it to disconnect.
// This is separate from unregister to handle clients in the globalClients map during
// PublishGlobal operations.
func (h *Hub) unregisterGlobal(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, exists := h.globalClients[c]; !exists {
		return
	}
	delete(h.globalClients, c)
	c.disconnect()
}

// Upgrader is the shared gorilla/websocket upgrader.
// CheckOrigin always returns true because the server's CORS middleware handles
// origin policy for regular HTTP requests; WebSocket upgrade requests go through
// our own auth layer before the connection is accepted.
var Upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}
