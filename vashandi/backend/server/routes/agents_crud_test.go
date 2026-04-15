package routes

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
)

// setupAgentsCRUDTestDB creates an isolated in-memory SQLite database for agent CRUD tests.
func setupAgentsCRUDTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dbName := fmt.Sprintf("file::memory:?cache=shared&agents_crud_%s=1", url.QueryEscape(t.Name()))
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.Exec("DROP TABLE IF EXISTS agent_api_keys")
	db.Exec("DROP TABLE IF EXISTS agent_config_revisions")
	db.Exec("DROP TABLE IF EXISTS agent_task_sessions")
	db.Exec("DROP TABLE IF EXISTS agent_runtime_state")
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
	db.Exec(`CREATE TABLE agent_runtime_state (
		agent_id text PRIMARY KEY,
		company_id text NOT NULL,
		adapter_type text NOT NULL,
		session_id text,
		state_json text NOT NULL DEFAULT '{}',
		last_run_id text,
		last_run_status text,
		total_input_tokens integer NOT NULL DEFAULT 0,
		total_output_tokens integer NOT NULL DEFAULT 0,
		total_cached_input_tokens integer NOT NULL DEFAULT 0,
		total_cost_cents integer NOT NULL DEFAULT 0,
		last_error text,
		created_at datetime DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)
	db.Exec(`CREATE TABLE agent_task_sessions (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		agent_id text NOT NULL,
		adapter_type text NOT NULL,
		task_key text NOT NULL,
		session_params_json text DEFAULT '{}',
		session_display_id text,
		last_run_id text,
		last_error text,
		created_at datetime DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)
	db.Exec(`CREATE TABLE agent_config_revisions (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		agent_id text NOT NULL,
		created_by_agent_id text,
		created_by_user_id text,
		source text NOT NULL DEFAULT 'patch',
		rolled_back_from_revision_id text,
		changed_keys text NOT NULL DEFAULT '[]',
		before_config text NOT NULL,
		after_config text NOT NULL,
		created_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)
	db.Exec(`CREATE TABLE agent_api_keys (
		id text PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
		agent_id text NOT NULL,
		company_id text NOT NULL,
		name text NOT NULL,
		key_hash text NOT NULL,
		last_used_at datetime,
		revoked_at datetime,
		created_at datetime DEFAULT CURRENT_TIMESTAMP
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

func TestGetAgentAPIKeysHandler_StripsSensitiveFields(t *testing.T) {
	db := setupAgentsCRUDTestDB(t)
	db.Exec("INSERT INTO agents (id, company_id, name, role, adapter_type, adapter_config, runtime_config, permissions) VALUES ('api-ag', 'comp-1', 'API Agent', 'general', 'process', '{}', '{}', '{}')")
	db.Exec("INSERT INTO agent_api_keys (id, agent_id, company_id, name, key_hash) VALUES ('key-1', 'api-ag', 'comp-1', 'Primary', 'hash-1')")
	db.Exec("INSERT INTO agent_api_keys (id, agent_id, company_id, name, key_hash, revoked_at) VALUES ('key-2', 'api-ag', 'comp-1', 'Revoked', 'hash-2', CURRENT_TIMESTAMP)")

	router := chi.NewRouter()
	router.Get("/agents/{id}/keys", GetAgentAPIKeysHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/agents/api-ag/keys", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	var keys []map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&keys); err != nil {
		t.Fatalf("decode keys: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected only active keys, got %d", len(keys))
	}
	if _, ok := keys[0]["keyHash"]; ok {
		t.Fatal("expected response to omit keyHash")
	}
	if _, ok := keys[0]["KeyHash"]; ok {
		t.Fatal("expected response to omit KeyHash")
	}
}

func TestCreateAndRevokeAgentAPIKeyHandler(t *testing.T) {
	db := setupAgentsCRUDTestDB(t)
	db.Exec("INSERT INTO agents (id, company_id, name, role, adapter_type, adapter_config, runtime_config, permissions) VALUES ('api-create', 'comp-1', 'API Agent', 'general', 'process', '{}', '{}', '{}')")

	router := chi.NewRouter()
	router.Post("/agents/{id}/keys", CreateAgentAPIKeyHandler(db))
	router.Delete("/agents/{id}/keys/{keyId}", RevokeAgentAPIKeyHandler(db))

	createReq := httptest.NewRequest(http.MethodPost, "/agents/api-create/keys", bytes.NewBufferString(`{"name":"CLI key"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRes := httptest.NewRecorder()
	router.ServeHTTP(createRes, createReq)

	if createRes.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", createRes.Code, createRes.Body.String())
	}

	var created map[string]string
	if err := json.NewDecoder(createRes.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	token := created["token"]
	if !strings.HasPrefix(token, "pcp_agent_") {
		t.Fatalf("expected generated agent token, got %q", token)
	}

	sum := sha256.Sum256([]byte(token))
	expectedHash := hex.EncodeToString(sum[:])

	var stored models.AgentAPIKey
	if err := db.First(&stored, "id = ?", created["id"]).Error; err != nil {
		t.Fatalf("load stored key: %v", err)
	}
	if stored.KeyHash != expectedHash {
		t.Fatalf("expected stored hash %q, got %q", expectedHash, stored.KeyHash)
	}
	if stored.RevokedAt != nil {
		t.Fatal("expected new API key to be active")
	}

	revokeReq := httptest.NewRequest(http.MethodDelete, "/agents/api-create/keys/"+created["id"], nil)
	revokeRes := httptest.NewRecorder()
	router.ServeHTTP(revokeRes, revokeReq)
	if revokeRes.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d; body: %s", revokeRes.Code, revokeRes.Body.String())
	}

	if err := db.First(&stored, "id = ?", created["id"]).Error; err != nil {
		t.Fatalf("reload stored key: %v", err)
	}
	if stored.RevokedAt == nil {
		t.Fatal("expected revoked_at to be set")
	}
}
