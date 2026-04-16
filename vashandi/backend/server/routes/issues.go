package routes

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"github.com/chifamba/vashandi/vashandi/backend/server/services"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

func sha256hash(data []byte) [32]byte {
	return sha256.Sum256(data)
}

type issueAttachmentResponse struct {
	ID               string    `json:"id"`
	CompanyID        string    `json:"companyId"`
	IssueID          string    `json:"issueId"`
	IssueCommentID   *string   `json:"issueCommentId"`
	AssetID          string    `json:"assetId"`
	Provider         string    `json:"provider"`
	ObjectKey        string    `json:"objectKey"`
	ContentType      string    `json:"contentType"`
	ByteSize         int       `json:"byteSize"`
	Sha256           string    `json:"sha256"`
	OriginalFilename *string   `json:"originalFilename"`
	CreatedByAgentID *string   `json:"createdByAgentId"`
	CreatedByUserID  *string   `json:"createdByUserId"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
	ContentPath      string    `json:"contentPath"`
}

func buildIssueAttachmentResponse(attachment models.IssueAttachment, asset models.Asset) issueAttachmentResponse {
	return issueAttachmentResponse{
		ID:               attachment.ID,
		CompanyID:        attachment.CompanyID,
		IssueID:          attachment.IssueID,
		IssueCommentID:   attachment.IssueCommentID,
		AssetID:          attachment.AssetID,
		Provider:         asset.Provider,
		ObjectKey:        asset.ObjectKey,
		ContentType:      asset.ContentType,
		ByteSize:         asset.ByteSize,
		Sha256:           asset.Sha256,
		OriginalFilename: asset.OriginalFilename,
		CreatedByAgentID: asset.CreatedByAgentID,
		CreatedByUserID:  asset.CreatedByUserID,
		CreatedAt:        attachment.CreatedAt,
		UpdatedAt:        attachment.UpdatedAt,
		ContentPath:      fmt.Sprintf("/api/attachments/%s/content", attachment.ID),
	}
}

func attachmentDisposition(contentType string) string {
	if strings.HasPrefix(strings.ToLower(contentType), "image/") {
		return "inline"
	}
	return "attachment"
}

func sanitizeAttachmentFilename(name string) string {
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == '\\' || r == '"' || r == '\'' {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "" {
		return "attachment"
	}
	return name
}

func buildAttachmentContentDisposition(disposition, filename string) string {
	if value := mime.FormatMediaType(disposition, map[string]string{
		"filename": sanitizeAttachmentFilename(filename),
	}); value != "" {
		return value
	}
	return disposition
}

// IssueRoutes handles HTTP requests for issues
type IssueRoutes struct {
	db        *gorm.DB
	service   *services.IssueService
	feedback  *services.FeedbackService
	documents *services.DocumentService
}

// NewIssueRoutes creates a new IssueRoutes
func NewIssueRoutes(db *gorm.DB, activity *services.ActivityService) *IssueRoutes {
	return &IssueRoutes{
		db:        db,
		service:   services.NewIssueService(db, activity),
		feedback:  services.NewFeedbackService(db),
		documents: services.NewDocumentService(db),
	}
}

// ListAllIssuesHandler returns issues across all companies (admin)
func (ir *IssueRoutes) ListAllIssuesHandler(w http.ResponseWriter, r *http.Request) {
	var issues []models.Issue
	q := ir.db.WithContext(r.Context()).Order("created_at DESC").Limit(100)
	if status := r.URL.Query().Get("status"); status != "" {
		q = q.Where("status = ?", status)
	}
	q.Find(&issues)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(issues)
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

// ReleaseIssueHandler clears the checkout lock fields on an issue.
func (ir *IssueRoutes) ReleaseIssueHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var issue models.Issue
	if err := ir.db.WithContext(r.Context()).First(&issue, "id = ?", id).Error; err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	if err := ir.db.WithContext(r.Context()).Model(&issue).Updates(map[string]interface{}{
		"checkout_run_id":     nil,
		"execution_locked_at": nil,
	}).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(issue)
}

// ListIssueLabelsHandler returns all labels for a company.
func ListIssueLabelsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")
		var labels []models.Label
		if err := db.WithContext(r.Context()).Where("company_id = ?", companyID).Find(&labels).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(labels)
	}
}

// CreateLabelHandler creates a new label for a company.
func CreateLabelHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")
		var body struct {
			Name  string `json:"name"`
			Color string `json:"color"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		label := models.Label{
			CompanyID: companyID,
			Name:      body.Name,
			Color:     body.Color,
		}
		if err := db.WithContext(r.Context()).Create(&label).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(label)
	}
}

