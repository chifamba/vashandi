package routes

import (
	"encoding/json"
	"net/http"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"github.com/chifamba/vashandi/vashandi/backend/server/services"
	"gorm.io/gorm"
)

// HeartbeatWakeupHandler triggers an agent run
func HeartbeatWakeupHandler(db *gorm.DB) http.HandlerFunc {
	secrets := services.NewSecretService(db)
	service := services.NewHeartbeatService(db, secrets, nil)
	
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			CompanyID string                 `json:"companyId"`
			AgentID   string                 `json:"agentId"`
			Source    string                 `json:"source"`
			Context   map[string]interface{} `json:"context"`
		}
		
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		run, err := service.Wakeup(r.Context(), input.CompanyID, input.AgentID, services.WakeupOptions{
			Source:        input.Source,
			TriggerDetail: "manual",
			Context:       input.Context,
		})
		
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(run)
	}
}

// ListHeartbeatRunsHandler returns a list of heartbeat runs
func ListHeartbeatRunsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := r.URL.Query().Get("companyId")
		if companyID == "" {
			http.Error(w, "companyId is required", http.StatusBadRequest)
			return
		}

		var runs []models.HeartbeatRun
		if err := db.Where("company_id = ?", companyID).Order("created_at DESC").Limit(50).Find(&runs).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(runs)
	}
}
