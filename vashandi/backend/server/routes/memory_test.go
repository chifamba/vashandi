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

func setupMemoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&memory_test=1"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	return db
}

func TestMemoryBindingsHandler_ReturnsEmptyArray(t *testing.T) {
	db := setupMemoryTestDB(t)

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/memory/bindings", MemoryBindingsHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-a/memory/bindings", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	var result []interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty array, got %d items", len(result))
	}
}

func TestMemoryOperationsHandler_ReturnsEmptyArray(t *testing.T) {
	db := setupMemoryTestDB(t)

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/memory/operations", MemoryOperationsHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-a/memory/operations", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	var result []interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty array, got %d items", len(result))
	}
}