// DeleteLabelHandler deletes a label by ID.
func DeleteLabelHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		labelID := chi.URLParam(r, "labelId")
		if err := db.WithContext(r.Context()).Delete(&models.Label{}, "id = ?", labelID).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// actorUserID returns the user ID from the actor context, or "anonymous" as fallback.
func actorUserID(r *http.Request) string {
	actor := GetActorInfo(r)
	if actor.UserID != "" {
		return actor.UserID
	}
	return "anonymous"
}

// MarkIssueReadHandler upserts an issue_read_state for the current user.
func (ir *IssueRoutes) MarkIssueReadHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var issue models.Issue
	if err := ir.db.WithContext(r.Context()).First(&issue, "id = ?", id).Error; err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	userID := actorUserID(r)
	state := models.IssueReadState{
		CompanyID:  issue.CompanyID,
		IssueID:    id,
		UserID:     userID,
		LastReadAt: time.Now(),
	}
	if err := ir.db.WithContext(r.Context()).
		Where("company_id = ? AND issue_id = ? AND user_id = ?", issue.CompanyID, id, userID).
		Assign(models.IssueReadState{LastReadAt: state.LastReadAt}).
		FirstOrCreate(&state).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state)
}

// UnmarkIssueReadHandler deletes an issue_read_state for the current user.
func (ir *IssueRoutes) UnmarkIssueReadHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	userID := actorUserID(r)
	if err := ir.db.WithContext(r.Context()).
		Where("issue_id = ? AND user_id = ?", id, userID).
		Delete(&models.IssueReadState{}).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ArchiveIssueInboxHandler upserts an issue_inbox_archive for the current user.
func (ir *IssueRoutes) ArchiveIssueInboxHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var issue models.Issue
	if err := ir.db.WithContext(r.Context()).First(&issue, "id = ?", id).Error; err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	userID := actorUserID(r)
	archive := models.IssueInboxArchive{
		CompanyID:  issue.CompanyID,
		IssueID:    id,
		UserID:     userID,
		ArchivedAt: time.Now(),
	}
	if err := ir.db.WithContext(r.Context()).
		Where("company_id = ? AND issue_id = ? AND user_id = ?", issue.CompanyID, id, userID).
		FirstOrCreate(&archive).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(archive)
}

// UnarchiveIssueInboxHandler deletes an issue_inbox_archive for the current user.
func (ir *IssueRoutes) UnarchiveIssueInboxHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	userID := actorUserID(r)
	if err := ir.db.WithContext(r.Context()).
		Where("issue_id = ? AND user_id = ?", id, userID).
		Delete(&models.IssueInboxArchive{}).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListIssueApprovalsHandler returns all approvals linked to an issue.
func (ir *IssueRoutes) ListIssueApprovalsHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var approvals []models.IssueApproval
	if err := ir.db.WithContext(r.Context()).Preload("Approval").Where("issue_id = ?", id).Find(&approvals).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(approvals)
}

