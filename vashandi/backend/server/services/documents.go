package services

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"gorm.io/gorm"
)

var (
	ErrDocumentNotFound        = errors.New("document not found")
	ErrDocumentRevisionNotFound = errors.New("document revision not found")
	ErrDocumentUpdateRequiresBaseRevision = errors.New("document update requires baseRevisionId")
	ErrDocumentConcurrentUpdate = errors.New("document was updated by someone else")
	ErrDocumentDoesNotExistYet = errors.New("document does not exist yet")
	ErrDocumentKeyAlreadyExists = errors.New("document key already exists on this issue")
	ErrInvalidDocumentKey      = errors.New("invalid document key")
	ErrIssueNotFound           = errors.New("issue not found")
	ErrRevisionAlreadyLatest   = errors.New("selected revision is already the latest revision")
)

// issueDocumentKeyPattern validates document keys: lowercase letters, numbers, underscore or hyphen
var issueDocumentKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// DocumentService manages issue documents and revisions
type DocumentService struct {
	db *gorm.DB
}

// NewDocumentService creates a new DocumentService
func NewDocumentService(db *gorm.DB) *DocumentService {
	return &DocumentService{db: db}
}

// IssueDocumentResult represents a document joined with its issue link
type IssueDocumentResult struct {
	ID                   string     `json:"id"`
	CompanyID            string     `json:"companyId"`
	IssueID              string     `json:"issueId"`
	Key                  string     `json:"key"`
	Title                *string    `json:"title"`
	Format               string     `json:"format"`
	Body                 *string    `json:"body,omitempty"`
	LatestRevisionID     *string    `json:"latestRevisionId"`
	LatestRevisionNumber int        `json:"latestRevisionNumber"`
	CreatedByAgentID     *string    `json:"createdByAgentId"`
	CreatedByUserID      *string    `json:"createdByUserId"`
	UpdatedByAgentID     *string    `json:"updatedByAgentId"`
	UpdatedByUserID      *string    `json:"updatedByUserId"`
	CreatedAt            time.Time  `json:"createdAt"`
	UpdatedAt            time.Time  `json:"updatedAt"`
}

// DocumentRevisionResult represents a document revision
type DocumentRevisionResult struct {
	ID               string    `json:"id"`
	CompanyID        string    `json:"companyId"`
	DocumentID       string    `json:"documentId"`
	IssueID          string    `json:"issueId"`
	Key              string    `json:"key"`
	RevisionNumber   int       `json:"revisionNumber"`
	Title            *string   `json:"title"`
	Format           string    `json:"format"`
	Body             string    `json:"body"`
	ChangeSummary    *string   `json:"changeSummary"`
	CreatedByAgentID *string   `json:"createdByAgentId"`
	CreatedByUserID  *string   `json:"createdByUserId"`
	CreatedAt        time.Time `json:"createdAt"`
}

// LegacyPlanDocument represents a plan extracted from issue description
type LegacyPlanDocument struct {
	Key    string `json:"key"`
	Body   string `json:"body"`
	Source string `json:"source"`
}

// IssueDocumentPayload is the response for getIssueDocumentPayload
type IssueDocumentPayload struct {
	PlanDocument       *IssueDocumentResult `json:"planDocument"`
	DocumentSummaries  []IssueDocumentResult `json:"documentSummaries"`
	LegacyPlanDocument *LegacyPlanDocument   `json:"legacyPlanDocument"`
}

// UpsertIssueDocumentInput represents input for creating/updating a document
type UpsertIssueDocumentInput struct {
	IssueID          string
	Key              string
	Title            *string
	Format           string
	Body             string
	ChangeSummary    *string
	BaseRevisionID   *string
	CreatedByAgentID *string
	CreatedByUserID  *string
	CreatedByRunID   *string
}

// UpsertIssueDocumentResult is the response for upsertIssueDocument
type UpsertIssueDocumentResult struct {
	Created  bool               `json:"created"`
	Document IssueDocumentResult `json:"document"`
}

