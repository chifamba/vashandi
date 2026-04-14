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

func setupDashboardTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&dashboard_test=1"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	for _, tbl := range []string{"activity_logs", "memory_operations", "budget_policies", "cost_events", "approvals", "issues", "agents", "heartbeat_runs"} {
		db.Exec("DROP TABLE IF EXISTS " + tbl)
	}
	db.Exec(`CREATE TABLE agents (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		name text NOT NULL,
		status text NOT NULL DEFAULT 'idle',
		adapter_type text NOT NULL DEFAULT 'process',
		adapter_config text NOT NULL DEFAULT '{}',
		runtime_config text NOT NULL DEFAULT '{}',
		budget_monthly_cents integer NOT NULL DEFAULT 0,
		spent_monthly_cents integer NOT NULL DEFAULT 0,
		permissions text NOT NULL DEFAULT '{}',
		created_at datetime,
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
	db.Exec(`CREATE TABLE approvals (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		type text NOT NULL DEFAULT 'generic',
		status text NOT NULL DEFAULT 'pending',
		payload text NOT NULL DEFAULT '{}',
		created_at datetime,
		updated_at datetime
	)`)
	db.Exec(`CREATE TABLE cost_events (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		agent_id text NOT NULL,
		provider text NOT NULL,
		biller text NOT NULL DEFAULT 'unknown',
		billing_type text NOT NULL DEFAULT 'unknown',
		model text NOT NULL,
		input_tokens integer NOT NULL DEFAULT 0,
		cached_input_tokens integer NOT NULL DEFAULT 0,
		output_tokens integer NOT NULL DEFAULT 0,
		cost_cents integer NOT NULL DEFAULT 0,
		amount real NOT NULL DEFAULT 0,
		occurred_at datetime,
		created_at datetime
	)`)
	db.Exec(`CREATE TABLE budget_policies (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		scope_type text NOT NULL,
		scope_id text NOT NULL,
		metric text NOT NULL DEFAULT 'billed_cents',
		window_kind text NOT NULL,
		amount integer NOT NULL DEFAULT 0,
		limit_amount real NOT NULL DEFAULT 0,
		warn_percent integer NOT NULL DEFAULT 80,
		hard_stop_enabled boolean NOT NULL DEFAULT 1,
		notify_enabled boolean NOT NULL DEFAULT 1,
		is_active boolean NOT NULL DEFAULT 1,
		created_at datetime,
		updated_at datetime
	)`)
	db.Exec(`CREATE TABLE memory_operations (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		success boolean NOT NULL DEFAULT 0,
		created_at datetime
	)`)
	db.Exec(`CREATE TABLE activity_logs (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		actor_type text NOT NULL DEFAULT 'system',
		actor_id text NOT NULL,
		action text NOT NULL,
		entity_type text NOT NULL,
		entity_id text NOT NULL,
		created_at datetime
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
		created_at datetime,
		updated_at datetime
	)`)
	return db
}

func TestDashboardHandler_CompanyScoping(t *testing.T) {
	db := setupDashboardTestDB(t)
	db.Exec("INSERT INTO agents (id, company_id, name, status) VALUES ('a1', 'comp-a', 'Agent1', 'active')")
	db.Exec("INSERT INTO agents (id, company_id, name, status) VALUES ('a2', 'comp-a', 'Agent2', 'paused')")
	db.Exec("INSERT INTO agents (id, company_id, name, status) VALUES ('a3', 'comp-b', 'Agent3', 'active')")
	db.Exec("INSERT INTO issues (id, company_id, title, status) VALUES ('i1', 'comp-a', 'Issue1', 'open')")
	db.Exec("INSERT INTO issues (id, company_id, title, status) VALUES ('i2', 'comp-a', 'Issue2', 'done')")
	db.Exec("INSERT INTO issues (id, company_id, title, status) VALUES ('i3', 'comp-a', 'Issue3', 'in_progress')")
	db.Exec("INSERT INTO approvals (id, company_id, type, status, payload) VALUES ('ap1', 'comp-a', 'run', 'pending', '{}')")

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/dashboard", DashboardHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-a/dashboard", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var summary DashboardSummary
	json.NewDecoder(w.Body).Decode(&summary)

	if summary.TotalAgents != 2 {
		t.Errorf("expected 2 total agents, got %d", summary.TotalAgents)
	}
	if summary.ActiveAgents != 1 {
		t.Errorf("expected 1 active agent, got %d", summary.ActiveAgents)
	}
	if summary.PausedAgents != 1 {
		t.Errorf("expected 1 paused agent, got %d", summary.PausedAgents)
	}
	if summary.TotalIssues != 3 {
		t.Errorf("expected 3 total issues, got %d", summary.TotalIssues)
	}
	if summary.OpenIssues != 1 {
		t.Errorf("expected 1 open issue, got %d", summary.OpenIssues)
	}
	if summary.InProgressIssues != 1 {
		t.Errorf("expected 1 in-progress issue, got %d", summary.InProgressIssues)
	}
	if summary.DoneIssues != 1 {
		t.Errorf("expected 1 done issue, got %d", summary.DoneIssues)
	}
	if summary.PendingApprovals != 1 {
		t.Errorf("expected 1 pending approval, got %d", summary.PendingApprovals)
	}
}

func TestDashboardHandler_MissingCompanyID(t *testing.T) {
	db := setupDashboardTestDB(t)

	// Call handler directly without chi router → companyId will be empty
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	w := httptest.NewRecorder()

	DashboardHandler(db)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 when companyId is missing, got %d", w.Code)
	}
}

func TestDashboardHandler_EmptyCompany(t *testing.T) {
	db := setupDashboardTestDB(t)

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/dashboard", DashboardHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/empty-comp/dashboard", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var summary DashboardSummary
	json.NewDecoder(w.Body).Decode(&summary)
	if summary.TotalAgents != 0 {
		t.Errorf("expected 0 agents, got %d", summary.TotalAgents)
	}
	if summary.TotalIssues != 0 {
		t.Errorf("expected 0 issues, got %d", summary.TotalIssues)
	}
}

func TestPlatformMetricsHandler(t *testing.T) {
	db := setupDashboardTestDB(t)
	db.Exec("INSERT INTO agents (id, company_id, name, status) VALUES ('a1', 'comp-a', 'Agent1', 'active')")
	db.Exec("INSERT INTO agents (id, company_id, name, status) VALUES ('a2', 'comp-b', 'Agent2', 'idle')")
	db.Exec("INSERT INTO heartbeat_runs (id, company_id, agent_id, status, task_id) VALUES ('r1', 'comp-a', 'a1', 'active', 't1')")
	db.Exec("INSERT INTO heartbeat_runs (id, company_id, agent_id, status, task_id) VALUES ('r2', 'comp-a', 'a1', 'error', 't2')")
	db.Exec("INSERT INTO heartbeat_runs (id, company_id, agent_id, status, task_id) VALUES ('r3', 'comp-a', 'a1', 'completed', 't3')")

	req := httptest.NewRequest(http.MethodGet, "/platform/metrics", nil)
	w := httptest.NewRecorder()

	PlatformMetricsHandler(db)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var metrics PlatformMetrics
	json.NewDecoder(w.Body).Decode(&metrics)

	if metrics.TotalAgents != 2 {
		t.Errorf("expected 2 total agents, got %d", metrics.TotalAgents)
	}
	if metrics.ActiveRuns != 1 {
		t.Errorf("expected 1 active run, got %d", metrics.ActiveRuns)
	}
}
