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

func setupSidebarBadgesTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&sidebar_badges_test=1"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	for _, tbl := range []string{"issues", "heartbeat_runs", "join_requests", "approvals"} {
		db.Exec("DROP TABLE IF EXISTS " + tbl)
	}
	db.Exec(`CREATE TABLE approvals (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		type text NOT NULL DEFAULT 'generic',
		status text NOT NULL DEFAULT 'pending',
		payload text NOT NULL DEFAULT '{}',
		created_at datetime,
		updated_at datetime
	)`)
	db.Exec(`CREATE TABLE join_requests (
		id text PRIMARY KEY,
		invite_id text NOT NULL,
		company_id text NOT NULL,
		request_type text NOT NULL,
		status text NOT NULL DEFAULT 'pending_approval',
		request_ip text NOT NULL DEFAULT '0.0.0.0',
		created_at datetime,
		updated_at datetime
	)`)
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
	db.Exec(`CREATE TABLE issues (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		title text NOT NULL,
		status text NOT NULL DEFAULT 'backlog',
		priority text NOT NULL DEFAULT 'medium',
		origin_kind text NOT NULL DEFAULT 'manual',
		request_depth integer NOT NULL DEFAULT 0,
		created_at datetime,
		updated_at datetime
	)`)
	return db
}

func TestSidebarBadgesHandler(t *testing.T) {
	db := setupSidebarBadgesTestDB(t)

	// Create test data for comp-a
	db.Exec("INSERT INTO approvals (id, company_id, type, status, payload) VALUES ('a1', 'comp-a', 'run', 'pending', '{}')")
	db.Exec("INSERT INTO approvals (id, company_id, type, status, payload) VALUES ('a2', 'comp-a', 'run', 'approved', '{}')")
	db.Exec("INSERT INTO join_requests (id, invite_id, company_id, request_type, status) VALUES ('jr1', 'inv-1', 'comp-a', 'agent', 'pending_approval')")
	db.Exec("INSERT INTO heartbeat_runs (id, company_id, agent_id, status, task_id, created_at) VALUES ('r1', 'comp-a', 'agent-1', 'failed', 't1', datetime('now'))")
	db.Exec("INSERT INTO issues (id, company_id, title, status) VALUES ('i1', 'comp-a', 'Open Issue', 'open')")
	db.Exec("INSERT INTO issues (id, company_id, title, status) VALUES ('i2', 'comp-a', 'Done Issue', 'done')")
	db.Exec("INSERT INTO issues (id, company_id, title, status) VALUES ('i3', 'comp-a', 'In Progress', 'in_progress')")

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/sidebar-badges", SidebarBadgesHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-a/sidebar-badges", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var badges map[string]int64
	json.NewDecoder(w.Body).Decode(&badges)

	if badges["pendingApprovals"] != 1 {
		t.Errorf("expected 1 pending approval, got %d", badges["pendingApprovals"])
	}
	if badges["pendingJoinRequests"] != 1 {
		t.Errorf("expected 1 pending join request, got %d", badges["pendingJoinRequests"])
	}
	if badges["failedRuns"] != 1 {
		t.Errorf("expected 1 failed run, got %d", badges["failedRuns"])
	}
	// openIssues excludes 'done' and 'cancelled'
	if badges["openIssues"] != 2 {
		t.Errorf("expected 2 open issues (open + in_progress), got %d", badges["openIssues"])
	}
}

func TestSidebarBadgesHandler_EmptyCompany(t *testing.T) {
	db := setupSidebarBadgesTestDB(t)

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/sidebar-badges", SidebarBadgesHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/empty-comp/sidebar-badges", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var badges map[string]int64
	json.NewDecoder(w.Body).Decode(&badges)

	if badges["pendingApprovals"] != 0 {
		t.Errorf("expected 0 pending approvals, got %d", badges["pendingApprovals"])
	}
	if badges["openIssues"] != 0 {
		t.Errorf("expected 0 open issues, got %d", badges["openIssues"])
	}
}