// LinkIssueApprovalHandler links an approval to an issue.
func (ir *IssueRoutes) LinkIssueApprovalHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var issue models.Issue
	if err := ir.db.WithContext(r.Context()).First(&issue, "id = ?", id).Error; err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	var body struct {
		ApprovalID string `json:"approvalId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	actor := GetActorInfo(r)
	link := models.IssueApproval{
		CompanyID:  issue.CompanyID,
		IssueID:    id,
		ApprovalID: body.ApprovalID,
	}
	if actor.UserID != "" {
		link.LinkedByUserID = &actor.UserID
	}
	if err := ir.db.WithContext(r.Context()).Create(&link).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(link)
}

// UnlinkIssueApprovalHandler removes an approval link from an issue.
func (ir *IssueRoutes) UnlinkIssueApprovalHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	approvalID := chi.URLParam(r, "approvalId")
	if err := ir.db.WithContext(r.Context()).
		Where("issue_id = ? AND approval_id = ?", id, approvalID).
		Delete(&models.IssueApproval{}).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListIssueAttachmentsHandler returns all attachments for an issue.
func (ir *IssueRoutes) ListIssueAttachmentsHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var attachments []models.IssueAttachment
	if err := ir.db.WithContext(r.Context()).Where("issue_id = ?", id).Find(&attachments).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	assetIDs := make([]string, 0, len(attachments))
	for _, attachment := range attachments {
		assetIDs = append(assetIDs, attachment.AssetID)
	}
	assetsByID := map[string]models.Asset{}
	if len(assetIDs) > 0 {
		var assets []models.Asset
		if err := ir.db.WithContext(r.Context()).Where("id IN ?", assetIDs).Find(&assets).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, asset := range assets {
			assetsByID[asset.ID] = asset
		}
	}
	responses := make([]issueAttachmentResponse, 0, len(attachments))
	for _, attachment := range attachments {
		asset, ok := assetsByID[attachment.AssetID]
		if !ok {
			continue
		}
		responses = append(responses, buildIssueAttachmentResponse(attachment, asset))
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(responses)
}

// DeleteAttachmentHandler deletes an issue attachment by ID.
func DeleteAttachmentHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		attachmentID := chi.URLParam(r, "attachmentId")
		result := db.WithContext(r.Context()).Delete(&models.IssueAttachment{}, "id = ?", attachmentID)
		if result.Error != nil {
			http.Error(w, result.Error.Error(), http.StatusInternalServerError)
			return
		}
		if result.RowsAffected == 0 {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}
}

// ListIssueFeedbackVotesHandler returns all feedback votes for an issue.
func (ir *IssueRoutes) ListIssueFeedbackVotesHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var issue models.Issue
	if err := ir.db.WithContext(r.Context()).First(&issue, "id = ?", id).Error; err != nil {
		http.Error(w, "Issue not found", http.StatusNotFound)
		return
	}
	if err := AssertCompanyAccess(r, issue.CompanyID); err != nil {
		http.Error(w, "Issue not found", http.StatusNotFound)
		return
	}
	if err := AssertBoard(r); err != nil {
		http.Error(w, "Only board users can view feedback votes", http.StatusForbidden)
		return
	}
	votes, err := ir.feedback.ListVotesForUser(r.Context(), id, actorUserID(r))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(votes)
}

// UpsertIssueFeedbackVoteHandler creates or updates a feedback vote for an issue.
func (ir *IssueRoutes) UpsertIssueFeedbackVoteHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := AssertBoard(r); err != nil {
		http.Error(w, "Only board users can vote on AI feedback", http.StatusForbidden)
		return
	}
	var body struct {
		TargetType   string  `json:"targetType"`
		TargetID     string  `json:"targetId"`
		Vote         string  `json:"vote"`
		Reason       *string `json:"reason"`
		AllowSharing bool    `json:"allowSharing"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	vote, err := ir.feedback.SaveVote(r.Context(), services.FeedbackVoteInput{
		IssueID:      id,
		TargetType:   body.TargetType,
		TargetID:     body.TargetID,
		Vote:         body.Vote,
		Reason:       body.Reason,
		AuthorUserID: actorUserID(r),
		AllowSharing: body.AllowSharing,
	})
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			http.Error(w, "Issue not found", http.StatusNotFound)
		case errors.Is(err, services.ErrFeedbackTargetNotFound):
			http.Error(w, "Feedback target not found", http.StatusNotFound)
		case errors.Is(err, services.ErrFeedbackUnsupportedTarget), errors.Is(err, services.ErrFeedbackVoteNotAllowed), errors.Is(err, services.ErrFeedbackInvalidVote):
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(vote)
}

