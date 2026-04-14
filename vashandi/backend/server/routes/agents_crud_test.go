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

// setupAgentsCRUDTestDB creates an isolated in-memory SQLite database for agent CRUD tests.
func setupAgentsCRUDTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&agents_crud=1"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.Exec("DROP TABLE IF EXISTS agents")
	db.Exec(`CREATE TABLE agents (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		name text NOT NULL,
		role text NOT NULL DEFAULT 'general',
		title text,
		icon text,
		status text NOT NULL DEFAULT 'idle',
		reports_to text,
		capabilities text,
		adapter_type text NOT NULL DEFAULT 'process',
		adapter_config text NOT NULL DEFAULT '{}',
		runtime_config text NOT NULL DEFAULT '{}',
		budget_monthly_cents integer NOT NULL DEFAULT 0,
		spent_monthly_cents integer NOT NULL DEFAULT 0,
		pause_reason text,
		paused_at datetime,
		permissions text NOT NULL DEFAULT '{}',
		last_heartbeat_at datetime,
		metadata text,
		created_at datetime,
		updated_at datetime,
		deleted_at datetime
	)`)
	return db
}

func TestListAgentsHandler_CompanyScoping(t *testing.T) {
	db := setupAgentsCRUDTestDB(t)
	db.Exec("INSERT INTO agents (id, company_id, name, role, adapter_type, adapter_config, runtime_config, permissions) VALUES ('a1', 'comp-1', 'Agent One', 'general', 'process', '{}', '{}', '{}')")
	db.Exec("INSERT INTO agents (id, company_id, name, role, adapter_type, adapter_config, runtime_config, permissions) VALUES ('a2', 'comp-2', 'Agent Two', 'general', 'process', '{}', '{}', '{}')")

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/agents", ListAgentsHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-1/agents", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var agents []models.Agent
	json.NewDecoder(w.Body).Decode(&agents)
	if len(agents) != 1 {
		t.Errorf("expected 1 agent for comp-1, got %d", len(agents))
	}
	if len(agents) > 0 && agents[0].CompanyID != "comp-1" {
		t.Errorf("expected agent scoped to comp-1, got %q", agents[0].CompanyID)
	}
}

func TestListAgentsHandler_EmptyList(t *testing.T) {
	db := setupAgentsCRUDTestDB(t)

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/agents", ListAgentsHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-empty/agents", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var agents []models.Agent
	json.NewDecoder(w.Body).Decode(&agents)
	if len(agents) != 0 {
		t.Errorf("expected 0 agents, got %d", len(agents))
	}
}

