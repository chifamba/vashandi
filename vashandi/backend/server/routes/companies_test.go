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

func setupCompaniesTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&companies_test=1"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.Exec("DROP TABLE IF EXISTS issues")
	db.Exec("DROP TABLE IF EXISTS agents")
	db.Exec("DROP TABLE IF EXISTS companies")
	db.Exec(`CREATE TABLE companies (
		id text PRIMARY KEY,
		name text NOT NULL,
		description text,
		status text NOT NULL DEFAULT 'active',
		pause_reason text,
		paused_at datetime,
		issue_prefix text NOT NULL DEFAULT 'PAP',
		issue_counter integer NOT NULL DEFAULT 0,
		budget_monthly_cents integer NOT NULL DEFAULT 0,
		spent_monthly_cents integer NOT NULL DEFAULT 0,
		require_board_approval_for_new_agents boolean NOT NULL DEFAULT 1,
		feedback_data_sharing_enabled boolean NOT NULL DEFAULT 0,
		feedback_data_sharing_consent_at datetime,
		feedback_data_sharing_consent_by_user_id text,
		feedback_data_sharing_terms_version text,
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

func TestListCompaniesHandler(t *testing.T) {
	db := setupCompaniesTestDB(t)
	db.Exec("INSERT INTO companies (id, name, issue_prefix) VALUES ('c1', 'Alpha Inc', 'ALP')")
	db.Exec("INSERT INTO companies (id, name, issue_prefix) VALUES ('c2', 'Beta Corp', 'BET')")

	req := httptest.NewRequest(http.MethodGet, "/companies", nil)
	w := httptest.NewRecorder()

	ListCompaniesHandler(db)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var companies []models.Company
	json.NewDecoder(w.Body).Decode(&companies)
	if len(companies) != 2 {
		t.Errorf("expected 2 companies, got %d", len(companies))
	}
}

func TestGetCompanyHandler_Found(t *testing.T) {
	db := setupCompaniesTestDB(t)
	db.Exec("INSERT INTO companies (id, name, issue_prefix) VALUES ('comp-abc', 'My Company', 'MYC')")

	router := chi.NewRouter()
	router.Get("/companies/{id}", GetCompanyHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-abc", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var company models.Company
	json.NewDecoder(w.Body).Decode(&company)
	if company.ID != "comp-abc" {
		t.Errorf("expected ID 'comp-abc', got %q", company.ID)
	}
	if company.Name != "My Company" {
		t.Errorf("expected name 'My Company', got %q", company.Name)
	}
}

func TestGetCompanyHandler_NotFound(t *testing.T) {
	db := setupCompaniesTestDB(t)

	router := chi.NewRouter()
	router.Get("/companies/{id}", GetCompanyHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/missing", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestUpdateCompanyHandler(t *testing.T) {
	db := setupCompaniesTestDB(t)
	db.Exec("INSERT INTO companies (id, name, issue_prefix) VALUES ('comp-upd', 'Old Name', 'OLD')")

	router := chi.NewRouter()
	router.Put("/companies/{id}", UpdateCompanyHandler(db))

	body, _ := json.Marshal(map[string]string{"name": "New Name"})
	req := httptest.NewRequest(http.MethodPut, "/companies/comp-upd", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestUpdateCompanyHandler_NotFound(t *testing.T) {
	db := setupCompaniesTestDB(t)

	router := chi.NewRouter()
	router.Put("/companies/{id}", UpdateCompanyHandler(db))

	body, _ := json.Marshal(map[string]string{"name": "Ghost"})
	req := httptest.NewRequest(http.MethodPut, "/companies/missing", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestUpdateCompanyHandler_FilteredFields(t *testing.T) {
	db := setupCompaniesTestDB(t)
	db.Exec("INSERT INTO companies (id, name, issue_prefix) VALUES ('comp-filt', 'Name', 'FLT')")

	router := chi.NewRouter()
	router.Put("/companies/{id}", UpdateCompanyHandler(db))

	// Try to update an un-allowed field (issue_prefix) — should not change
	body, _ := json.Marshal(map[string]interface{}{
		"name":         "Updated",
		"issue_prefix": "BAD",
	})
	req := httptest.NewRequest(http.MethodPut, "/companies/comp-filt", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Verify issue_prefix was not changed
	var company models.Company
	db.First(&company, "id = ?", "comp-filt")
	if company.IssuePrefix != "FLT" {
		t.Errorf("expected issue_prefix 'FLT' unchanged, got %q", company.IssuePrefix)
	}
}

func TestDeleteCompanyHandler(t *testing.T) {
	db := setupCompaniesTestDB(t)
	db.Exec("INSERT INTO companies (id, name, issue_prefix) VALUES ('comp-del', 'Delete Me', 'DEL')")

	router := chi.NewRouter()
	router.Delete("/companies/{id}", DeleteCompanyHandler(db))

	req := httptest.NewRequest(http.MethodDelete, "/companies/comp-del", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}

	var count int64
	db.Model(&models.Company{}).Where("id = ?", "comp-del").Count(&count)
	if count != 0 {
		t.Errorf("expected company to be deleted, found %d row(s)", count)
	}
}

func TestUpdateCompanyBrandingHandler(t *testing.T) {
	db := setupCompaniesTestDB(t)
	db.Exec("INSERT INTO companies (id, name, issue_prefix) VALUES ('comp-brand', 'Branded Co', 'BRD')")

	router := chi.NewRouter()
	router.Put("/companies/{id}/branding", UpdateCompanyBrandingHandler(db))

	color := "#ff0000"
	displayName := "New Brand Name"
	body, _ := json.Marshal(map[string]*string{
		"primaryColor": &color,
		"displayName":  &displayName,
	})
	req := httptest.NewRequest(http.MethodPut, "/companies/comp-brand/branding", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestUpdateCompanyBrandingHandler_NotFound(t *testing.T) {
	db := setupCompaniesTestDB(t)

	router := chi.NewRouter()
	router.Put("/companies/{id}/branding", UpdateCompanyBrandingHandler(db))

	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest(http.MethodPut, "/companies/missing/branding", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestGetCompanyStatsHandler(t *testing.T) {
	db := setupCompaniesTestDB(t)
	db.Exec("INSERT INTO companies (id, name, issue_prefix) VALUES ('c1', 'Co1', 'CO1')")
	db.Exec("INSERT INTO companies (id, name, issue_prefix) VALUES ('c2', 'Co2', 'CO2')")
	db.Exec("INSERT INTO agents (id, company_id, name, status) VALUES ('a1', 'c1', 'Agent1', 'active')")
	db.Exec("INSERT INTO issues (id, company_id, title, status) VALUES ('i1', 'c1', 'Open Issue', 'open')")
	db.Exec("INSERT INTO issues (id, company_id, title, status) VALUES ('i2', 'c1', 'Done Issue', 'done')")

	req := httptest.NewRequest(http.MethodGet, "/companies/stats", nil)
	w := httptest.NewRecorder()

	GetCompanyStatsHandler(db)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var stats map[string]int64
	json.NewDecoder(w.Body).Decode(&stats)
	if stats["totalCompanies"] != 2 {
		t.Errorf("expected 2 total companies, got %d", stats["totalCompanies"])
	}
	if stats["activeAgents"] != 1 {
		t.Errorf("expected 1 active agent, got %d", stats["activeAgents"])
	}
	if stats["openIssues"] != 1 {
		t.Errorf("expected 1 open issue, got %d", stats["openIssues"])
	}
}

func TestExportCompanyHandler_NotImplemented(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/companies/comp-1/exports", nil)
	w := httptest.NewRecorder()

	ExportCompanyHandler()(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d", w.Code)
	}
}

func TestImportCompanyHandler_NotImplemented(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/companies/comp-1/imports/apply", nil)
	w := httptest.NewRecorder()

	ImportCompanyHandler()(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d", w.Code)
	}
}
