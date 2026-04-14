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
)

func setupChatTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&chat_test=1"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	return db
}

func TestCeoChatIngestionHandler_BadBody(t *testing.T) {
	db := setupChatTestDB(t)

	router := chi.NewRouter()
	router.Post("/companies/{companyId}/ceo/chat", CeoChatIngestionHandler(db))

	req := httptest.NewRequest(http.MethodPost, "/companies/comp-a/ceo/chat", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad body, got %d", w.Code)
	}
}

func TestCeoChatIngestionHandler_ReturnsIngested(t *testing.T) {
	db := setupChatTestDB(t)

	router := chi.NewRouter()
	router.Post("/companies/{companyId}/ceo/chat", CeoChatIngestionHandler(db))

	body := `{"message":"Set Q2 goals to increase revenue 20%"}`
	req := httptest.NewRequest(http.MethodPost, "/companies/comp-a/ceo/chat", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// The handler always returns 200 even if the external call fails (fire-and-forget)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result map[string]string
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if result["status"] != "ingested" {
		t.Errorf("expected status 'ingested', got %q", result["status"])
	}
}

func TestRegisterChatRoutes(t *testing.T) {
	db := setupChatTestDB(t)

	router := chi.NewRouter()
	router.Route("/companies/{companyId}", func(r chi.Router) {
		RegisterChatRoutes(r, db)
	})

	body := `{"message":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/companies/comp-a/ceo/chat", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 via registered routes, got %d", w.Code)
	}
}
