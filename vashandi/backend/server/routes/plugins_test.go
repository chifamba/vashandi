package routes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chifamba/vashandi/vashandi/backend/server/services"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type mockPluginToolExecutor struct {
	result interface{}
	err    error
}

func (f mockPluginToolExecutor) ExecuteTool(_ context.Context, _ string, _ interface{}, _ interface{}) (interface{}, error) {
	return f.result, f.err
}

func setupPluginsRouteTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	// Use test name as part of DSN to ensure isolation.
	dsn := "file::memory:?cache=shared&plugins_route_test_" + strings.ReplaceAll(t.Name(), "/", "_") + "=1"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.Exec("DROP TABLE IF EXISTS plugins")
	db.Exec("DROP TABLE IF EXISTS activity_log")
	db.Exec(`CREATE TABLE plugins (
		id text PRIMARY KEY,
		plugin_key text NOT NULL UNIQUE,
		package_name text NOT NULL,
		version text NOT NULL,
		api_version integer NOT NULL DEFAULT 1,
		categories text NOT NULL DEFAULT '[]',
		manifest_json text NOT NULL DEFAULT '{}',
		status text NOT NULL DEFAULT 'installed',
		install_order integer,
		package_path text,
		last_error text,
		installed_at datetime DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)
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
		created_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)
	return db
}

func TestListPluginsHandler_Empty(t *testing.T) {
	db := setupPluginsRouteTestDB(t)
	activity := services.NewActivityService(db)

	req := httptest.NewRequest(http.MethodGet, "/plugins", nil)
	req = withBoardActorRequest(req)
	w := httptest.NewRecorder()

	ListPluginsHandler(db, activity)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var plugins []interface{}
	if err := json.NewDecoder(w.Body).Decode(&plugins); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins, got %d", len(plugins))
	}
}

func TestListPluginsHandler_WithPlugins(t *testing.T) {
	db := setupPluginsRouteTestDB(t)
	activity := services.NewActivityService(db)

	db.Exec("INSERT INTO plugins (id, plugin_key, package_name, version, manifest_json, status) VALUES ('p1', 'slack', 'slack-plugin', '1.0.0', '{}', 'installed')")
	db.Exec("INSERT INTO plugins (id, plugin_key, package_name, version, manifest_json, status) VALUES ('p2', 'removed', 'removed-plugin', '1.0.0', '{}', 'uninstalled')")

	req := httptest.NewRequest(http.MethodGet, "/plugins", nil)
	req = withBoardActorRequest(req)
	w := httptest.NewRecorder()

	ListPluginsHandler(db, activity)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var plugins []interface{}
	json.NewDecoder(w.Body).Decode(&plugins) //nolint:errcheck
	if len(plugins) != 1 {
		t.Errorf("expected 1 installed plugin, got %d", len(plugins))
	}
}

func TestListPluginsHandler_ContentType(t *testing.T) {
	db := setupPluginsRouteTestDB(t)
	activity := services.NewActivityService(db)

	req := httptest.NewRequest(http.MethodGet, "/plugins", nil)
	req = withBoardActorRequest(req)
	w := httptest.NewRecorder()

	ListPluginsHandler(db, activity)(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
}

func TestListPluginsHandler_Unauthenticated(t *testing.T) {
	db := setupPluginsRouteTestDB(t)
	activity := services.NewActivityService(db)

	req := httptest.NewRequest(http.MethodGet, "/plugins", nil)
	// No actor set — should get 403.
	w := httptest.NewRecorder()

	ListPluginsHandler(db, activity)(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for unauthenticated request, got %d", w.Code)
	}
}

func TestListPluginsHandler_StatusFilter(t *testing.T) {
	db := setupPluginsRouteTestDB(t)
	activity := services.NewActivityService(db)

	db.Exec("INSERT INTO plugins (id, plugin_key, package_name, version, manifest_json, status) VALUES ('p1', 'slack', 'slack-plugin', '1.0.0', '{}', 'ready')")
	db.Exec("INSERT INTO plugins (id, plugin_key, package_name, version, manifest_json, status) VALUES ('p2', 'github', 'gh-plugin', '1.0.0', '{}', 'disabled')")

	req := httptest.NewRequest(http.MethodGet, "/plugins?status=ready", nil)
	req = withBoardActorRequest(req)
	w := httptest.NewRecorder()

	ListPluginsHandler(db, activity)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var plugins []interface{}
	json.NewDecoder(w.Body).Decode(&plugins) //nolint:errcheck
	if len(plugins) != 1 {
		t.Errorf("expected 1 ready plugin, got %d", len(plugins))
	}
}

func TestListPluginsHandler_InvalidStatusFilter(t *testing.T) {
	db := setupPluginsRouteTestDB(t)
	activity := services.NewActivityService(db)

	req := httptest.NewRequest(http.MethodGet, "/plugins?status=bogus", nil)
	req = withBoardActorRequest(req)
	w := httptest.NewRecorder()

	ListPluginsHandler(db, activity)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid status filter, got %d", w.Code)
	}
}

func TestGetPluginExamplesHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/plugins/examples", nil)
	req = withBoardActorRequest(req)
	w := httptest.NewRecorder()

	GetPluginExamplesHandler()(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var examples []interface{}
	if err := json.NewDecoder(w.Body).Decode(&examples); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(examples) == 0 {
		t.Errorf("expected at least one bundled example")
	}
}

func TestGetPluginHealthHandler(t *testing.T) {
	db := setupPluginsRouteTestDB(t)
	db.Exec("INSERT INTO plugins (id, plugin_key, package_name, version, manifest_json, status) VALUES ('ph1', 'healthy.plugin', 'healthy-pkg', '1.0.0', '{\"id\":\"healthy.plugin\"}', 'ready')")

	req := httptest.NewRequest(http.MethodGet, "/plugins/ph1/health", nil)
	req = withBoardActorRequest(req)
	w := httptest.NewRecorder()

	// Use a chi router context so chi.URLParam works.
	chiCtx := newChiCtxWithParams(req, map[string]string{"pluginId": "ph1"})
	GetPluginHealthHandler(db)(w, chiCtx)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if result["healthy"] != true {
		t.Errorf("expected healthy=true for ready plugin")
	}
}

func TestGetPluginHandler_NotFound(t *testing.T) {
	db := setupPluginsRouteTestDB(t)

	req := httptest.NewRequest(http.MethodGet, "/plugins/nonexistent", nil)
	req = withBoardActorRequest(req)
	w := httptest.NewRecorder()

	chiCtx := newChiCtxWithParams(req, map[string]string{"pluginId": "nonexistent"})
	GetPluginHandler(db)(w, chiCtx)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestExecutePluginToolHandler_LogsMCPInvocation(t *testing.T) {
	db := setupPluginsRouteTestDB(t)
	activity := services.NewActivityService(db)

	body := strings.NewReader(`{
		"tool":"plugin.tool",
		"parameters":{"q":"search"},
		"runContext":{"companyId":"comp-a","agentId":"agent-1","runId":"run-1","projectId":"proj-1"}
	}`)
	req := httptest.NewRequest(http.MethodPost, "/plugins/tools/execute", body)
	req = withBoardActorRequest(req)
	w := httptest.NewRecorder()

	ExecutePluginToolHandler(mockPluginToolExecutor{result: map[string]string{"status": "ok"}}, activity)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var count int64
	db.Raw("SELECT COUNT(*) FROM activity_log WHERE company_id = ? AND action = ?", "comp-a", "mcp_tool_invoked").Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 logged MCP invocation, got %d", count)
	}
}
