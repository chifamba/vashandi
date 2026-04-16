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

func ListApprovalsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")
		status := r.URL.Query().Get("status")
		var approvals []models.Approval
		q := db.WithContext(r.Context()).Where("company_id = ?", companyID)
		if status != "" {
			q = q.Where("status = ?", status)
		}
		q.Order("created_at DESC").Find(&approvals)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(approvals)
	}
}

func CreateApprovalHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")
		var approval models.Approval
		if err := json.NewDecoder(r.Body).Decode(&approval); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		approval.CompanyID = companyID
		approval.Status = "pending"
		if err := db.WithContext(r.Context()).Create(&approval).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(approval)
	}
}

func ApproveHandler(db *gorm.DB, heartbeatSvc *services.HeartbeatService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if !requireBoard(w, r) {
			return
		}
		var approval models.Approval
		if err := db.WithContext(r.Context()).First(&approval, "id = ?", id).Error; err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		if !requireCompanyAccess(w, r, approval.CompanyID) {
			return
		}
		now := time.Now()
		approval.Status = "approved"
		approval.DecidedAt = &now
		db.WithContext(r.Context()).Save(&approval)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(approval)
	}
}

func RejectHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if !requireBoard(w, r) {
			return
		}
		var approval models.Approval
		if err := db.WithContext(r.Context()).First(&approval, "id = ?", id).Error; err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		if !requireCompanyAccess(w, r, approval.CompanyID) {
			return
		}
		now := time.Now()
		approval.Status = "rejected"
		approval.DecidedAt = &now
		db.WithContext(r.Context()).Save(&approval)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(approval)
	}
}

func AddApprovalCommentHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		approvalID := chi.URLParam(r, "id")
		var comment models.ApprovalComment
		if err := json.NewDecoder(r.Body).Decode(&comment); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		comment.ApprovalID = approvalID

		var approval models.Approval
		if err := db.WithContext(r.Context()).First(&approval, "id = ?", approvalID).Error; err != nil {
			http.Error(w, "Approval not found", http.StatusNotFound)
			return
		}
		comment.CompanyID = approval.CompanyID

		if err := db.WithContext(r.Context()).Create(&comment).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(comment)
	}
}

func GetApprovalHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var approval models.Approval
		if err := db.WithContext(r.Context()).First(&approval, "id = ?", id).Error; err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(approval)
	}
}

func GetApprovalIssuesHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		approvalID := chi.URLParam(r, "id")
		var issueApprovals []models.IssueApproval
		db.WithContext(r.Context()).Where("approval_id = ?", approvalID).Find(&issueApprovals)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(issueApprovals)
	}
}

func ResubmitApprovalHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var approval models.Approval
		if err := db.WithContext(r.Context()).First(&approval, "id = ?", id).Error; err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		if !requireCompanyAccess(w, r, approval.CompanyID) {
			return
		}
		actor := GetActorInfo(r)
		if actor.IsAgent {
			if approval.RequestedByAgentID == nil || *approval.RequestedByAgentID != actor.AgentID {
				http.Error(w, "Only requesting agent can resubmit this approval", http.StatusForbidden)
				return
			}
		}
		approval.Status = "pending"
		approval.DecidedAt = nil
		db.WithContext(r.Context()).Save(&approval)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(approval)
	}
}

func GetApprovalCommentsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		approvalID := chi.URLParam(r, "id")
		var comments []models.ApprovalComment
		db.WithContext(r.Context()).Where("approval_id = ?", approvalID).Order("created_at ASC").Find(&comments)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(comments)
	}
}

// requestRevisionBody represents the optional request body for the request-revision endpoint.
type requestRevisionBody struct {
	DecidedByUserID string `json:"decidedByUserId,omitempty"`
	DecisionNote    string `json:"decisionNote,omitempty"`
}

func RequestRevisionHandler(db *gorm.DB, activitySvc *services.ActivityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if !requireBoard(w, r) {
			return
		}
		var approval models.Approval
		if err := db.WithContext(r.Context()).First(&approval, "id = ?", id).Error; err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		if !requireCompanyAccess(w, r, approval.CompanyID) {
			return
		}

		// Parse optional body
		var body requestRevisionBody
		_ = json.NewDecoder(r.Body).Decode(&body) // ignore error for empty body

		actor := GetActorInfo(r)
		decidedBy := body.DecidedByUserID
		if decidedBy == "" {
			decidedBy = "board"
		}
		actorID := actor.UserID
		if actorID == "" {
			actorID = "board"
		}

		now := time.Now()
		approval.Status = "revision_requested"
		approval.DecidedAt = &now
		if body.DecidedByUserID != "" {
			approval.DecidedByUserID = &body.DecidedByUserID
		} else {
			approval.DecidedByUserID = nil
		}
		if body.DecisionNote != "" {
			approval.DecisionNote = &body.DecisionNote
		}
		db.WithContext(r.Context()).Save(&approval)

		// Log activity
		if activitySvc != nil {
			_, _ = activitySvc.Log(r.Context(), services.LogEntry{
				CompanyID:  approval.CompanyID,
				ActorType:  "user",
				ActorID:    actorID,
				Action:     "approval.revision_requested",
				EntityType: "approval",
				EntityID:   approval.ID,
				Details:    map[string]interface{}{"type": approval.Type},
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(approval)
	}
}
