package routes

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func setupPortabilityTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(
		"file:portability_"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared",
	), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.Exec(`CREATE TABLE IF NOT EXISTS companies (
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
		logo_asset_id text,
		is_archived boolean NOT NULL DEFAULT 0,
		created_at datetime,
		updated_at datetime
	)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS agents (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		name text NOT NULL,
		role text NOT NULL DEFAULT 'agent',
		title text,
		icon text,
		capabilities text,
		status text NOT NULL DEFAULT 'idle',
		adapter_type text NOT NULL DEFAULT 'process',
		adapter_config text NOT NULL DEFAULT '{}',
		runtime_config text NOT NULL DEFAULT '{}',
		budget_monthly_cents integer NOT NULL DEFAULT 0,
		spent_monthly_cents integer NOT NULL DEFAULT 0,
		permissions text NOT NULL DEFAULT '{}',
		reports_to text,
		pause_reason text,
		paused_at datetime,
		last_heartbeat_at datetime,
		metadata text,
		created_at datetime,
		updated_at datetime
	)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS company_memberships (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		principal_type text NOT NULL,
		principal_id text NOT NULL,
		status text NOT NULL DEFAULT 'active',
		membership_role text,
		created_at datetime,
		updated_at datetime
	)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS projects (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		name text NOT NULL,
		description text,
		status text NOT NULL DEFAULT 'backlog',
		lead_agent_id text,
		target_date datetime,
		color text,
		env text,
		pause_reason text,
		paused_at datetime,
		execution_workspace_policy text,
		archived_at datetime,
		created_at datetime,
		updated_at datetime
	)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS issues (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		project_id text,
		title text NOT NULL,
		description text,
		status text NOT NULL DEFAULT 'backlog',
		priority text NOT NULL DEFAULT 'medium',
		origin_kind text NOT NULL DEFAULT 'manual',
		assignee_agent_id text,
		identifier text,
		billing_code text,
		request_depth integer NOT NULL DEFAULT 0,
		created_at datetime,
		updated_at datetime
	)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS routines (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		project_id text NOT NULL,
		title text NOT NULL,
		description text,
		assignee_agent_id text NOT NULL,
		priority text NOT NULL DEFAULT 'medium',
		status text NOT NULL DEFAULT 'active',
		concurrency_policy text NOT NULL DEFAULT 'coalesce_if_active',
		catch_up_policy text NOT NULL DEFAULT 'skip_missed',
		variables text NOT NULL DEFAULT '[]',
		created_at datetime,
		updated_at datetime
	)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS routine_triggers (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		routine_id text NOT NULL,
		kind text NOT NULL,
		label text,
		enabled boolean NOT NULL DEFAULT 1,
		cron_expression text,
		timezone text,
		signing_mode text,
		replay_window_sec integer,
		created_at datetime,
		updated_at datetime
	)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS company_skills (
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

func TestPreviewExportCompanyHandler(t *testing.T) {
	db := setupPortabilityTestDB(t)
	db.Exec("INSERT INTO companies (id, name, status, issue_prefix, require_board_approval_for_new_agents) VALUES ('comp-export-1', 'Acme Corp', 'active', 'ACM', 1)")
	db.Exec("INSERT INTO agents (id, company_id, name, role, status, adapter_type, adapter_config, runtime_config, permissions) VALUES ('agent-export-1', 'comp-export-1', 'CEO Agent', 'ceo', 'idle', 'process', '{}', '{}', '{}')")

	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = r.WithContext(WithActor(r.Context(), ActorInfo{UserID: "board-user", ActorType: "user"}))
			next.ServeHTTP(w, r)
		})
	})
	router.Post("/companies/{companyId}/exports/preview", PreviewExportCompanyHandler(db))

	body := `{"include":{"company":true,"agents":true}}`
	req := httptest.NewRequest(http.MethodPost, "/companies/comp-export-1/exports/preview", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result["rootPath"] == nil {
		t.Error("expected rootPath in response")
	}
	if result["manifest"] == nil {
		t.Error("expected manifest in response")
	}
	if result["fileInventory"] == nil {
		t.Error("expected fileInventory in response")
	}
}

func TestExportCompanyHandler(t *testing.T) {
	db := setupPortabilityTestDB(t)
	db.Exec("INSERT INTO companies (id, name, status, issue_prefix, require_board_approval_for_new_agents) VALUES ('comp-export-2', 'Beta Inc', 'active', 'BET', 1)")

	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = r.WithContext(WithActor(r.Context(), ActorInfo{UserID: "board-user", ActorType: "user"}))
			next.ServeHTTP(w, r)
		})
	})
	router.Post("/companies/{companyId}/exports", ExportCompanyHandler(db))

	body := `{"include":{"company":true,"agents":false}}`
	req := httptest.NewRequest(http.MethodPost, "/companies/comp-export-2/exports", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result["files"] == nil {
		t.Error("expected files in response")
	}
}

func TestPreviewImportCompanyHandler(t *testing.T) {
	db := setupPortabilityTestDB(t)
	db.Exec("INSERT INTO companies (id, name, status, issue_prefix, require_board_approval_for_new_agents) VALUES ('comp-import-target', 'Target Corp', 'active', 'TGT', 1)")

	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = r.WithContext(WithActor(r.Context(), ActorInfo{UserID: "board-user", ActorType: "user"}))
			next.ServeHTTP(w, r)
		})
	})
	router.Post("/companies/{companyId}/imports/preview", PreviewImportCompanyHandler(db))

	body := `{
		"source": {
			"type": "inline",
			"files": {
				"COMPANY.md": "---\nname: Source Co\n---\n"
			}
		},
		"include": {"company": true, "agents": false},
		"target": {"mode": "existing_company", "companyId": "comp-import-target"}
	}`
	req := httptest.NewRequest(http.MethodPost, "/companies/comp-import-target/imports/preview", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result["plan"] == nil {
		t.Error("expected plan in response")
	}
}

func TestPreviewImportCompanyHandler_AgentSafeRouteRejectsDifferentTargetCompany(t *testing.T) {
	db := setupPortabilityTestDB(t)
	db.Exec("INSERT INTO companies (id, name, status, issue_prefix, require_board_approval_for_new_agents) VALUES ('route-company', 'Route Co', 'active', 'RTE', 1)")
	db.Exec("INSERT INTO companies (id, name, status, issue_prefix, require_board_approval_for_new_agents) VALUES ('other-company', 'Other Co', 'active', 'OTH', 1)")
	db.Exec("INSERT INTO agents (id, company_id, name, role, status, adapter_type, adapter_config, runtime_config, permissions) VALUES ('ceo-agent-route', 'route-company', 'CEO', 'ceo', 'idle', 'process', '{}', '{}', '{}')")

	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = r.WithContext(WithActor(r.Context(), ActorInfo{
				AgentID:   "ceo-agent-route",
				CompanyID: "route-company",
				IsAgent:   true,
				ActorType: "agent",
			}))
			next.ServeHTTP(w, r)
		})
	})
	router.Post("/companies/{companyId}/imports/preview", PreviewImportCompanyHandler(db))

	body := `{
		"source": {"type": "inline", "files": {"COMPANY.md": "---\nname: Source Co\n---\n"}},
		"include": {"company": true},
		"target": {"mode": "existing_company", "companyId": "other-company"}
	}`
	req := httptest.NewRequest(http.MethodPost, "/companies/route-company/imports/preview", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestImportCompanyHandler(t *testing.T) {
	db := setupPortabilityTestDB(t)
	db.Exec("INSERT INTO companies (id, name, status, issue_prefix, require_board_approval_for_new_agents) VALUES ('comp-import-dest', 'Dest Corp', 'active', 'DST', 1)")

	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = r.WithContext(WithActor(r.Context(), ActorInfo{UserID: "board-user", ActorType: "user"}))
			next.ServeHTTP(w, r)
		})
	})
	router.Post("/companies/{companyId}/imports/apply", ImportCompanyHandler(db))

	body := `{
		"source": {
			"type": "inline",
			"files": {
				"COMPANY.md": "---\nname: Imported Co\ndescription: Test import\n---\n"
			}
		},
		"include": {"company": true, "agents": false},
		"target": {"mode": "existing_company", "companyId": "comp-import-dest"}
	}`
	req := httptest.NewRequest(http.MethodPost, "/companies/comp-import-dest/imports/apply", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result["company"] == nil {
		t.Error("expected company in response")
	}
}

func TestImportCompanyHandler_AgentSafeRouteRejectsReplace(t *testing.T) {
	db := setupPortabilityTestDB(t)
	db.Exec("INSERT INTO companies (id, name, status, issue_prefix, require_board_approval_for_new_agents) VALUES ('route-company-import', 'Route Import Co', 'active', 'RTC', 1)")
	db.Exec("INSERT INTO agents (id, company_id, name, role, status, adapter_type, adapter_config, runtime_config, permissions) VALUES ('ceo-agent-import', 'route-company-import', 'CEO', 'ceo', 'idle', 'process', '{}', '{}', '{}')")

	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = r.WithContext(WithActor(r.Context(), ActorInfo{
				AgentID:   "ceo-agent-import",
				CompanyID: "route-company-import",
				IsAgent:   true,
				ActorType: "agent",
			}))
			next.ServeHTTP(w, r)
		})
	})
	router.Post("/companies/{companyId}/imports/apply", ImportCompanyHandler(db))

	body := `{
		"source": {"type": "inline", "files": {"COMPANY.md": "---\nname: Imported Co\n---\n"}},
		"include": {"company": true},
		"target": {"mode": "existing_company", "companyId": "route-company-import"},
		"collisionStrategy": "replace"
	}`
	req := httptest.NewRequest(http.MethodPost, "/companies/route-company-import/imports/apply", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPortabilityExport_AgentCEOAllowed(t *testing.T) {
	db := setupPortabilityTestDB(t)
	db.Exec("INSERT INTO companies (id, name, status, issue_prefix, require_board_approval_for_new_agents) VALUES ('comp-ceo-1', 'CEO Co', 'active', 'CEO', 1)")
	db.Exec("INSERT INTO agents (id, company_id, name, role, status, adapter_type, adapter_config, runtime_config, permissions) VALUES ('ceo-agent-1', 'comp-ceo-1', 'The CEO', 'ceo', 'idle', 'process', '{}', '{}', '{}')")

	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = r.WithContext(WithActor(r.Context(), ActorInfo{
				AgentID:   "ceo-agent-1",
				CompanyID: "comp-ceo-1",
				IsAgent:   true,
				ActorType: "agent",
			}))
			next.ServeHTTP(w, r)
		})
	})
	router.Post("/companies/{companyId}/exports/preview", PreviewExportCompanyHandler(db))

	body := `{"include":{"company":true}}`
	req := httptest.NewRequest(http.MethodPost, "/companies/comp-ceo-1/exports/preview", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for CEO agent, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPortabilityExport_NonCEOAgentForbidden(t *testing.T) {
	db := setupPortabilityTestDB(t)
	db.Exec("INSERT INTO companies (id, name, status, issue_prefix, require_board_approval_for_new_agents) VALUES ('comp-nceo-1', 'NonCEO Co', 'active', 'NCO', 1)")
	db.Exec("INSERT INTO agents (id, company_id, name, role, status, adapter_type, adapter_config, runtime_config, permissions) VALUES ('eng-agent-1', 'comp-nceo-1', 'Engineer', 'engineer', 'idle', 'process', '{}', '{}', '{}')")

	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = r.WithContext(WithActor(r.Context(), ActorInfo{
				AgentID:   "eng-agent-1",
				CompanyID: "comp-nceo-1",
				IsAgent:   true,
				ActorType: "agent",
			}))
			next.ServeHTTP(w, r)
		})
	})
	router.Post("/companies/{companyId}/exports/preview", PreviewExportCompanyHandler(db))

	body := `{"include":{"company":true}}`
	req := httptest.NewRequest(http.MethodPost, "/companies/comp-nceo-1/exports/preview", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for non-CEO agent, got %d", w.Code)
	}
}

func TestPortabilityImport_NewCompany(t *testing.T) {
	db := setupPortabilityTestDB(t)

	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = r.WithContext(WithActor(r.Context(), ActorInfo{UserID: "board-user", ActorType: "user"}))
			next.ServeHTTP(w, r)
		})
	})
	router.Post("/companies/{companyId}/imports/apply", ImportCompanyHandler(db))

	body := `{
		"source": {
			"type": "inline",
			"files": {
				"COMPANY.md": "---\nname: Brand New Co\n---\n",
				"agents/my-agent/AGENTS.md": "---\nname: My Agent\n---\nYou are a helpful agent.\n"
			}
		},
		"include": {"company": true, "agents": true},
		"target": {"mode": "new_company", "newCompanyName": "Brand New Co"}
	}`
	req := httptest.NewRequest(http.MethodPost, "/companies/whatever/imports/apply", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result map[string]interface{}
	json.NewDecoder(w.Body).Decode(&result)
	company, ok := result["company"].(map[string]interface{})
	if !ok || company["action"] != "created" {
		t.Errorf("expected company.action = 'created', got %v", result["company"])
	}
	agents, ok := result["agents"].([]interface{})
	if !ok || len(agents) == 0 {
		t.Error("expected at least one agent in result")
	} else {
		ag := agents[0].(map[string]interface{})
		if ag["action"] != "created" {
			t.Errorf("expected agent.action = 'created', got %v", ag["action"])
		}
	}
}
