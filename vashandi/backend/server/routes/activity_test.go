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

func setupActivityTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&activity_test=1"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.Exec("DROP TABLE IF EXISTS activity_log")
	db.Exec("DROP TABLE IF EXISTS heartbeat_runs")
	db.Exec("DROP TABLE IF EXISTS issues")
	db.Exec(`CREATE TABLE activity_log (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		actor_type text NOT NULL DEFAULT 'system',
		actor_id text NOT NULL,
		action text NOT NULL,
		entity_type text NOT NULL,
		entity_id text NOT NULL,
		agent_id text,
		run_id text,
		details text,
		created_at datetime
	)`)
	db.Exec(`CREATE TABLE heartbeat_runs (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		agent_id text NOT NULL,
		invocation_source text NOT NULL DEFAULT 'on_demand',
		status text NOT NULL DEFAULT 'queued',
		started_at datetime,
		task_id text NOT NULL DEFAULT '',
		log_compressed boolean NOT NULL DEFAULT 0,
		process_loss_retry_count integer NOT NULL DEFAULT 0,
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
		checkout_run_id text,
		execution_run_id text,
		created_at datetime,
		updated_at datetime
	)`)
	return db
}

func TestListActivityHandler_CompanyScoping(t *testing.T) {
	db := setupActivityTestDB(t)
	db.Exec("INSERT INTO activity_log (id, company_id, actor_type, actor_id, action, entity_type, entity_id) VALUES ('al1', 'comp-a', 'system', 's', 'issue.created', 'issue', 'i1')")
	db.Exec("INSERT INTO activity_log (id, company_id, actor_type, actor_id, action, entity_type, entity_id) VALUES ('al2', 'comp-b', 'system', 's', 'issue.created', 'issue', 'i2')")

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/activity", ListActivityHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-a/activity", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var activities []models.ActivityLog
	json.NewDecoder(w.Body).Decode(&activities)
	if len(activities) != 1 {
		t.Errorf("expected 1 activity for comp-a, got %d", len(activities))
	}
}

func TestListActivityHandler_MissingCompanyID(t *testing.T) {
	db := setupActivityTestDB(t)

	req := httptest.NewRequest(http.MethodGet, "/activity", nil)
	w := httptest.NewRecorder()

	ListActivityHandler(db)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 when companyId is missing, got %d", w.Code)
	}
}

func TestListActivityHandler_EntityFilter(t *testing.T) {
	db := setupActivityTestDB(t)
	db.Exec("INSERT INTO activity_log (id, company_id, actor_type, actor_id, action, entity_type, entity_id) VALUES ('al1', 'comp-a', 'system', 's', 'issue.created', 'issue', 'i1')")
	db.Exec("INSERT INTO activity_log (id, company_id, actor_type, actor_id, action, entity_type, entity_id) VALUES ('al2', 'comp-a', 'system', 's', 'agent.updated', 'agent', 'a1')")

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/activity", ListActivityHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-a/activity?entityType=issue", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var activities []models.ActivityLog
	json.NewDecoder(w.Body).Decode(&activities)
	if len(activities) != 1 {
		t.Errorf("expected 1 issue activity, got %d", len(activities))
	}
}

func TestCreateActivityHandler(t *testing.T) {
	db := setupActivityTestDB(t)

	router := chi.NewRouter()
	router.Post("/companies/{companyId}/activity", CreateActivityHandler(db))

	body, _ := json.Marshal(map[string]string{
		"actorType":  "user",
		"actorId":    "user-1",
		"action":     "issue.created",
		"entityType": "issue",
		"entityId":   "issue-1",
	})
	req := httptest.NewRequest(http.MethodPost, "/companies/comp-xyz/activity", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", w.Code, w.Body.String())
	}

	var activity models.ActivityLog
	json.NewDecoder(w.Body).Decode(&activity)
	if activity.CompanyID != "comp-xyz" {
		t.Errorf("expected CompanyID 'comp-xyz', got %q", activity.CompanyID)
	}
}

func TestCreateActivityHandler_MissingCompanyID(t *testing.T) {
	db := setupActivityTestDB(t)

	body, _ := json.Marshal(map[string]string{
		"actorType": "user",
		"actorId":   "user-1",
		"action":    "test",
	})
	req := httptest.NewRequest(http.MethodPost, "/activity", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	CreateActivityHandler(db)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 when companyId is missing, got %d", w.Code)
	}
}

func TestCreateActivityHandler_BadBody(t *testing.T) {
	db := setupActivityTestDB(t)

	router := chi.NewRouter()
	router.Post("/companies/{companyId}/activity", CreateActivityHandler(db))

	req := httptest.NewRequest(http.MethodPost, "/companies/comp-1/activity", bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestListIssueActivityHandler(t *testing.T) {
	db := setupActivityTestDB(t)
	db.Exec("INSERT INTO activity_log (id, company_id, actor_type, actor_id, action, entity_type, entity_id) VALUES ('al1', 'comp-a', 'system', 's', 'issue.created', 'issue', 'iss-1')")
	db.Exec("INSERT INTO activity_log (id, company_id, actor_type, actor_id, action, entity_type, entity_id) VALUES ('al2', 'comp-a', 'system', 's', 'issue.updated', 'issue', 'iss-1')")
	db.Exec("INSERT INTO activity_log (id, company_id, actor_type, actor_id, action, entity_type, entity_id) VALUES ('al3', 'comp-a', 'system', 's', 'issue.created', 'issue', 'iss-2')")

	router := chi.NewRouter()
	router.Get("/issues/{id}/activity", ListIssueActivityHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/issues/iss-1/activity", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var activities []models.ActivityLog
	json.NewDecoder(w.Body).Decode(&activities)
	if len(activities) != 2 {
		t.Errorf("expected 2 activities for iss-1, got %d", len(activities))
	}
}

func TestListHeartbeatRunIssuesHandler(t *testing.T) {
	db := setupActivityTestDB(t)
	db.Exec("INSERT INTO issues (id, company_id, title, checkout_run_id) VALUES ('iss-1', 'comp-a', 'Issue1', 'run-1')")
	db.Exec("INSERT INTO issues (id, company_id, title, execution_run_id) VALUES ('iss-2', 'comp-a', 'Issue2', 'run-1')")
	db.Exec("INSERT INTO issues (id, company_id, title, checkout_run_id) VALUES ('iss-3', 'comp-a', 'Issue3', 'run-2')")

	router := chi.NewRouter()
	router.Get("/heartbeat-runs/{runId}/issues", ListHeartbeatRunIssuesHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/heartbeat-runs/run-1/issues", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var issues []models.Issue
	json.NewDecoder(w.Body).Decode(&issues)
	if len(issues) != 2 {
		t.Errorf("expected 2 issues for run-1, got %d", len(issues))
	}
}
