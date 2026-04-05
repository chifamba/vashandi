package routes

import (
	"encoding/json"
	"net/http"

	"github.com/chifamba/paperclip/backend/db/models"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// ListCompanySecretsHandler lists company secrets (without values)
func ListCompanySecretsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		companyID := chi.URLParam(r, "companyId")

		var secrets []models.CompanySecret
		// CompanySecret doesn't actually contain the material (CompanySecretVersion does).
		// So querying CompanySecret is safe to expose metadata.
		if err := db.Where("company_id = ?", companyID).Order("created_at desc").Find(&secrets).Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to fetch company secrets"})
			return
		}

		json.NewEncoder(w).Encode(secrets)
	}
}

// CreateCompanySecretHandler creates a new company secret
func CreateCompanySecretHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotImplemented)
		json.NewEncoder(w).Encode(map[string]string{"error": "Secret creation is not yet implemented in the Go port (pending crypto adapter)"})
	}
}

// DeleteCompanySecretHandler deletes a company secret
func DeleteCompanySecretHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := chi.URLParam(r, "id")

		var secret models.CompanySecret
		if err := db.Where("id = ?", id).First(&secret).Error; err != nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Company secret not found"})
			return
		}

		if err := db.Delete(&secret).Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to delete company secret"})
			return
		}

		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}
}