// RestoreIssueDocumentResult is the response for restoreIssueDocumentRevision
type RestoreIssueDocumentResult struct {
	RestoredFromRevisionID     string             `json:"restoredFromRevisionId"`
	RestoredFromRevisionNumber int                `json:"restoredFromRevisionNumber"`
	Document                   IssueDocumentResult `json:"document"`
}

// normalizeDocumentKey validates and normalizes a document key
func normalizeDocumentKey(key string) (string, error) {
	normalized := strings.TrimSpace(strings.ToLower(key))
	if normalized == "" || len(normalized) > 64 {
		return "", ErrInvalidDocumentKey
	}
	if !issueDocumentKeyPattern.MatchString(normalized) {
		return "", ErrInvalidDocumentKey
	}
	return normalized, nil
}

// ExtractLegacyPlanBody extracts plan content from a <plan></plan> block in an issue description
func ExtractLegacyPlanBody(description *string) *string {
	if description == nil || *description == "" {
		return nil
	}
	// Case-insensitive regex to match <plan>...</plan>
	re := regexp.MustCompile(`(?is)<plan>\s*([\s\S]*?)\s*</plan>`)
	match := re.FindStringSubmatch(*description)
	if len(match) < 2 {
		return nil
	}
	body := strings.TrimSpace(match[1])
	if body == "" {
		return nil
	}
	return &body
}

// mapIssueDocumentRow maps database rows to IssueDocumentResult
func mapIssueDocumentRow(
	doc models.Document,
	issueDoc models.IssueDocument,
	includeBody bool,
) IssueDocumentResult {
	result := IssueDocumentResult{
		ID:                   doc.ID,
		CompanyID:            doc.CompanyID,
		IssueID:              issueDoc.IssueID,
		Key:                  issueDoc.Key,
		Title:                doc.Title,
		Format:               doc.Format,
		LatestRevisionID:     doc.LatestRevisionID,
		LatestRevisionNumber: doc.LatestRevisionNumber,
		CreatedByAgentID:     doc.CreatedByAgentID,
		CreatedByUserID:      doc.CreatedByUserID,
		UpdatedByAgentID:     doc.UpdatedByAgentID,
		UpdatedByUserID:      doc.UpdatedByUserID,
		CreatedAt:            doc.CreatedAt,
		UpdatedAt:            doc.UpdatedAt,
	}
	if includeBody {
		result.Body = &doc.LatestBody
	}
	return result
}

// GetIssueDocumentPayload returns all document info for an issue including legacy plan extraction
func (s *DocumentService) GetIssueDocumentPayload(ctx context.Context, issueID string, issueDescription *string) (*IssueDocumentPayload, error) {
	// Get plan document
	var planDoc *IssueDocumentResult
	var planIssueDoc models.IssueDocument
	if err := s.db.WithContext(ctx).
		Where("issue_id = ? AND key = ?", issueID, "plan").
		First(&planIssueDoc).Error; err == nil {
		var doc models.Document
		if err := s.db.WithContext(ctx).First(&doc, "id = ?", planIssueDoc.DocumentID).Error; err == nil {
			mapped := mapIssueDocumentRow(doc, planIssueDoc, true)
			planDoc = &mapped
		}
	}

	// Get all document summaries (without body)
	var issueDocs []models.IssueDocument
	s.db.WithContext(ctx).
		Where("issue_id = ?", issueID).
		Order("key ASC, updated_at DESC").
		Find(&issueDocs)

	summaries := make([]IssueDocumentResult, 0, len(issueDocs))
	for _, issueDoc := range issueDocs {
		var doc models.Document
		if err := s.db.WithContext(ctx).First(&doc, "id = ?", issueDoc.DocumentID).Error; err == nil {
			summaries = append(summaries, mapIssueDocumentRow(doc, issueDoc, false))
		}
	}

	// Check for legacy plan in description only if no plan document exists
	var legacyPlan *LegacyPlanDocument
	if planDoc == nil {
		if legacyBody := ExtractLegacyPlanBody(issueDescription); legacyBody != nil {
			legacyPlan = &LegacyPlanDocument{
				Key:    "plan",
				Body:   *legacyBody,
				Source: "issue_description",
			}
		}
	}

	return &IssueDocumentPayload{
		PlanDocument:       planDoc,
		DocumentSummaries:  summaries,
		LegacyPlanDocument: legacyPlan,
	}, nil
}

