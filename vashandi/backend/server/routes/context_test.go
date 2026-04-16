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

func TestRegisterContextRoutes_RunStart(t *testing.T) {
	router := chi.NewRouter()
	RegisterContextRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/triggers/run_start", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "run_start_forwarded" {
		t.Errorf("expected status run_start_forwarded, got %s", resp["status"])
	}
}

func TestRegisterContextRoutes_RunComplete(t *testing.T) {
	router := chi.NewRouter()
	RegisterContextRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/triggers/run_complete", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "run_complete_forwarded" {
		t.Errorf("expected status run_complete_forwarded, got %s", resp["status"])
	}
}

func TestRegisterContextRoutes_Checkout(t *testing.T) {
	router := chi.NewRouter()
	RegisterContextRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/triggers/checkout", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "checkout_forwarded" {
		t.Errorf("expected status checkout_forwarded, got %s", resp["status"])
	}
}

func TestPreRunHydrationHandler(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&context_pre=1"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	handler := PreRunHydrationHandler(db)
	req := httptest.NewRequest(http.MethodPost, "/pre-run-hydration", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "run_start_forwarded" {
		t.Errorf("expected status run_start_forwarded, got %s", resp["status"])
	}
}

func TestPostRunCaptureHandler(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&context_post=1"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	handler := PostRunCaptureHandler(db)
	req := httptest.NewRequest(http.MethodPost, "/post-run-capture", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "run_complete_forwarded" {
		t.Errorf("expected status run_complete_forwarded, got %s", resp["status"])
	}
}

func TestListContextOperationsHandler(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&context_list=1"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/context", ListContextOperationsHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-a/context", nil)
	req = req.WithContext(WithActor(req.Context(), ActorInfo{UserID: "user-1", ActorType: "user"}))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var operations []ContextOperationDescriptor
	if err := json.NewDecoder(w.Body).Decode(&operations); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(operations) != 2 {
		t.Fatalf("expected 2 operations, got %d", len(operations))
	}
}

func TestGetContextOperationHandler(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&context_get=1"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/context/{operation}", GetContextOperationHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-a/context/hydrate", nil)
	req = req.WithContext(WithActor(req.Context(), ActorInfo{UserID: "user-1", ActorType: "user"}))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var operation ContextOperationDescriptor
	if err := json.NewDecoder(w.Body).Decode(&operation); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if operation.Name != "hydrate" {
		t.Fatalf("expected hydrate, got %s", operation.Name)
	}
}
