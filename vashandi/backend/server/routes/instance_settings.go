package routes

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var (
	defaultGeneralSettings = map[string]any{
		"censorUsernameInLogs":          false,
		"keyboardShortcuts":             false,
		"feedbackDataSharingPreference": "prompt",
		"backupRetention": map[string]any{
			"dailyDays":     7,
			"weeklyWeeks":   4,
			"monthlyMonths": 1,
		},
	}
	defaultExperimentalSettings = map[string]any{
		"enableIsolatedWorkspaces":     false,
		"autoRestartDevServerWhenIdle": false,
	}
)

func loadOrCreateInstanceSetting(db *gorm.DB, r *http.Request) (*models.InstanceSetting, error) {
	var setting models.InstanceSetting
	if err := db.WithContext(r.Context()).Where("singleton_key = ?", "default").First(&setting).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		setting = models.InstanceSetting{
			ID:           uuid.NewString(),
			SingletonKey: "default",
			General:      datatypes.JSON([]byte(`{}`)),
			Experimental: datatypes.JSON([]byte(`{}`)),
		}
		if err := db.WithContext(r.Context()).Create(&setting).Error; err != nil {
			return nil, err
		}
	}
	return &setting, nil
}

func decodeSettings(raw datatypes.JSON, defaults map[string]any) map[string]any {
	merged := map[string]any{}
	for key, value := range defaults {
		merged[key] = value
	}
	if len(raw) == 0 {
		return merged
	}
	var existing map[string]any
	if err := json.Unmarshal(raw, &existing); err != nil {
		return merged
	}
	for key, value := range existing {
		merged[key] = value
	}
	return merged
}

func mergeSettings(raw datatypes.JSON, patch map[string]any) datatypes.JSON {
	current := decodeSettings(raw, map[string]any{})
	for key, value := range patch {
		current[key] = value
	}
	body, _ := json.Marshal(current)
	return datatypes.JSON(body)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func requireBoard(w http.ResponseWriter, r *http.Request) bool {
	if err := AssertBoard(r); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return false
	}
	return true
}

func requireCompanyAccess(w http.ResponseWriter, r *http.Request, companyID string) bool {
	if err := AssertCompanyAccess(r, companyID); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return false
	}
	return true
}

func requireInstanceAdmin(w http.ResponseWriter, r *http.Request) bool {
	if err := AssertInstanceAdmin(r); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return false
	}
	return true
}

func GetGeneralSettingsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireBoard(w, r) {
			return
		}
		setting, err := loadOrCreateInstanceSetting(db, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, decodeSettings(setting.General, defaultGeneralSettings))
	}
}

func UpdateGeneralSettingsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireInstanceAdmin(w, r) {
			return
		}
		setting, err := loadOrCreateInstanceSetting(db, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var patch map[string]any
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		setting.General = mergeSettings(setting.General, patch)
		if err := db.WithContext(r.Context()).Save(setting).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, decodeSettings(setting.General, defaultGeneralSettings))
	}
}

func GetExperimentalSettingsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireBoard(w, r) {
			return
		}
		setting, err := loadOrCreateInstanceSetting(db, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, decodeSettings(setting.Experimental, defaultExperimentalSettings))
	}
}

func UpdateExperimentalSettingsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireInstanceAdmin(w, r) {
			return
		}
		setting, err := loadOrCreateInstanceSetting(db, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var patch map[string]any
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		setting.Experimental = mergeSettings(setting.Experimental, patch)
		if err := db.WithContext(r.Context()).Save(setting).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, decodeSettings(setting.Experimental, defaultExperimentalSettings))
	}
}
