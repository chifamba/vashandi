package realtime

import (
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// ServeWs handles websocket requests from the peer.
func ServeWs(manager *Manager, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	// Extract company ID from query parameters or URL, depending on how it's routed.
	// The Node.js version looks at auth tokens and query strings.
	companyID := r.URL.Query().Get("companyId")
	if companyID == "" {
		// A fallback way to extract it if not in query param but in path
		parts := strings.Split(r.URL.Path, "/")
		for i, part := range parts {
			if part == "companies" && i+1 < len(parts) {
				companyID = parts[i+1]
			}
		}
	}

	client := &Client{
		ID:        uuid.New().String(),
		CompanyID: companyID,
		Conn:      conn,
		Send:      make(chan []byte, 256),
		Manager:   manager,
	}
	client.Manager.register <- client

	// Allow collection of memory referenced by the caller by doing all work in
	// new goroutines.
	go client.WritePump()
	go client.ReadPump()
}
