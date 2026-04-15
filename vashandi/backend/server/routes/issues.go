package routes

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
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
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, name)
	return strings.ReplaceAll(name, `"`, "")
}

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
	var votes []models.FeedbackVote
	if err := ir.db.WithContext(r.Context()).Where("issue_id = ?", id).Find(&votes).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(votes)
}

// UpsertIssueFeedbackVoteHandler creates or updates a feedback vote for an issue.
func (ir *IssueRoutes) UpsertIssueFeedbackVoteHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var issue models.Issue
	if err := ir.db.WithContext(r.Context()).First(&issue, "id = ?", id).Error; err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	var body struct {
		TargetType string  `json:"targetType"`
		TargetID   string  `json:"targetId"`
		Vote       string  `json:"vote"`
		Reason     *string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	userID := actorUserID(r)
	vote := models.FeedbackVote{
		CompanyID:    issue.CompanyID,
		IssueID:      id,
		TargetType:   body.TargetType,
		TargetID:     body.TargetID,
		AuthorUserID: userID,
		Vote:         body.Vote,
		Reason:       body.Reason,
	}
	if err := ir.db.WithContext(r.Context()).
		Where("company_id = ? AND issue_id = ? AND target_type = ? AND target_id = ? AND author_user_id = ?",
			issue.CompanyID, id, body.TargetType, body.TargetID, userID).
		Assign(models.FeedbackVote{Vote: body.Vote, Reason: body.Reason}).
		FirstOrCreate(&vote).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(vote)
}

// ListIssueDocumentsHandler returns all documents linked to an issue.
func (ir *IssueRoutes) ListIssueDocumentsHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var docs []models.IssueDocument
	if err := ir.db.WithContext(r.Context()).Where("issue_id = ?", id).Order("updated_at DESC").Find(&docs).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Enrich with document data where available.
	type docEntry struct {
		models.IssueDocument
		Document *models.Document `json:"document,omitempty"`
	}
	result := make([]docEntry, 0, len(docs))
	for _, d := range docs {
		entry := docEntry{IssueDocument: d}
		var document models.Document
		if err := ir.db.WithContext(r.Context()).First(&document, "id = ?", d.DocumentID).Error; err == nil {
			entry.Document = &document
		}
		result = append(result, entry)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// GetIssueDocumentHandler returns a single document linked to an issue by key.
func (ir *IssueRoutes) GetIssueDocumentHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	key := chi.URLParam(r, "key")
	var issueDoc models.IssueDocument
	if err := ir.db.WithContext(r.Context()).
		Where("issue_id = ? AND key = ?", id, key).
		First(&issueDoc).Error; err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	var document models.Document
	if err := ir.db.WithContext(r.Context()).First(&document, "id = ?", issueDoc.DocumentID).Error; err != nil {
		http.Error(w, "Document not found", http.StatusNotFound)
		return
	}
	type response struct {
		models.IssueDocument
		Document models.Document `json:"document"`
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response{IssueDocument: issueDoc, Document: document})
}

// UpsertIssueDocumentHandler creates or updates a document linked to an issue.
func (ir *IssueRoutes) UpsertIssueDocumentHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	key := chi.URLParam(r, "key")
	var issue models.Issue
	if err := ir.db.WithContext(r.Context()).First(&issue, "id = ?", id).Error; err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	var body struct {
		Title  *string `json:"title"`
		Body   string  `json:"body"`
		Format string  `json:"format"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.Format == "" {
		body.Format = "markdown"
	}

	// Look up existing link.
	var issueDoc models.IssueDocument
	isNew := false
	if err := ir.db.WithContext(r.Context()).
		Where("issue_id = ? AND key = ?", id, key).
		First(&issueDoc).Error; err != nil {
		isNew = true
	}

	actor := GetActorInfo(r)
	if isNew {
		doc := models.Document{
			CompanyID:  issue.CompanyID,
			Title:      body.Title,
			Format:     body.Format,
			LatestBody: body.Body,
		}
		if actor.UserID != "" {
			doc.CreatedByUserID = &actor.UserID
			doc.UpdatedByUserID = &actor.UserID
		}
		if err := ir.db.WithContext(r.Context()).Create(&doc).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		issueDoc = models.IssueDocument{
			CompanyID:  issue.CompanyID,
			IssueID:    id,
			DocumentID: doc.ID,
			Key:        key,
		}
		if err := ir.db.WithContext(r.Context()).Create(&issueDoc).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(issueDoc)
		return
	}

	// Update existing document.
	updates := map[string]interface{}{
		"latest_body": body.Body,
		"format":      body.Format,
		"title":       body.Title,
	}
	if actor.UserID != "" {
		updates["updated_by_user_id"] = actor.UserID
	}
	if err := ir.db.WithContext(r.Context()).Model(&models.Document{}).
		Where("id = ?", issueDoc.DocumentID).Updates(updates).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(issueDoc)
}

// DeleteIssueDocumentHandler removes a document link from an issue.
func (ir *IssueRoutes) DeleteIssueDocumentHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	key := chi.URLParam(r, "key")
	if err := ir.db.WithContext(r.Context()).
		Where("issue_id = ? AND key = ?", id, key).
		Delete(&models.IssueDocument{}).Error; err != nil {
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
	var issueDoc models.IssueDocument
	if err := ir.db.WithContext(r.Context()).
		First(&issueDoc, "issue_id = ? AND key = ?", id, key).Error; err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	var revisions []models.DocumentRevision
	ir.db.WithContext(r.Context()).
		Where("document_id = ?", issueDoc.DocumentID).
		Order("revision_number ASC").
		Find(&revisions)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(revisions)
}

// UploadIssueAttachmentHandler handles POST /companies/:companyId/issues/:issueId/attachments
func UploadIssueAttachmentHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")
		issueID := chi.URLParam(r, "issueId")
		var issue models.Issue
		if err := db.WithContext(r.Context()).First(&issue, "id = ?", issueID).Error; err != nil {
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
		w.Header().Set("Content-Length", fmt.Sprintf("%d", asset.ByteSize))
		w.Header().Set("Cache-Control", "private, max-age=60")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`%s; filename="%s"`, attachmentDisposition(contentType), filename))
		w.WriteHeader(http.StatusOK)
	}
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
