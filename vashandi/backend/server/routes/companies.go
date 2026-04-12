package routes

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/chifamba/paperclip/backend/db/models"
	"github.com/chifamba/paperclip/backend/server/services"
)

// ListCompaniesHandler returns a list of companies
func ListCompaniesHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var companies []models.Company
		if err := db.Find(&companies).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(companies)
	}
}

// GetCompanyHandler returns a specific company
func GetCompanyHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var company models.Company
		if err := db.First(&company, "id = ?", id).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				http.Error(w, "Company not found", http.StatusNotFound)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(company)
	}
}

// CreateCompanyHandler creates a new company and seeds OpenBrain
func CreateCompanyHandler(db *gorm.DB) http.HandlerFunc {
	adapter := services.NewOpenBrainAdapter()
	return func(w http.ResponseWriter, r *http.Request) {
		var company models.Company
		if err := json.NewDecoder(r.Body).Decode(&company); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := db.Create(&company).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Seed initial memory context
		metadata := map[string]string{
			"source": "initial_onboarding",
			"type":   "brain_md",
		}

		seedText := "Initial company knowledge base and context. Welcome to Vashandi!"

		// Run ingestion async
		go func() {
			_ = adapter.IngestMemory(r.Context(), company.ID, seedText, metadata)
		}()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(company)
	}
}