// ListIssueDocuments returns all documents for an issue with their bodies
func (s *DocumentService) ListIssueDocuments(ctx context.Context, issueID string) ([]IssueDocumentResult, error) {
	var issueDocs []models.IssueDocument
	if err := s.db.WithContext(ctx).
		Where("issue_id = ?", issueID).
		Order("key ASC, updated_at DESC").
		Find(&issueDocs).Error; err != nil {
		return nil, err
	}

	results := make([]IssueDocumentResult, 0, len(issueDocs))
	for _, issueDoc := range issueDocs {
		var doc models.Document
		if err := s.db.WithContext(ctx).First(&doc, "id = ?", issueDoc.DocumentID).Error; err == nil {
			results = append(results, mapIssueDocumentRow(doc, issueDoc, true))
		}
	}
	return results, nil
}

// GetIssueDocumentByKey returns a single document by issue ID and key
func (s *DocumentService) GetIssueDocumentByKey(ctx context.Context, issueID string, rawKey string) (*IssueDocumentResult, error) {
	key, err := normalizeDocumentKey(rawKey)
	if err != nil {
		return nil, err
	}

	var issueDoc models.IssueDocument
	if err := s.db.WithContext(ctx).
		Where("issue_id = ? AND key = ?", issueID, key).
		First(&issueDoc).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDocumentNotFound
		}
		return nil, err
	}

	var doc models.Document
	if err := s.db.WithContext(ctx).First(&doc, "id = ?", issueDoc.DocumentID).Error; err != nil {
		return nil, ErrDocumentNotFound
	}

	result := mapIssueDocumentRow(doc, issueDoc, true)
	return &result, nil
}

// ListIssueDocumentRevisions returns all revisions for a document
func (s *DocumentService) ListIssueDocumentRevisions(ctx context.Context, issueID string, rawKey string) ([]DocumentRevisionResult, error) {
	key, err := normalizeDocumentKey(rawKey)
	if err != nil {
		return nil, err
	}

	var issueDoc models.IssueDocument
	if err := s.db.WithContext(ctx).
		Where("issue_id = ? AND key = ?", issueID, key).
		First(&issueDoc).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDocumentNotFound
		}
		return nil, err
	}

	var revisions []models.DocumentRevision
	if err := s.db.WithContext(ctx).
		Where("document_id = ?", issueDoc.DocumentID).
		Order("revision_number DESC").
		Find(&revisions).Error; err != nil {
		return nil, err
	}

	results := make([]DocumentRevisionResult, 0, len(revisions))
	for _, rev := range revisions {
		results = append(results, DocumentRevisionResult{
			ID:               rev.ID,
			CompanyID:        rev.CompanyID,
			DocumentID:       rev.DocumentID,
			IssueID:          issueDoc.IssueID,
			Key:              issueDoc.Key,
			RevisionNumber:   rev.RevisionNumber,
			Title:            rev.Title,
			Format:           rev.Format,
			Body:             rev.Body,
			ChangeSummary:    rev.ChangeSummary,
			CreatedByAgentID: rev.CreatedByAgentID,
			CreatedByUserID:  rev.CreatedByUserID,
			CreatedAt:        rev.CreatedAt,
		})
	}
	return results, nil
}

