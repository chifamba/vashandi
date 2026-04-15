package realtime

import (
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// Hub maintains per-company sets of active WebSocket clients and provides a
// Publish method for broadcasting live events to all subscribers of a company.
type Hub struct {
	mu      sync.RWMutex
	clients map[string]map[*Client]bool // companyID → set of clients
}

// NewHub creates a ready-to-use Hub. No background goroutine is needed; all
// operations are protected by a read-write mutex.
func NewHub() *Hub {
	return &Hub{
		clients: make(map[string]map[*Client]bool),
	}
}

// Publish sends data to every client subscribed to companyID.
// Slow clients whose send buffer is full are disconnected.
// It is safe to call Publish concurrently with other Hub operations.
func (h *Hub) Publish(companyID string, data []byte) {
	h.mu.RLock()
	company := h.clients[companyID]
	snapshot := make([]*Client, 0, len(company))
	for c := range company {
		snapshot = append(snapshot, c)
	}
	h.mu.RUnlock()

	for _, c := range snapshot {
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
