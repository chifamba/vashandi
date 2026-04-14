package routes

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupInstanceSettingsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&instance_settings_test=1"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.Exec("DROP TABLE IF EXISTS instance_settings")
	db.Exec(`CREATE TABLE instance_settings (
		id text PRIMARY KEY,
		singleton_key text NOT NULL DEFAULT 'default',
		general text NOT NULL DEFAULT '{}',
		experimental text NOT NULL DEFAULT '{}',
		created_at datetime,
		updated_at datetime
	)`)
	return db
}

func TestGetGeneralSettingsHandler_DefaultRow(t *testing.T) {
	db := setupInstanceSettingsTestDB(t)

	req := httptest.NewRequest(http.MethodGet, "/settings/general", nil)
	w := httptest.NewRecorder()

	GetGeneralSettingsHandler(db)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result map[string]interface{}
	json.NewDecoder(w.Body).Decode(&result)
	if _, ok := result["general"]; !ok {
		t.Error("expected 'general' key in response")
	}
}

func TestUpdateGeneralSettingsHandler(t *testing.T) {
	db := setupInstanceSettingsTestDB(t)

	body, _ := json.Marshal(map[string]interface{}{
		"instanceName": "My Vashandi",
		"maxAgents":    50,
	})
	req := httptest.NewRequest(http.MethodPut, "/settings/general", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	UpdateGeneralSettingsHandler(db)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	var result map[string]interface{}
	json.NewDecoder(w.Body).Decode(&result)
	if _, ok := result["general"]; !ok {
		t.Error("expected 'general' key in response")
	}
}

func TestUpdateGeneralSettingsHandler_BadBody(t *testing.T) {
	db := setupInstanceSettingsTestDB(t)

	req := httptest.NewRequest(http.MethodPut, "/settings/general", bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	UpdateGeneralSettingsHandler(db)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestGetExperimentalSettingsHandler(t *testing.T) {
	db := setupInstanceSettingsTestDB(t)

	req := httptest.NewRequest(http.MethodGet, "/settings/experimental", nil)
	w := httptest.NewRecorder()

	GetExperimentalSettingsHandler(db)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result map[string]interface{}
	json.NewDecoder(w.Body).Decode(&result)
	if _, ok := result["experimental"]; !ok {
		t.Error("expected 'experimental' key in response")
	}
}

func TestUpdateExperimentalSettingsHandler(t *testing.T) {
	db := setupInstanceSettingsTestDB(t)

	body, _ := json.Marshal(map[string]interface{}{
		"featureFlag": true,
	})
	req := httptest.NewRequest(http.MethodPut, "/settings/experimental", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	UpdateExperimentalSettingsHandler(db)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestUpdateExperimentalSettingsHandler_BadBody(t *testing.T) {
	db := setupInstanceSettingsTestDB(t)

	req := httptest.NewRequest(http.MethodPut, "/settings/experimental", bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	UpdateExperimentalSettingsHandler(db)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
