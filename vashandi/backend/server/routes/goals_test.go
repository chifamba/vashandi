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

func setupGoalsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&goals_test=1"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.Exec("DROP TABLE IF EXISTS goals")
	db.Exec(`CREATE TABLE goals (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		title text NOT NULL,
		description text,
		level text NOT NULL DEFAULT 'task',
		status text NOT NULL DEFAULT 'planned',
		parent_id text,
		owner_agent_id text,
		created_at datetime,
		updated_at datetime
	)`)
	return db
}

func TestListGoalsHandler_CompanyScoping(t *testing.T) {
	db := setupGoalsTestDB(t)
	db.Exec("INSERT INTO goals (id, company_id, title, level, status) VALUES ('g1', 'comp-a', 'Goal A', 'task', 'planned')")
	db.Exec("INSERT INTO goals (id, company_id, title, level, status) VALUES ('g2', 'comp-b', 'Goal B', 'task', 'planned')")

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/goals", ListGoalsHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-a/goals", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var goals []models.Goal
	json.NewDecoder(w.Body).Decode(&goals)
	if len(goals) != 1 {
		t.Errorf("expected 1 goal for comp-a, got %d", len(goals))
	}
	if len(goals) > 0 && goals[0].CompanyID != "comp-a" {
		t.Errorf("expected goal scoped to comp-a, got %q", goals[0].CompanyID)
	}
}

func TestListGoalsHandler_MissingCompanyID(t *testing.T) {
	db := setupGoalsTestDB(t)

	// No companyId URL param
	req := httptest.NewRequest(http.MethodGet, "/goals", nil)
	w := httptest.NewRecorder()

	ListGoalsHandler(db)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 when companyId is missing, got %d", w.Code)
	}
}

func TestGetGoalHandler_Found(t *testing.T) {
	db := setupGoalsTestDB(t)
	db.Exec("INSERT INTO goals (id, company_id, title, level, status) VALUES ('goal-1', 'comp-1', 'My Goal', 'task', 'planned')")

	router := chi.NewRouter()
	router.Get("/goals/{id}", GetGoalHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/goals/goal-1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var goal models.Goal
	json.NewDecoder(w.Body).Decode(&goal)
	if goal.ID != "goal-1" {
		t.Errorf("expected ID 'goal-1', got %q", goal.ID)
	}
}

func TestGetGoalHandler_NotFound(t *testing.T) {
	db := setupGoalsTestDB(t)

	router := chi.NewRouter()
	router.Get("/goals/{id}", GetGoalHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/goals/missing", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestCreateGoalHandler_CompanyScoping(t *testing.T) {
	db := setupGoalsTestDB(t)

	router := chi.NewRouter()
	router.Post("/companies/{companyId}/goals", CreateGoalHandler(db))

	body, _ := json.Marshal(map[string]string{
		"title":  "New Goal",
		"level":  "task",
		"status": "planned",
	})
	req := httptest.NewRequest(http.MethodPost, "/companies/comp-xyz/goals", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", w.Code, w.Body.String())
	}

	var goal models.Goal
	json.NewDecoder(w.Body).Decode(&goal)
	if goal.CompanyID != "comp-xyz" {
		t.Errorf("expected CompanyID 'comp-xyz', got %q", goal.CompanyID)
	}
	if goal.Title != "New Goal" {
		t.Errorf("expected Title 'New Goal', got %q", goal.Title)
	}
}

func TestCreateGoalHandler_BadBody(t *testing.T) {
	db := setupGoalsTestDB(t)

	router := chi.NewRouter()
	router.Post("/companies/{companyId}/goals", CreateGoalHandler(db))

	req := httptest.NewRequest(http.MethodPost, "/companies/comp-1/goals", bytes.NewBufferString("notjson"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestUpdateGoalHandler(t *testing.T) {
	db := setupGoalsTestDB(t)
	db.Exec("INSERT INTO goals (id, company_id, title, level, status) VALUES ('upd-goal', 'comp-1', 'Old Title', 'task', 'planned')")

	router := chi.NewRouter()
	router.Put("/goals/{id}", UpdateGoalHandler(db))

	body, _ := json.Marshal(map[string]string{"title": "Updated Title", "status": "active"})
	req := httptest.NewRequest(http.MethodPut, "/goals/upd-goal", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestUpdateGoalHandler_NotFound(t *testing.T) {
	db := setupGoalsTestDB(t)

	router := chi.NewRouter()
	router.Put("/goals/{id}", UpdateGoalHandler(db))

	body, _ := json.Marshal(map[string]string{"title": "Ghost"})
	req := httptest.NewRequest(http.MethodPut, "/goals/does-not-exist", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestDeleteGoalHandler(t *testing.T) {
	db := setupGoalsTestDB(t)
	db.Exec("INSERT INTO goals (id, company_id, title, level, status) VALUES ('del-goal', 'comp-1', 'Delete Me', 'task', 'planned')")

	router := chi.NewRouter()
	router.Delete("/goals/{id}", DeleteGoalHandler(db))

	req := httptest.NewRequest(http.MethodDelete, "/goals/del-goal", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (with deleted goal body), got %d", w.Code)
	}

	var count int64
	db.Model(&models.Goal{}).Where("id = ?", "del-goal").Count(&count)
	if count != 0 {
		t.Errorf("expected goal to be deleted, found %d row(s)", count)
	}
}

func TestDeleteGoalHandler_NotFound(t *testing.T) {
	db := setupGoalsTestDB(t)

	router := chi.NewRouter()
	router.Delete("/goals/{id}", DeleteGoalHandler(db))

	req := httptest.NewRequest(http.MethodDelete, "/goals/ghost", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
