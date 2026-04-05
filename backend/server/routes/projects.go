package routes

import (
	"encoding/json"
	"net/http"

	"github.com/chifamba/paperclip/backend/db/models"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// ListProjectsHandler lists projects for a company
func ListProjectsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		companyID := chi.URLParam(r, "companyId")

		if companyID == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "companyId is required"})
			return
		}

		var projects []models.Project
		db.Where("company_id = ? AND archived_at IS NULL", companyID).Order("created_at DESC").Find(&projects)

		json.NewEncoder(w).Encode(projects)
	}
}

// GetProjectHandler gets a specific project
func GetProjectHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := chi.URLParam(r, "id")

		var project models.Project
		if err := db.Where("id = ?", id).First(&project).Error; err != nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Project not found"})
			return
		}

		json.NewEncoder(w).Encode(project)
	}
}

// CreateProjectHandler creates a new project
func CreateProjectHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		companyID := chi.URLParam(r, "companyId")

		var project models.Project
		if err := json.NewDecoder(r.Body).Decode(&project); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid payload"})
			return
		}

		project.CompanyID = companyID

		if err := db.Create(&project).Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to create project"})
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(project)
	}
}

// UpdateProjectHandler updates an existing project
func UpdateProjectHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := chi.URLParam(r, "id")

		var updates map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid payload"})
			return
		}

		var project models.Project
		if err := db.Where("id = ?", id).First(&project).Error; err != nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Project not found"})
			return
		}


		// Sanitize mass assignment
		delete(updates, "id")
		delete(updates, "company_id")
		delete(updates, "created_at")
		delete(updates, "updated_at")

		if err := db.Model(&project).Updates(updates).Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to update project"})
			return
		}

		json.NewEncoder(w).Encode(project)
	}
}

// ArchiveProjectHandler soft-deletes a project
func ArchiveProjectHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := chi.URLParam(r, "id")



		if err := db.Model(&models.Project{}).Where("id = ?", id).Update("archived_at", "now()").Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to archive project"})
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// ListProjectWorkspacesHandler lists workspaces for a project
func ListProjectWorkspacesHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := chi.URLParam(r, "id")

		var workspaces []models.ProjectWorkspace
		if err := db.Where("project_id = ?", id).Find(&workspaces).Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to fetch workspaces"})
			return
		}

		json.NewEncoder(w).Encode(workspaces)
	}
}

// CreateProjectWorkspaceHandler creates a new workspace for a project
func CreateProjectWorkspaceHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := chi.URLParam(r, "id")

		var workspace models.ProjectWorkspace
		if err := json.NewDecoder(r.Body).Decode(&workspace); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid payload"})
			return
		}

		// Ensure the project exists and get its companyId
		var project models.Project
		if err := db.Where("id = ?", id).First(&project).Error; err != nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Project not found"})
			return
		}

		workspace.ProjectID = id
		workspace.CompanyID = project.CompanyID

		if err := db.Create(&workspace).Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to create workspace"})
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(workspace)
	}
}

// DeleteProjectHandler deletes a project
func DeleteProjectHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := chi.URLParam(r, "id")

		var project models.Project
		if err := db.Where("id = ?", id).First(&project).Error; err != nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Project not found"})
			return
		}

		if err := db.Delete(&project).Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to delete project"})
			return
		}

		json.NewEncoder(w).Encode(project)
	}
}
