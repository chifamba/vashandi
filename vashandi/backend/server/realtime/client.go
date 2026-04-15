package realtime

import (
	"log/slog"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// writeWait is the maximum duration to wait when writing a message.
	writeWait = 10 * time.Second

	// pongWait is the maximum duration to wait for a pong reply.
	pongWait = 60 * time.Second

	// pingPeriod is how often the server pings the client. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// maxMessageSize is the maximum inbound message size the server will accept.
	maxMessageSize = 512
)

// Client is the bridge between an individual WebSocket connection and the Hub.
type Client struct {
	hub       *Hub
	companyID string
	conn      *websocket.Conn
	send      chan []byte
}

// readPump drains incoming messages and monitors ping/pong liveness.
// It calls unregister and closes the connection when the remote end disconnects.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister(c)
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	if err := c.conn.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
		slog.Debug("ws: set read deadline failed", "error", err)
		return
	}
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				slog.Warn("ws: unexpected close", "company", c.companyID, "error", err)
			}
			break
		}
		// Clients are read-only; incoming messages are intentionally ignored.
	}
}

// writePump forwards events from the send channel to the WebSocket connection
// and sends periodic pings to detect dead connections.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				slog.Debug("ws: set write deadline failed", "error", err)
				return
			}
			if !ok {
				// Hub closed the channel.
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				slog.Warn("ws: write error", "company", c.companyID, "error", err)
				return
			}

		case <-ticker.C:
			if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				slog.Debug("ws: set write deadline failed", "error", err)
				return
			}
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
