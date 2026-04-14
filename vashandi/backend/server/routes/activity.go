package routes

import (
	"encoding/json"
	"net/http"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// ListActivityHandler returns an http.HandlerFunc to list activity events
func ListActivityHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		companyID := chi.URLParam(r, "companyId")

		if companyID == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "companyId is required"})
			return
		}

		agentID := r.URL.Query().Get("agentId")
		entityType := r.URL.Query().Get("entityType")
		entityID := r.URL.Query().Get("entityId")

		var activities []models.ActivityLog
		query := db.Where("company_id = ?", companyID)

		if agentID != "" {
			query = query.Where("agent_id = ?", agentID)
		}
		if entityType != "" {
			query = query.Where("entity_type = ?", entityType)
		}
		if entityID != "" {
			query = query.Where("entity_id = ?", entityID)
		}

		query.Order("created_at DESC").Limit(50).Find(&activities)

		json.NewEncoder(w).Encode(activities)
	}
}

// CreateActivityHandler returns an http.HandlerFunc to create a new activity event
func CreateActivityHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		companyID := chi.URLParam(r, "companyId")

		if companyID == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "companyId is required"})
			return
		}

		var payload models.ActivityLog
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid payload"})
			return
		}

		payload.CompanyID = companyID

		if err := db.Create(&payload).Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to create activity"})
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(payload)
	}
}

func ListIssueActivityHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
issueID := chi.URLParam(r, "id")
var activities []models.ActivityLog
db.WithContext(r.Context()).
Where("entity_type = ? AND entity_id = ?", "issue", issueID).
Order("created_at ASC").
Find(&activities)
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(activities)
}
}

func ListIssueRunsHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode([]struct{}{})
}
}
