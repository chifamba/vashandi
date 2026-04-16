package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/chifamba/vashandi/vashandi/backend/server/services"
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

func setupInstanceSettingsSvc(t *testing.T) *services.InstanceSettingsService {
	t.Helper()
	return services.NewInstanceSettingsService(setupInstanceSettingsTestDB(t))
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
	svc := setupInstanceSettingsSvc(t)

	req := httptest.NewRequest(http.MethodGet, "/instance/settings/experimental", nil)
	req = withActor(req, ActorInfo{UserID: "local-board", ActorType: "board", IsInstanceAdmin: true})
	w := httptest.NewRecorder()

	GetExperimentalSettingsHandler(svc)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := decodeJSONBody(t, w.Body); got["enableIsolatedWorkspaces"] != false || got["autoRestartDevServerWhenIdle"] != false {
		t.Fatalf("unexpected body: %#v", got)
	}
}

func TestUpdateExperimentalSettingsHandler_AllowsLocalBoardUsers(t *testing.T) {
	svc := setupInstanceSettingsSvc(t)

	body, _ := json.Marshal(map[string]any{"enableIsolatedWorkspaces": true})
	req := httptest.NewRequest(http.MethodPatch, "/instance/settings/experimental", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = withActor(req, ActorInfo{UserID: "local-board", ActorType: "board", IsInstanceAdmin: true})
	w := httptest.NewRecorder()

	UpdateExperimentalSettingsHandler(svc)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", w.Code, w.Body.String())
	}
	if got := decodeJSONBody(t, w.Body); got["enableIsolatedWorkspaces"] != true {
		t.Fatalf("unexpected body: %#v", got)
	}
}

func TestUpdateExperimentalSettingsHandler_AllowsGuardedAutoRestartSetting(t *testing.T) {
	svc := setupInstanceSettingsSvc(t)

	body, _ := json.Marshal(map[string]any{"autoRestartDevServerWhenIdle": true})
	req := httptest.NewRequest(http.MethodPatch, "/instance/settings/experimental", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = withActor(req, ActorInfo{UserID: "local-board", ActorType: "board", IsInstanceAdmin: true})
	w := httptest.NewRecorder()

	UpdateExperimentalSettingsHandler(svc)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", w.Code, w.Body.String())
	}
	if got := decodeJSONBody(t, w.Body); got["autoRestartDevServerWhenIdle"] != true {
		t.Fatalf("unexpected body: %#v", got)
	}
}

func TestGetGeneralSettingsHandler_AllowsBoardUsers(t *testing.T) {
	svc := setupInstanceSettingsSvc(t)

	req := httptest.NewRequest(http.MethodGet, "/instance/settings/general", nil)
	req = withActor(req, ActorInfo{UserID: "user-1", ActorType: "board"})
	w := httptest.NewRecorder()

	GetGeneralSettingsHandler(svc)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	got := decodeJSONBody(t, w.Body)
	if got["censorUsernameInLogs"] != false || got["keyboardShortcuts"] != false || got["feedbackDataSharingPreference"] != "prompt" {
		t.Fatalf("unexpected body: %#v", got)
	}
}

func TestUpdateGeneralSettingsHandler_AllowsLocalBoardUsers(t *testing.T) {
	svc := setupInstanceSettingsSvc(t)

	body, _ := json.Marshal(map[string]any{
		"censorUsernameInLogs":          true,
		"keyboardShortcuts":             true,
		"feedbackDataSharingPreference": "allowed",
	})
	req := httptest.NewRequest(http.MethodPatch, "/instance/settings/general", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = withActor(req, ActorInfo{UserID: "local-board", ActorType: "board", IsInstanceAdmin: true})
	w := httptest.NewRecorder()

	UpdateGeneralSettingsHandler(svc)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", w.Code, w.Body.String())
	}
	got := decodeJSONBody(t, w.Body)
	if got["censorUsernameInLogs"] != true || got["keyboardShortcuts"] != true || got["feedbackDataSharingPreference"] != "allowed" {
		t.Fatalf("unexpected body: %#v", got)
	}
}

