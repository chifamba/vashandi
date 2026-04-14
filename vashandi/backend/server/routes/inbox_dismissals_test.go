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

func setupInboxDismissalsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&inbox_dismissals_test=1"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.Exec("DROP TABLE IF EXISTS inbox_dismissals")
	db.Exec(`CREATE TABLE inbox_dismissals (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		user_id text NOT NULL,
		item_key text NOT NULL,
		dismissed_at datetime,
		created_at datetime,
		updated_at datetime,
		UNIQUE(company_id, user_id, item_key)
	)`)
	return db
}

func TestListInboxDismissalsHandler_CompanyScoping(t *testing.T) {
	db := setupInboxDismissalsTestDB(t)
	db.Exec("INSERT INTO inbox_dismissals (id, company_id, user_id, item_key) VALUES ('d1', 'comp-a', 'user-1', 'run:r1')")
	db.Exec("INSERT INTO inbox_dismissals (id, company_id, user_id, item_key) VALUES ('d2', 'comp-b', 'user-1', 'run:r2')")

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/inbox-dismissals", ListInboxDismissalsHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-a/inbox-dismissals", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var dismissals []models.InboxDismissal
	json.NewDecoder(w.Body).Decode(&dismissals)
	if len(dismissals) != 1 {
		t.Errorf("expected 1 dismissal for comp-a, got %d", len(dismissals))
	}
}

func TestListInboxDismissalsHandler_UserFilter(t *testing.T) {
	db := setupInboxDismissalsTestDB(t)
	db.Exec("INSERT INTO inbox_dismissals (id, company_id, user_id, item_key) VALUES ('d1', 'comp-a', 'user-1', 'run:r1')")
	db.Exec("INSERT INTO inbox_dismissals (id, company_id, user_id, item_key) VALUES ('d2', 'comp-a', 'user-2', 'run:r2')")

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/inbox-dismissals", ListInboxDismissalsHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-a/inbox-dismissals?userId=user-1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var dismissals []models.InboxDismissal
	json.NewDecoder(w.Body).Decode(&dismissals)
	if len(dismissals) != 1 {
		t.Errorf("expected 1 dismissal for user-1, got %d", len(dismissals))
	}
}

func TestListInboxDismissalsHandler_EmptyResult(t *testing.T) {
	db := setupInboxDismissalsTestDB(t)

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/inbox-dismissals", ListInboxDismissalsHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-empty/inbox-dismissals", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var dismissals []models.InboxDismissal
	json.NewDecoder(w.Body).Decode(&dismissals)
	if len(dismissals) != 0 {
		t.Errorf("expected 0 dismissals, got %d", len(dismissals))
	}
}

func TestCreateInboxDismissalHandler(t *testing.T) {
	db := setupInboxDismissalsTestDB(t)

	router := chi.NewRouter()
	router.Post("/companies/{companyId}/inbox-dismissals", CreateInboxDismissalHandler(db))

	body, _ := json.Marshal(map[string]string{
		"userId":  "user-1",
		"itemKey": "run:r1",
	})
	req := httptest.NewRequest(http.MethodPost, "/companies/comp-xyz/inbox-dismissals", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", w.Code, w.Body.String())
	}

	var d models.InboxDismissal
	json.NewDecoder(w.Body).Decode(&d)
	if d.CompanyID != "comp-xyz" {
		t.Errorf("expected CompanyID 'comp-xyz', got %q", d.CompanyID)
	}
	if d.UserID != "user-1" {
		t.Errorf("expected UserID 'user-1', got %q", d.UserID)
	}
	if d.ItemKey != "run:r1" {
		t.Errorf("expected ItemKey 'run:r1', got %q", d.ItemKey)
	}
}

func TestCreateInboxDismissalHandler_BadBody(t *testing.T) {
	db := setupInboxDismissalsTestDB(t)

	router := chi.NewRouter()
	router.Post("/companies/{companyId}/inbox-dismissals", CreateInboxDismissalHandler(db))

	req := httptest.NewRequest(http.MethodPost, "/companies/comp-1/inbox-dismissals", bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestCreateInboxDismissalHandler_Idempotent(t *testing.T) {
	db := setupInboxDismissalsTestDB(t)

	router := chi.NewRouter()
	router.Post("/companies/{companyId}/inbox-dismissals", CreateInboxDismissalHandler(db))

	body, _ := json.Marshal(map[string]string{
		"userId":  "user-1",
		"itemKey": "run:r1",
	})

	// First creation
	req := httptest.NewRequest(http.MethodPost, "/companies/comp-xyz/inbox-dismissals", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d", w.Code)
	}

	// Second creation (same key) — should succeed idempotently
	body2, _ := json.Marshal(map[string]string{
		"userId":  "user-1",
		"itemKey": "run:r1",
	})
	req2 := httptest.NewRequest(http.MethodPost, "/companies/comp-xyz/inbox-dismissals", bytes.NewBuffer(body2))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusCreated {
		t.Fatalf("idempotent create: expected 201, got %d; body: %s", w2.Code, w2.Body.String())
	}

	// Verify only one row exists
	var count int64
	db.Model(&models.InboxDismissal{}).Where("company_id = ? AND user_id = ? AND item_key = ?", "comp-xyz", "user-1", "run:r1").Count(&count)
	if count != 1 {
		t.Errorf("expected exactly 1 dismissal after idempotent create, got %d", count)
	}
}
