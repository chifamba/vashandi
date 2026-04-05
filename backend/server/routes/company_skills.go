package routes

import (
	"encoding/json"
	"net/http"

	"github.com/chifamba/paperclip/backend/db/models"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// ListCompanySkillsHandler lists company skills
func ListCompanySkillsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		companyID := chi.URLParam(r, "companyId")

		var skills []models.CompanySkill
		if err := db.Where("company_id = ?", companyID).Order("created_at desc").Find(&skills).Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to fetch company skills"})
			return
		}

		json.NewEncoder(w).Encode(skills)
	}
}

// CreateCompanySkillHandler creates a new company skill
func CreateCompanySkillHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		companyID := chi.URLParam(r, "companyId")

		var skill models.CompanySkill
		if err := json.NewDecoder(r.Body).Decode(&skill); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid payload"})
			return
		}

		skill.CompanyID = companyID

		if err := db.Create(&skill).Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to create company skill"})
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(skill)
	}
}

// DeleteCompanySkillHandler deletes a company skill
func DeleteCompanySkillHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := chi.URLParam(r, "id")

		var skill models.CompanySkill
		if err := db.Where("id = ?", id).First(&skill).Error; err != nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Company skill not found"})
			return
		}

		if err := db.Delete(&skill).Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to delete company skill"})
			return
		}

		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}
}
