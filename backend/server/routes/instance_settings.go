package routes

import (
	"encoding/json"
	"net/http"

	"github.com/chifamba/paperclip/backend/db/models"
	"gorm.io/gorm"
)

// GetInstanceSettingsGeneralHandler fetches general instance settings
func GetInstanceSettingsGeneralHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		var settings models.InstanceSetting
		if err := db.First(&settings).Error; err != nil {
			// If not found, a default row could be created, but for simple GET we just return 404
			if err == gorm.ErrRecordNotFound {
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(map[string]string{"error": "Instance settings not initialized"})
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to fetch instance settings"})
			return
		}

		json.NewEncoder(w).Encode(settings.General)
	}
}

// UpdateInstanceSettingsGeneralHandler updates general instance settings
func UpdateInstanceSettingsGeneralHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		var updates map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid payload"})
			return
		}

		var settings models.InstanceSetting
		if err := db.First(&settings).Error; err != nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Instance settings not initialized"})
			return
		}

		// Because General is a JSONB column, updating it partially via map directly onto the Model
		// requires care or merging in Go. For this port, we map the provided payload to the `general` column.
		if err := db.Model(&settings).Update("general", updates).Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to update general settings"})
			return
		}

		json.NewEncoder(w).Encode(settings.General)
	}
}

// GetInstanceSettingsExperimentalHandler fetches experimental instance settings
func GetInstanceSettingsExperimentalHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		var settings models.InstanceSetting
		if err := db.First(&settings).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(map[string]string{"error": "Instance settings not initialized"})
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to fetch instance settings"})
			return
		}

		json.NewEncoder(w).Encode(settings.Experimental)
	}
}

// UpdateInstanceSettingsExperimentalHandler updates experimental instance settings
func UpdateInstanceSettingsExperimentalHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		var updates map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid payload"})
			return
		}

		var settings models.InstanceSetting
		if err := db.First(&settings).Error; err != nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Instance settings not initialized"})
			return
		}

		if err := db.Model(&settings).Update("experimental", updates).Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to update experimental settings"})
			return
		}

		json.NewEncoder(w).Encode(settings.Experimental)
	}
}
