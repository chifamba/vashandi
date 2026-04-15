package routes

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

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

// assertCanManagePortability checks that the request actor may manage portability
// for the given company. Board users are always allowed. Agent actors must be
// CEO agents scoped to the same company.
func assertCanManagePortability(r *http.Request, db *gorm.DB, companyID, capability string) error {
	actor := GetActorInfo(r)
	if !actor.IsAgent {
		// Board / system access
		if err := AssertBoard(r); err != nil {
			return err
		}
		return nil
	}
	if actor.AgentID == "" {
		return fmt.Errorf("agent authentication required")
	}
	var agent models.Agent
	if err := db.WithContext(r.Context()).First(&agent, "id = ?", actor.AgentID).Error; err != nil {
		return fmt.Errorf("agent not found")
	}
	if agent.CompanyID != companyID {
		return fmt.Errorf("agent key cannot access another company")
	}
	if !strings.EqualFold(strings.TrimSpace(agent.Role), "ceo") {
		return fmt.Errorf("only CEO agents can manage company %s", capability)
	}
	return nil
}

// PreviewExportCompanyHandler — POST /companies/:companyId/exports/preview
func PreviewExportCompanyHandler(db *gorm.DB) http.HandlerFunc {
	svc := services.NewPortabilityService(db)
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")
		if err := assertCanManagePortability(r, db, companyID, "exports"); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		var req services.ExportRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		result, err := svc.PreviewExport(r.Context(), companyID, req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}

// ExportCompanyHandler — POST /companies/:companyId/exports
func ExportCompanyHandler(db *gorm.DB) http.HandlerFunc {
	svc := services.NewPortabilityService(db)
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")
		if err := assertCanManagePortability(r, db, companyID, "exports"); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		var req services.ExportRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		result, err := svc.ExportBundle(r.Context(), companyID, req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}

// PreviewImportCompanyHandler — POST /companies/:companyId/imports/preview
func PreviewImportCompanyHandler(db *gorm.DB) http.HandlerFunc {
	svc := services.NewPortabilityService(db)
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")
		if err := assertCanManagePortability(r, db, companyID, "imports"); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		var req services.ImportRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		actor := GetActorInfo(r)
		mode := services.ImportModeBoardFull
		if actor.IsAgent {
			mode = services.ImportModeAgentSafe
		}
		result, err := svc.PreviewImport(r.Context(), req, mode)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}

// ImportCompanyHandler — POST /companies/:companyId/imports/apply
func ImportCompanyHandler(db *gorm.DB) http.HandlerFunc {
	svc := services.NewPortabilityService(db)
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")
		if err := assertCanManagePortability(r, db, companyID, "imports"); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		var req services.ImportRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		actor := GetActorInfo(r)
		mode := services.ImportModeBoardFull
		if actor.IsAgent {
			mode = services.ImportModeAgentSafe
		}
		actorUserID := actor.UserID
		result, err := svc.ImportBundle(r.Context(), req, actorUserID, mode)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}

// ListCompanyFeedbackTracesHandler returns feedback traces for a company (board only).
func ListCompanyFeedbackTracesHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := AssertBoard(r); err != nil {
			http.Error(w, "Only board users can view feedback traces", http.StatusForbidden)
			return
		}
		companyID := chi.URLParam(r, "companyId")

		q := r.URL.Query()
		filters := map[string]interface{}{"company_id": companyID}
		var extra []string
		var extraArgs []interface{}
		if v := q.Get("issueId"); v != "" {
			filters["issue_id"] = v
		}
		if v := q.Get("projectId"); v != "" {
			filters["project_id"] = v
		}
		if v := q.Get("targetType"); v != "" {
			filters["target_type"] = v
		}
		if v := q.Get("vote"); v != "" {
			filters["vote"] = v
		}
		if v := q.Get("status"); v != "" {
			filters["status"] = v
		}
		if q.Get("sharedOnly") == "true" {
			extra = append(extra, "fe.status != ?")
			extraArgs = append(extraArgs, "local_only")
		}
		if v := q.Get("from"); v != "" {
			extra = append(extra, "fe.created_at >= ?")
			extraArgs = append(extraArgs, v)
		}
		if v := q.Get("to"); v != "" {
			extra = append(extra, "fe.created_at <= ?")
			extraArgs = append(extraArgs, v)
		}

		includePayload := q.Get("includePayload") == "true"
		rows, err := queryFeedbackTraces(r.Context(), db, filters, extra, extraArgs)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		out := make([]feedbackTraceResponse, 0, len(rows))
		for _, row := range rows {
			out = append(out, buildFeedbackTraceResponse(row, includePayload))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	}
}
