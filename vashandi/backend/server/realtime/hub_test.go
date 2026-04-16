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

	msg, err := readWithTimeout(t, conn, 5*time.Second)
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
	msg, err := readWithTimeout(t, connA, 5*time.Second)
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

// TestLiveEventsHandler_GlobalChannel verifies that a board user can subscribe
// to the global channel ("*") and receives events published via PublishGlobal.
func TestLiveEventsHandler_GlobalChannel(t *testing.T) {
	db := setupHubTestDB(t)
	hub, router := newTestRouter(db, "local_trusted")

	server := httptest.NewServer(router)
	defer server.Close()

	// Subscribe to the global channel
	conn, resp, err := wsConnect(t, server, "/api/companies/*/events/ws", nil)
	if err != nil {
		t.Fatalf("dial global: %v (status %v)", err, resp)
	}
	defer conn.Close()

	// Also subscribe a company-specific client to verify isolation
	connCompany, _, err := wsConnect(t, server, "/api/companies/company-1/events/ws", nil)
	if err != nil {
		t.Fatalf("dial company-1: %v", err)
	}
	defer connCompany.Close()

	// Publish a global event
	globalEvent := map[string]any{"type": "plugin.ui.updated", "global": true}
	data, _ := json.Marshal(globalEvent)
	hub.PublishGlobal(data)

	// Global subscriber should receive the event.
	msg, err := readWithTimeout(t, conn, 5*time.Second)
	if err != nil {
		t.Fatalf("global read: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(msg, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["type"] != "plugin.ui.updated" {
		t.Errorf("global subscriber got unexpected type %q", got["type"])
	}

	// Company-specific subscriber should NOT receive the global event.
	connCompany.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	_, _, err = connCompany.ReadMessage()
	if err == nil {
		t.Error("company-1 subscriber received a global event it should not have")
	}
}

// TestLiveEventsHandler_GlobalChannelIsolatedFromCompany verifies that events
// published to a company do not reach global subscribers.
func TestLiveEventsHandler_GlobalChannelIsolatedFromCompany(t *testing.T) {
	db := setupHubTestDB(t)
	hub, router := newTestRouter(db, "local_trusted")

	server := httptest.NewServer(router)
	defer server.Close()

	// Subscribe to the global channel
	connGlobal, _, err := wsConnect(t, server, "/api/companies/*/events/ws", nil)
	if err != nil {
		t.Fatalf("dial global: %v", err)
	}
	defer connGlobal.Close()

	// Subscribe to a company
	connCompany, _, err := wsConnect(t, server, "/api/companies/company-1/events/ws", nil)
	if err != nil {
		t.Fatalf("dial company-1: %v", err)
	}
	defer connCompany.Close()

	// Publish a company-scoped event
	data, _ := json.Marshal(map[string]any{"type": "agent.status"})
	hub.Publish("company-1", data)

	// Company subscriber should receive it
	msg, err := readWithTimeout(t, connCompany, 5*time.Second)
	if err != nil {
		t.Fatalf("company read: %v", err)
	}
	var got map[string]any
	_ = json.Unmarshal(msg, &got)
	if got["type"] != "agent.status" {
		t.Errorf("company subscriber got unexpected type %q", got["type"])
	}

	// Global subscriber should NOT receive company-scoped events
	connGlobal.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	_, _, err = connGlobal.ReadMessage()
	if err == nil {
		t.Error("global subscriber received a company-scoped event it should not have")
	}
}

// TestLiveEvent_Envelope verifies that PublishEvent wraps the payload in a
// LiveEvent envelope with the expected fields.
func TestLiveEvent_Envelope(t *testing.T) {
	hub := NewHub()

	// Create a test channel to receive the event
	ch, cancel := hub.Subscribe("company-1")
	defer cancel()

	// Publish an event using the envelope method
	payload := json.RawMessage(`{"runId":"run-123","status":"running"}`)
	hub.PublishEvent("company-1", "heartbeat.run.status", payload)

	// Read the event from the channel
	select {
	case msg := <-ch:
		var event LiveEvent
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatalf("unmarshal LiveEvent: %v", err)
		}

		// Verify envelope fields
		if event.ID <= 0 {
			t.Errorf("expected positive event ID, got %d", event.ID)
		}
		if event.CompanyID != "company-1" {
			t.Errorf("expected companyId 'company-1', got %q", event.CompanyID)
		}
		if event.Type != "heartbeat.run.status" {
			t.Errorf("expected type 'heartbeat.run.status', got %q", event.Type)
		}
		if event.CreatedAt == "" {
			t.Error("expected non-empty createdAt")
		}
		// Verify createdAt is a valid RFC3339 timestamp
		if _, err := time.Parse(time.RFC3339, event.CreatedAt); err != nil {
			t.Errorf("createdAt is not valid RFC3339: %v", err)
		}

		// Verify payload
		var payloadData map[string]interface{}
		if err := json.Unmarshal(event.Payload, &payloadData); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		if payloadData["runId"] != "run-123" {
			t.Errorf("expected payload runId 'run-123', got %v", payloadData["runId"])
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

// TestLiveEvent_AutoIncrementingID verifies that event IDs are auto-incrementing.
func TestLiveEvent_AutoIncrementingID(t *testing.T) {
	hub := NewHub()

	// Create a test channel to receive events
	ch, cancel := hub.Subscribe("company-1")
	defer cancel()

	// Publish multiple events
	for i := 0; i < 3; i++ {
		hub.PublishEvent("company-1", "test.event", json.RawMessage(`{}`))
	}

	// Collect event IDs
	var ids []int64
	for i := 0; i < 3; i++ {
		select {
		case msg := <-ch:
			var event LiveEvent
			if err := json.Unmarshal(msg, &event); err != nil {
				t.Fatalf("unmarshal event %d: %v", i, err)
			}
			ids = append(ids, event.ID)
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for event %d", i)
		}
	}

	// Verify IDs are monotonically increasing
	for i := 1; i < len(ids); i++ {
		if ids[i] <= ids[i-1] {
			t.Errorf("event IDs not monotonically increasing: %v", ids)
		}
	}
}

// TestPublishGlobalEvent_Envelope verifies that PublishGlobalEvent wraps the
// payload in a LiveEvent envelope with companyId set to "*".
func TestPublishGlobalEvent_Envelope(t *testing.T) {
	hub := NewHub()

	// Create a global test channel
	ch, cancel := hub.SubscribeGlobal()
	defer cancel()

	// Publish a global event using the envelope method
	payload := json.RawMessage(`{"message":"broadcast"}`)
	hub.PublishGlobalEvent("plugin.ui.updated", payload)

	// Read the event from the channel
	select {
	case msg := <-ch:
		var event LiveEvent
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatalf("unmarshal LiveEvent: %v", err)
		}

		// Verify envelope fields
		if event.ID <= 0 {
			t.Errorf("expected positive event ID, got %d", event.ID)
		}
		if event.CompanyID != "*" {
			t.Errorf("expected companyId '*', got %q", event.CompanyID)
		}
		if event.Type != "plugin.ui.updated" {
			t.Errorf("expected type 'plugin.ui.updated', got %q", event.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for global event")
	}
}
