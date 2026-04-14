package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupMCPGovernanceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&mcp_gov_test=1"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.Exec("DROP TABLE IF EXISTS agent_mcp_entitlements")
	db.Exec("DROP TABLE IF EXISTS mcp_entitlement_profiles")
	db.Exec("DROP TABLE IF EXISTS mcp_tool_definitions")
	db.Exec(`CREATE TABLE mcp_tool_definitions (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		name text NOT NULL,
		description text,
		schema_json text NOT NULL DEFAULT '{}',
		source text NOT NULL,
		created_at datetime,
		updated_at datetime
	)`)
	db.Exec(`CREATE TABLE mcp_entitlement_profiles (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		name text NOT NULL,
		tool_ids text NOT NULL DEFAULT '',
		created_at datetime,
		updated_at datetime
	)`)
	db.Exec(`CREATE TABLE agent_mcp_entitlements (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		agent_id text NOT NULL,
		profile_id text NOT NULL,
		created_at datetime,
		updated_at datetime
	)`)
	return db
}

func TestMCPToolsHandler_CompanyScoping(t *testing.T) {
	db := setupMCPGovernanceTestDB(t)
	db.Exec("INSERT INTO mcp_tool_definitions (id, company_id, name, source) VALUES ('t1', 'c1', 'read_file', 'builtin')")
	db.Exec("INSERT INTO mcp_tool_definitions (id, company_id, name, source) VALUES ('t2', 'c2', 'write_file', 'builtin')")

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/mcp/tools", MCPToolsHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/c1/mcp/tools", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var tools []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&tools)
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool for c1, got %d", len(tools))
	}
	if tools[0]["name"] != "read_file" {
		t.Errorf("expected tool name 'read_file', got %v", tools[0]["name"])
	}
}

func TestMCPToolsHandler_Empty(t *testing.T) {
	db := setupMCPGovernanceTestDB(t)

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/mcp/tools", MCPToolsHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/c1/mcp/tools", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}
}

func TestMCPToolsHandler_MissingCompanyId(t *testing.T) {
	db := setupMCPGovernanceTestDB(t)

	// When companyId is empty string in the URL param (chi will provide empty string)
	handler := MCPToolsHandler(db)
	req := httptest.NewRequest(http.MethodGet, "/mcp/tools", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing companyId, got %d", w.Code)
	}
}

func TestMCPProfilesHandler_CompanyScoping(t *testing.T) {
	db := setupMCPGovernanceTestDB(t)
	db.Exec("INSERT INTO mcp_entitlement_profiles (id, company_id, name, tool_ids) VALUES ('p1', 'c1', 'default', '{}')")
	db.Exec("INSERT INTO mcp_entitlement_profiles (id, company_id, name, tool_ids) VALUES ('p2', 'c2', 'admin', '{}')")

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/mcp/profiles", MCPProfilesHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/c1/mcp/profiles", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var profiles []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&profiles)
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile for c1, got %d", len(profiles))
	}
}

func TestMCPProfilesHandler_ContentType(t *testing.T) {
	db := setupMCPGovernanceTestDB(t)

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/mcp/profiles", MCPProfilesHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/c1/mcp/profiles", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}
}

func TestAgentMCPToolsHandler_NoEntitlements(t *testing.T) {
	db := setupMCPGovernanceTestDB(t)

	router := chi.NewRouter()
	router.Get("/agents/{agentId}/mcp/tools", AgentMCPToolsHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/agents/a1/mcp/tools", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var tools []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&tools)
	if len(tools) != 0 {
		t.Fatalf("expected 0 tools for agent without entitlements, got %d", len(tools))
	}
}

func TestAgentMCPToolsHandler_MissingAgentId(t *testing.T) {
	db := setupMCPGovernanceTestDB(t)

	handler := AgentMCPToolsHandler(db)
	req := httptest.NewRequest(http.MethodGet, "/mcp/tools", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing agentId, got %d", w.Code)
	}
}
