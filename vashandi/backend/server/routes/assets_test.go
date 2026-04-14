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

func setupAssetsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&assets_test=1"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	for _, tbl := range []string{"company_logos", "issue_attachments", "assets", "companies"} {
		db.Exec("DROP TABLE IF EXISTS " + tbl)
	}
	db.Exec(`CREATE TABLE companies (
		id text PRIMARY KEY,
		name text NOT NULL,
		status text NOT NULL DEFAULT 'active',
		issue_prefix text NOT NULL DEFAULT 'PAP',
		issue_counter integer NOT NULL DEFAULT 0,
		budget_monthly_cents integer NOT NULL DEFAULT 0,
		spent_monthly_cents integer NOT NULL DEFAULT 0,
		require_board_approval_for_new_agents boolean NOT NULL DEFAULT 1,
		feedback_data_sharing_enabled boolean NOT NULL DEFAULT 0,
		is_archived boolean NOT NULL DEFAULT 0,
		created_at datetime,
		updated_at datetime
	)`)
	db.Exec(`CREATE TABLE assets (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		provider text NOT NULL,
		object_key text NOT NULL,
		content_type text NOT NULL,
		byte_size integer NOT NULL,
		sha256 text NOT NULL,
		original_filename text,
		created_by_agent_id text,
		created_by_user_id text,
		created_at datetime,
		updated_at datetime
	)`)
	db.Exec(`CREATE TABLE issue_attachments (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		issue_id text NOT NULL,
		asset_id text NOT NULL,
		issue_comment_id text,
		created_at datetime,
		updated_at datetime
	)`)
	db.Exec(`CREATE TABLE company_logos (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		asset_id text NOT NULL,
		created_at datetime,
		updated_at datetime
	)`)

	db.Exec("INSERT INTO companies (id, name) VALUES ('comp-a', 'Alpha')")
	return db
}

// ---------- GetAssetHandler ----------

func TestGetAssetHandler_Found(t *testing.T) {
	db := setupAssetsTestDB(t)
	db.Exec("INSERT INTO assets (id, company_id, provider, object_key, content_type, byte_size, sha256) VALUES ('a1', 'comp-a', 'local', 'comp-a/abc/file.txt', 'text/plain', 100, 'abc123')")

	router := chi.NewRouter()
	router.Get("/assets/{id}", GetAssetHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/assets/a1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	var asset map[string]interface{}
	json.NewDecoder(w.Body).Decode(&asset)
	if asset["ID"] != "a1" {
		t.Errorf("expected asset ID a1, got %v", asset["ID"])
	}
}

func TestGetAssetHandler_NotFound(t *testing.T) {
	db := setupAssetsTestDB(t)

	router := chi.NewRouter()
	router.Get("/assets/{id}", GetAssetHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/assets/nonexistent", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ---------- GetAssetContentHandler ----------

func TestGetAssetContentHandler_Found(t *testing.T) {
	db := setupAssetsTestDB(t)
	db.Exec("INSERT INTO assets (id, company_id, provider, object_key, content_type, byte_size, sha256) VALUES ('a1', 'comp-a', 'local', 'comp-a/abc/file.txt', 'text/plain', 100, 'abc123')")

	router := chi.NewRouter()
	router.Get("/assets/{assetId}/content", GetAssetContentHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/assets/a1/content", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
}

func TestGetAssetContentHandler_NotFound(t *testing.T) {
	db := setupAssetsTestDB(t)

	router := chi.NewRouter()
	router.Get("/assets/{assetId}/content", GetAssetContentHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/assets/nonexistent/content", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ---------- GetAttachmentContentHandler ----------

func TestGetAttachmentContentHandler_Found(t *testing.T) {
	db := setupAssetsTestDB(t)
	db.Exec("INSERT INTO assets (id, company_id, provider, object_key, content_type, byte_size, sha256) VALUES ('a1', 'comp-a', 'local', 'comp-a/abc/file.txt', 'text/plain', 100, 'abc123')")
	db.Exec("INSERT INTO issue_attachments (id, company_id, issue_id, asset_id) VALUES ('att1', 'comp-a', 'i1', 'a1')")

	router := chi.NewRouter()
	router.Get("/attachments/{attachmentId}/content", GetAttachmentContentHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/attachments/att1/content", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestGetAttachmentContentHandler_NotFound(t *testing.T) {
	db := setupAssetsTestDB(t)

	router := chi.NewRouter()
	router.Get("/attachments/{attachmentId}/content", GetAttachmentContentHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/attachments/nonexistent/content", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ---------- DeleteAttachmentHandler ----------

func TestDeleteAttachmentHandler_Success(t *testing.T) {
	db := setupAssetsTestDB(t)
	db.Exec("INSERT INTO assets (id, company_id, provider, object_key, content_type, byte_size, sha256) VALUES ('a1', 'comp-a', 'local', 'comp-a/abc/file.txt', 'text/plain', 100, 'abc123')")
	db.Exec("INSERT INTO issue_attachments (id, company_id, issue_id, asset_id) VALUES ('att1', 'comp-a', 'i1', 'a1')")

	router := chi.NewRouter()
	router.Delete("/attachments/{attachmentId}", DeleteAttachmentHandler(db))

	req := httptest.NewRequest(http.MethodDelete, "/attachments/att1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}
