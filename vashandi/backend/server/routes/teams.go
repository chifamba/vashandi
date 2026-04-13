package routes

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
)

// TeamsHandler serves the teams list for a company
func TeamsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		companyID := chi.URLParam(r, "companyId")

		if companyID == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "companyId is required"})
			return
		}

		var teams []models.Team
		db.Where("company_id = ?", companyID).Find(&teams)

		json.NewEncoder(w).Encode(teams)
	}
}

// TeamHandler serves details for a specific team
func TeamHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		teamID := chi.URLParam(r, "teamId")

		if teamID == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "teamId is required"})
			return
		}

		var team models.Team
		if result := db.First(&team, "id = ?", teamID); result.Error != nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "team not found"})
			return
		}

		json.NewEncoder(w).Encode(team)
	}
}
