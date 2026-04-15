package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"github.com/chifamba/vashandi/vashandi/backend/server/services"
)

// mockMemoryAdapter is a test double for services.MemoryAdapter that routes
// RegisterAgent / DeregisterAgent calls as HTTP requests to a configurable base URL.
type mockMemoryAdapter struct {
	baseURL string
}

func (m *mockMemoryAdapter) IngestMemory(_ context.Context, _, _ string, _ map[string]string) error {
	return nil
}
func (m *mockMemoryAdapter) CreateMemory(_ context.Context, _ string, _ services.MemoryPayload) error {
	return nil
}
func (m *mockMemoryAdapter) QueryMemory(_ context.Context, _, _ string, _ int) ([]services.MemoryResult, error) {
	return nil, nil
}
func (m *mockMemoryAdapter) CompileContext(_ context.Context, _ services.ContextRequest) (map[string]interface{}, error) {
	return nil, nil
}
func (m *mockMemoryAdapter) RegisterAgent(_ context.Context, namespaceID, agentID, name string) error {
	body, _ := json.Marshal(map[string]string{"agent_id": agentID, "name": name})
	url := fmt.Sprintf("%s/internal/v1/namespaces/%s/agents", m.baseURL, namespaceID)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body)) //nolint:noctx
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
func (m *mockMemoryAdapter) DeregisterAgent(_ context.Context, namespaceID, agentID string) error {
	url := fmt.Sprintf("%s/internal/v1/namespaces/%s/agents/%s", m.baseURL, namespaceID, agentID)
	req, _ := http.NewRequest(http.MethodDelete, url, nil)
	resp, err := http.DefaultClient.Do(req) //nolint:noctx
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
func (m *mockMemoryAdapter) HandleTrigger(_ context.Context, _, _ string, _ services.TriggerRequest) (*services.TriggerResponse, error) {
	return nil, nil
}
func (m *mockMemoryAdapter) ExportAudit(_ context.Context, _, _ string) ([]byte, string, error) {
	return nil, "", nil
}
func (m *mockMemoryAdapter) ArchiveNamespace(_ context.Context, _ string) error { return nil }
func (m *mockMemoryAdapter) DeleteNamespace(_ context.Context, _ string) error  { return nil }
func (m *mockMemoryAdapter) ListProposals(_ context.Context, _ string) ([]map[string]interface{}, error) {
	return nil, nil
}
func (m *mockMemoryAdapter) ResolveProposal(_ context.Context, _, _, _ string) error { return nil }

func setupTestDB(t *testing.T) *gorm.DB {
	// Use unique in-memory db per test
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	// AutoMigrate is better, we'll try to use sqlite compatible struct or just provide fake types
	db.Exec("DROP TABLE IF EXISTS agents;")
	err = db.Exec(`CREATE TABLE agents (
        id text PRIMARY KEY,
        company_id text,
        name text,
        role text,
        title text,
        icon text,
        status text,
        reports_to text,
        capabilities text,
        adapter_type text,
        adapter_config text,
        runtime_config text,
        budget_monthly_cents integer,
        spent_monthly_cents integer,
        pause_reason text,
        paused_at datetime,
        permissions text,
        last_heartbeat_at datetime,
        metadata text,
        created_at datetime,
        updated_at datetime,
        deleted_at datetime
    )`).Error
	if err != nil {
		t.Fatalf("Failed to migrate test database: %v", err)
	}

	return db
}

func TestCreateAgentHandlerWebhook(t *testing.T) {
	db := setupTestDB(t)

	var wg sync.WaitGroup
	wg.Add(1)

	var receivedMethod string
	var receivedPath string
	var receivedBody map[string]string

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedPath = r.URL.Path

		if err := json.NewDecoder(r.Body).Decode(&receivedBody); err == nil {
			// Body decoded
		}

		w.WriteHeader(http.StatusOK)
		wg.Done()
	}))
	defer mockServer.Close()

	router := chi.NewRouter()
	router.Post("/companies/{companyId}/agents", CreateAgentHandler(db, &mockMemoryAdapter{baseURL: mockServer.URL}))

	agentData := map[string]string{
		"id":   "agent-123",
		"name": "Test Agent",
		"role": "Assistant",
	}
	bodyBytes, _ := json.Marshal(agentData)
	req := httptest.NewRequest("POST", "/companies/comp-123/agents", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("Expected status 201, got %d. Body: %s", w.Code, w.Body.String())
	}

	c := make(chan struct{})
	go func() {
		wg.Wait()
		close(c)
	}()

	select {
	case <-c:
		// success
	case <-time.After(2 * time.Second):
		t.Fatalf("Timeout waiting for webhook request")
	}

	if receivedMethod != "POST" {
		t.Errorf("Expected method POST, got %s", receivedMethod)
	}
	if !strings.Contains(receivedPath, "/internal/v1/namespaces/comp-123/agents") {
		t.Errorf("Expected path to contain namespace comp-123, got %s", receivedPath)
	}
	if receivedBody["agent_id"] == "" {
		t.Errorf("Expected agent_id in body, got empty")
	}
}