// feedbackTraceRow holds the result of a feedback_exports + issues join.
type feedbackTraceRow struct {
	models.FeedbackExport
	IssueIdentifier *string `gorm:"column:issue_identifier"`
	IssueTitle      string  `gorm:"column:issue_title"`
}

// feedbackTraceResponse is the JSON response for a FeedbackTrace.
type feedbackTraceResponse struct {
	ID               string      `json:"id"`
	CompanyID        string      `json:"companyId"`
	FeedbackVoteID   string      `json:"feedbackVoteId"`
	IssueID          string      `json:"issueId"`
	ProjectID        *string     `json:"projectId"`
	IssueIdentifier  *string     `json:"issueIdentifier"`
	IssueTitle       string      `json:"issueTitle"`
	AuthorUserID     string      `json:"authorUserId"`
	TargetType       string      `json:"targetType"`
	TargetID         string      `json:"targetId"`
	Vote             string      `json:"vote"`
	Status           string      `json:"status"`
	Destination      *string     `json:"destination"`
	ExportID         *string     `json:"exportId"`
	ConsentVersion   *string     `json:"consentVersion"`
	SchemaVersion    string      `json:"schemaVersion"`
	BundleVersion    string      `json:"bundleVersion"`
	PayloadVersion   string      `json:"payloadVersion"`
	PayloadDigest    *string     `json:"payloadDigest"`
	PayloadSnapshot  interface{} `json:"payloadSnapshot"`
	TargetSummary    interface{} `json:"targetSummary"`
	RedactionSummary interface{} `json:"redactionSummary"`
	AttemptCount     int         `json:"attemptCount"`
	LastAttemptedAt  *time.Time  `json:"lastAttemptedAt"`
	ExportedAt       *time.Time  `json:"exportedAt"`
	FailureReason    *string     `json:"failureReason"`
	CreatedAt        time.Time   `json:"createdAt"`
	UpdatedAt        time.Time   `json:"updatedAt"`
}

func buildFeedbackTraceResponse(row feedbackTraceRow, includePayload bool) feedbackTraceResponse {
	var payload interface{}
	if includePayload {
		payload = row.PayloadSnapshot
	}
	var targetSummary interface{}
	if len(row.TargetSummary) > 0 {
		targetSummary = row.TargetSummary
	}
	var redactionSummary interface{}
	if len(row.RedactionSummary) > 0 {
		redactionSummary = row.RedactionSummary
	}
	return feedbackTraceResponse{
		ID:               row.ID,
		CompanyID:        row.CompanyID,
		FeedbackVoteID:   row.FeedbackVoteID,
		IssueID:          row.IssueID,
		ProjectID:        row.ProjectID,
		IssueIdentifier:  row.IssueIdentifier,
		IssueTitle:       row.IssueTitle,
		AuthorUserID:     row.AuthorUserID,
		TargetType:       row.TargetType,
		TargetID:         row.TargetID,
		Vote:             row.Vote,
		Status:           row.Status,
		Destination:      row.Destination,
		ExportID:         row.ExportID,
		ConsentVersion:   row.ConsentVersion,
		SchemaVersion:    row.SchemaVersion,
		BundleVersion:    row.BundleVersion,
		PayloadVersion:   row.PayloadVersion,
		PayloadDigest:    row.PayloadDigest,
		PayloadSnapshot:  payload,
		TargetSummary:    targetSummary,
		RedactionSummary: redactionSummary,
		AttemptCount:     row.AttemptCount,
		LastAttemptedAt:  row.LastAttemptedAt,
		ExportedAt:       row.ExportedAt,
		FailureReason:    row.FailureReason,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}
}

