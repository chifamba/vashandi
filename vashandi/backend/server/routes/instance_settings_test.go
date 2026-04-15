package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/go-chi/chi/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupInstanceSettingsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dbURI := "file:instance_settings_" + url.QueryEscape(t.Name()) + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dbURI), &gorm.Config{})
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

func withActor(req *http.Request, actor ActorInfo) *http.Request {
	return req.WithContext(WithActor(req.Context(), actor))
}

func decodeJSONBody(t *testing.T, body *bytes.Buffer) map[string]any {
	t.Helper()
	var decoded map[string]any
	if err := json.NewDecoder(body).Decode(&decoded); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return decoded
}

func TestGetExperimentalSettingsHandler_AllowsLocalBoardUsers(t *testing.T) {
	db := setupInstanceSettingsTestDB(t)

	req := httptest.NewRequest(http.MethodGet, "/instance/settings/experimental", nil)
	req = withActor(req, ActorInfo{UserID: "local-board", ActorType: "board", IsInstanceAdmin: true})
	w := httptest.NewRecorder()

	GetExperimentalSettingsHandler(db)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := decodeJSONBody(t, w.Body); got["enableIsolatedWorkspaces"] != false || got["autoRestartDevServerWhenIdle"] != false {
		t.Fatalf("unexpected body: %#v", got)
	}
}

func TestUpdateExperimentalSettingsHandler_AllowsLocalBoardUsers(t *testing.T) {
	db := setupInstanceSettingsTestDB(t)

	body, _ := json.Marshal(map[string]any{"enableIsolatedWorkspaces": true})
	req := httptest.NewRequest(http.MethodPatch, "/instance/settings/experimental", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = withActor(req, ActorInfo{UserID: "local-board", ActorType: "board", IsInstanceAdmin: true})
	w := httptest.NewRecorder()

	UpdateExperimentalSettingsHandler(db)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", w.Code, w.Body.String())
	}
	if got := decodeJSONBody(t, w.Body); got["enableIsolatedWorkspaces"] != true {
		t.Fatalf("unexpected body: %#v", got)
	}
}

func TestUpdateExperimentalSettingsHandler_AllowsGuardedAutoRestartSetting(t *testing.T) {
	db := setupInstanceSettingsTestDB(t)

	body, _ := json.Marshal(map[string]any{"autoRestartDevServerWhenIdle": true})
	req := httptest.NewRequest(http.MethodPatch, "/instance/settings/experimental", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = withActor(req, ActorInfo{UserID: "local-board", ActorType: "board", IsInstanceAdmin: true})
	w := httptest.NewRecorder()

	UpdateExperimentalSettingsHandler(db)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", w.Code, w.Body.String())
	}
	if got := decodeJSONBody(t, w.Body); got["autoRestartDevServerWhenIdle"] != true {
		t.Fatalf("unexpected body: %#v", got)
	}
}

func TestGetGeneralSettingsHandler_AllowsBoardUsers(t *testing.T) {
	db := setupInstanceSettingsTestDB(t)

	req := httptest.NewRequest(http.MethodGet, "/instance/settings/general", nil)
	req = withActor(req, ActorInfo{UserID: "user-1", ActorType: "board"})
	w := httptest.NewRecorder()

	GetGeneralSettingsHandler(db)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	got := decodeJSONBody(t, w.Body)
	if got["censorUsernameInLogs"] != false || got["keyboardShortcuts"] != false || got["feedbackDataSharingPreference"] != "prompt" {
		t.Fatalf("unexpected body: %#v", got)
	}
}

func TestUpdateGeneralSettingsHandler_AllowsLocalBoardUsers(t *testing.T) {
	db := setupInstanceSettingsTestDB(t)

	body, _ := json.Marshal(map[string]any{
		"censorUsernameInLogs":          true,
		"keyboardShortcuts":             true,
		"feedbackDataSharingPreference": "allowed",
	})
	req := httptest.NewRequest(http.MethodPatch, "/instance/settings/general", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = withActor(req, ActorInfo{UserID: "local-board", ActorType: "board", IsInstanceAdmin: true})
	w := httptest.NewRecorder()

	UpdateGeneralSettingsHandler(db)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", w.Code, w.Body.String())
	}
	got := decodeJSONBody(t, w.Body)
	if got["censorUsernameInLogs"] != true || got["keyboardShortcuts"] != true || got["feedbackDataSharingPreference"] != "allowed" {
		t.Fatalf("unexpected body: %#v", got)
	}
}

func TestUpdateGeneralSettingsHandler_RejectsNonAdminBoardUsers(t *testing.T) {
	db := setupInstanceSettingsTestDB(t)

	body, _ := json.Marshal(map[string]any{"censorUsernameInLogs": true})
	req := httptest.NewRequest(http.MethodPatch, "/instance/settings/general", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = withActor(req, ActorInfo{UserID: "user-1", ActorType: "board"})
	w := httptest.NewRecorder()

	UpdateGeneralSettingsHandler(db)(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestUpdateGeneralSettingsHandler_RejectsAgentCallers(t *testing.T) {
	db := setupInstanceSettingsTestDB(t)

	body, _ := json.Marshal(map[string]any{"feedbackDataSharingPreference": "not_allowed"})
	req := httptest.NewRequest(http.MethodPatch, "/instance/settings/general", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = withActor(req, ActorInfo{AgentID: "agent-1", IsAgent: true, ActorType: "agent"})
	w := httptest.NewRecorder()

	UpdateGeneralSettingsHandler(db)(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestInstanceSettingsRoutes_ExposeUiPaths(t *testing.T) {
	db := setupInstanceSettingsTestDB(t)
	router := chi.NewRouter()
	router.Get("/api/instance/settings/general", GetGeneralSettingsHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/api/instance/settings/general", nil)
	req = req.WithContext(WithActor(context.Background(), ActorInfo{UserID: "user-1", ActorType: "board"}))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
