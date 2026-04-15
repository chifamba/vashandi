package routes

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// ---- shared test DB helpers ------------------------------------------------

func setupRealtimeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf(
		"file:realtime_%s?mode=memory&cache=shared",
		strings.NewReplacer("/", "_", " ", "_", ":", "_").Replace(t.Name()),
	)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.Exec("DROP TABLE IF EXISTS heartbeat_run_events")
	db.Exec("DROP TABLE IF EXISTS heartbeat_runs")
	db.Exec(`CREATE TABLE heartbeat_runs (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		agent_id text NOT NULL,
		status text NOT NULL DEFAULT 'queued',
		invocation_source text NOT NULL DEFAULT 'on_demand',
		task_id text NOT NULL DEFAULT '',
		log_compressed boolean NOT NULL DEFAULT 0,
		process_loss_retry_count integer NOT NULL DEFAULT 0,
		created_at datetime DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime
	)`)
	db.Exec(`CREATE TABLE heartbeat_run_events (
		id integer PRIMARY KEY AUTOINCREMENT,
		company_id text NOT NULL,
		run_id text NOT NULL,
		agent_id text NOT NULL,
		seq integer NOT NULL,
		event_type text NOT NULL,
		stream text,
		level text,
		color text,
		message text,
		payload text DEFAULT '{}',
		created_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)
	return db
}

// ---- ListHeartbeatRunEventsHandler tests -----------------------------------

func TestListHeartbeatRunEventsHandler_Basic(t *testing.T) {
	db := setupRealtimeTestDB(t)
	db.Exec("INSERT INTO heartbeat_runs (id, company_id, agent_id, status) VALUES ('run-1', 'comp-1', 'agent-1', 'completed')")
	db.Exec("INSERT INTO heartbeat_run_events (company_id, run_id, agent_id, seq, event_type, message) VALUES ('comp-1', 'run-1', 'agent-1', 1, 'log', 'hello')")
	db.Exec("INSERT INTO heartbeat_run_events (company_id, run_id, agent_id, seq, event_type, message) VALUES ('comp-1', 'run-1', 'agent-1', 2, 'log', 'world')")

	router := chi.NewRouter()
	router.Get("/heartbeat-runs/{runId}/events", ListHeartbeatRunEventsHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/heartbeat-runs/run-1/events", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var events []map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&events); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}
}

func TestListHeartbeatRunEventsHandler_NotFound(t *testing.T) {
	db := setupRealtimeTestDB(t)

	router := chi.NewRouter()
	router.Get("/heartbeat-runs/{runId}/events", ListHeartbeatRunEventsHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/heartbeat-runs/nonexistent/events", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestListHeartbeatRunEventsHandler_AfterSeq(t *testing.T) {
	db := setupRealtimeTestDB(t)
	db.Exec("INSERT INTO heartbeat_runs (id, company_id, agent_id, status) VALUES ('run-2', 'comp-1', 'agent-1', 'completed')")
	for i := 1; i <= 5; i++ {
		db.Exec("INSERT INTO heartbeat_run_events (company_id, run_id, agent_id, seq, event_type) VALUES ('comp-1', 'run-2', 'agent-1', ?, 'log')", i)
	}

	router := chi.NewRouter()
	router.Get("/heartbeat-runs/{runId}/events", ListHeartbeatRunEventsHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/heartbeat-runs/run-2/events?afterSeq=3", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var events []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&events) //nolint:errcheck
	if len(events) != 2 {
		t.Errorf("expected 2 events after seq 3 (seq 4 and 5), got %d", len(events))
	}
}

func TestListHeartbeatRunEventsHandler_Limit(t *testing.T) {
	db := setupRealtimeTestDB(t)
	db.Exec("INSERT INTO heartbeat_runs (id, company_id, agent_id, status) VALUES ('run-3', 'comp-1', 'agent-1', 'completed')")
	for i := 1; i <= 10; i++ {
		db.Exec("INSERT INTO heartbeat_run_events (company_id, run_id, agent_id, seq, event_type) VALUES ('comp-1', 'run-3', 'agent-1', ?, 'log')", i)
	}

	router := chi.NewRouter()
	router.Get("/heartbeat-runs/{runId}/events", ListHeartbeatRunEventsHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/heartbeat-runs/run-3/events?limit=3", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var events []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&events) //nolint:errcheck
	if len(events) != 3 {
		t.Errorf("expected 3 events with limit=3, got %d", len(events))
	}
}

func TestListHeartbeatRunEventsHandler_EmptyResult(t *testing.T) {
	db := setupRealtimeTestDB(t)
	db.Exec("INSERT INTO heartbeat_runs (id, company_id, agent_id, status) VALUES ('run-4', 'comp-1', 'agent-1', 'running')")

	router := chi.NewRouter()
	router.Get("/heartbeat-runs/{runId}/events", ListHeartbeatRunEventsHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/heartbeat-runs/run-4/events", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var events []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&events) //nolint:errcheck
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}

func TestListHeartbeatRunEventsHandler_ContentType(t *testing.T) {
	db := setupRealtimeTestDB(t)
	db.Exec("INSERT INTO heartbeat_runs (id, company_id, agent_id, status) VALUES ('run-5', 'comp-1', 'agent-1', 'running')")

	router := chi.NewRouter()
	router.Get("/heartbeat-runs/{runId}/events", ListHeartbeatRunEventsHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/heartbeat-runs/run-5/events", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected application/json content-type, got %q", ct)
	}
}

// ---- SidebarBadgesSSEHandler tests -----------------------------------------

// noopSubscribe returns a channel that never sends and a cancel function.
// Useful for testing that the SSE handler sends the initial badge push and
// then blocks until the client disconnects.
func noopSubscribe(_ string) (<-chan []byte, func()) {
	ch := make(chan []byte)
	return ch, func() {}
}

func TestSidebarBadgesSSEHandler_InitialPush(t *testing.T) {
	db := setupSidebarBadgesTestDB(t)
	db.Exec("INSERT INTO approvals (id, company_id, type, status, payload) VALUES ('a-sse-1', 'comp-sse', 'run', 'pending', '{}')")

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/sidebar-badges/stream", SidebarBadgesSSEHandler(db, noopSubscribe))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-sse/sidebar-badges/stream", nil)
	w := httptest.NewRecorder()

	// The handler blocks until context is cancelled. We use a ResponseRecorder
	// which implements http.Flusher via a no-op, so we can inspect the first
	// SSE data line that was written before the handler started blocking.
	done := make(chan struct{})
	go func() {
		defer close(done)
		router.ServeHTTP(w, req)
	}()

	// Give the handler a moment to write the initial event then cancel.
	time.Sleep(50 * time.Millisecond)
	req.Context() // ping — req is already cancelled by the test runner
	// Cancel via a derived context isn't needed because httptest.Request's
	// context is done when the test exits. For a deterministic check we
	// inspect what was written so far.
	body := w.Body.String()

	// The response must start with a valid SSE data line.
	if !strings.HasPrefix(body, "data: ") {
		t.Errorf("expected SSE data prefix, got %q", body)
	}

	// The data line must contain a JSON object with badge keys.
	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			payload := strings.TrimPrefix(line, "data: ")
			var badges map[string]int64
			if err := json.Unmarshal([]byte(payload), &badges); err != nil {
				t.Errorf("badge payload is not valid JSON: %v", err)
			}
			if _, ok := badges["pendingApprovals"]; !ok {
				t.Errorf("expected pendingApprovals key in badges, got %v", badges)
			}
			if badges["pendingApprovals"] != 1 {
				t.Errorf("expected 1 pending approval, got %d", badges["pendingApprovals"])
			}
			break
		}
	}
}

func TestSidebarBadgesSSEHandler_ContentType(t *testing.T) {
	db := setupSidebarBadgesTestDB(t)

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/sidebar-badges/stream", SidebarBadgesSSEHandler(db, noopSubscribe))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-ct/sidebar-badges/stream", nil)
	w := httptest.NewRecorder()

	go router.ServeHTTP(w, req)
	time.Sleep(30 * time.Millisecond)

	ct := w.Header().Get("Content-Type")
	if ct != "text/event-stream" {
		t.Errorf("expected text/event-stream, got %q", ct)
	}
}

func TestSidebarBadgesSSEHandler_HubEventTriggersPush(t *testing.T) {
	db := setupSidebarBadgesTestDB(t)

	// A subscribe function that lets us inject an event programmatically.
	eventCh := make(chan []byte, 1)
	subscribe := func(_ string) (<-chan []byte, func()) {
		return eventCh, func() {}
	}

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/sidebar-badges/stream", SidebarBadgesSSEHandler(db, subscribe))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-hub/sidebar-badges/stream", nil)
	w := httptest.NewRecorder()

	go router.ServeHTTP(w, req)
	time.Sleep(30 * time.Millisecond) // let initial push complete

	// Add an approval and inject a hub event.
	db.Exec("INSERT INTO approvals (id, company_id, type, status, payload) VALUES ('a-hub-1', 'comp-hub', 'run', 'pending', '{}')")
	eventCh <- []byte(`{"type":"heartbeat.run.status"}`)
	time.Sleep(50 * time.Millisecond)

	body := w.Body.String()
	lines := strings.Split(strings.TrimSpace(body), "\n")
	// Expect at least two "data: " lines (initial push + post-event push).
	dataLines := 0
	for _, l := range lines {
		if strings.HasPrefix(l, "data: ") {
			dataLines++
		}
	}
	if dataLines < 2 {
		t.Errorf("expected at least 2 SSE data lines (initial + after hub event), got %d\nbody: %q", dataLines, body)
	}
}
