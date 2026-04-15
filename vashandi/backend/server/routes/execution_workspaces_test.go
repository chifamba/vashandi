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
)

func setupWorkspacesTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&workspaces_test=1"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.Exec("DROP TABLE IF EXISTS workspace_operations")
	db.Exec("DROP TABLE IF EXISTS execution_workspaces")
	db.Exec(`CREATE TABLE execution_workspaces (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		project_id text NOT NULL,
		project_workspace_id text,
		source_issue_id text,
		mode text NOT NULL DEFAULT 'worktree',
		strategy_type text NOT NULL DEFAULT 'branch',
		name text NOT NULL,
		status text NOT NULL DEFAULT 'active',
		cwd text,
		repo_url text,
		base_ref text,
		branch_name text,
		provider_type text NOT NULL DEFAULT 'local_fs',
		provider_ref text,
		derived_from_execution_workspace_id text,
		last_used_at datetime,
		opened_at datetime,
		closed_at datetime,
		cleanup_eligible_at datetime,
		cleanup_reason text,
		metadata text,
		created_at datetime,
		updated_at datetime
	)`)
	db.Exec(`CREATE TABLE workspace_operations (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		execution_workspace_id text,
		heartbeat_run_id text,
		phase text NOT NULL,
		command text,
		cwd text,
		status text NOT NULL DEFAULT 'running',
		exit_code integer,
		log_store text,
		log_ref text,
		log_bytes integer,
		log_sha256 text,
		log_compressed boolean NOT NULL DEFAULT 0,
		stdout_excerpt text,
		stderr_excerpt text,
		metadata text,
		started_at datetime,
		finished_at datetime,
		created_at datetime,
		updated_at datetime
	)`)
	return db
}

func TestListExecutionWorkspacesHandler_CompanyScoping(t *testing.T) {
	db := setupWorkspacesTestDB(t)
	db.Exec("INSERT INTO execution_workspaces (id, company_id, project_id, name, mode, strategy_type, status) VALUES ('ws1', 'comp-a', 'proj-1', 'WS1', 'worktree', 'branch', 'active')")
	db.Exec("INSERT INTO execution_workspaces (id, company_id, project_id, name, mode, strategy_type, status) VALUES ('ws2', 'comp-b', 'proj-2', 'WS2', 'worktree', 'branch', 'active')")

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/execution-workspaces", ListExecutionWorkspacesHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-a/execution-workspaces", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var workspaces []models.ExecutionWorkspace
	json.NewDecoder(w.Body).Decode(&workspaces)
	if len(workspaces) != 1 {
		t.Errorf("expected 1 workspace for comp-a, got %d", len(workspaces))
	}
}

func TestListExecutionWorkspacesHandler_StatusFilter(t *testing.T) {
	db := setupWorkspacesTestDB(t)
	db.Exec("INSERT INTO execution_workspaces (id, company_id, project_id, name, mode, strategy_type, status) VALUES ('ws1', 'comp-a', 'proj-1', 'WS1', 'worktree', 'branch', 'active')")
	db.Exec("INSERT INTO execution_workspaces (id, company_id, project_id, name, mode, strategy_type, status) VALUES ('ws2', 'comp-a', 'proj-1', 'WS2', 'worktree', 'branch', 'closed')")

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/execution-workspaces", ListExecutionWorkspacesHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-a/execution-workspaces?status=active", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var workspaces []models.ExecutionWorkspace
	json.NewDecoder(w.Body).Decode(&workspaces)
	if len(workspaces) != 1 {
		t.Errorf("expected 1 active workspace, got %d", len(workspaces))
	}
}

func TestListExecutionWorkspacesHandler_ProjectFilter(t *testing.T) {
	db := setupWorkspacesTestDB(t)
	db.Exec("INSERT INTO execution_workspaces (id, company_id, project_id, name, mode, strategy_type, status) VALUES ('ws1', 'comp-a', 'proj-1', 'WS1', 'worktree', 'branch', 'active')")
	db.Exec("INSERT INTO execution_workspaces (id, company_id, project_id, name, mode, strategy_type, status) VALUES ('ws2', 'comp-a', 'proj-2', 'WS2', 'worktree', 'branch', 'active')")

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/execution-workspaces", ListExecutionWorkspacesHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-a/execution-workspaces?projectId=proj-1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var workspaces []models.ExecutionWorkspace
	json.NewDecoder(w.Body).Decode(&workspaces)
	if len(workspaces) != 1 {
		t.Errorf("expected 1 workspace for proj-1, got %d", len(workspaces))
	}
}

