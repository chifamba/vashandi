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

func setupProjectsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&projects_test=1"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.Exec("DROP TABLE IF EXISTS projects")
	db.Exec(`CREATE TABLE projects (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		goal_id text,
		name text NOT NULL,
		description text,
		status text NOT NULL DEFAULT 'backlog',
		lead_agent_id text,
		target_date text,
		color text,
		pause_reason text,
		paused_at datetime,
		execution_workspace_policy text,
		archived_at datetime,
		created_at datetime,
		updated_at datetime
	)`)
	return db
}

func TestListProjectsHandler_CompanyScoping(t *testing.T) {
	db := setupProjectsTestDB(t)
	db.Exec("INSERT INTO projects (id, company_id, name, status) VALUES ('p1', 'comp-a', 'Alpha', 'active')")
	db.Exec("INSERT INTO projects (id, company_id, name, status) VALUES ('p2', 'comp-b', 'Beta', 'active')")

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/projects", ListProjectsHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-a/projects", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var projects []models.Project
	json.NewDecoder(w.Body).Decode(&projects)
	if len(projects) != 1 {
		t.Errorf("expected 1 project for comp-a, got %d", len(projects))
	}
	if len(projects) > 0 && projects[0].CompanyID != "comp-a" {
		t.Errorf("expected project scoped to comp-a, got %q", projects[0].CompanyID)
	}
}

func TestGetProjectHandler_Found(t *testing.T) {
	db := setupProjectsTestDB(t)
	db.Exec("INSERT INTO projects (id, company_id, name, status) VALUES ('proj-123', 'comp-1', 'Test Project', 'active')")

	router := chi.NewRouter()
	router.Get("/projects/{id}", GetProjectHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/projects/proj-123", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var project models.Project
	json.NewDecoder(w.Body).Decode(&project)
	if project.ID != "proj-123" {
		t.Errorf("expected project ID 'proj-123', got %q", project.ID)
	}
}

func TestGetProjectHandler_NotFound(t *testing.T) {
	db := setupProjectsTestDB(t)

	router := chi.NewRouter()
	router.Get("/projects/{id}", GetProjectHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/projects/nonexistent", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestCreateProjectHandler_CompanyScoping(t *testing.T) {
	db := setupProjectsTestDB(t)

	router := chi.NewRouter()
	router.Post("/companies/{companyId}/projects", CreateProjectHandler(db))

	body, _ := json.Marshal(map[string]string{
		"name":   "New Project",
		"status": "backlog",
	})
	req := httptest.NewRequest(http.MethodPost, "/companies/comp-new/projects", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", w.Code, w.Body.String())
	}

	var project models.Project
	json.NewDecoder(w.Body).Decode(&project)
	if project.CompanyID != "comp-new" {
		t.Errorf("expected CompanyID 'comp-new', got %q", project.CompanyID)
	}
	if project.Name != "New Project" {
		t.Errorf("expected Name 'New Project', got %q", project.Name)
	}
}

func TestUpdateProjectHandler(t *testing.T) {
	db := setupProjectsTestDB(t)
	db.Exec("INSERT INTO projects (id, company_id, name, status) VALUES ('upd-proj', 'comp-1', 'Old Name', 'backlog')")

	router := chi.NewRouter()
	router.Put("/projects/{id}", UpdateProjectHandler(db))

	body, _ := json.Marshal(map[string]string{"name": "Updated Name"})
	req := httptest.NewRequest(http.MethodPut, "/projects/upd-proj", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	var project models.Project
	json.NewDecoder(w.Body).Decode(&project)
	if project.Name != "Updated Name" {
		t.Errorf("expected Name 'Updated Name', got %q", project.Name)
	}
}

func TestUpdateProjectHandler_NotFound(t *testing.T) {
	db := setupProjectsTestDB(t)

	router := chi.NewRouter()
	router.Put("/projects/{id}", UpdateProjectHandler(db))

	body, _ := json.Marshal(map[string]string{"name": "Ghost"})
	req := httptest.NewRequest(http.MethodPut, "/projects/missing", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestDeleteProjectHandler(t *testing.T) {
	db := setupProjectsTestDB(t)
	db.Exec("INSERT INTO projects (id, company_id, name, status) VALUES ('del-proj', 'comp-1', 'Delete Me', 'active')")

	router := chi.NewRouter()
	router.Delete("/projects/{id}", DeleteProjectHandler(db))

	req := httptest.NewRequest(http.MethodDelete, "/projects/del-proj", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}

	var count int64
	db.Model(&models.Project{}).Where("id = ?", "del-proj").Count(&count)
	if count != 0 {
		t.Errorf("expected project to be deleted, found %d row(s)", count)
	}
}

func TestCreateProjectHandler_BadBody(t *testing.T) {
	db := setupProjectsTestDB(t)

	router := chi.NewRouter()
	router.Post("/companies/{companyId}/projects", CreateProjectHandler(db))

	req := httptest.NewRequest(http.MethodPost, "/companies/comp-1/projects", bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