func TestUpdateGeneralSettingsHandler_PersistsS3StorageConfigForUi(t *testing.T) {
	svc := setupInstanceSettingsSvc(t)

	storagePatch := map[string]any{
		"storage": map[string]any{
			"provider": "s3",
			"localDisk": map[string]any{
				"baseDir": "/var/lib/vashandi/storage",
			},
			"s3": map[string]any{
				"bucket":         "attachments",
				"region":         "af-south-1",
				"endpoint":       "https://minio.internal",
				"prefix":         "companies",
				"forcePathStyle": true,
			},
		},
	}
	body, _ := json.Marshal(storagePatch)
	req := httptest.NewRequest(http.MethodPatch, "/instance/settings/general", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = withActor(req, ActorInfo{UserID: "local-board", ActorType: "board", IsInstanceAdmin: true})
	w := httptest.NewRecorder()

	UpdateGeneralSettingsHandler(svc)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", w.Code, w.Body.String())
	}

	got := decodeJSONBody(t, w.Body)
	storage, ok := got["storage"].(map[string]any)
	if !ok {
		t.Fatalf("expected storage object, got %#v", got["storage"])
	}
	if storage["provider"] != "s3" {
		t.Fatalf("expected s3 provider, got %#v", storage["provider"])
	}

	s3Config, ok := storage["s3"].(map[string]any)
	if !ok {
		t.Fatalf("expected s3 config object, got %#v", storage["s3"])
	}
	if s3Config["bucket"] != "attachments" {
		t.Fatalf("expected bucket attachments, got %#v", s3Config["bucket"])
	}
	if s3Config["region"] != "af-south-1" {
		t.Fatalf("expected region af-south-1, got %#v", s3Config["region"])
	}
	if s3Config["endpoint"] != "https://minio.internal" {
		t.Fatalf("expected endpoint https://minio.internal, got %#v", s3Config["endpoint"])
	}
	if s3Config["prefix"] != "companies" {
		t.Fatalf("expected prefix companies, got %#v", s3Config["prefix"])
	}
	if s3Config["forcePathStyle"] != true {
		t.Fatalf("expected forcePathStyle true, got %#v", s3Config["forcePathStyle"])
	}

	getReq := httptest.NewRequest(http.MethodGet, "/instance/settings/general", nil)
	getReq = withActor(getReq, ActorInfo{UserID: "user-1", ActorType: "board"})
	getW := httptest.NewRecorder()

	GetGeneralSettingsHandler(svc)(getW, getReq)

	if getW.Code != http.StatusOK {
		t.Fatalf("expected 200 from readback, got %d; body=%s", getW.Code, getW.Body.String())
	}

	readback := decodeJSONBody(t, getW.Body)
	if readbackStorage, ok := readback["storage"].(map[string]any); !ok || readbackStorage["provider"] != "s3" {
		t.Fatalf("expected persisted storage config on readback, got %#v", readback["storage"])
	}
}

func TestUpdateGeneralSettingsHandler_RejectsNonAdminBoardUsers(t *testing.T) {
	svc := setupInstanceSettingsSvc(t)

	body, _ := json.Marshal(map[string]any{"censorUsernameInLogs": true})
	req := httptest.NewRequest(http.MethodPatch, "/instance/settings/general", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = withActor(req, ActorInfo{UserID: "user-1", ActorType: "board"})
	w := httptest.NewRecorder()

	UpdateGeneralSettingsHandler(svc)(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestUpdateGeneralSettingsHandler_RejectsAgentCallers(t *testing.T) {
	svc := setupInstanceSettingsSvc(t)

	body, _ := json.Marshal(map[string]any{"feedbackDataSharingPreference": "not_allowed"})
	req := httptest.NewRequest(http.MethodPatch, "/instance/settings/general", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = withActor(req, ActorInfo{AgentID: "agent-1", IsAgent: true, ActorType: "agent"})
	w := httptest.NewRecorder()

	UpdateGeneralSettingsHandler(svc)(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestInstanceSettingsRoutes_ExposeUiPaths(t *testing.T) {
	svc := setupInstanceSettingsSvc(t)
	router := chi.NewRouter()
	router.Get("/api/instance/settings/general", GetGeneralSettingsHandler(svc))

	req := httptest.NewRequest(http.MethodGet, "/api/instance/settings/general", nil)
	req = req.WithContext(WithActor(context.Background(), ActorInfo{UserID: "user-1", ActorType: "board"}))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