func TestGetAgentHandler_Found(t *testing.T) {
	db := setupAgentsCRUDTestDB(t)
	db.Exec("INSERT INTO agents (id, company_id, name, role, adapter_type, adapter_config, runtime_config, permissions) VALUES ('ag-1', 'comp-1', 'Findable', 'general', 'process', '{}', '{}', '{}')")

	router := chi.NewRouter()
	router.Get("/agents/{id}", GetAgentHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/agents/ag-1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var agent models.Agent
	json.NewDecoder(w.Body).Decode(&agent)
	if agent.ID != "ag-1" {
		t.Errorf("expected ID 'ag-1', got %q", agent.ID)
	}
	if agent.Name != "Findable" {
		t.Errorf("expected Name 'Findable', got %q", agent.Name)
	}
}

func TestGetAgentHandler_NotFound(t *testing.T) {
	db := setupAgentsCRUDTestDB(t)

	router := chi.NewRouter()
	router.Get("/agents/{id}", GetAgentHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/agents/nonexistent-agent", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestCreateAgentHandler_SetsCompanyID(t *testing.T) {
	db := setupAgentsCRUDTestDB(t)

	router := chi.NewRouter()
	router.Post("/companies/{companyId}/agents", CreateAgentHandler(db, nil))

	body, _ := json.Marshal(map[string]string{
		"name": "New Agent",
		"role": "assistant",
	})
	req := httptest.NewRequest(http.MethodPost, "/companies/comp-new/agents", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", w.Code, w.Body.String())
	}

	var agent models.Agent
	json.NewDecoder(w.Body).Decode(&agent)
	if agent.CompanyID != "comp-new" {
		t.Errorf("expected CompanyID 'comp-new', got %q", agent.CompanyID)
	}
}

func TestCreateAgentHandler_DefaultsPermissions(t *testing.T) {
	db := setupAgentsCRUDTestDB(t)

	router := chi.NewRouter()
	router.Post("/companies/{companyId}/agents", CreateAgentHandler(db, nil))

	body, _ := json.Marshal(map[string]string{"name": "Perm Agent"})
	req := httptest.NewRequest(http.MethodPost, "/companies/comp-1/agents", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", w.Code, w.Body.String())
	}

	var agent models.Agent
	json.NewDecoder(w.Body).Decode(&agent)
	// Permissions should be non-empty (default "{}")
	if len(agent.Permissions) == 0 {
		t.Error("expected non-empty Permissions on created agent")
	}
}

func TestCreateAgentHandler_BadBody(t *testing.T) {
	db := setupAgentsCRUDTestDB(t)

	router := chi.NewRouter()
	router.Post("/companies/{companyId}/agents", CreateAgentHandler(db, nil))

	req := httptest.NewRequest(http.MethodPost, "/companies/comp-1/agents", bytes.NewBufferString("notjson"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestUpdateAgentHandler_UpdatesName(t *testing.T) {
	db := setupAgentsCRUDTestDB(t)
	db.Exec("INSERT INTO agents (id, company_id, name, role, adapter_type, adapter_config, runtime_config, permissions) VALUES ('upd-ag', 'comp-1', 'Old Name', 'general', 'process', '{}', '{}', '{}')")

	router := chi.NewRouter()
	router.Patch("/agents/{id}", UpdateAgentHandler(db))

	body, _ := json.Marshal(map[string]string{"name": "New Name"})
	req := httptest.NewRequest(http.MethodPatch, "/agents/upd-ag", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	var agent models.Agent
	json.NewDecoder(w.Body).Decode(&agent)
	if agent.Name != "New Name" {
		t.Errorf("expected Name 'New Name', got %q", agent.Name)
	}
}

func TestUpdateAgentHandler_NotFound(t *testing.T) {
	db := setupAgentsCRUDTestDB(t)

	router := chi.NewRouter()
	router.Patch("/agents/{id}", UpdateAgentHandler(db))

	body, _ := json.Marshal(map[string]string{"name": "Ghost"})
	req := httptest.NewRequest(http.MethodPatch, "/agents/ghost-id", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestUpdateAgentHandler_MergesRuntimeConfig(t *testing.T) {
	db := setupAgentsCRUDTestDB(t)
	db.Exec(`INSERT INTO agents (id, company_id, name, role, adapter_type, adapter_config, runtime_config, permissions) VALUES ('rc-ag', 'comp-1', 'RC Agent', 'general', 'process', '{}', '{"existing":"value"}', '{}')`)

	router := chi.NewRouter()
	router.Patch("/agents/{id}", UpdateAgentHandler(db))

	body, _ := json.Marshal(map[string]interface{}{
		"runtimeConfig": map[string]interface{}{"newKey": "newValue"},
	})
	req := httptest.NewRequest(http.MethodPatch, "/agents/rc-ag", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	var agent models.Agent
	json.NewDecoder(w.Body).Decode(&agent)

	var rc map[string]interface{}
	json.Unmarshal(agent.RuntimeConfig, &rc)
	if rc["newKey"] != "newValue" {
		t.Errorf("expected newKey='newValue' in merged runtimeConfig, got %v", rc["newKey"])
	}
}

func TestPauseAgentHandler(t *testing.T) {
	db := setupAgentsCRUDTestDB(t)
	db.Exec("INSERT INTO agents (id, company_id, name, role, status, adapter_type, adapter_config, runtime_config, permissions) VALUES ('pause-ag', 'comp-1', 'Pause Me', 'general', 'idle', 'process', '{}', '{}', '{}')")

	router := chi.NewRouter()
	router.Post("/agents/{id}/pause", PauseAgentHandler(db))

	req := httptest.NewRequest(http.MethodPost, "/agents/pause-ag/pause", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	var agent models.Agent
	json.NewDecoder(w.Body).Decode(&agent)
	if agent.Status != "paused" {
		t.Errorf("expected Status 'paused', got %q", agent.Status)
	}
}

func TestResumeAgentHandler(t *testing.T) {
	db := setupAgentsCRUDTestDB(t)
	db.Exec("INSERT INTO agents (id, company_id, name, role, status, adapter_type, adapter_config, runtime_config, permissions) VALUES ('resume-ag', 'comp-1', 'Resume Me', 'general', 'paused', 'process', '{}', '{}', '{}')")

	router := chi.NewRouter()
	router.Post("/agents/{id}/resume", ResumeAgentHandler(db))

	req := httptest.NewRequest(http.MethodPost, "/agents/resume-ag/resume", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	var agent models.Agent
	json.NewDecoder(w.Body).Decode(&agent)
	if agent.Status != "active" {
		t.Errorf("expected Status 'active', got %q", agent.Status)
	}
}
