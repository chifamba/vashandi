package routes

import (
	"encoding/json"
	"net/http"

	"github.com/chifamba/paperclip/backend/db/models"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// ListIssuesHandler lists issues for a company
func ListIssuesHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		companyID := chi.URLParam(r, "companyId")

		var issues []models.Issue
		query := db.Where("company_id = ?", companyID)

		if projectID := r.URL.Query().Get("projectId"); projectID != "" {
			query = query.Where("project_id = ?", projectID)
		}
		if status := r.URL.Query().Get("status"); status != "" {
			query = query.Where("status = ?", status)
		}
		if assigneeAgentID := r.URL.Query().Get("assigneeAgentId"); assigneeAgentID != "" {
			query = query.Where("assignee_agent_id = ?", assigneeAgentID)
		}

		if err := query.Order("created_at desc").Find(&issues).Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to fetch issues"})
			return
		}

		json.NewEncoder(w).Encode(issues)
	}
}

// GetIssueHandler fetches a single issue
func GetIssueHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := chi.URLParam(r, "id")

		var issue models.Issue
		if err := db.Where("id = ?", id).First(&issue).Error; err != nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Issue not found"})
			return
		}

		json.NewEncoder(w).Encode(issue)
	}
}

// CreateIssueHandler creates a new issue
func CreateIssueHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		companyID := chi.URLParam(r, "companyId")

		var issue models.Issue
		if err := json.NewDecoder(r.Body).Decode(&issue); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid payload"})
			return
		}

		issue.CompanyID = companyID

		if err := db.Create(&issue).Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to create issue"})
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(issue)
	}
}

// UpdateIssueHandler updates an existing issue
func UpdateIssueHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := chi.URLParam(r, "id")

		var updates map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid payload"})
			return
		}

		var issue models.Issue
		if err := db.Where("id = ?", id).First(&issue).Error; err != nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Issue not found"})
			return
		}


		// Sanitize mass assignment
		delete(updates, "id")
		delete(updates, "company_id")
		delete(updates, "created_at")
		delete(updates, "updated_at")

		if err := db.Model(&issue).Updates(updates).Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to update issue"})
			return
		}

		json.NewEncoder(w).Encode(issue)
	}
}

// DeleteIssueHandler deletes an issue
func DeleteIssueHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := chi.URLParam(r, "id")

		var issue models.Issue
		if err := db.Where("id = ?", id).First(&issue).Error; err != nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Issue not found"})
			return
		}

		if err := db.Delete(&issue).Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to delete issue"})
			return
		}

		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}
}

// ListIssueCommentsHandler lists comments for an issue
func ListIssueCommentsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := chi.URLParam(r, "id")

		var comments []models.IssueComment
		if err := db.Where("issue_id = ?", id).Order("created_at asc").Find(&comments).Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to fetch comments"})
			return
		}

		json.NewEncoder(w).Encode(comments)
	}
}

// CreateIssueCommentHandler creates a comment on an issue
func CreateIssueCommentHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := chi.URLParam(r, "id")

		var issue models.Issue
		if err := db.Where("id = ?", id).First(&issue).Error; err != nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Issue not found"})
			return
		}

		var comment models.IssueComment
		if err := json.NewDecoder(r.Body).Decode(&comment); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid payload"})
			return
		}

		comment.IssueID = id
		comment.CompanyID = issue.CompanyID

		if err := db.Create(&comment).Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to create comment"})
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(comment)
	}
}