// queryFeedbackTraces runs a filtered JOIN query over feedback_exports + issues.
func queryFeedbackTraces(ctx context.Context, db *gorm.DB, filters map[string]interface{}, conditions []string, args []interface{}) ([]feedbackTraceRow, error) {
	query := db.WithContext(ctx).
		Table("feedback_exports fe").
		Select("fe.*, i.identifier AS issue_identifier, i.title AS issue_title").
		Joins("INNER JOIN issues i ON i.id = fe.issue_id")

	for col, val := range filters {
		query = query.Where("fe."+col+" = ?", val)
	}
	for i, cond := range conditions {
		query = query.Where(cond, args[i])
	}
	query = query.Order("fe.created_at DESC")

	var rows []feedbackTraceRow
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// ListIssueFeedbackTracesHandler returns feedback traces for an issue (board only).
func (ir *IssueRoutes) ListIssueFeedbackTracesHandler(w http.ResponseWriter, r *http.Request) {
	ListIssueFeedbackTracesHandler(ir.feedback).ServeHTTP(w, r)
}

// GetFeedbackTraceByIDHandler returns a single feedback trace (board only).
func (ir *IssueRoutes) GetFeedbackTraceByIDHandler(w http.ResponseWriter, r *http.Request) {
	GetFeedbackTraceHandler(ir.feedback).ServeHTTP(w, r)
}

// feedbackTraceBundleResponse is the simplified JSON response for a FeedbackTraceBundle.
type feedbackTraceBundleResponse struct {
	TraceID                string        `json:"traceId"`
	ExportID               *string       `json:"exportId"`
	CompanyID              string        `json:"companyId"`
	IssueID                string        `json:"issueId"`
	IssueIdentifier        *string       `json:"issueIdentifier"`
	AdapterType            interface{}   `json:"adapterType"`
	CaptureStatus          string        `json:"captureStatus"`
	Notes                  []string      `json:"notes"`
	Envelope               interface{}   `json:"envelope"`
	Surface                interface{}   `json:"surface"`
	PaperclipRun           interface{}   `json:"paperclipRun"`
	RawAdapterTrace        interface{}   `json:"rawAdapterTrace"`
	NormalizedAdapterTrace interface{}   `json:"normalizedAdapterTrace"`
	Privacy                interface{}   `json:"privacy"`
	Integrity              interface{}   `json:"integrity"`
	Files                  []interface{} `json:"files"`
}

// GetFeedbackTraceBundleHandler returns the bundle for a feedback trace (board only).
func (ir *IssueRoutes) GetFeedbackTraceBundleHandler(w http.ResponseWriter, r *http.Request) {
	GetFeedbackTraceBundleHandler(ir.feedback).ServeHTTP(w, r)
}

// ListIssueDocumentsHandler returns all documents linked to an issue.
func (ir *IssueRoutes) ListIssueDocumentsHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	docs, err := ir.documents.ListIssueDocuments(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(docs)
}

// GetIssueDocumentHandler returns a single document linked to an issue by key.
func (ir *IssueRoutes) GetIssueDocumentHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	key := chi.URLParam(r, "key")
	doc, err := ir.documents.GetIssueDocumentByKey(r.Context(), id, key)
	if err != nil {
		if errors.Is(err, services.ErrDocumentNotFound) || errors.Is(err, services.ErrInvalidDocumentKey) {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(doc)
}

// UpsertIssueDocumentHandler creates or updates a document linked to an issue.
func (ir *IssueRoutes) UpsertIssueDocumentHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	key := chi.URLParam(r, "key")

	var body struct {
		Title          *string `json:"title"`
		Body           string  `json:"body"`
		Format         string  `json:"format"`
		ChangeSummary  *string `json:"changeSummary"`
		BaseRevisionID *string `json:"baseRevisionId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.Format == "" {
		body.Format = "markdown"
	}

	actor := GetActorInfo(r)
	var createdByAgentID *string
	var createdByUserID *string
	if actor.AgentID != "" {
		createdByAgentID = &actor.AgentID
	}
	if actor.UserID != "" {
		createdByUserID = &actor.UserID
	}

	result, err := ir.documents.UpsertIssueDocument(r.Context(), services.UpsertIssueDocumentInput{
		IssueID:          id,
		Key:              key,
		Title:            body.Title,
		Format:           body.Format,
		Body:             body.Body,
		ChangeSummary:    body.ChangeSummary,
		BaseRevisionID:   body.BaseRevisionID,
		CreatedByAgentID: createdByAgentID,
		CreatedByUserID:  createdByUserID,
	})
	if err != nil {
		switch {
		case errors.Is(err, services.ErrIssueNotFound):
			http.Error(w, "Issue not found", http.StatusNotFound)
		case errors.Is(err, services.ErrInvalidDocumentKey):
			http.Error(w, "Invalid document key", http.StatusUnprocessableEntity)
		case errors.Is(err, services.ErrDocumentUpdateRequiresBaseRevision):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "Document update requires baseRevisionId"})
		case errors.Is(err, services.ErrDocumentConcurrentUpdate):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "Document was updated by someone else"})
		case errors.Is(err, services.ErrDocumentDoesNotExistYet):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "Document does not exist yet"})
		case errors.Is(err, services.ErrDocumentKeyAlreadyExists):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "Document key already exists on this issue"})
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if result.Created {
		w.WriteHeader(http.StatusCreated)
	}
	json.NewEncoder(w).Encode(result)
}

// DeleteIssueDocumentHandler removes a document link from an issue.
func (ir *IssueRoutes) DeleteIssueDocumentHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	key := chi.URLParam(r, "key")

	_, err := ir.documents.DeleteIssueDocument(r.Context(), id, key)
	if err != nil {
		if errors.Is(err, services.ErrInvalidDocumentKey) {
			http.Error(w, "Invalid document key", http.StatusUnprocessableEntity)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// UpdateWorkProductHandler applies partial updates to an IssueWorkProduct.
func UpdateWorkProductHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		wpID := chi.URLParam(r, "id")
		var wp models.IssueWorkProduct
		if err := db.WithContext(r.Context()).First(&wp, "id = ?", wpID).Error; err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		var updates map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := db.WithContext(r.Context()).Model(&wp).Updates(updates).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(wp)
	}
}

// DeleteWorkProductHandler deletes an IssueWorkProduct by ID.
func DeleteWorkProductHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		wpID := chi.URLParam(r, "id")
		if err := db.WithContext(r.Context()).Delete(&models.IssueWorkProduct{}, "id = ?", wpID).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// GetIssueCommentHandler returns a single comment by ID for an issue.
func (ir *IssueRoutes) GetIssueCommentHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	commentID := chi.URLParam(r, "commentId")
	var comment models.IssueComment
	if err := ir.db.WithContext(r.Context()).
		Where("id = ? AND issue_id = ?", commentID, id).
		First(&comment).Error; err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(comment)
}

// GetIssueHeartbeatContextHandler returns the heartbeat context for an issue.
func (ir *IssueRoutes) GetIssueHeartbeatContextHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var issue models.Issue
	if err := ir.db.WithContext(r.Context()).
		Preload("AssigneeAgent").Preload("Project").
		First(&issue, "id = ?", id).Error; err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"issue":         issue,
		"activeRunId":   issue.ExecutionRunID,
		"checkoutRunId": issue.CheckoutRunID,
	})
}

// ListIssueDocumentRevisionsHandler returns document revisions for a given issue document.
func (ir *IssueRoutes) ListIssueDocumentRevisionsHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	key := chi.URLParam(r, "key")

	revisions, err := ir.documents.ListIssueDocumentRevisions(r.Context(), id, key)
	if err != nil {
		if errors.Is(err, services.ErrDocumentNotFound) || errors.Is(err, services.ErrInvalidDocumentKey) {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(revisions)
}

// RestoreIssueDocumentRevisionHandler restores a document to a previous revision.
func (ir *IssueRoutes) RestoreIssueDocumentRevisionHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	key := chi.URLParam(r, "key")
	revisionID := chi.URLParam(r, "revisionId")

	actor := GetActorInfo(r)
	var createdByAgentID *string
	var createdByUserID *string
	if actor.AgentID != "" {
		createdByAgentID = &actor.AgentID
	}
	if actor.UserID != "" {
		createdByUserID = &actor.UserID
	}

	result, err := ir.documents.RestoreIssueDocumentRevision(r.Context(), id, key, revisionID, createdByAgentID, createdByUserID)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrDocumentNotFound):
			http.Error(w, "Document not found", http.StatusNotFound)
		case errors.Is(err, services.ErrDocumentRevisionNotFound):
			http.Error(w, "Document revision not found", http.StatusNotFound)
		case errors.Is(err, services.ErrInvalidDocumentKey):
			http.Error(w, "Invalid document key", http.StatusUnprocessableEntity)
		case errors.Is(err, services.ErrRevisionAlreadyLatest):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "Selected revision is already the latest revision"})
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// GetIssueDocumentPayloadHandler returns the document payload for an issue.
func (ir *IssueRoutes) GetIssueDocumentPayloadHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var issue models.Issue
	if err := ir.db.WithContext(r.Context()).Select("id", "description").First(&issue, "id = ?", id).Error; err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	payload, err := ir.documents.GetIssueDocumentPayload(r.Context(), id, issue.Description)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(payload)
}

// UploadIssueAttachmentHandler handles POST /companies/:companyId/issues/:issueId/attachments
func UploadIssueAttachmentHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")
		issueID := chi.URLParam(r, "issueId")
		var issue models.Issue
		if err := db.WithContext(r.Context()).First(&issue, "id = ?", issueID).Error; err != nil {
			if err != gorm.ErrRecordNotFound {
				http.Error(w, "Failed to load issue", http.StatusInternalServerError)
				return
			}
			http.Error(w, "Issue not found", http.StatusNotFound)
			return
		}
		if issue.CompanyID != companyID {
			http.Error(w, "Issue does not belong to company", http.StatusUnprocessableEntity)
			return
		}
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer file.Close()
		data, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		hash := fmt.Sprintf("%x", sha256hash(data))
		fname := header.Filename
		actor := GetActorInfo(r)
		var createdByUserID *string
		if actor.UserID != "" {
			createdByUserID = &actor.UserID
		}
		var createdByAgentID *string
		if actor.AgentID != "" {
			createdByAgentID = &actor.AgentID
		}
		asset := models.Asset{
			CompanyID:        companyID,
			Provider:         "local",
			ObjectKey:        companyID + "/" + hash + "/" + fname,
			ContentType:      header.Header.Get("Content-Type"),
			ByteSize:         len(data),
			Sha256:           hash,
			OriginalFilename: &fname,
			CreatedByAgentID: createdByAgentID,
			CreatedByUserID:  createdByUserID,
		}
		if err := db.WithContext(r.Context()).Create(&asset).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		attachment := models.IssueAttachment{
			CompanyID: companyID,
			IssueID:   issueID,
			AssetID:   asset.ID,
		}
		if err := db.WithContext(r.Context()).Create(&attachment).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(buildIssueAttachmentResponse(attachment, asset))
	}
}

// GetAttachmentContentHandler handles GET /attachments/:attachmentId/content
func GetAttachmentContentHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		attachmentID := chi.URLParam(r, "attachmentId")
		var attachment models.IssueAttachment
		if err := db.WithContext(r.Context()).First(&attachment, "id = ?", attachmentID).Error; err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		var asset models.Asset
		if err := db.WithContext(r.Context()).First(&asset, "id = ?", attachment.AssetID).Error; err != nil {
			http.Error(w, "Asset not found", http.StatusNotFound)
			return
		}
		contentType := asset.ContentType
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		filename := "attachment"
		if asset.OriginalFilename != nil && *asset.OriginalFilename != "" {
			filename = sanitizeAttachmentFilename(*asset.OriginalFilename)
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "private, max-age=60")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Disposition", buildAttachmentContentDisposition(attachmentDisposition(contentType), filename))
		w.WriteHeader(http.StatusOK)
	}
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
