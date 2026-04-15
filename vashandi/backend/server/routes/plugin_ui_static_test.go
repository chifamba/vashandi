package routes

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupPluginUIStaticTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file::memory:?cache=shared&%s=1", url.QueryEscape(t.Name()))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
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

func pluginUIStaticRouter(db *gorm.DB) *chi.Mux {
	router := chi.NewRouter()
	router.Get("/_plugins/{pluginId}/ui/*", PluginUIStaticHandler(db))
	return router
}

func writePluginUIFixture(t *testing.T, fileName string, contents string) string {
	t.Helper()

	packageDir := t.TempDir()
	uiDir := filepath.Join(packageDir, "ui", "dist")
	if err := os.MkdirAll(uiDir, 0o755); err != nil {
		t.Fatalf("mkdir ui dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(uiDir, fileName), []byte(contents), 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}
	return packageDir
}

func insertPluginUIFixture(t *testing.T, db *gorm.DB, id string, pluginKey string, status string, packagePath string, manifest string) {
	t.Helper()

	if err := db.Exec(
		"INSERT INTO plugins (id, plugin_key, package_name, version, manifest_json, status, package_path) VALUES (?, ?, ?, '1.0.0', ?, ?, ?)",
		id,
		pluginKey,
		pluginKey+"-pkg",
		manifest,
		status,
		packagePath,
	).Error; err != nil {
		t.Fatalf("insert plugin: %v", err)
	}
}

func TestPluginUIStaticHandler_ServesReadyPluginByKey(t *testing.T) {
	db := setupPluginUIStaticTestDB(t)
	packageDir := writePluginUIFixture(t, "index.js", `console.log("ok")`)
	insertPluginUIFixture(t, db, "plugin-1", "acme.ready", "ready", packageDir, `{"entrypoints":{"ui":"./ui/dist"}}`)

	req := httptest.NewRequest(http.MethodGet, "/_plugins/acme.ready/ui/index.js", nil)
	w := httptest.NewRecorder()
	pluginUIStaticRouter(db).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "application/javascript; charset=utf-8" {
		t.Fatalf("expected JS content type, got %q", got)
	}
	if got := w.Header().Get("Cache-Control"); got != "public, max-age=0, must-revalidate" {
		t.Fatalf("expected revalidate cache control, got %q", got)
	}
	if got := w.Header().Get("ETag"); got == "" {
		t.Fatal("expected ETag header")
	}
	if body := strings.TrimSpace(w.Body.String()); body != `console.log("ok")` {
		t.Fatalf("unexpected body %q", body)
	}
}

func TestPluginUIStaticHandler_RejectsPluginWithoutReadyStatus(t *testing.T) {
	db := setupPluginUIStaticTestDB(t)
	packageDir := writePluginUIFixture(t, "index.js", `console.log("nope")`)
	insertPluginUIFixture(t, db, "plugin-2", "acme.pending", "installed", packageDir, `{"entrypoints":{"ui":"./ui/dist"}}`)

	req := httptest.NewRequest(http.MethodGet, "/_plugins/acme.pending/ui/index.js", nil)
	w := httptest.NewRecorder()
	pluginUIStaticRouter(db).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestPluginUIStaticHandler_RequiresUIEntrypoint(t *testing.T) {
	db := setupPluginUIStaticTestDB(t)
	packageDir := writePluginUIFixture(t, "index.js", `console.log("missing manifest")`)
	insertPluginUIFixture(t, db, "plugin-3", "acme.no-ui", "ready", packageDir, `{}`)

	req := httptest.NewRequest(http.MethodGet, "/_plugins/acme.no-ui/ui/index.js", nil)
	w := httptest.NewRecorder()
	pluginUIStaticRouter(db).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestPluginUIStaticHandler_BlocksSymlinkTraversal(t *testing.T) {
	db := setupPluginUIStaticTestDB(t)
	packageDir := t.TempDir()
	uiDir := filepath.Join(packageDir, "ui", "dist")
	if err := os.MkdirAll(uiDir, 0o755); err != nil {
		t.Fatalf("mkdir ui dir: %v", err)
	}
	targetDir := t.TempDir()
	targetFile := filepath.Join(targetDir, "secret.txt")
	if err := os.WriteFile(targetFile, []byte("secret"), 0o644); err != nil {
		t.Fatalf("write target file: %v", err)
	}
	if err := os.Symlink(targetFile, filepath.Join(uiDir, "escape.txt")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	insertPluginUIFixture(t, db, "plugin-4", "acme.symlink", "ready", packageDir, `{"entrypoints":{"ui":"./ui/dist"}}`)

	req := httptest.NewRequest(http.MethodGet, "/_plugins/acme.symlink/ui/escape.txt", nil)
	w := httptest.NewRecorder()
	pluginUIStaticRouter(db).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestPluginUIStaticHandler_ReturnsNotModifiedForMatchingETag(t *testing.T) {
	db := setupPluginUIStaticTestDB(t)
	packageDir := writePluginUIFixture(t, "index.js", `console.log("etag")`)
	insertPluginUIFixture(t, db, "plugin-5", "acme.etag", "ready", packageDir, `{"entrypoints":{"ui":"./ui/dist"}}`)
	router := pluginUIStaticRouter(db)

	firstReq := httptest.NewRequest(http.MethodGet, "/_plugins/acme.etag/ui/index.js", nil)
	firstRes := httptest.NewRecorder()
	router.ServeHTTP(firstRes, firstReq)
	if firstRes.Code != http.StatusOK {
		t.Fatalf("expected first request 200, got %d", firstRes.Code)
	}

	etag := firstRes.Header().Get("ETag")
	if etag == "" {
		t.Fatal("expected ETag header on first response")
	}

	secondReq := httptest.NewRequest(http.MethodGet, "/_plugins/acme.etag/ui/index.js", nil)
	secondReq.Header.Set("If-None-Match", etag)
	secondRes := httptest.NewRecorder()
	router.ServeHTTP(secondRes, secondReq)

	if secondRes.Code != http.StatusNotModified {
		t.Fatalf("expected 304, got %d", secondRes.Code)
	}
}

func TestPluginUIStaticHandler_UsesImmutableCachingForContentHashedAssets(t *testing.T) {
	db := setupPluginUIStaticTestDB(t)
	packageDir := writePluginUIFixture(t, "index-a1b2c3d4.js", `console.log("hashed")`)
	insertPluginUIFixture(t, db, "plugin-6", "acme.hashed", "ready", packageDir, `{"entrypoints":{"ui":"./ui/dist"}}`)

	req := httptest.NewRequest(http.MethodGet, "/_plugins/acme.hashed/ui/index-a1b2c3d4.js", nil)
	w := httptest.NewRecorder()
	pluginUIStaticRouter(db).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := w.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("expected immutable cache control, got %q", got)
	}
	if got := w.Header().Get("ETag"); got != "" {
		t.Fatalf("expected no ETag for immutable asset, got %q", got)
	}
	if body := strings.TrimSpace(w.Body.String()); body != `console.log("hashed")` {
		t.Fatalf("unexpected body %q", body)
	}
}