func TestDeleteAgentHandlerWebhook(t *testing.T) {
	db := setupTestDB(t)

	agent := models.Agent{
		ID:        "agent-123",
		CompanyID: "comp-123",
		Name:      "Test Agent",
		Role:      "Assistant",
	}
	db.Create(&agent)

	var wg sync.WaitGroup
	wg.Add(1)

	var receivedMethod string
	var receivedPath string

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		wg.Done()
	}))
	defer mockServer.Close()

	router := chi.NewRouter()
	router.Delete("/agents/{id}", DeleteAgentHandler(db, &mockMemoryAdapter{baseURL: mockServer.URL}))

	req := httptest.NewRequest("DELETE", "/agents/agent-123", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("Expected status 204, got %d", w.Code)
	}

	c := make(chan struct{})
	go func() {
		wg.Wait()
		close(c)
	}()

	select {
	case <-c:
		// success
	case <-time.After(2 * time.Second):
		t.Fatalf("Timeout waiting for webhook request")
	}

	if receivedMethod != "DELETE" {
		t.Errorf("Expected method DELETE, got %s", receivedMethod)
	}
	if !strings.Contains(receivedPath, "/internal/v1/namespaces/comp-123/agents/agent-123") {
		t.Errorf("Expected path to contain namespace and agent ID, got %s", receivedPath)
	}

	var count int64
	db.Model(&models.Agent{}).Where("id = ?", "agent-123").Count(&count)
	if count != 0 {
		t.Errorf("Expected agent to be deleted from DB, but found %d", count)
	}
}

type roundTripperFunc struct {
	fn func(*http.Request) (*http.Response, error)
}

func (r *roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return r.fn(req)
}

func TestGetAdapterModelsHandler_ReturnsAdapterModelArray(t *testing.T) {
	router := chi.NewRouter()
	router.Get("/companies/{companyId}/adapters/{type}/models", GetAdapterModelsHandler(nil))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-1/adapters/codex/models", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	if got := w.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", got)
	}

	var models []map[string]string
	if err := json.NewDecoder(w.Body).Decode(&models); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("expected non-empty model list")
	}
	if models[0]["id"] == "" || models[0]["label"] == "" {
		t.Fatalf("expected adapter model objects with id and label, got %#v", models[0])
	}
}

func TestGetAdapterModelsHandler_UnknownAdapterReturnsEmptyArray(t *testing.T) {
	router := chi.NewRouter()
	router.Get("/companies/{companyId}/adapters/{type}/models", GetAdapterModelsHandler(nil))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-1/adapters/unknown/models", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var models []map[string]string
	if err := json.NewDecoder(w.Body).Decode(&models); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(models) != 0 {
		t.Fatalf("expected empty array for unknown adapter, got %#v", models)
	}
}
