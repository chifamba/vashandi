package routes

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"net/url"
	"testing"

	"github.com/go-chi/chi/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAssetsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dbName := "file:assets_" + url.QueryEscape(t.Name()) + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	for _, tbl := range []string{"company_logos", "issue_attachments", "assets", "issues", "companies"} {
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
	db.Exec(`CREATE TABLE issues (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		title text NOT NULL,
		status text NOT NULL DEFAULT 'backlog',
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
	if got := w.Header().Get("Content-Type"); got != "text/plain" {
		t.Fatalf("expected content type text/plain, got %q", got)
	}
	if got := w.Header().Get("Content-Disposition"); got != `attachment; filename="attachment"` && got != "attachment; filename=attachment" {
		t.Fatalf("expected attachment disposition, got %q", got)
	}
	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("expected nosniff header, got %q", got)
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

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var body map[string]bool
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if !body["ok"] {
		t.Fatalf("expected ok=true body, got %#v", body)
	}
}

func TestDeleteAttachmentHandler_NotFound(t *testing.T) {
	db := setupAssetsTestDB(t)

	router := chi.NewRouter()
	router.Delete("/attachments/{attachmentId}", DeleteAttachmentHandler(db))

	req := httptest.NewRequest(http.MethodDelete, "/attachments/missing", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestUploadIssueAttachmentHandler_AcceptsZipUpload(t *testing.T) {
	db := setupAssetsTestDB(t)
	db.Exec("INSERT INTO issues (id, company_id, title, status) VALUES ('issue-1', 'comp-a', 'Attachment target', 'backlog')")

	router := chi.NewRouter()
	router.Post("/companies/{companyId}/issues/{issueId}/attachments", UploadIssueAttachmentHandler(db))

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="file"; filename="bundle.zip"`)
	header.Set("Content-Type", "application/zip")
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write([]byte("zip")); err != nil {
		t.Fatalf("write upload body: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/companies/comp-a/issues/issue-1/attachments", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req = req.WithContext(WithActor(req.Context(), ActorInfo{UserID: "user-1", ActorType: "board"}))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", w.Code, w.Body.String())
	}

	var attachment map[string]any
	if err := json.NewDecoder(w.Body).Decode(&attachment); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	if attachment["companyId"] != "comp-a" {
		t.Fatalf("expected companyId comp-a, got %#v", attachment["companyId"])
	}
	if attachment["issueId"] != "issue-1" {
		t.Fatalf("expected issueId issue-1, got %#v", attachment["issueId"])
	}
	if attachment["contentType"] != "application/zip" {
		t.Fatalf("expected contentType application/zip, got %#v", attachment["contentType"])
	}
	if attachment["originalFilename"] != "bundle.zip" {
		t.Fatalf("expected originalFilename bundle.zip, got %#v", attachment["originalFilename"])
	}
	if attachment["contentPath"] == "" {
		t.Fatal("expected contentPath in upload response")
	}

	var assetCount int64
	if err := db.Table("assets").Where("company_id = ? AND content_type = ? AND original_filename = ?", "comp-a", "application/zip", "bundle.zip").Count(&assetCount).Error; err != nil {
		t.Fatalf("count assets: %v", err)
	}
	if assetCount != 1 {
		t.Fatalf("expected 1 stored asset, got %d", assetCount)
	}
}

func TestUploadIssueAttachmentHandler_RejectsCrossCompanyIssue(t *testing.T) {
	db := setupAssetsTestDB(t)
	db.Exec("INSERT INTO issues (id, company_id, title, status) VALUES ('issue-1', 'comp-b', 'Attachment target', 'backlog')")

	router := chi.NewRouter()
	router.Post("/companies/{companyId}/issues/{issueId}/attachments", UploadIssueAttachmentHandler(db))

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="file"; filename="bundle.zip"`)
	header.Set("Content-Type", "application/zip")
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write([]byte("zip")); err != nil {
		t.Fatalf("write upload body: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/companies/comp-a/issues/issue-1/attachments", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", w.Code)
	}
}

func TestGetAttachmentContentHandler_HTMLDownloadSetsNosniff(t *testing.T) {
	db := setupAssetsTestDB(t)
	db.Exec("INSERT INTO assets (id, company_id, provider, object_key, content_type, byte_size, sha256, original_filename) VALUES ('a-html', 'comp-a', 'local', 'comp-a/abc/report.html', 'text/html', 4, 'abc123', 'report.html')")
	db.Exec("INSERT INTO issue_attachments (id, company_id, issue_id, asset_id) VALUES ('att-html', 'comp-a', 'i1', 'a-html')")

	router := chi.NewRouter()
	router.Get("/attachments/{attachmentId}/content", GetAttachmentContentHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/attachments/att-html/content", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "text/html" {
		t.Fatalf("expected text/html, got %q", got)
	}
	if got := w.Header().Get("Content-Disposition"); got != `attachment; filename="report.html"` && got != "attachment; filename=report.html" {
		t.Fatalf("expected attachment content disposition, got %q", got)
	}
	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("expected nosniff, got %q", got)
	}
}

func TestGetAttachmentContentHandler_ImageStaysInline(t *testing.T) {
	db := setupAssetsTestDB(t)
	db.Exec("INSERT INTO assets (id, company_id, provider, object_key, content_type, byte_size, sha256, original_filename) VALUES ('a-img', 'comp-a', 'local', 'comp-a/abc/preview.png', 'image/png', 4, 'abc123', 'preview.png')")
	db.Exec("INSERT INTO issue_attachments (id, company_id, issue_id, asset_id) VALUES ('att-img', 'comp-a', 'i1', 'a-img')")

	router := chi.NewRouter()
	router.Get("/attachments/{attachmentId}/content", GetAttachmentContentHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/attachments/att-img/content", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := w.Header().Get("Content-Disposition"); got != `inline; filename="preview.png"` && got != "inline; filename=preview.png" {
		t.Fatalf("expected inline content disposition, got %q", got)
	}
}
