package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestHealthHandler_NilDB(t *testing.T) {
	handler := HealthHandler(nil)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("expected status 'ok', got %q", resp.Status)
	}
	if resp.Version == "" {
		t.Errorf("expected non-empty version")
	}
}

func TestHealthHandler_WithDB(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:health1?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	handler := HealthHandler(db)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("expected status 'ok', got %q", resp.Status)
	}
	if resp.DeploymentMode == "" {
		t.Errorf("expected DeploymentMode to be set with valid DB")
	}
}

func TestHealthHandler_AuthenticatedBootstrapPendingWithInvite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:health2?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Exec(`CREATE TABLE instance_user_roles (id text, user_id text, role text)`).Error; err != nil {
		t.Fatalf("create roles table: %v", err)
	}
	if err := db.Exec(`CREATE TABLE invites (
		id text,
		invite_type text,
		expires_at datetime,
		revoked_at datetime,
		accepted_at datetime
	)`).Error; err != nil {
		t.Fatalf("create invites table: %v", err)
	}
	if err := db.Exec(
		"INSERT INTO invites (id, invite_type, expires_at) VALUES (?, ?, ?)",
		"inv-1",
		"bootstrap_ceo",
		time.Now().Add(time.Hour),
	).Error; err != nil {
		t.Fatalf("insert invite: %v", err)
	}

	handler := HealthHandler(db, HealthHandlerOptions{DeploymentMode: "authenticated", DeploymentExposure: "private", AuthReady: true, CompanyDeletionEnabled: true})
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	var resp HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.BootstrapStatus != "bootstrap_pending" {
		t.Fatalf("expected bootstrap_pending, got %q", resp.BootstrapStatus)
	}
	if !resp.BootstrapInvite {
		t.Fatalf("expected bootstrap invite to be active")
	}
}

func TestHealthHandler_AuthenticatedReadyWhenAdminExists(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:health3?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Exec(`CREATE TABLE instance_user_roles (id text, user_id text, role text)`).Error; err != nil {
		t.Fatalf("create roles table: %v", err)
	}
	if err := db.Exec(`INSERT INTO instance_user_roles (id, user_id, role) VALUES (?, ?, ?)`, "role-1", "user-1", "instance_admin").Error; err != nil {
		t.Fatalf("insert admin role: %v", err)
	}

	handler := HealthHandler(db, HealthHandlerOptions{DeploymentMode: "authenticated", DeploymentExposure: "private", AuthReady: true, CompanyDeletionEnabled: true})
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	var resp HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.BootstrapStatus != "ready" {
		t.Fatalf("expected ready, got %q", resp.BootstrapStatus)
	}
	if resp.BootstrapInvite {
		t.Fatalf("expected bootstrap invite to be inactive")
	}
}

func TestHealthHandler_ContentType(t *testing.T) {
	handler := HealthHandler(nil)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
}
