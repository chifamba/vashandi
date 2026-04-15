package realtime

import (
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// Hub maintains per-company sets of active WebSocket clients and SSE subscribers
// and provides a Publish method for broadcasting live events to all of them.
type Hub struct {
	mu             sync.RWMutex
	clients        map[string]map[*Client]bool    // companyID → set of WS clients
	sseSubscribers map[string]map[chan []byte]bool // companyID → set of SSE channels
}

// NewHub creates a ready-to-use Hub. No background goroutine is needed; all
// operations are protected by a read-write mutex.
func NewHub() *Hub {
	return &Hub{
		clients:        make(map[string]map[*Client]bool),
		sseSubscribers: make(map[string]map[chan []byte]bool),
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

// Publish sends data to every WebSocket client and SSE subscriber for companyID.
// Slow WebSocket clients whose send buffer is full are disconnected.
// Slow SSE subscribers are skipped (their buffered channel will eventually drain).
// It is safe to call Publish concurrently with other Hub operations.
func (h *Hub) Publish(companyID string, data []byte) {
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

func (h *Hub) register(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[c.companyID] == nil {
		h.clients[c.companyID] = make(map[*Client]bool)
	}
	h.clients[c.companyID][c] = true
}

// unregister removes the client from the hub and signals it to disconnect.
// Calling unregister more than once for the same client is safe.
func (h *Hub) unregister(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
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

// Upgrader is the shared gorilla/websocket upgrader.
// CheckOrigin always returns true because the server's CORS middleware handles
// origin policy for regular HTTP requests; WebSocket upgrade requests go through
// our own auth layer before the connection is accepted.
var Upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}