// UpsertIssueDocument creates or updates a document with revision tracking
func (s *DocumentService) UpsertIssueDocument(ctx context.Context, input UpsertIssueDocumentInput) (*UpsertIssueDocumentResult, error) {
	key, err := normalizeDocumentKey(input.Key)
	if err != nil {
		return nil, err
	}

	// Get the issue to get company ID
	var issue models.Issue
	if err := s.db.WithContext(ctx).
		Select("id", "company_id").
		First(&issue, "id = ?", input.IssueID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrIssueNotFound
		}
		return nil, err
	}

	var result *UpsertIssueDocumentResult

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()

		// Check for existing document link
		var existingIssueDoc models.IssueDocument
		var existingDoc models.Document
		documentExists := false

		if err := tx.Where("issue_id = ? AND key = ?", issue.ID, key).First(&existingIssueDoc).Error; err == nil {
			if err := tx.First(&existingDoc, "id = ?", existingIssueDoc.DocumentID).Error; err == nil {
				documentExists = true
			}
		}

		if documentExists {
			// Update existing document
			if input.BaseRevisionID == nil || *input.BaseRevisionID == "" {
				return ErrDocumentUpdateRequiresBaseRevision
			}
			if existingDoc.LatestRevisionID == nil || *input.BaseRevisionID != *existingDoc.LatestRevisionID {
				return ErrDocumentConcurrentUpdate
			}

			nextRevisionNumber := existingDoc.LatestRevisionNumber + 1

			// Create new revision
			revision := models.DocumentRevision{
				CompanyID:        issue.CompanyID,
				DocumentID:       existingDoc.ID,
				RevisionNumber:   nextRevisionNumber,
				Title:            input.Title,
				Format:           input.Format,
				Body:             input.Body,
				ChangeSummary:    input.ChangeSummary,
				CreatedByAgentID: input.CreatedByAgentID,
				CreatedByUserID:  input.CreatedByUserID,
				CreatedByRunID:   input.CreatedByRunID,
				CreatedAt:        now,
			}
			if err := tx.Create(&revision).Error; err != nil {
				return err
			}

			// Update document
			updates := map[string]interface{}{
				"title":                  input.Title,
				"format":                 input.Format,
				"latest_body":            input.Body,
				"latest_revision_id":     revision.ID,
				"latest_revision_number": nextRevisionNumber,
				"updated_by_agent_id":    input.CreatedByAgentID,
				"updated_by_user_id":     input.CreatedByUserID,
				"updated_at":             now,
			}
			if err := tx.Model(&models.Document{}).Where("id = ?", existingDoc.ID).Updates(updates).Error; err != nil {
				return err
			}

			// Update issue document link
			tx.Model(&models.IssueDocument{}).Where("document_id = ?", existingDoc.ID).Update("updated_at", now)

			body := input.Body
			result = &UpsertIssueDocumentResult{
				Created: false,
				Document: IssueDocumentResult{
					ID:                   existingDoc.ID,
					CompanyID:            existingDoc.CompanyID,
					IssueID:              existingIssueDoc.IssueID,
					Key:                  existingIssueDoc.Key,
					Title:                input.Title,
					Format:               input.Format,
					Body:                 &body,
					LatestRevisionID:     &revision.ID,
					LatestRevisionNumber: nextRevisionNumber,
					CreatedByAgentID:     existingDoc.CreatedByAgentID,
					CreatedByUserID:      existingDoc.CreatedByUserID,
					UpdatedByAgentID:     input.CreatedByAgentID,
					UpdatedByUserID:      input.CreatedByUserID,
					CreatedAt:            existingDoc.CreatedAt,
					UpdatedAt:            now,
				},
			}
			return nil
		}

		// Document doesn't exist yet
		if input.BaseRevisionID != nil && *input.BaseRevisionID != "" {
			return ErrDocumentDoesNotExistYet
		}

		// Create new document
		doc := models.Document{
			CompanyID:            issue.CompanyID,
			Title:                input.Title,
			Format:               input.Format,
			LatestBody:           input.Body,
			LatestRevisionNumber: 1,
			CreatedByAgentID:     input.CreatedByAgentID,
			CreatedByUserID:      input.CreatedByUserID,
			UpdatedByAgentID:     input.CreatedByAgentID,
			UpdatedByUserID:      input.CreatedByUserID,
			CreatedAt:            now,
			UpdatedAt:            now,
		}
		if err := tx.Create(&doc).Error; err != nil {
			return err
		}

		// Create first revision
		revision := models.DocumentRevision{
			CompanyID:        issue.CompanyID,
			DocumentID:       doc.ID,
			RevisionNumber:   1,
			Title:            input.Title,
			Format:           input.Format,
			Body:             input.Body,
			ChangeSummary:    input.ChangeSummary,
			CreatedByAgentID: input.CreatedByAgentID,
			CreatedByUserID:  input.CreatedByUserID,
			CreatedByRunID:   input.CreatedByRunID,
			CreatedAt:        now,
		}
		if err := tx.Create(&revision).Error; err != nil {
			return err
		}

		// Update document with revision ID
		if err := tx.Model(&doc).Update("latest_revision_id", revision.ID).Error; err != nil {
			return err
		}

		// Create issue document link
		issueDoc := models.IssueDocument{
			CompanyID:  issue.CompanyID,
			IssueID:    issue.ID,
			DocumentID: doc.ID,
			Key:        key,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		if err := tx.Create(&issueDoc).Error; err != nil {
			// Check for unique violation (key already exists)
			if strings.Contains(err.Error(), "23505") || strings.Contains(err.Error(), "duplicate key") {
				return ErrDocumentKeyAlreadyExists
			}
			return err
		}

		body := input.Body
		result = &UpsertIssueDocumentResult{
			Created: true,
			Document: IssueDocumentResult{
				ID:                   doc.ID,
				CompanyID:            issue.CompanyID,
				IssueID:              issue.ID,
				Key:                  key,
				Title:                doc.Title,
				Format:               doc.Format,
				Body:                 &body,
				LatestRevisionID:     &revision.ID,
				LatestRevisionNumber: 1,
				CreatedByAgentID:     doc.CreatedByAgentID,
				CreatedByUserID:      doc.CreatedByUserID,
				UpdatedByAgentID:     doc.UpdatedByAgentID,
				UpdatedByUserID:      doc.UpdatedByUserID,
				CreatedAt:            doc.CreatedAt,
				UpdatedAt:            doc.UpdatedAt,
			},
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	return result, nil
}

// RestoreIssueDocumentRevision restores a document to a previous revision
func (s *DocumentService) RestoreIssueDocumentRevision(ctx context.Context, issueID, rawKey, revisionID string, createdByAgentID, createdByUserID *string) (*RestoreIssueDocumentResult, error) {
	key, err := normalizeDocumentKey(rawKey)
	if err != nil {
		return nil, err
	}

	var result *RestoreIssueDocumentResult

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Get existing document
		var issueDoc models.IssueDocument
		if err := tx.Where("issue_id = ? AND key = ?", issueID, key).First(&issueDoc).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrDocumentNotFound
			}
			return err
		}

		var doc models.Document
		if err := tx.First(&doc, "id = ?", issueDoc.DocumentID).Error; err != nil {
			return ErrDocumentNotFound
		}

		// Get the revision to restore
		var revision models.DocumentRevision
		if err := tx.Where("id = ? AND document_id = ?", revisionID, doc.ID).First(&revision).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrDocumentRevisionNotFound
			}
			return err
		}

		// Check if already latest
		if doc.LatestRevisionID != nil && *doc.LatestRevisionID == revision.ID {
			return ErrRevisionAlreadyLatest
		}

		now := time.Now()
		nextRevisionNumber := doc.LatestRevisionNumber + 1

		// Create new revision (copy of the old one)
		changeSummary := fmt.Sprintf("Restored from revision %d", revision.RevisionNumber)
		
		restoredRevision := models.DocumentRevision{
			CompanyID:        doc.CompanyID,
			DocumentID:       doc.ID,
			RevisionNumber:   nextRevisionNumber,
			Title:            revision.Title,
			Format:           revision.Format,
			Body:             revision.Body,
			ChangeSummary:    &changeSummary,
			CreatedByAgentID: createdByAgentID,
			CreatedByUserID:  createdByUserID,
			CreatedAt:        now,
		}
		if err := tx.Create(&restoredRevision).Error; err != nil {
			return err
		}

		// Update document
		updates := map[string]interface{}{
			"title":                  revision.Title,
			"format":                 revision.Format,
			"latest_body":            revision.Body,
			"latest_revision_id":     restoredRevision.ID,
			"latest_revision_number": nextRevisionNumber,
			"updated_by_agent_id":    createdByAgentID,
			"updated_by_user_id":     createdByUserID,
			"updated_at":             now,
		}
		if err := tx.Model(&models.Document{}).Where("id = ?", doc.ID).Updates(updates).Error; err != nil {
			return err
		}

		// Update issue document link
		tx.Model(&models.IssueDocument{}).Where("document_id = ?", doc.ID).Update("updated_at", now)

		body := revision.Body
		result = &RestoreIssueDocumentResult{
			RestoredFromRevisionID:     revision.ID,
			RestoredFromRevisionNumber: revision.RevisionNumber,
			Document: IssueDocumentResult{
				ID:                   doc.ID,
				CompanyID:            doc.CompanyID,
				IssueID:              issueDoc.IssueID,
				Key:                  issueDoc.Key,
				Title:                revision.Title,
				Format:               revision.Format,
				Body:                 &body,
				LatestRevisionID:     &restoredRevision.ID,
				LatestRevisionNumber: nextRevisionNumber,
				CreatedByAgentID:     doc.CreatedByAgentID,
				CreatedByUserID:      doc.CreatedByUserID,
				UpdatedByAgentID:     createdByAgentID,
				UpdatedByUserID:      createdByUserID,
				CreatedAt:            doc.CreatedAt,
				UpdatedAt:            now,
			},
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	return result, nil
}

