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

func setupRoutinesTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&routines_test=1"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.Exec("DROP TABLE IF EXISTS routine_triggers")
	db.Exec("DROP TABLE IF EXISTS routines")
	db.Exec("DROP TABLE IF EXISTS heartbeat_runs")
	db.Exec(`CREATE TABLE routines (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		project_id text NOT NULL,
		goal_id text,
		parent_issue_id text,
		title text NOT NULL,
		description text,
		assignee_agent_id text NOT NULL,
		priority text NOT NULL DEFAULT 'medium',
		status text NOT NULL DEFAULT 'active',
		concurrency_policy text NOT NULL DEFAULT 'coalesce_if_active',
		catch_up_policy text NOT NULL DEFAULT 'skip_missed',
		variables text NOT NULL DEFAULT '[]',
		created_by_agent_id text,
		created_by_user_id text,
		updated_by_agent_id text,
		updated_by_user_id text,
		last_triggered_at datetime,
		last_enqueued_at datetime,
		created_at datetime,
		updated_at datetime
	)`)
	db.Exec(`CREATE TABLE routine_triggers (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		routine_id text NOT NULL,
		kind text NOT NULL,
		label text,
		enabled boolean NOT NULL DEFAULT 1,
		cron_expression text,
		timezone text,
		next_run_at datetime,
		last_fired_at datetime,
		public_id text,
		secret_id text,
		signing_mode text,
		replay_window_sec integer,
		last_rotated_at datetime,
		last_result text,
		created_by_agent_id text,
		created_by_user_id text,
		updated_by_agent_id text,
		updated_by_user_id text,
		created_at datetime,
		updated_at datetime
	)`)
	db.Exec(`CREATE TABLE heartbeat_runs (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		agent_id text NOT NULL,
		invocation_source text NOT NULL DEFAULT 'on_demand',
		trigger_detail text,
		status text NOT NULL DEFAULT 'queued',
		task_id text NOT NULL DEFAULT '',
		log_compressed boolean NOT NULL DEFAULT 0,
		process_loss_retry_count integer NOT NULL DEFAULT 0,
		created_at datetime,
		updated_at datetime
	)`)
	return db
}

func TestListRoutinesHandler_CompanyScoping(t *testing.T) {
	db := setupRoutinesTestDB(t)
	db.Exec("INSERT INTO routines (id, company_id, project_id, title, assignee_agent_id) VALUES ('r1', 'comp-a', 'proj-1', 'Daily Check', 'agent-1')")
	db.Exec("INSERT INTO routines (id, company_id, project_id, title, assignee_agent_id) VALUES ('r2', 'comp-b', 'proj-2', 'Weekly Scan', 'agent-2')")

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/routines", ListRoutinesHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-a/routines", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var routines []models.Routine
	json.NewDecoder(w.Body).Decode(&routines)
	if len(routines) != 1 {
		t.Errorf("expected 1 routine for comp-a, got %d", len(routines))
	}
}

