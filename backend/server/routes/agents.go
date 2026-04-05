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
		db.Where("company_id = ? AND status != ?", companyID, "terminated").Order("created_at DESC").Find(&agents)

		json.NewEncoder(w).Encode(agents)
	}
}

// UpdateAgentHandler updates an existing agent
func UpdateAgentHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := chi.URLParam(r, "id")

		var updates map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid payload"})
			return
		}

		var agent models.Agent
		if err := db.Where("id = ?", id).First(&agent).Error; err != nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Agent not found"})
			return
		}

		if err := db.Model(&agent).Updates(updates).Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to update agent"})
			return
		}

		json.NewEncoder(w).Encode(agent)
	}
}

// DeleteAgentHandler deletes an agent
func DeleteAgentHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := db.Delete(&models.Agent{}, "id = ?", id).Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// PauseAgentHandler pauses an agent
func PauseAgentHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		updates := map[string]interface{}{
			"status":       "paused",
			"pause_reason": "manual",
			"paused_at":    "now()",
		}
		if err := db.Model(&models.Agent{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ResumeAgentHandler resumes a paused agent
func ResumeAgentHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		updates := map[string]interface{}{
			"status":       "idle",
			"pause_reason": nil,
			"paused_at":    nil,
		}
		if err := db.Model(&models.Agent{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// TerminateAgentHandler terminates an agent
func TerminateAgentHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := db.Model(&models.Agent{}).Where("id = ?", id).Update("status", "terminated").Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ListAgentKeysHandler lists API keys for an agent
func ListAgentKeysHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := chi.URLParam(r, "id")

		var keys []models.AgentAPIKey
		if err := db.Where("agent_id = ?", id).Find(&keys).Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(keys)
	}
}

// CreateAgentKeyHandler creates a new API key for an agent
func CreateAgentKeyHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := chi.URLParam(r, "id")

		var payload struct {
			Name string `json:"name"`
		}
		json.NewDecoder(r.Body).Decode(&payload)

		// In a real port, we'd generate a secure token and hash it.
		// For now, we'll implement a simple version.
		token := "pcp_fake_token_" + id // Placeholder for real crypto logic
		key := models.AgentAPIKey{
			AgentID: id,
			Name:    payload.Name,
			KeyHash: token, // This should be the hash
		}

		if err := db.Create(&key).Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Return the cleartext token once
		response := map[string]interface{}{
			"id":    key.ID,
			"name":  key.Name,
			"token": token,
		}
		json.NewEncoder(w).Encode(response)
	}
}

// RevokeAgentKeyHandler revokes an API key
func RevokeAgentKeyHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := db.Model(&models.AgentAPIKey{}).Where("id = ?", id).Update("revoked_at", "now()").Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
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
