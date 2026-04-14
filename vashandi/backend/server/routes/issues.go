package routes

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"github.com/chifamba/vashandi/vashandi/backend/server/services"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// IssueRoutes handles HTTP requests for issues
type IssueRoutes struct {
	db      *gorm.DB
	service *services.IssueService
}

// NewIssueRoutes creates a new IssueRoutes
func NewIssueRoutes(db *gorm.DB, activity *services.ActivityService) *IssueRoutes {
	return &IssueRoutes{
		db:      db,
		service: services.NewIssueService(db, activity),
	}
}

// ListIssuesHandler returns a list of issues
func (ir *IssueRoutes) ListIssuesHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	filters := map[string]interface{}{
		"status":          r.URL.Query().Get("status"),
		"assigneeAgentId": r.URL.Query().Get("assigneeAgentId"),
		"projectId":       r.URL.Query().Get("projectId"),
	}

	issues, err := ir.service.ListIssues(r.Context(), companyID, filters)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(issues)
}

// GetIssueHandler returns a single issue
func (ir *IssueRoutes) GetIssueHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var issue models.Issue
	if err := ir.db.WithContext(r.Context()).Preload("AssigneeAgent").Preload("Project").First(&issue, "id = ?", id).Error; err != nil {
		http.Error(w, "Issue not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(issue)
}

// CreateIssueHandler creates a new issue
func (ir *IssueRoutes) CreateIssueHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	var issue models.Issue
	if err := json.NewDecoder(r.Body).Decode(&issue); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	issue.CompanyID = companyID
	created, err := ir.service.CreateIssue(r.Context(), &issue)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(created)
}

// TransitionIssueHandler handles status changes
func (ir *IssueRoutes) TransitionIssueHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	companyID := r.URL.Query().Get("companyId") // Simplified for parity; in production this comes from context/payload
	
	var payload struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	updated, err := ir.service.TransitionStatus(r.Context(), id, companyID, payload.Status)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict) // Preserving 409 for invalid transitions
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated)
}

// UpdateIssueHandler updates an issue's fields.
func (ir *IssueRoutes) UpdateIssueHandler(w http.ResponseWriter, r *http.Request) {
id := chi.URLParam(r, "id")
var issue models.Issue
if err := ir.db.WithContext(r.Context()).First(&issue, "id = ?", id).Error; err != nil {
http.Error(w, "Not found", http.StatusNotFound)
return
}
if err := json.NewDecoder(r.Body).Decode(&issue); err != nil {
http.Error(w, err.Error(), http.StatusBadRequest)
return
}
ir.db.WithContext(r.Context()).Save(&issue)
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(issue)
}

// DeleteIssueHandler soft-deletes an issue via hidden_at.
func (ir *IssueRoutes) DeleteIssueHandler(w http.ResponseWriter, r *http.Request) {
id := chi.URLParam(r, "id")
var issue models.Issue
if err := ir.db.WithContext(r.Context()).First(&issue, "id = ?", id).Error; err != nil {
http.Error(w, "Not found", http.StatusNotFound)
return
}
now := time.Now()
issue.HiddenAt = &now
ir.db.WithContext(r.Context()).Save(&issue)
w.WriteHeader(http.StatusNoContent)
}

// AddIssueCommentHandler creates a comment on an issue.
func (ir *IssueRoutes) AddIssueCommentHandler(w http.ResponseWriter, r *http.Request) {
id := chi.URLParam(r, "id")
var issue models.Issue
if err := ir.db.WithContext(r.Context()).First(&issue, "id = ?", id).Error; err != nil {
http.Error(w, "Not found", http.StatusNotFound)
return
}
var comment models.IssueComment
if err := json.NewDecoder(r.Body).Decode(&comment); err != nil {
http.Error(w, err.Error(), http.StatusBadRequest)
return
}
comment.IssueID = id
comment.CompanyID = issue.CompanyID
if err := ir.db.WithContext(r.Context()).Create(&comment).Error; err != nil {
http.Error(w, err.Error(), http.StatusInternalServerError)
return
}
w.Header().Set("Content-Type", "application/json")
w.WriteHeader(http.StatusCreated)
json.NewEncoder(w).Encode(comment)
}

// ListIssueCommentsHandler lists comments for an issue.
func (ir *IssueRoutes) ListIssueCommentsHandler(w http.ResponseWriter, r *http.Request) {
id := chi.URLParam(r, "id")
var comments []models.IssueComment
ir.db.WithContext(r.Context()).Where("issue_id = ?", id).Order("created_at ASC").Find(&comments)
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(comments)
}

// CreateWorkProductHandler creates a work product for an issue.
func (ir *IssueRoutes) CreateWorkProductHandler(w http.ResponseWriter, r *http.Request) {
id := chi.URLParam(r, "id")
var issue models.Issue
if err := ir.db.WithContext(r.Context()).First(&issue, "id = ?", id).Error; err != nil {
http.Error(w, "Not found", http.StatusNotFound)
return
}
var wp models.IssueWorkProduct
if err := json.NewDecoder(r.Body).Decode(&wp); err != nil {
http.Error(w, err.Error(), http.StatusBadRequest)
return
}
wp.IssueID = id
wp.CompanyID = issue.CompanyID
if err := ir.db.WithContext(r.Context()).Create(&wp).Error; err != nil {
http.Error(w, err.Error(), http.StatusInternalServerError)
return
}
w.Header().Set("Content-Type", "application/json")
w.WriteHeader(http.StatusCreated)
json.NewEncoder(w).Encode(wp)
}

// ListWorkProductsHandler lists work products for an issue.
func (ir *IssueRoutes) ListWorkProductsHandler(w http.ResponseWriter, r *http.Request) {
id := chi.URLParam(r, "id")
var wps []models.IssueWorkProduct
ir.db.WithContext(r.Context()).Where("issue_id = ?", id).Order("created_at DESC").Find(&wps)
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(wps)
}

// BulkUpdateIssuesHandler handles bulk updates to issues.
func (ir *IssueRoutes) BulkUpdateIssuesHandler(w http.ResponseWriter, r *http.Request) {
companyID := chi.URLParam(r, "companyId")
var body struct {
IDs    []string               `json:"ids"`
Update map[string]interface{} `json:"update"`
}
if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
http.Error(w, err.Error(), http.StatusBadRequest)
return
}
if len(body.IDs) == 0 {
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(map[string]int{"updated": 0})
return
}
result := ir.db.WithContext(r.Context()).Model(&models.Issue{}).
Where("id IN ? AND company_id = ?", body.IDs, companyID).
Updates(body.Update)
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(map[string]int64{"updated": result.RowsAffected})
}
