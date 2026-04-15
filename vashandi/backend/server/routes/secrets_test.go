package routes

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
)

func setupSecretsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&secrets_test=1"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.Exec("DROP TABLE IF EXISTS company_secret_versions")
	db.Exec("DROP TABLE IF EXISTS company_secrets")
	db.Exec("DROP TABLE IF EXISTS companies")
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
		created_at datetime,
		updated_at datetime
	)`)
	db.Exec(`CREATE TABLE company_secrets (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		name text NOT NULL,
		provider text NOT NULL DEFAULT 'local_encrypted',
		external_ref text,
		latest_version integer NOT NULL DEFAULT 1,
		description text,
		created_by_agent_id text,
		created_by_user_id text,
		created_at datetime,
		updated_at datetime
	)`)
	db.Exec(`CREATE TABLE company_secret_versions (
		id text PRIMARY KEY,
		secret_id text NOT NULL,
		version integer NOT NULL,
		material text NOT NULL,
		value_sha256 text NOT NULL DEFAULT '',
		created_by_agent_id text,
		created_by_user_id text,
		created_at datetime,
		revoked_at datetime
	)`)
	return db
}

func TestListSecretProvidersHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/secret-providers", nil)
	w := httptest.NewRecorder()

	ListSecretProvidersHandler()(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var providers []string
	if err := json.NewDecoder(w.Body).Decode(&providers); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(providers) == 0 {
		t.Error("expected at least one secret provider")
	}

	found := false
	for _, p := range providers {
		if p == "local_encrypted" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'local_encrypted' in providers list")
	}
}

func TestCreateSecretHandler_CompanyScoping(t *testing.T) {
	db := setupSecretsTestDB(t)

	router := chi.NewRouter()
	router.Post("/companies/{companyId}/secrets", CreateSecretHandler(db))

	body, _ := json.Marshal(map[string]string{
		"name":     "MY_API_KEY",
		"provider": "local_encrypted",
		"value":    "supersecret",
	})
	req := httptest.NewRequest(http.MethodPost, "/companies/comp-abc/secrets", bytes.NewBuffer(body))
	req = withBoardActorRequest(req)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", w.Code, w.Body.String())
	}

	var secret models.CompanySecret
	if err := json.NewDecoder(w.Body).Decode(&secret); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if secret.CompanyID != "comp-abc" {
		t.Errorf("expected CompanyID 'comp-abc', got %q", secret.CompanyID)
	}
	if secret.Name != "MY_API_KEY" {
		t.Errorf("expected Name 'MY_API_KEY', got %q", secret.Name)
	}
}

func TestCreateSecretHandler_DefaultsToLocalEncrypted(t *testing.T) {
	db := setupSecretsTestDB(t)

	router := chi.NewRouter()
	router.Post("/companies/{companyId}/secrets", CreateSecretHandler(db))

	body, _ := json.Marshal(map[string]string{
		"name": "NO_PROVIDER_KEY",
	})
	req := httptest.NewRequest(http.MethodPost, "/companies/comp-xyz/secrets", bytes.NewBuffer(body))
	req = withBoardActorRequest(req)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", w.Code, w.Body.String())
	}

	var secret models.CompanySecret
	json.NewDecoder(w.Body).Decode(&secret)
	if secret.Provider != "local_encrypted" {
		t.Errorf("expected provider 'local_encrypted', got %q", secret.Provider)
	}
}

func TestListSecretsHandler_CompanyScoping(t *testing.T) {
	db := setupSecretsTestDB(t)

	// Seed two secrets for different companies.
	db.Exec("INSERT INTO company_secrets (id, company_id, name, provider, latest_version) VALUES ('s1', 'comp-1', 'KEY_A', 'local_encrypted', 1)")
	db.Exec("INSERT INTO company_secrets (id, company_id, name, provider, latest_version) VALUES ('s2', 'comp-2', 'KEY_B', 'local_encrypted', 1)")

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/secrets", ListSecretsHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-1/secrets", nil)
	req = withBoardActorRequest(req)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var secrets []models.CompanySecret
	json.NewDecoder(w.Body).Decode(&secrets)
	if len(secrets) != 1 {
		t.Errorf("expected 1 secret for comp-1, got %d", len(secrets))
	}
	if len(secrets) > 0 && secrets[0].CompanyID != "comp-1" {
		t.Errorf("expected secret belonging to comp-1, got %q", secrets[0].CompanyID)
	}
}

func TestRotateSecretHandler_IncrementsVersion(t *testing.T) {
	db := setupSecretsTestDB(t)

	db.Exec("INSERT INTO company_secrets (id, company_id, name, provider, latest_version) VALUES ('sec-1', 'comp-1', 'ROTATE_KEY', 'local_encrypted', 1)")
	db.Exec("INSERT INTO company_secret_versions (id, secret_id, version, material) VALUES ('v1', 'sec-1', 1, '{\"value\":\"old\"}')")

	router := chi.NewRouter()
	router.Put("/secrets/{id}/rotate", RotateSecretHandler(db))

	body, _ := json.Marshal(map[string]string{"value": "newvalue"})
	req := httptest.NewRequest(http.MethodPut, "/secrets/sec-1/rotate", bytes.NewBuffer(body))
	req = withBoardActorRequest(req)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	var result map[string]int
	json.NewDecoder(w.Body).Decode(&result)
	if result["version"] != 2 {
		t.Errorf("expected version 2 after rotation, got %d", result["version"])
	}
}

func TestRotateSecretHandler_NotFound(t *testing.T) {
	db := setupSecretsTestDB(t)

	router := chi.NewRouter()
	router.Put("/secrets/{id}/rotate", RotateSecretHandler(db))

	body, _ := json.Marshal(map[string]string{"value": "v"})
	req := httptest.NewRequest(http.MethodPut, "/secrets/nonexistent/rotate", bytes.NewBuffer(body))
	req = withBoardActorRequest(req)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestDeleteSecretHandler(t *testing.T) {
	db := setupSecretsTestDB(t)
	db.Exec("INSERT INTO company_secrets (id, company_id, name, provider, latest_version) VALUES ('del-sec', 'comp-1', 'DELETE_ME', 'local_encrypted', 1)")

	router := chi.NewRouter()
	router.Delete("/secrets/{id}", DeleteSecretHandler(db))

	req := httptest.NewRequest(http.MethodDelete, "/secrets/del-sec", nil)
	req = withBoardActorRequest(req)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}

	var count int64
	db.Model(&models.CompanySecret{}).Where("id = ?", "del-sec").Count(&count)
	if count != 0 {
		t.Errorf("expected secret to be deleted, found %d row(s)", count)
	}
}

func TestUpdateSecretHandler(t *testing.T) {
	db := setupSecretsTestDB(t)
	db.Exec("INSERT INTO company_secrets (id, company_id, name, provider, latest_version) VALUES ('upd-sec', 'comp-1', 'OLD_NAME', 'local_encrypted', 1)")

	router := chi.NewRouter()
	router.Put("/secrets/{id}", UpdateSecretHandler(db))

	body, _ := json.Marshal(map[string]string{"name": "NEW_NAME"})
	req := httptest.NewRequest(http.MethodPut, "/secrets/upd-sec", bytes.NewBuffer(body))
	req = withBoardActorRequest(req)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	var updated models.CompanySecret
	json.NewDecoder(w.Body).Decode(&updated)
	if updated.Name != "NEW_NAME" {
		t.Errorf("expected updated Name 'NEW_NAME', got %q", updated.Name)
	}
}

func TestCreateSecretHandler_StoresMaterialWithoutExposingValue(t *testing.T) {
	db := setupSecretsTestDB(t)

	router := chi.NewRouter()
	router.Post("/companies/{companyId}/secrets", CreateSecretHandler(db))

	body, _ := json.Marshal(map[string]string{
		"name":  "SCOPED_KEY",
		"value": "supersecret",
	})
	req := httptest.NewRequest(http.MethodPost, "/companies/comp-1/secrets", bytes.NewBuffer(body))
	req = withBoardActorRequest(req)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "supersecret") {
		t.Fatalf("expected response to omit plaintext secret value, got %s", w.Body.String())
	}

	var version models.CompanySecretVersion
	if err := db.First(&version).Error; err != nil {
		t.Fatalf("load version: %v", err)
	}
	if !bytes.Contains(version.Material, []byte("supersecret")) {
		t.Fatalf("expected stored material to include provided value, got %s", string(version.Material))
	}
}

func TestListSecretsHandler_RejectsAgentActor(t *testing.T) {
	db := setupSecretsTestDB(t)

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/secrets", ListSecretsHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-1/secrets", nil)
	req = withAgentActorRequest(req, "agent-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d; body: %s", w.Code, w.Body.String())
	}
}
