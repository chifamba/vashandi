package routes

import (
	"encoding/json"
	"net/http"

	"github.com/chifamba/paperclip/backend/db/models"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// ListExecutionWorkspacesHandler lists execution workspaces for a company
func ListExecutionWorkspacesHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		companyID := chi.URLParam(r, "companyId")

		var workspaces []models.ExecutionWorkspace
		if err := db.Where("company_id = ?", companyID).Order("created_at desc").Find(&workspaces).Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to fetch execution workspaces"})
			return
		}

		json.NewEncoder(w).Encode(workspaces)
	}
}

// GetExecutionWorkspaceHandler fetches a single execution workspace
func GetExecutionWorkspaceHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := chi.URLParam(r, "id")

		var workspace models.ExecutionWorkspace
		if err := db.Where("id = ?", id).First(&workspace).Error; err != nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Execution workspace not found"})
			return
		}

		json.NewEncoder(w).Encode(workspace)
	}
}

// UpdateExecutionWorkspaceHandler updates an existing execution workspace
func UpdateExecutionWorkspaceHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := chi.URLParam(r, "id")

		var updates map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid payload"})
			return
		}

		// Sanitize mass assignment
		delete(updates, "id")
		delete(updates, "company_id")
		delete(updates, "created_at")
		delete(updates, "updated_at")

		var workspace models.ExecutionWorkspace
		if err := db.Where("id = ?", id).First(&workspace).Error; err != nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Execution workspace not found"})
			return
		}

		if err := db.Model(&workspace).Updates(updates).Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to update execution workspace"})
			return
		}

		json.NewEncoder(w).Encode(workspace)
	}
}
