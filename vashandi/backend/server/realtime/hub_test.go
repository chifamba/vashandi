package realtime

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupHubTestDB creates an in-memory SQLite database with the agent_api_keys table.
func setupHubTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dbName := fmt.Sprintf("file:realtime_%s?mode=memory&cache=shared", url.QueryEscape(t.Name()))
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.Exec(`CREATE TABLE IF NOT EXISTS agent_api_keys (
		id TEXT PRIMARY KEY,
		company_id TEXT,
		agent_id TEXT,
		key_hash TEXT,
		revoked_at DATETIME,
		last_used_at DATETIME
	)`)
	return db
}

// wsConnect dials the given test server URL as a WebSocket.
func wsConnect(t *testing.T, server *httptest.Server, path string, headers http.Header) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	u := "ws" + strings.TrimPrefix(server.URL, "http") + path
	return websocket.DefaultDialer.Dial(u, headers)
}

// newTestRouter wires a Hub into a minimal chi router.
func newTestRouter(db *gorm.DB, deploymentMode string) (*Hub, *chi.Mux) {
	hub := NewHub()
	r := chi.NewRouter()
	r.Get("/api/companies/{companyId}/events/ws", hub.LiveEventsHandler(db, deploymentMode))
	return hub, r
}

// readWithTimeout reads a single WebSocket message, retrying until the deadline.
// This avoids relying on an arbitrary sleep to wait for the writePump goroutine.
func readWithTimeout(t *testing.T, conn *websocket.Conn, timeout time.Duration) ([]byte, error) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	conn.SetReadDeadline(deadline)
	_, msg, err := conn.ReadMessage()
	return msg, err
}

// TestLiveEventsHandler_LocalTrusted verifies that in local_trusted mode an
// anonymous WebSocket client is accepted and receives published events.
func TestLiveEventsHandler_LocalTrusted(t *testing.T) {
	db := setupHubTestDB(t)
	hub, router := newTestRouter(db, "local_trusted")

	server := httptest.NewServer(router)
	defer server.Close()

	conn, resp, err := wsConnect(t, server, "/api/companies/company-1/events/ws", nil)
	if err != nil {
		t.Fatalf("dial: %v (status %v)", err, resp)
	}
	defer conn.Close()

	// Publish an event.
	event := map[string]any{"type": "agent.status", "companyId": "company-1"}
	data, _ := json.Marshal(event)
	hub.Publish("company-1", data)

	msg, err := readWithTimeout(t, conn, 2*time.Second)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(msg, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["type"] != "agent.status" {
		t.Errorf("unexpected event type %q", got["type"])
	}
}

// TestLiveEventsHandler_AuthenticatedForbidden verifies that in authenticated
// mode an anonymous client (no token) receives 403.
func TestLiveEventsHandler_AuthenticatedForbidden(t *testing.T) {
	db := setupHubTestDB(t)
	_, router := newTestRouter(db, "authenticated")

	server := httptest.NewServer(router)
	defer server.Close()

	_, resp, err := wsConnect(t, server, "/api/companies/company-1/events/ws", nil)
	if err == nil {
		t.Fatal("expected connection to fail, but it succeeded")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Errorf("expected 403, got %d", status)
	}
}

// TestLiveEventsHandler_EventsNotLeakedAcrossCompanies verifies that events
// published to company-A are not delivered to a client subscribed to company-B.
func TestLiveEventsHandler_EventsNotLeakedAcrossCompanies(t *testing.T) {
	db := setupHubTestDB(t)
	hub, router := newTestRouter(db, "local_trusted")

	server := httptest.NewServer(router)
	defer server.Close()

	connA, _, err := wsConnect(t, server, "/api/companies/company-A/events/ws", nil)
	if err != nil {
		t.Fatalf("dial company-A: %v", err)
	}
	defer connA.Close()

	connB, _, err := wsConnect(t, server, "/api/companies/company-B/events/ws", nil)
	if err != nil {
		t.Fatalf("dial company-B: %v", err)
	}
	defer connB.Close()

	// Publish only to company-A.
	data, _ := json.Marshal(map[string]any{"type": "heartbeat.run.status"})
	hub.Publish("company-A", data)

	// company-A client should receive the event.
	msg, err := readWithTimeout(t, connA, 2*time.Second)
	if err != nil {
		t.Fatalf("company-A read: %v", err)
	}
	var got map[string]any
	_ = json.Unmarshal(msg, &got)
	if got["type"] != "heartbeat.run.status" {
		t.Errorf("company-A got unexpected type %q", got["type"])
	}

	// company-B client should NOT receive any message within the timeout.
	connB.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	_, _, err = connB.ReadMessage()
	if err == nil {
		t.Error("company-B received a message it should not have")
	}
}
