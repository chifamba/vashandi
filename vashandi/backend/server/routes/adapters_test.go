package routes

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func strReader(s string) io.Reader {
	return strings.NewReader(s)
}

func setupAdaptersTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&adapters_test=1"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.Exec("DROP TABLE IF EXISTS plugins")
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
	return db
}

func TestListAdaptersHandler_BuiltinAdapters(t *testing.T) {
	db := setupAdaptersTestDB(t)

	req := httptest.NewRequest(http.MethodGet, "/adapters", nil)
	w := httptest.NewRecorder()

	ListAdaptersHandler(db)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}

	builtin, ok := resp["builtin"].([]interface{})
	if !ok {
		t.Fatal("expected 'builtin' array in response")
	}
	if len(builtin) < 4 {
		t.Errorf("expected at least 4 builtin adapters, got %d", len(builtin))
	}
}

func TestListAdaptersHandler_IncludesPluginAdapters(t *testing.T) {
	db := setupAdaptersTestDB(t)
	db.Exec("INSERT INTO plugins (id, plugin_key, package_name, version, manifest_json, status) VALUES ('p1', 'custom-adapter', 'Custom Adapter', '1.0.0', '{}', 'installed')")

	req := httptest.NewRequest(http.MethodGet, "/adapters", nil)
	w := httptest.NewRecorder()

	ListAdaptersHandler(db)(w, req)

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)

	plugins, ok := resp["plugins"].([]interface{})
	if !ok {
		t.Fatal("expected 'plugins' array in response")
	}
	if len(plugins) != 1 {
		t.Errorf("expected 1 plugin adapter, got %d", len(plugins))
	}
}

func TestListAdaptersHandler_ContentType(t *testing.T) {
	db := setupAdaptersTestDB(t)

	req := httptest.NewRequest(http.MethodGet, "/adapters", nil)
	w := httptest.NewRecorder()

	ListAdaptersHandler(db)(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
}

func TestGetAdapterConfigurationHandler_Known(t *testing.T) {
	router := chi.NewRouter()
	router.Get("/llms/{adapterType}", GetAdapterConfigurationHandler())

	req := httptest.NewRequest(http.MethodGet, "/llms/claude", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if body == "" {
		t.Error("expected non-empty body for known adapter type")
	}
}

func TestGetAdapterConfigurationHandler_Unknown(t *testing.T) {
	router := chi.NewRouter()
	router.Get("/llms/{adapterType}", GetAdapterConfigurationHandler())

	req := httptest.NewRequest(http.MethodGet, "/llms/nonexistent", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestInstallAdapterHandler(t *testing.T) {
	db := setupAdaptersTestDB(t)

	body := `{"packageUrl":"https://example.com/plugin.tgz"}`
	req := httptest.NewRequest(http.MethodPost, "/adapters/install", strReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	InstallAdapterHandler(db)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Errorf("expected status 'ok', got %v", resp["status"])
	}
}

func TestOverrideAdapterHandler(t *testing.T) {
	db := setupAdaptersTestDB(t)

	router := chi.NewRouter()
	router.Patch("/adapters/{type}/override", OverrideAdapterHandler(db))

	body := `{"model":"claude-4-opus"}`
	req := httptest.NewRequest(http.MethodPatch, "/adapters/claude/override", strReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["adapterType"] != "claude" {
		t.Errorf("expected adapterType 'claude', got %v", resp["adapterType"])
	}
}

func TestReloadAdapterHandler(t *testing.T) {
	db := setupAdaptersTestDB(t)

	router := chi.NewRouter()
	router.Post("/adapters/{type}/reload", ReloadAdapterHandler(db))

	req := httptest.NewRequest(http.MethodPost, "/adapters/claude/reload", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "reloaded" {
		t.Errorf("expected status 'reloaded', got %q", resp["status"])
	}
}

func TestGetAdapterConfigSchemaHandler(t *testing.T) {
	db := setupAdaptersTestDB(t)

	router := chi.NewRouter()
	router.Get("/adapters/{type}/config-schema", GetAdapterConfigSchemaHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/adapters/claude/config-schema", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["adapterType"] != "claude" {
		t.Errorf("expected adapterType 'claude', got %v", resp["adapterType"])
	}
	if resp["schema"] == nil {
		t.Error("expected schema to be present")
	}
}

func TestDeleteAdapterHandler(t *testing.T) {
	db := setupAdaptersTestDB(t)
	db.Exec("INSERT INTO plugins (id, plugin_key, package_name, version, manifest_json, status) VALUES ('p1', 'custom', 'Custom', '1.0.0', '{}', 'installed')")

	router := chi.NewRouter()
	router.Delete("/adapters/{type}", DeleteAdapterHandler(db))

	req := httptest.NewRequest(http.MethodDelete, "/adapters/custom", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}

	var status string
	db.Raw("SELECT status FROM plugins WHERE plugin_key = 'custom'").Scan(&status)
	if status != "uninstalled" {
		t.Errorf("expected status 'uninstalled', got %q", status)
	}
}
