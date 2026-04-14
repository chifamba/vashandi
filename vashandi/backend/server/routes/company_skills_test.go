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

func setupCompanySkillsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&company_skills_test=1"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.Exec("DROP TABLE IF EXISTS company_skills")
	db.Exec(`CREATE TABLE company_skills (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		key text NOT NULL,
		slug text NOT NULL,
		name text NOT NULL,
		description text,
		markdown text NOT NULL DEFAULT '',
		source_type text NOT NULL DEFAULT 'local_path',
		source_locator text,
		source_ref text,
		trust_level text NOT NULL DEFAULT 'markdown_only',
		compatibility text NOT NULL DEFAULT 'compatible',
		file_inventory text NOT NULL DEFAULT '[]',
		metadata text,
		created_at datetime,
		updated_at datetime
	)`)
	return db
}

func TestListCompanySkillsHandler_CompanyScoping(t *testing.T) {
	db := setupCompanySkillsTestDB(t)
	db.Exec("INSERT INTO company_skills (id, company_id, key, slug, name, markdown) VALUES ('s1', 'c1', 'test-skill', 'test-skill', 'Test Skill', '# Hello')")
	db.Exec("INSERT INTO company_skills (id, company_id, key, slug, name, markdown) VALUES ('s2', 'c2', 'other-skill', 'other-skill', 'Other Skill', '# Other')")

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/skills", ListCompanySkillsHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/c1/skills", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var skills []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&skills)
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill for c1, got %d", len(skills))
	}
	if skills[0]["Name"] != "Test Skill" {
		t.Errorf("expected skill Name 'Test Skill', got %v", skills[0]["Name"])
	}
}

func TestListCompanySkillsHandler_Empty(t *testing.T) {
	db := setupCompanySkillsTestDB(t)

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/skills", ListCompanySkillsHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/c1/skills", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestCreateCompanySkillHandler(t *testing.T) {
	db := setupCompanySkillsTestDB(t)

	router := chi.NewRouter()
	router.Post("/companies/{companyId}/skills", CreateCompanySkillHandler(db))

	body := map[string]string{
		"id":       "skill-new",
		"key":      "new-skill",
		"slug":     "new-skill",
		"name":     "New Skill",
		"markdown": "# New Skill Content",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/companies/c1/skills", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var created map[string]interface{}
	json.NewDecoder(w.Body).Decode(&created)
	if created["CompanyID"] != "c1" {
		t.Errorf("expected CompanyID c1, got %v", created["CompanyID"])
	}
}

func TestCreateCompanySkillHandler_BadBody(t *testing.T) {
	db := setupCompanySkillsTestDB(t)

	router := chi.NewRouter()
	router.Post("/companies/{companyId}/skills", CreateCompanySkillHandler(db))

	req := httptest.NewRequest(http.MethodPost, "/companies/c1/skills", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetCompanySkillHandler_Found(t *testing.T) {
	db := setupCompanySkillsTestDB(t)
	db.Exec("INSERT INTO company_skills (id, company_id, key, slug, name, markdown) VALUES ('s1', 'c1', 'test', 'test', 'Test', '# Test')")

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/skills/{skillId}", GetCompanySkillHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/c1/skills/s1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestGetCompanySkillHandler_NotFound(t *testing.T) {
	db := setupCompanySkillsTestDB(t)

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/skills/{skillId}", GetCompanySkillHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/c1/skills/nonexistent", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestGetCompanySkillHandler_CrossCompanyBlock(t *testing.T) {
	db := setupCompanySkillsTestDB(t)
	db.Exec("INSERT INTO company_skills (id, company_id, key, slug, name, markdown) VALUES ('s1', 'c2', 'test', 'test', 'Test', '# Test')")

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/skills/{skillId}", GetCompanySkillHandler(db))

	// Skill s1 belongs to c2, request is for c1
	req := httptest.NewRequest(http.MethodGet, "/companies/c1/skills/s1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for cross-company access, got %d", w.Code)
	}
}

func TestDeleteCompanySkillHandler(t *testing.T) {
	db := setupCompanySkillsTestDB(t)
	db.Exec("INSERT INTO company_skills (id, company_id, key, slug, name, markdown) VALUES ('s1', 'c1', 'test', 'test', 'Test', '# Test')")

	router := chi.NewRouter()
	router.Delete("/companies/{companyId}/skills/{skillId}", DeleteCompanySkillHandler(db))

	req := httptest.NewRequest(http.MethodDelete, "/companies/c1/skills/s1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
}

func TestGetCompanySkillUpdateStatusHandler(t *testing.T) {
	db := setupCompanySkillsTestDB(t)
	db.Exec("INSERT INTO company_skills (id, company_id, key, slug, name, markdown, compatibility, source_type) VALUES ('s1', 'c1', 'test', 'test', 'Test', '# Test', 'compatible', 'local_path')")

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/skills/{skillId}/update-status", GetCompanySkillUpdateStatusHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/c1/skills/s1/update-status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["skillId"] != "s1" {
		t.Errorf("expected skillId s1, got %v", resp["skillId"])
	}
	if resp["hasUpdate"] != false {
		t.Errorf("expected hasUpdate false, got %v", resp["hasUpdate"])
	}
}

func TestGetCompanySkillFilesHandler(t *testing.T) {
	db := setupCompanySkillsTestDB(t)
	db.Exec("INSERT INTO company_skills (id, company_id, key, slug, name, markdown, file_inventory) VALUES ('s1', 'c1', 'test', 'test', 'Test', '# Test', '[]')")

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/skills/{skillId}/files", GetCompanySkillFilesHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/c1/skills/s1/files", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["skillId"] != "s1" {
		t.Errorf("expected skillId s1, got %v", resp["skillId"])
	}
}

func TestInstallUpdateCompanySkillHandler_NoUpdate(t *testing.T) {
	db := setupCompanySkillsTestDB(t)
	db.Exec("INSERT INTO company_skills (id, company_id, key, slug, name, markdown) VALUES ('s1', 'c1', 'test', 'test', 'Test', '# Test')")

	router := chi.NewRouter()
	router.Post("/companies/{companyId}/skills/{skillId}/install-update", InstallUpdateCompanySkillHandler(db))

	req := httptest.NewRequest(http.MethodPost, "/companies/c1/skills/s1/install-update", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "no_update_available" {
		t.Errorf("expected status no_update_available, got %v", resp["status"])
	}
}
