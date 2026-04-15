package realtime

import (
	"log/slog"
	"sync"
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
	// send carries outbound messages. It is never closed by the hub; instead the
	// hub closes done to signal the client goroutines to exit, avoiding the
	// send-to-closed-channel race.
	send          chan []byte
	done          chan struct{} // closed once via closeOnce to signal disconnect
	closeOnce     sync.Once
	closeConnOnce sync.Once
}

// disconnect signals the client goroutines to exit. Safe to call multiple times.
func (c *Client) disconnect() {
	c.closeOnce.Do(func() { close(c.done) })
}

// closeConn closes the underlying WebSocket connection exactly once.
func (c *Client) closeConn() {
	c.closeConnOnce.Do(func() { c.conn.Close() })
}

// readPump drains incoming messages and monitors ping/pong liveness.
// It calls unregister and closes the connection when the remote end disconnects.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister(c)
		c.closeConn()
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
		c.closeConn()
	}()

	for {
		select {
		case message := <-c.send:
			if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				slog.Debug("ws: set write deadline failed", "error", err)
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				slog.Warn("ws: write error", "company", c.companyID, "error", err)
				return
			}

		case <-c.done:
			// Hub signalled disconnect; send a close frame and exit.
			_ = c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			return

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