func TestGetRoutineHandler_Found(t *testing.T) {
	db := setupRoutinesTestDB(t)
	db.Exec("INSERT INTO routines (id, company_id, project_id, title, assignee_agent_id) VALUES ('rtn-1', 'comp-1', 'proj-1', 'My Routine', 'agent-1')")

	router := chi.NewRouter()
	router.Get("/routines/{id}", GetRoutineHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/routines/rtn-1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var routine models.Routine
	json.NewDecoder(w.Body).Decode(&routine)
	if routine.ID != "rtn-1" {
		t.Errorf("expected ID 'rtn-1', got %q", routine.ID)
	}
}

func TestGetRoutineHandler_NotFound(t *testing.T) {
	db := setupRoutinesTestDB(t)

	router := chi.NewRouter()
	router.Get("/routines/{id}", GetRoutineHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/routines/missing", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestCreateRoutineHandler(t *testing.T) {
	db := setupRoutinesTestDB(t)

	router := chi.NewRouter()
	router.Post("/companies/{companyId}/routines", CreateRoutineHandler(db))

	body, _ := json.Marshal(map[string]string{
		"title":           "New Routine",
		"projectId":       "proj-1",
		"assigneeAgentId": "agent-1",
	})
	req := httptest.NewRequest(http.MethodPost, "/companies/comp-xyz/routines", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", w.Code, w.Body.String())
	}

	var routine models.Routine
	json.NewDecoder(w.Body).Decode(&routine)
	if routine.CompanyID != "comp-xyz" {
		t.Errorf("expected CompanyID 'comp-xyz', got %q", routine.CompanyID)
	}
}

func TestCreateRoutineHandler_BadBody(t *testing.T) {
	db := setupRoutinesTestDB(t)

	router := chi.NewRouter()
	router.Post("/companies/{companyId}/routines", CreateRoutineHandler(db))

	req := httptest.NewRequest(http.MethodPost, "/companies/comp-1/routines", bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestUpdateRoutineHandler(t *testing.T) {
	db := setupRoutinesTestDB(t)
	db.Exec("INSERT INTO routines (id, company_id, project_id, title, assignee_agent_id) VALUES ('rtn-upd', 'comp-1', 'proj-1', 'Old Title', 'agent-1')")

	router := chi.NewRouter()
	router.Put("/routines/{id}", UpdateRoutineHandler(db))

	body, _ := json.Marshal(map[string]string{"title": "Updated Title"})
	req := httptest.NewRequest(http.MethodPut, "/routines/rtn-upd", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestUpdateRoutineHandler_NotFound(t *testing.T) {
	db := setupRoutinesTestDB(t)

	router := chi.NewRouter()
	router.Put("/routines/{id}", UpdateRoutineHandler(db))

	body, _ := json.Marshal(map[string]string{"title": "Ghost"})
	req := httptest.NewRequest(http.MethodPut, "/routines/missing", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestDeleteRoutineHandler(t *testing.T) {
	db := setupRoutinesTestDB(t)
	db.Exec("INSERT INTO routines (id, company_id, project_id, title, assignee_agent_id) VALUES ('rtn-del', 'comp-1', 'proj-1', 'Delete Me', 'agent-1')")

	router := chi.NewRouter()
	router.Delete("/routines/{id}", DeleteRoutineHandler(db))

	req := httptest.NewRequest(http.MethodDelete, "/routines/rtn-del", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
}

func TestListRoutineRunsHandler(t *testing.T) {
	db := setupRoutinesTestDB(t)
	db.Exec("INSERT INTO heartbeat_runs (id, company_id, agent_id, trigger_detail, status, task_id) VALUES ('run-1', 'comp-1', 'agent-1', 'rtn-1', 'completed', 't1')")
	db.Exec("INSERT INTO heartbeat_runs (id, company_id, agent_id, trigger_detail, status, task_id) VALUES ('run-2', 'comp-1', 'agent-1', 'rtn-1', 'running', 't2')")
	db.Exec("INSERT INTO heartbeat_runs (id, company_id, agent_id, trigger_detail, status, task_id) VALUES ('run-3', 'comp-1', 'agent-1', 'rtn-2', 'completed', 't3')")

	router := chi.NewRouter()
	router.Get("/routines/{id}/runs", ListRoutineRunsHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/routines/rtn-1/runs", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var runs []models.HeartbeatRun
	json.NewDecoder(w.Body).Decode(&runs)
	if len(runs) != 2 {
		t.Errorf("expected 2 runs for rtn-1, got %d", len(runs))
	}
}

func TestCreateRoutineTriggerHandler(t *testing.T) {
	db := setupRoutinesTestDB(t)
	db.Exec("INSERT INTO routines (id, company_id, project_id, title, assignee_agent_id) VALUES ('rtn-trg', 'comp-1', 'proj-1', 'Trigger Test', 'agent-1')")

	router := chi.NewRouter()
	router.Post("/routines/{id}/triggers", CreateRoutineTriggerHandler(db))

	body, _ := json.Marshal(map[string]string{
		"kind":           "cron",
		"cronExpression": "0 8 * * *",
	})
	req := httptest.NewRequest(http.MethodPost, "/routines/rtn-trg/triggers", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", w.Code, w.Body.String())
	}

	var trigger models.RoutineTrigger
	json.NewDecoder(w.Body).Decode(&trigger)
	if trigger.RoutineID != "rtn-trg" {
		t.Errorf("expected RoutineID 'rtn-trg', got %q", trigger.RoutineID)
	}
	if trigger.CompanyID != "comp-1" {
		t.Errorf("expected CompanyID 'comp-1', got %q", trigger.CompanyID)
	}
}

func TestCreateRoutineTriggerHandler_RoutineNotFound(t *testing.T) {
	db := setupRoutinesTestDB(t)

	router := chi.NewRouter()
	router.Post("/routines/{id}/triggers", CreateRoutineTriggerHandler(db))

	body, _ := json.Marshal(map[string]string{"kind": "cron"})
	req := httptest.NewRequest(http.MethodPost, "/routines/missing/triggers", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestDeleteRoutineTriggerHandler(t *testing.T) {
	db := setupRoutinesTestDB(t)
	db.Exec("INSERT INTO routine_triggers (id, company_id, routine_id, kind) VALUES ('trg-del', 'comp-1', 'rtn-1', 'cron')")

	router := chi.NewRouter()
	router.Delete("/triggers/{triggerId}", DeleteRoutineTriggerHandler(db))

	req := httptest.NewRequest(http.MethodDelete, "/triggers/trg-del", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
}

func TestFirePublicRoutineTriggerHandler_Found(t *testing.T) {
	db := setupRoutinesTestDB(t)
	db.Exec("INSERT INTO routine_triggers (id, company_id, routine_id, kind, public_id, enabled) VALUES ('trg-pub', 'comp-1', 'rtn-1', 'webhook', 'pub-abc', 1)")

	router := chi.NewRouter()
	router.Post("/triggers/fire/{publicId}", FirePublicRoutineTriggerHandler(db))

	req := httptest.NewRequest(http.MethodPost, "/triggers/fire/pub-abc", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	var result map[string]interface{}
	json.NewDecoder(w.Body).Decode(&result)
	if result["fired"] != true {
		t.Errorf("expected fired=true, got %v", result["fired"])
	}
}

func TestFirePublicRoutineTriggerHandler_NotFound(t *testing.T) {
	db := setupRoutinesTestDB(t)

	router := chi.NewRouter()
	router.Post("/triggers/fire/{publicId}", FirePublicRoutineTriggerHandler(db))

	req := httptest.NewRequest(http.MethodPost, "/triggers/fire/nonexistent", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestRunRoutineNowHandler(t *testing.T) {
	db := setupRoutinesTestDB(t)
	db.Exec("INSERT INTO routines (id, company_id, project_id, title, assignee_agent_id) VALUES ('rtn-now', 'comp-1', 'proj-1', 'Run Now', 'agent-1')")

	router := chi.NewRouter()
	router.Post("/routines/{id}/run", RunRoutineNowHandler(db))

	req := httptest.NewRequest(http.MethodPost, "/routines/rtn-now/run", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", w.Code)
	}
}

func TestRunRoutineNowHandler_NotFound(t *testing.T) {
	db := setupRoutinesTestDB(t)

	router := chi.NewRouter()
	router.Post("/routines/{id}/run", RunRoutineNowHandler(db))

	req := httptest.NewRequest(http.MethodPost, "/routines/missing/run", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
