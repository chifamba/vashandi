package routes

import (
	"encoding/json"
	"net/http"

	"github.com/chifamba/paperclip/backend/db/models"
	"github.com/chifamba/paperclip/backend/internal/portability"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// ListCompaniesHandler lists all companies accessible to the user
func ListCompaniesHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		var companies []models.Company
		// In a real auth implementation, filter by user access
		if err := db.Where("archived_at IS NULL").Find(&companies).Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Failed to fetch companies"})
			return
		}

		json.NewEncoder(w).Encode(companies)
	}
}

// GetCompanyHandler fetches a single company
func GetCompanyHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := chi.URLParam(r, "id")

		var company models.Company
		if err := db.Where("id = ?", id).First(&company).Error; err != nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Company not found"})
			return
		}

		json.NewEncoder(w).Encode(company)
	}
}

// CreateCompanyHandler creates a new company
func CreateCompanyHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		var company models.Company
		if err := json.NewDecoder(r.Body).Decode(&company); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid payload"})
			return
		}

		if err := db.Create(&company).Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to create company"})
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(company)
	}
}

// UpdateCompanyHandler updates an existing company
func UpdateCompanyHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := chi.URLParam(r, "id")

		var updates map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid payload"})
			return
		}

		var company models.Company
		if err := db.Where("id = ?", id).First(&company).Error; err != nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Company not found"})
			return
		}

		if err := db.Model(&company).Updates(updates).Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to update company"})
			return
		}

		json.NewEncoder(w).Encode(company)
	}
}

// DeleteCompanyHandler fully deletes a company
func DeleteCompanyHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := chi.URLParam(r, "id")

		var company models.Company
		if err := db.Where("id = ?", id).First(&company).Error; err != nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Company not found"})
			return
		}

		if err := db.Delete(&company).Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to delete company"})
			return
		}

		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}
}

// ArchiveCompanyHandler soft-deletes a company
func ArchiveCompanyHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := chi.URLParam(r, "id")

		if err := db.Model(&models.Company{}).Where("id = ?", id).Update("archived_at", "now()").Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to archive company"})
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// CompanyStatsHandler returns statistics for all companies
func CompanyStatsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		type StatResult struct {
			CompanyID string `json:"companyId"`
			Count     int    `json:"count"`
		}
		var results []StatResult

		if err := db.Model(&models.Agent{}).Select("company_id, count(*) as count").Group("company_id").Scan(&results).Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to fetch stats"})
			return
		}

		stats := make(map[string]map[string]interface{})
		for _, res := range results {
			stats[res.CompanyID] = map[string]interface{}{
				"agentCount": res.Count,
			}
		}

		json.NewEncoder(w).Encode(stats)
	}
}

// ListFeedbackTracesHandler returns feedback traces for a company
func ListFeedbackTracesHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		companyID := chi.URLParam(r, "companyId")

		query := db.Table("feedback_exports").
			Select("feedback_exports.*, issues.identifier as issue_identifier, issues.title as issue_title").
			Joins("join issues on feedback_exports.issue_id = issues.id").
			Where("feedback_exports.company_id = ?", companyID)

		// Filters
		if targetType := r.URL.Query().Get("targetType"); targetType != "" {
			query = query.Where("feedback_exports.target_type = ?", targetType)
		}
		if vote := r.URL.Query().Get("vote"); vote != "" {
			query = query.Where("feedback_exports.vote = ?", vote)
		}
		if status := r.URL.Query().Get("status"); status != "" {
			query = query.Where("feedback_exports.status = ?", status)
		}
		if issueID := r.URL.Query().Get("issueId"); issueID != "" {
			query = query.Where("feedback_exports.issue_id = ?", issueID)
		}
		if projectID := r.URL.Query().Get("projectId"); projectID != "" {
			query = query.Where("feedback_exports.project_id = ?", projectID)
		}
		if from := r.URL.Query().Get("from"); from != "" {
			query = query.Where("feedback_exports.created_at >= ?", from)
		}
		if to := r.URL.Query().Get("to"); to != "" {
			query = query.Where("feedback_exports.created_at <= ?", to)
		}

		type TraceResult struct {
			models.FeedbackExport
			IssueIdentifier *string `json:"issueIdentifier"`
			IssueTitle      string  `json:"issueTitle"`
		}
		var results []TraceResult

		if err := query.Order("feedback_exports.created_at desc").Scan(&results).Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to fetch feedback traces"})
			return
		}

		json.NewEncoder(w).Encode(results)
	}
}

// ExportCompanyHandler generates and returns a company export manifest
func ExportCompanyHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		companyID := chi.URLParam(r, "companyId")

		manifest, err := portability.ExportCompany(db, companyID)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		json.NewEncoder(w).Encode(manifest)
	}
}

// ImportCompanyHandler imports a company from a manifest
func ImportCompanyHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		var manifest portability.Manifest
		if err := json.NewDecoder(r.Body).Decode(&manifest); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid manifest payload"})
			return
		}

		id, err := portability.ImportCompany(db, &manifest)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		json.NewEncoder(w).Encode(map[string]string{"id": id})
	}
}
