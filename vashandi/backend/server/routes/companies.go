package routes

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"github.com/chifamba/vashandi/vashandi/backend/server/services"
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
func CreateCompanyHandler(db *gorm.DB, secrets *services.SecretService, memory services.MemoryAdapter) http.HandlerFunc {
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

		// Generate OpenBrain Service Token
		_, err := secrets.GenerateOpenBrainToken(company.ID, "", 4) // Admin tier for service
		if err == nil {
			// In a real system, we'd store this in company_secrets
			// For now, we use it for the initial seed
			go func() {
				// We need a version of IngestMemory that takes a token override or we use the default
				// Since we are porting, we'll assume the adapter uses the token provided or env
				_ = memory.IngestMemory(r.Context(), company.ID, seedText, metadata)
			}()
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(company)
	}
}

func UpdateCompanyHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var company models.Company
		if err := db.WithContext(r.Context()).First(&company, "id = ?", id).Error; err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		var data map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		allowed := map[string]bool{"name": true, "description": true, "website": true, "runtime_config": true}
		filtered := map[string]interface{}{}
		for k, v := range data {
			if allowed[k] {
				filtered[k] = v
			}
		}
		if err := db.WithContext(r.Context()).Model(&company).Updates(filtered).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(company)
	}
}

func DeleteCompanyHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := db.WithContext(r.Context()).Where("id = ?", id).Delete(&models.Company{}).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func UpdateCompanyBrandingHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var company models.Company
		if err := db.WithContext(r.Context()).First(&company, "id = ?", id).Error; err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		var body struct {
			LogoURL      *string `json:"logoUrl"`
			PrimaryColor *string `json:"primaryColor"`
			DisplayName  *string `json:"displayName"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		updates := map[string]interface{}{}
		if body.PrimaryColor != nil {
			updates["brand_color"] = *body.PrimaryColor
		}
		if body.DisplayName != nil {
			updates["name"] = *body.DisplayName
		}
		if len(updates) > 0 {
			db.WithContext(r.Context()).Model(&company).Updates(updates)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(company)
	}
}

func GetCompanyStatsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var totalCompanies, activeAgents, openIssues int64
		db.WithContext(r.Context()).Model(&models.Company{}).Count(&totalCompanies)
		db.WithContext(r.Context()).Model(&models.Agent{}).Where("status = ?", "active").Count(&activeAgents)
		db.WithContext(r.Context()).Model(&models.Issue{}).
			Where("status NOT IN ?", []string{"done", "cancelled"}).
			Count(&openIssues)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int64{
			"totalCompanies": totalCompanies,
			"activeAgents":   activeAgents,
			"openIssues":     openIssues,
		})
	}
}

// ArchiveCompanyHandler archives a company and notifies OpenBrain
func ArchiveCompanyHandler(db *gorm.DB, memory services.MemoryAdapter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var company models.Company
		if err := db.First(&company, "id = ?", id).Error; err != nil {
			http.Error(w, "Company not found", http.StatusNotFound)
			return
		}

		// Update status
		if err := db.Model(&company).Update("is_archived", true).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Notify OpenBrain (async)
		go func() {
			_ = memory.ArchiveNamespace(context.Background(), id)
		}()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "archived"})
	}
}

// ExportCompanyHandler — POST /companies/:id/exports
// Stub: company export not yet implemented in the Go backend.
func ExportCompanyHandler() http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
w.Header().Set("Content-Type", "application/json")
w.WriteHeader(http.StatusNotImplemented)
json.NewEncoder(w).Encode(map[string]string{
"status":  "export_not_implemented",
"message": "Company export is not yet available in the Go backend",
})
}
}

// ImportCompanyHandler — POST /companies/:id/imports/apply
// Stub: company import not yet implemented in the Go backend.
func ImportCompanyHandler() http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
w.Header().Set("Content-Type", "application/json")
w.WriteHeader(http.StatusNotImplemented)
json.NewEncoder(w).Encode(map[string]string{
"status":  "import_not_implemented",
"message": "Company import is not yet available in the Go backend",
})
}
}
