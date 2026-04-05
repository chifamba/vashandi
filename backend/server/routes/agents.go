package routes

import (
	"encoding/json"
	"net/http"

	"github.com/chifamba/paperclip/backend/db/models"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// ListAgentsHandler lists agents for a company
func ListAgentsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		companyID := chi.URLParam(r, "companyId")

		if companyID == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "companyId is required"})
			return
		}

		var agents []models.Agent
		db.Where("company_id = ? AND archived_at IS NULL", companyID).Order("created_at DESC").Find(&agents)

		json.NewEncoder(w).Encode(agents)
	}
}

// GetAgentHandler gets a specific agent
func GetAgentHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := chi.URLParam(r, "id")

		var agent models.Agent
		if err := db.Where("id = ?", id).First(&agent).Error; err != nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Agent not found"})
			return
		}

		json.NewEncoder(w).Encode(agent)
	}
}

// CreateAgentHandler creates a new agent
func CreateAgentHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		companyID := chi.URLParam(r, "companyId")

		var agent models.Agent
		if err := json.NewDecoder(r.Body).Decode(&agent); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid payload"})
			return
		}

		agent.CompanyID = companyID

		if err := db.Create(&agent).Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to create agent"})
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(agent)
	}
}