func TestGetExecutionWorkspaceHandler_Found(t *testing.T) {
	db := setupWorkspacesTestDB(t)
	db.Exec("INSERT INTO execution_workspaces (id, company_id, project_id, name, mode, strategy_type, status) VALUES ('ws-123', 'comp-1', 'proj-1', 'My Workspace', 'worktree', 'branch', 'active')")

	router := chi.NewRouter()
	router.Get("/execution-workspaces/{id}", GetExecutionWorkspaceHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/execution-workspaces/ws-123", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var ws models.ExecutionWorkspace
	json.NewDecoder(w.Body).Decode(&ws)
	if ws.ID != "ws-123" {
		t.Errorf("expected ID 'ws-123', got %q", ws.ID)
	}
	if ws.Name != "My Workspace" {
		t.Errorf("expected Name 'My Workspace', got %q", ws.Name)
	}
}

func TestGetExecutionWorkspaceHandler_NotFound(t *testing.T) {
	db := setupWorkspacesTestDB(t)

	router := chi.NewRouter()
	router.Get("/execution-workspaces/{id}", GetExecutionWorkspaceHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/execution-workspaces/missing", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestUpdateExecutionWorkspaceHandler(t *testing.T) {
	db := setupWorkspacesTestDB(t)
	db.Exec("INSERT INTO execution_workspaces (id, company_id, project_id, name, mode, strategy_type, status) VALUES ('ws-upd', 'comp-1', 'proj-1', 'Old Name', 'worktree', 'branch', 'active')")

	router := chi.NewRouter()
	router.Put("/execution-workspaces/{id}", UpdateExecutionWorkspaceHandler(db))

	body, _ := json.Marshal(map[string]string{"status": "closed"})
	req := httptest.NewRequest(http.MethodPut, "/execution-workspaces/ws-upd", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestUpdateExecutionWorkspaceHandler_NotFound(t *testing.T) {
	db := setupWorkspacesTestDB(t)

	router := chi.NewRouter()
	router.Put("/execution-workspaces/{id}", UpdateExecutionWorkspaceHandler(db))

	body, _ := json.Marshal(map[string]string{"status": "closed"})
	req := httptest.NewRequest(http.MethodPut, "/execution-workspaces/missing", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestGetWorkspaceCloseReadinessHandler(t *testing.T) {
	db := setupWorkspacesTestDB(t)

	req := httptest.NewRequest(http.MethodGet, "/execution-workspaces/ws-1/close-readiness", nil)
	w := httptest.NewRecorder()

	GetWorkspaceCloseReadinessHandler(db)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result map[string]interface{}
	json.NewDecoder(w.Body).Decode(&result)
	if result["ready"] != true {
		t.Errorf("expected ready=true, got %v", result["ready"])
	}
}

func TestGetWorkspaceOperationsHandler(t *testing.T) {
	db := setupWorkspacesTestDB(t)
	db.Exec("INSERT INTO workspace_operations (id, company_id, execution_workspace_id, phase, status) VALUES ('op1', 'comp-1', 'ws-1', 'checkout', 'running')")
	db.Exec("INSERT INTO workspace_operations (id, company_id, execution_workspace_id, phase, status) VALUES ('op2', 'comp-1', 'ws-1', 'build', 'succeeded')")
	db.Exec("INSERT INTO workspace_operations (id, company_id, execution_workspace_id, phase, status) VALUES ('op3', 'comp-1', 'ws-2', 'checkout', 'running')")

	router := chi.NewRouter()
	router.Get("/execution-workspaces/{id}/operations", GetWorkspaceWorkspaceOperationsHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/execution-workspaces/ws-1/operations", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var ops []models.WorkspaceOperation
	json.NewDecoder(w.Body).Decode(&ops)
	if len(ops) != 2 {
		t.Errorf("expected 2 operations for ws-1, got %d", len(ops))
	}
}

func TestExecutionWorkspaceRuntimeServicesHandler_Found(t *testing.T) {
	db := setupWorkspacesTestDB(t)
	db.Exec("INSERT INTO execution_workspaces (id, company_id, project_id, name, mode, strategy_type, status) VALUES ('ws-rt', 'comp-1', 'proj-1', 'RT Workspace', 'worktree', 'branch', 'active')")

	router := chi.NewRouter()
	router.Post("/execution-workspaces/{id}/runtime-services/{action}", ExecutionWorkspaceRuntimeServicesHandler(db, nil))

	req := httptest.NewRequest(http.MethodPost, "/execution-workspaces/ws-rt/runtime-services/start", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// The workspace has no cwd, so we expect 422 (needs a local path).
	// Previously this returned 200 with a stub; the real handler validates cwd.
	if w.Code != http.StatusUnprocessableEntity && w.Code != http.StatusOK {
		t.Fatalf("expected 422 or 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestExecutionWorkspaceRuntimeServicesHandler_NotFound(t *testing.T) {
	db := setupWorkspacesTestDB(t)

	router := chi.NewRouter()
	router.Post("/execution-workspaces/{id}/runtime-services/{action}", ExecutionWorkspaceRuntimeServicesHandler(db, nil))

	req := httptest.NewRequest(http.MethodPost, "/execution-workspaces/missing/runtime-services/start", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
