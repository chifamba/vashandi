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

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"github.com/chifamba/vashandi/vashandi/backend/server/services"
)

func setupIssuesRouteTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&issues_route_test=1"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	for _, tbl := range []string{"issue_comments", "issue_work_products", "labels", "activity_log", "issues", "projects", "agents", "companies"} {
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
		brand_color text,
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
	db.Exec(`CREATE TABLE projects (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		name text NOT NULL,
		description text,
		status text NOT NULL DEFAULT 'active',
		identifier_prefix text,
		repo_url text,
		default_branch text,
		is_archived boolean NOT NULL DEFAULT 0,
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
	db.Exec(`CREATE TABLE issue_comments (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		issue_id text NOT NULL,
		author_agent_id text,
		author_user_id text,
		created_by_run_id text,
		body text NOT NULL,
		created_at datetime,
		updated_at datetime
	)`)
	db.Exec(`CREATE TABLE issue_work_products (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		project_id text,
		issue_id text NOT NULL,
		execution_workspace_id text,
		runtime_service_id text,
		type text NOT NULL,
		provider text NOT NULL,
		external_id text,
		title text NOT NULL,
		url text,
		status text NOT NULL,
		review_state text NOT NULL DEFAULT 'none',
		is_primary boolean NOT NULL DEFAULT 0,
		health_status text NOT NULL DEFAULT 'unknown',
		summary text,
		metadata text DEFAULT '{}',
		created_by_run_id text,
		created_at datetime,
		updated_at datetime
	)`)
	db.Exec(`CREATE TABLE labels (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		name text NOT NULL,
		color text NOT NULL,
		created_at datetime,
		updated_at datetime
	)`)
	db.Exec(`CREATE TABLE activity_log (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		actor_type text NOT NULL DEFAULT 'system',
		actor_id text NOT NULL DEFAULT 'system',
		action text NOT NULL,
		entity_type text NOT NULL DEFAULT '',
		entity_id text NOT NULL DEFAULT '',
		details text DEFAULT '{}',
		agent_id text,
		run_id text,
		created_at datetime
	)`)
	// Seed data
	db.Exec("INSERT INTO companies (id, name, issue_prefix, issue_counter) VALUES ('comp-a', 'Alpha', 'ALP', 5)")
	db.Exec("INSERT INTO companies (id, name, issue_prefix, issue_counter) VALUES ('comp-b', 'Beta', 'BET', 0)")
	return db
}

func newIssueRoutes(t *testing.T, db *gorm.DB) *IssueRoutes {
	t.Helper()
	actSvc := services.NewActivityService(db)
	return NewIssueRoutes(db, actSvc)
}

// ---------- ListIssuesHandler ----------

func TestListIssuesHandler_CompanyScoping(t *testing.T) {
	db := setupIssuesRouteTestDB(t)
	db.Exec("INSERT INTO issues (id, company_id, title, status, priority, origin_kind) VALUES ('i1', 'comp-a', 'Issue A1', 'backlog', 'medium', 'manual')")
	db.Exec("INSERT INTO issues (id, company_id, title, status, priority, origin_kind) VALUES ('i2', 'comp-b', 'Issue B1', 'backlog', 'medium', 'manual')")

	ir := newIssueRoutes(t, db)
	router := chi.NewRouter()
	router.Get("/companies/{companyId}/issues", ir.ListIssuesHandler)

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-a/issues", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	var issues []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&issues)
	if len(issues) != 1 {
		t.Errorf("expected 1 issue for comp-a, got %d", len(issues))
	}
}

func TestListIssuesHandler_StatusFilter(t *testing.T) {
	db := setupIssuesRouteTestDB(t)
	db.Exec("INSERT INTO issues (id, company_id, title, status, priority, origin_kind) VALUES ('i1', 'comp-a', 'Backlog', 'backlog', 'medium', 'manual')")
	db.Exec("INSERT INTO issues (id, company_id, title, status, priority, origin_kind) VALUES ('i2', 'comp-a', 'In Progress', 'in_progress', 'medium', 'manual')")

	ir := newIssueRoutes(t, db)
	router := chi.NewRouter()
	router.Get("/companies/{companyId}/issues", ir.ListIssuesHandler)

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-a/issues?status=in_progress", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var issues []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&issues)
	if len(issues) != 1 {
		t.Errorf("expected 1 issue with status in_progress, got %d", len(issues))
	}
}

// ---------- GetIssueHandler ----------

func TestGetIssueHandler_Found(t *testing.T) {
	db := setupIssuesRouteTestDB(t)
	db.Exec("INSERT INTO issues (id, company_id, title, status, priority, origin_kind) VALUES ('i1', 'comp-a', 'My Issue', 'backlog', 'medium', 'manual')")

	ir := newIssueRoutes(t, db)
	router := chi.NewRouter()
	router.Get("/issues/{id}", ir.GetIssueHandler)

	req := httptest.NewRequest(http.MethodGet, "/issues/i1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}
}

func TestGetIssueHandler_NotFound(t *testing.T) {
	db := setupIssuesRouteTestDB(t)

	ir := newIssueRoutes(t, db)
	router := chi.NewRouter()
	router.Get("/issues/{id}", ir.GetIssueHandler)

	req := httptest.NewRequest(http.MethodGet, "/issues/nonexistent", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ---------- CreateIssueHandler ----------

func TestCreateIssueHandler_Success(t *testing.T) {
	db := setupIssuesRouteTestDB(t)

	ir := newIssueRoutes(t, db)
	router := chi.NewRouter()
	router.Post("/companies/{companyId}/issues", ir.CreateIssueHandler)

	body := `{"title":"New task","priority":"high"}`
	req := httptest.NewRequest(http.MethodPost, "/companies/comp-a/issues", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", w.Code, w.Body.String())
	}

	var created map[string]interface{}
	json.NewDecoder(w.Body).Decode(&created)
	if created["CompanyID"] != "comp-a" {
		t.Errorf("expected CompanyID comp-a, got %v", created["CompanyID"])
	}
	if created["Status"] != "backlog" {
		t.Errorf("expected default status backlog, got %v", created["Status"])
	}
}

func TestCreateIssueHandler_BadBody(t *testing.T) {
	db := setupIssuesRouteTestDB(t)

	ir := newIssueRoutes(t, db)
	router := chi.NewRouter()
	router.Post("/companies/{companyId}/issues", ir.CreateIssueHandler)

	req := httptest.NewRequest(http.MethodPost, "/companies/comp-a/issues", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad body, got %d", w.Code)
	}
}

// ---------- UpdateIssueHandler ----------

func TestUpdateIssueHandler_Success(t *testing.T) {
	db := setupIssuesRouteTestDB(t)
	db.Exec("INSERT INTO issues (id, company_id, title, status, priority, origin_kind) VALUES ('i1', 'comp-a', 'Old Title', 'backlog', 'medium', 'manual')")

	ir := newIssueRoutes(t, db)
	router := chi.NewRouter()
	router.Put("/issues/{id}", ir.UpdateIssueHandler)

	body := `{"title":"New Title"}`
	req := httptest.NewRequest(http.MethodPut, "/issues/i1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestUpdateIssueHandler_NotFound(t *testing.T) {
	db := setupIssuesRouteTestDB(t)

	ir := newIssueRoutes(t, db)
	router := chi.NewRouter()
	router.Put("/issues/{id}", ir.UpdateIssueHandler)

	body := `{"title":"New Title"}`
	req := httptest.NewRequest(http.MethodPut, "/issues/nonexistent", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ---------- DeleteIssueHandler ----------

func TestDeleteIssueHandler_Success(t *testing.T) {
	db := setupIssuesRouteTestDB(t)
	db.Exec("INSERT INTO issues (id, company_id, title, status, priority, origin_kind) VALUES ('i1', 'comp-a', 'To Delete', 'backlog', 'medium', 'manual')")

	ir := newIssueRoutes(t, db)
	router := chi.NewRouter()
	router.Delete("/issues/{id}", ir.DeleteIssueHandler)

	req := httptest.NewRequest(http.MethodDelete, "/issues/i1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}

	// Verify hidden_at was set
	var issue models.Issue
	db.First(&issue, "id = ?", "i1")
	if issue.HiddenAt == nil {
		t.Error("expected hidden_at to be set after delete")
	}
}

func TestDeleteIssueHandler_NotFound(t *testing.T) {
	db := setupIssuesRouteTestDB(t)

	ir := newIssueRoutes(t, db)
	router := chi.NewRouter()
	router.Delete("/issues/{id}", ir.DeleteIssueHandler)

	req := httptest.NewRequest(http.MethodDelete, "/issues/nonexistent", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ---------- TransitionIssueHandler ----------

func TestTransitionIssueHandler_Success(t *testing.T) {
	db := setupIssuesRouteTestDB(t)
	db.Exec("INSERT INTO issues (id, company_id, title, status, priority, origin_kind) VALUES ('i1', 'comp-a', 'Task', 'backlog', 'medium', 'manual')")

	ir := newIssueRoutes(t, db)
	router := chi.NewRouter()
	router.Post("/issues/{id}/transition", ir.TransitionIssueHandler)

	body := `{"status":"in_progress"}`
	req := httptest.NewRequest(http.MethodPost, "/issues/i1/transition?companyId=comp-a", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	var result map[string]interface{}
	json.NewDecoder(w.Body).Decode(&result)
	if result["Status"] != "in_progress" {
		t.Errorf("expected status in_progress, got %v", result["Status"])
	}
}

func TestTransitionIssueHandler_BadBody(t *testing.T) {
	db := setupIssuesRouteTestDB(t)
	db.Exec("INSERT INTO issues (id, company_id, title, status, priority, origin_kind) VALUES ('i1', 'comp-a', 'Task', 'backlog', 'medium', 'manual')")

	ir := newIssueRoutes(t, db)
	router := chi.NewRouter()
	router.Post("/issues/{id}/transition", ir.TransitionIssueHandler)

	req := httptest.NewRequest(http.MethodPost, "/issues/i1/transition?companyId=comp-a", bytes.NewBufferString("bad"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ---------- AddIssueCommentHandler & ListIssueCommentsHandler ----------

func TestIssueComments_AddAndList(t *testing.T) {
	db := setupIssuesRouteTestDB(t)
	db.Exec("INSERT INTO issues (id, company_id, title, status, priority, origin_kind) VALUES ('i1', 'comp-a', 'Task', 'backlog', 'medium', 'manual')")

	ir := newIssueRoutes(t, db)
	router := chi.NewRouter()
	router.Post("/issues/{id}/comments", ir.AddIssueCommentHandler)
	router.Get("/issues/{id}/comments", ir.ListIssueCommentsHandler)

	// Add comment
	body := `{"body":"Hello world"}`
	req := httptest.NewRequest(http.MethodPost, "/issues/i1/comments", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 on add comment, got %d; body: %s", w.Code, w.Body.String())
	}

	// List comments
	req2 := httptest.NewRequest(http.MethodGet, "/issues/i1/comments", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 on list comments, got %d", w2.Code)
	}

	var comments []map[string]interface{}
	json.NewDecoder(w2.Body).Decode(&comments)
	if len(comments) != 1 {
		t.Errorf("expected 1 comment, got %d", len(comments))
	}
}

func TestAddIssueCommentHandler_IssueNotFound(t *testing.T) {
	db := setupIssuesRouteTestDB(t)

	ir := newIssueRoutes(t, db)
	router := chi.NewRouter()
	router.Post("/issues/{id}/comments", ir.AddIssueCommentHandler)

	body := `{"body":"Hello"}`
	req := httptest.NewRequest(http.MethodPost, "/issues/nonexistent/comments", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ---------- CreateWorkProduct & ListWorkProducts ----------

func TestIssueWorkProducts_CreateAndList(t *testing.T) {
	db := setupIssuesRouteTestDB(t)
	db.Exec("INSERT INTO issues (id, company_id, title, status, priority, origin_kind) VALUES ('i1', 'comp-a', 'Task', 'backlog', 'medium', 'manual')")

	ir := newIssueRoutes(t, db)
	router := chi.NewRouter()
	router.Post("/issues/{id}/work-products", ir.CreateWorkProductHandler)
	router.Get("/issues/{id}/work-products", ir.ListWorkProductsHandler)

	// Create
	body := `{"type":"pull_request","provider":"github","title":"PR #1","status":"open"}`
	req := httptest.NewRequest(http.MethodPost, "/issues/i1/work-products", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", w.Code, w.Body.String())
	}

	// List
	req2 := httptest.NewRequest(http.MethodGet, "/issues/i1/work-products", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}

	var wps []map[string]interface{}
	json.NewDecoder(w2.Body).Decode(&wps)
	if len(wps) != 1 {
		t.Errorf("expected 1 work product, got %d", len(wps))
	}
}

// ---------- BulkUpdateIssuesHandler ----------

func TestBulkUpdateIssuesHandler_UpdateMultiple(t *testing.T) {
	db := setupIssuesRouteTestDB(t)
	db.Exec("INSERT INTO issues (id, company_id, title, status, priority, origin_kind) VALUES ('i1', 'comp-a', 'T1', 'backlog', 'low', 'manual')")
	db.Exec("INSERT INTO issues (id, company_id, title, status, priority, origin_kind) VALUES ('i2', 'comp-a', 'T2', 'backlog', 'low', 'manual')")
	db.Exec("INSERT INTO issues (id, company_id, title, status, priority, origin_kind) VALUES ('i3', 'comp-b', 'T3', 'backlog', 'low', 'manual')")

	ir := newIssueRoutes(t, db)
	router := chi.NewRouter()
	router.Patch("/companies/{companyId}/issues/bulk", ir.BulkUpdateIssuesHandler)

	body := `{"ids":["i1","i2","i3"],"update":{"priority":"high"}}`
	req := httptest.NewRequest(http.MethodPatch, "/companies/comp-a/issues/bulk", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	var result map[string]interface{}
	json.NewDecoder(w.Body).Decode(&result)
	// Should only update 2 issues (comp-a scoping)
	updated := result["updated"]
	if updated != float64(2) {
		t.Errorf("expected 2 updated (company scoped), got %v", updated)
	}
}

func TestBulkUpdateIssuesHandler_EmptyIDs(t *testing.T) {
	db := setupIssuesRouteTestDB(t)

	ir := newIssueRoutes(t, db)
	router := chi.NewRouter()
	router.Patch("/companies/{companyId}/issues/bulk", ir.BulkUpdateIssuesHandler)

	body := `{"ids":[],"update":{"priority":"high"}}`
	req := httptest.NewRequest(http.MethodPatch, "/companies/comp-a/issues/bulk", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result map[string]interface{}
	json.NewDecoder(w.Body).Decode(&result)
	if result["updated"] != float64(0) {
		t.Errorf("expected 0 updated, got %v", result["updated"])
	}
}

// ---------- ReleaseIssueHandler ----------

func TestReleaseIssueHandler_Success(t *testing.T) {
	db := setupIssuesRouteTestDB(t)
	db.Exec("INSERT INTO issues (id, company_id, title, status, priority, origin_kind, checkout_run_id, execution_locked_at) VALUES ('i1', 'comp-a', 'Task', 'in_progress', 'medium', 'manual', 'run-1', datetime('now'))")

	ir := newIssueRoutes(t, db)
	router := chi.NewRouter()
	router.Post("/issues/{id}/release", ir.ReleaseIssueHandler)

	req := httptest.NewRequest(http.MethodPost, "/issues/i1/release", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestReleaseIssueHandler_NotFound(t *testing.T) {
	db := setupIssuesRouteTestDB(t)

	ir := newIssueRoutes(t, db)
	router := chi.NewRouter()
	router.Post("/issues/{id}/release", ir.ReleaseIssueHandler)

	req := httptest.NewRequest(http.MethodPost, "/issues/nonexistent/release", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ---------- ListAllIssuesHandler ----------

func TestListAllIssuesHandler_ReturnsAllCompanies(t *testing.T) {
	db := setupIssuesRouteTestDB(t)
	db.Exec("INSERT INTO issues (id, company_id, title, status, priority, origin_kind) VALUES ('i1', 'comp-a', 'A1', 'backlog', 'medium', 'manual')")
	db.Exec("INSERT INTO issues (id, company_id, title, status, priority, origin_kind) VALUES ('i2', 'comp-b', 'B1', 'backlog', 'medium', 'manual')")

	ir := newIssueRoutes(t, db)
	router := chi.NewRouter()
	router.Get("/admin/issues", ir.ListAllIssuesHandler)

	req := httptest.NewRequest(http.MethodGet, "/admin/issues", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var issues []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&issues)
	if len(issues) != 2 {
		t.Errorf("expected 2 issues (all companies), got %d", len(issues))
	}
}

func TestListAllIssuesHandler_StatusFilter(t *testing.T) {
	db := setupIssuesRouteTestDB(t)
	db.Exec("INSERT INTO issues (id, company_id, title, status, priority, origin_kind) VALUES ('i1', 'comp-a', 'A1', 'backlog', 'medium', 'manual')")
	db.Exec("INSERT INTO issues (id, company_id, title, status, priority, origin_kind) VALUES ('i2', 'comp-a', 'A2', 'done', 'medium', 'manual')")

	ir := newIssueRoutes(t, db)
	router := chi.NewRouter()
	router.Get("/admin/issues", ir.ListAllIssuesHandler)

	req := httptest.NewRequest(http.MethodGet, "/admin/issues?status=done", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var issues []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&issues)
	if len(issues) != 1 {
		t.Errorf("expected 1 done issue, got %d", len(issues))
	}
}

// ---------- Labels ----------

func TestListIssueLabelsHandler_CompanyScoping(t *testing.T) {
	db := setupIssuesRouteTestDB(t)
	db.Exec("INSERT INTO labels (id, company_id, name, color) VALUES ('l1', 'comp-a', 'bug', '#ff0000')")
	db.Exec("INSERT INTO labels (id, company_id, name, color) VALUES ('l2', 'comp-b', 'feature', '#00ff00')")

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/labels", ListIssueLabelsHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-a/labels", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var labels []models.Label
	json.NewDecoder(w.Body).Decode(&labels)
	if len(labels) != 1 {
		t.Errorf("expected 1 label for comp-a, got %d", len(labels))
	}
}

func TestCreateLabelHandler_Success(t *testing.T) {
	db := setupIssuesRouteTestDB(t)

	router := chi.NewRouter()
	router.Post("/companies/{companyId}/labels", CreateLabelHandler(db))

	body := `{"name":"enhancement","color":"#0000ff"}`
	req := httptest.NewRequest(http.MethodPost, "/companies/comp-a/labels", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestDeleteLabelHandler(t *testing.T) {
	db := setupIssuesRouteTestDB(t)
	db.Exec("INSERT INTO labels (id, company_id, name, color) VALUES ('l1', 'comp-a', 'bug', '#ff0000')")

	router := chi.NewRouter()
	router.Delete("/labels/{labelId}", DeleteLabelHandler(db))

	req := httptest.NewRequest(http.MethodDelete, "/labels/l1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

// ---------- ContentType checks ----------

func TestListIssuesHandler_ContentType(t *testing.T) {
	db := setupIssuesRouteTestDB(t)

	ir := newIssueRoutes(t, db)
	router := chi.NewRouter()
	router.Get("/companies/{companyId}/issues", ir.ListIssuesHandler)

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-a/issues", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
}
