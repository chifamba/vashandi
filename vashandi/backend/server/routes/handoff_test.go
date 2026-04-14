package routes

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupHandoffTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&handoff_test=1"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	for _, tbl := range []string{"heartbeat_runs", "issues", "agents", "companies"} {
		db.Exec("DROP TABLE IF EXISTS " + tbl)
	}
	db.Exec(`CREATE TABLE companies (
		id text PRIMARY KEY,
		name text NOT NULL,
		status text NOT NULL DEFAULT 'active',
		issue_prefix text NOT NULL DEFAULT 'PAP',
		issue_counter integer NOT NULL DEFAULT 0,
		budget_monthly_cents integer NOT NULL DEFAULT 0,
		spent_monthly_cents integer NOT NULL DEFAULT 0,
		require_board_approval_for_new_agents boolean NOT NULL DEFAULT 1,
		feedback_data_sharing_enabled boolean NOT NULL DEFAULT 0,
		is_archived boolean NOT NULL DEFAULT 0,
		created_at datetime,
		updated_at datetime
	)`)
	db.Exec(`CREATE TABLE agents (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		name text NOT NULL,
		role text NOT NULL DEFAULT 'general',
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
		project_id text,
		project_workspace_id text,
		goal_id text,
		parent_id text,
		title text NOT NULL,
		description text,
		status text NOT NULL DEFAULT 'backlog',
		priority text NOT NULL DEFAULT 'medium',
		assignee_agent_id text,
		assignee_user_id text,
		checkout_run_id text,
		execution_run_id text,
		execution_agent_name_key text,
		execution_locked_at datetime,
		created_by_agent_id text,
		created_by_user_id text,
		issue_number integer,
		identifier text,
		origin_kind text NOT NULL DEFAULT 'manual',
		origin_id text,
		origin_run_id text,
		request_depth integer NOT NULL DEFAULT 0,
		billing_code text,
		assignee_adapter_overrides text DEFAULT '{}',
		execution_workspace_id text,
		execution_workspace_preference text,
		execution_workspace_settings text DEFAULT '{}',
		started_at datetime,
		completed_at datetime,
		cancelled_at datetime,
		hidden_at datetime,
		created_at datetime,
		updated_at datetime
	)`)
	db.Exec(`CREATE TABLE heartbeat_runs (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		agent_id text NOT NULL,
		task_id text NOT NULL DEFAULT '',
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
		context_snapshot text DEFAULT '{}',
		handoff_markdown text,
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
		created_at datetime,
		updated_at datetime
	)`)

	db.Exec("INSERT INTO companies (id, name) VALUES ('comp-a', 'Alpha')")
	db.Exec("INSERT INTO agents (id, company_id, name) VALUES ('agent-1', 'comp-a', 'Worker')")
	db.Exec("INSERT INTO agents (id, company_id, name) VALUES ('agent-2', 'comp-a', 'Reviewer')")
	return db
}

func TestHandoffIssueHandler_Success(t *testing.T) {
	db := setupHandoffTestDB(t)
	db.Exec("INSERT INTO issues (id, company_id, title, assignee_agent_id, status) VALUES ('i1', 'comp-a', 'Task', 'agent-1', 'in_progress')")
	db.Exec("INSERT INTO heartbeat_runs (id, company_id, agent_id, task_id, status, context_snapshot) VALUES ('run-1', 'comp-a', 'agent-1', 'i1', 'running', '{\"key\":\"val\"}')")

	router := chi.NewRouter()
	router.Post("/issues/{id}/handoff", HandoffIssueHandler(db))

	body := `{"targetAgentId":"agent-2","handoffMarkdown":"## Context\nPlease review the PR."}`
	req := httptest.NewRequest(http.MethodPost, "/issues/i1/handoff", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	var result map[string]string
	json.NewDecoder(w.Body).Decode(&result)
	if result["status"] != "handoff_completed" {
		t.Errorf("expected status handoff_completed, got %q", result["status"])
	}
	// Note: newRunId may be empty on SQLite since gen_random_uuid() is PostgreSQL-only.
	// We verify the key exists in the response.
	if _, ok := result["newRunId"]; !ok {
		t.Error("expected newRunId key in response")
	}

	// Verify issue assignee was updated
	var issue struct {
		AssigneeAgentID *string `gorm:"column:assignee_agent_id"`
	}
	db.Raw("SELECT assignee_agent_id FROM issues WHERE id = ?", "i1").Scan(&issue)
	if issue.AssigneeAgentID == nil || *issue.AssigneeAgentID != "agent-2" {
		t.Errorf("expected assignee to be agent-2, got %v", issue.AssigneeAgentID)
	}
}

func TestHandoffIssueHandler_IssueNotFound(t *testing.T) {
	db := setupHandoffTestDB(t)

	router := chi.NewRouter()
	router.Post("/issues/{id}/handoff", HandoffIssueHandler(db))

	body := `{"targetAgentId":"agent-2","handoffMarkdown":"context"}`
	req := httptest.NewRequest(http.MethodPost, "/issues/nonexistent/handoff", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandoffIssueHandler_BadBody(t *testing.T) {
	db := setupHandoffTestDB(t)
	db.Exec("INSERT INTO issues (id, company_id, title, assignee_agent_id, status) VALUES ('i1', 'comp-a', 'Task', 'agent-1', 'in_progress')")

	router := chi.NewRouter()
	router.Post("/issues/{id}/handoff", HandoffIssueHandler(db))

	req := httptest.NewRequest(http.MethodPost, "/issues/i1/handoff", bytes.NewBufferString("bad json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandoffIssueHandler_HandoffMarkdownStored(t *testing.T) {
	db := setupHandoffTestDB(t)
	db.Exec("INSERT INTO issues (id, company_id, title, assignee_agent_id, status) VALUES ('i1', 'comp-a', 'Task', 'agent-1', 'in_progress')")
	db.Exec("INSERT INTO heartbeat_runs (id, company_id, agent_id, task_id, status) VALUES ('run-1', 'comp-a', 'agent-1', 'i1', 'running')")

	router := chi.NewRouter()
	router.Post("/issues/{id}/handoff", HandoffIssueHandler(db))

	body := `{"targetAgentId":"agent-2","handoffMarkdown":"review this PR"}`
	req := httptest.NewRequest(http.MethodPost, "/issues/i1/handoff", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Verify handoff markdown was stored on the original run
	var run struct {
		HandoffMarkdown *string `gorm:"column:handoff_markdown"`
	}
	db.Raw("SELECT handoff_markdown FROM heartbeat_runs WHERE id = ?", "run-1").Scan(&run)
	if run.HandoffMarkdown == nil || *run.HandoffMarkdown != "review this PR" {
		t.Errorf("expected handoff markdown on original run, got %v", run.HandoffMarkdown)
	}
}
