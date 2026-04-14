package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chifamba/vashandi/vashandi/backend/server/services"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupPluginsRouteTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&plugins_route_test=1"), &gorm.Config{})
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
	w := httptest.NewRecorder()

	ListPluginsHandler(db, activity)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var plugins []interface{}
	json.NewDecoder(w.Body).Decode(&plugins)
	if len(plugins) != 1 {
		t.Errorf("expected 1 installed plugin, got %d", len(plugins))
	}
}

func TestListPluginsHandler_ContentType(t *testing.T) {
	db := setupPluginsRouteTestDB(t)
	activity := services.NewActivityService(db)

	req := httptest.NewRequest(http.MethodGet, "/plugins", nil)
	w := httptest.NewRecorder()

	ListPluginsHandler(db, activity)(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
}
