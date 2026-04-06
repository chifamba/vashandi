package realtime

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Relaxed for the port. Real implementation needs origin checks.
	},
}

// Client represents a single websocket connection
type Client struct {
	ID        string
	CompanyID string // Used for filtering broadcasts to specific companies
	Conn      *websocket.Conn
	Send      chan []byte
	Manager   *Manager
}

// Manager orchestrates all websocket clients and broadcasts
type Manager struct {
	clients    map[*Client]bool
	register   chan *Client
	unregister chan *Client
	broadcast  chan EventMessage
	sync.RWMutex
}

// EventMessage represents a realtime event to be sent to clients
type EventMessage struct {
	CompanyID string      `json:"companyId"`
	Type      string      `json:"type"`
	Payload   interface{} `json:"payload"`
}

// NewManager creates a new Manager instance
func NewManager() *Manager {
	return &Manager{
		clients:    make(map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan EventMessage),
	}
}

// Run starts the manager's main loop to handle client registration and broadcasting
func (m *Manager) Run() {
	for {
		select {
		case client := <-m.register:
			m.Lock()
			m.clients[client] = true
			m.Unlock()
			slog.Info("Realtime client connected", "id", client.ID, "companyId", client.CompanyID)

		case client := <-m.unregister:
			m.Lock()
			if _, ok := m.clients[client]; ok {
				delete(m.clients, client)
				close(client.Send)
				slog.Info("Realtime client disconnected", "id", client.ID)
			}
			m.Unlock()

		case message := <-m.broadcast:
			msgBytes, err := json.Marshal(message)
			if err != nil {
				slog.Error("Failed to marshal event message", "error", err)
				continue
			}

			m.RLock()
			for client := range m.clients {
				// Broadcast only to clients in the same company, or if no company is specified
				if message.CompanyID == "" || client.CompanyID == message.CompanyID {
					select {
					case client.Send <- msgBytes:
					default:
						// If the send buffer is full, drop the client
						close(client.Send)
						delete(m.clients, client)
					}
				}
			}
			m.RUnlock()
		}
	}
}

// Broadcast sends an event to all connected clients (filtered by companyId if set)
func (m *Manager) Broadcast(companyID, eventType string, payload interface{}) {
	m.broadcast <- EventMessage{
		CompanyID: companyID,
		Type:      eventType,
		Payload:   payload,
	}
}

// ReadPump pumps messages from the websocket connection to the hub.
func (c *Client) ReadPump() {
	defer func() {
		c.Manager.unregister <- c
		c.Conn.Close()
	}()
	c.Conn.SetReadLimit(8192) // 8KB limit
	c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.Conn.SetPongHandler(func(string) error { c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second)); return nil })
	for {
		_, _, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				slog.Error("Websocket read error", "error", err)
			}
			break
		}
		// For Phase 4, we primarily use WS for pushing down state from server to client.
		// If client-to-server messaging is needed later, handle it here.
	}
}

// WritePump pumps messages from the hub to the websocket connection.
func (c *Client) WritePump() {
	ticker := time.NewTicker(54 * time.Second) // Send pings slightly before the 60s read deadline
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				// The hub closed the channel.
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Add queued chat messages to the current websocket message.
			n := len(c.Send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.Send)
			}

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
