package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupHeartbeatTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&heartbeat_test=1"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.Exec("DROP TABLE IF EXISTS heartbeat_runs")
	db.Exec(`CREATE TABLE heartbeat_runs (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		agent_id text NOT NULL,
		invocation_source text NOT NULL DEFAULT 'on_demand',
		trigger_detail text,
		status text NOT NULL DEFAULT 'queued',
		started_at datetime,
		finished_at datetime,
		error text,
		wakeup_request_id text,
		exit_code integer,
		signal text,
		usage_json text DEFAULT '{}',
		result_json text DEFAULT '{}',
		session_id_before text,
		session_id_after text,
		log_store text,
		log_ref text,
		log_bytes integer,
		log_sha256 text,
		log_compressed boolean NOT NULL DEFAULT 0,
		stdout_excerpt text,
		stderr_excerpt text,
		error_code text,
		external_run_id text,
		process_pid integer,
		process_started_at datetime,
		retry_of_run_id text,
		process_loss_retry_count integer NOT NULL DEFAULT 0,
		context_snapshot text DEFAULT '{}',
		handoff_markdown text,
		task_id text,
		created_at datetime,
		updated_at datetime
	)`)
	return db
}

func TestListHeartbeatRunsHandler_CompanyScoping(t *testing.T) {
	db := setupHeartbeatTestDB(t)
	db.Exec("INSERT INTO heartbeat_runs (id, company_id, agent_id, status) VALUES ('r1', 'c1', 'a1', 'completed')")
	db.Exec("INSERT INTO heartbeat_runs (id, company_id, agent_id, status) VALUES ('r2', 'c2', 'a2', 'running')")

	router := chi.NewRouter()
	router.Get("/heartbeat-runs", ListHeartbeatRunsHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/heartbeat-runs?companyId=c1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var runs []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&runs)
	if len(runs) != 1 {
		t.Fatalf("expected 1 run for c1, got %d", len(runs))
	}
}

func TestListHeartbeatRunsHandler_MissingCompanyId(t *testing.T) {
	db := setupHeartbeatTestDB(t)

	router := chi.NewRouter()
	router.Get("/heartbeat-runs", ListHeartbeatRunsHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/heartbeat-runs", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing companyId, got %d", w.Code)
	}
}

func TestListHeartbeatRunsHandler_ContentType(t *testing.T) {
	db := setupHeartbeatTestDB(t)

	router := chi.NewRouter()
	router.Get("/heartbeat-runs", ListHeartbeatRunsHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/heartbeat-runs?companyId=c1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}
}

func TestListHeartbeatRunsHandler_OrderDescending(t *testing.T) {
	db := setupHeartbeatTestDB(t)
	db.Exec("INSERT INTO heartbeat_runs (id, company_id, agent_id, status, created_at) VALUES ('r1', 'c1', 'a1', 'completed', '2026-01-01 00:00:00')")
	db.Exec("INSERT INTO heartbeat_runs (id, company_id, agent_id, status, created_at) VALUES ('r2', 'c1', 'a1', 'running', '2026-01-02 00:00:00')")

	router := chi.NewRouter()
	router.Get("/heartbeat-runs", ListHeartbeatRunsHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/heartbeat-runs?companyId=c1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var runs []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&runs)
	if len(runs) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(runs))
	}
	// Most recent first
	if runs[0]["ID"] != "r2" {
		t.Errorf("expected most recent run first (r2), got %v", runs[0]["ID"])
	}
}

func TestListHeartbeatRunsHandler_Empty(t *testing.T) {
	db := setupHeartbeatTestDB(t)

	router := chi.NewRouter()
	router.Get("/heartbeat-runs", ListHeartbeatRunsHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/heartbeat-runs?companyId=c1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var runs []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&runs)
	if len(runs) != 0 {
		t.Fatalf("expected 0 runs, got %d", len(runs))
	}
}
