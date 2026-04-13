package routes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/chifamba/vashandi/vashandi/backend/shared"
	"github.com/chifamba/vashandi/vashandi/backend/db/models"
)

// ListAgentsHandler returns a list of agents for a company
func ListAgentsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")

		var agents []models.Agent
		if err := db.Where("company_id = ?", companyID).Find(&agents).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(agents)
	}
}

// GetAgentHandler returns a specific agent
func GetAgentHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var agent models.Agent
		if err := db.First(&agent, "id = ?", id).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				http.Error(w, "Agent not found", http.StatusNotFound)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(agent)
	}
}

// CreateAgentHandler unmarshals JSON into an Agent, saves it, and triggers OpenBrain sync
func CreateAgentHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")

		var agent models.Agent
		if err := json.NewDecoder(r.Body).Decode(&agent); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		agent.CompanyID = companyID

		// Default permissions to an empty object if not provided to satisfy not-null constraints
		if len(agent.Permissions) == 0 {
			agent.Permissions = []byte("{}")
		}

		if err := db.Create(&agent).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Fire HTTP POST to OpenBrain webhook for Agent Sync Lifecycle Events (Task 2.3)
		go func(agentID, compID string) {
			url := fmt.Sprintf("http://openbrain:3101/internal/v1/namespaces/%s/agents", compID)

			payload := map[string]string{"agent_id": agentID}
			bodyBytes, _ := json.Marshal(payload)

			// Non-blocking fire and forget for sync webhook
			req, _ := http.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer dev_secret_token") // Dev default
			http.DefaultClient.Do(req)
		}(agent.ID, companyID)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(agent)
	}
}

// DeleteAgentHandler soft deletes an Agent and triggers OpenBrain namespace closure
func DeleteAgentHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var agent models.Agent
		if err := db.First(&agent, "id = ?", id).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				http.Error(w, "Agent not found", http.StatusNotFound)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		if err := db.Delete(&agent).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Fire HTTP DELETE to OpenBrain webhook for Agent Sync Lifecycle Events (Task 2.3)
		go func(agentID, compID string) {
			url := fmt.Sprintf("http://openbrain:3101/internal/v1/namespaces/%s/agents/%s", compID, agentID)

			req, _ := http.NewRequest("DELETE", url, nil)
			req.Header.Set("Authorization", "Bearer dev_secret_token") // Dev default
			http.DefaultClient.Do(req)
		}(agent.ID, agent.CompanyID)

		w.WriteHeader(http.StatusNoContent)
	}
}