// DeleteIssueDocument removes a document link and the document itself
func (s *DocumentService) DeleteIssueDocument(ctx context.Context, issueID, rawKey string) (*IssueDocumentResult, error) {
	key, err := normalizeDocumentKey(rawKey)
	if err != nil {
		return nil, err
	}

	var result *IssueDocumentResult

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var issueDoc models.IssueDocument
		if err := tx.Where("issue_id = ? AND key = ?", issueID, key).First(&issueDoc).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil // Not found, nothing to delete
			}
			return err
		}

		var doc models.Document
		if err := tx.First(&doc, "id = ?", issueDoc.DocumentID).Error; err != nil {
			return nil // Document not found, just delete the link
		}

		// Delete the issue document link
		if err := tx.Delete(&issueDoc).Error; err != nil {
			return err
		}

		// Delete the document (this will cascade delete revisions)
		if err := tx.Delete(&doc).Error; err != nil {
			return err
		}

		body := doc.LatestBody
		result = &IssueDocumentResult{
			ID:                   doc.ID,
			CompanyID:            doc.CompanyID,
			IssueID:              issueDoc.IssueID,
			Key:                  issueDoc.Key,
			Title:                doc.Title,
			Format:               doc.Format,
			Body:                 &body,
			LatestRevisionID:     doc.LatestRevisionID,
			LatestRevisionNumber: doc.LatestRevisionNumber,
			CreatedByAgentID:     doc.CreatedByAgentID,
			CreatedByUserID:      doc.CreatedByUserID,
			UpdatedByAgentID:     doc.UpdatedByAgentID,
			UpdatedByUserID:      doc.UpdatedByUserID,
			CreatedAt:            doc.CreatedAt,
			UpdatedAt:            doc.UpdatedAt,
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	return result, nil
}
