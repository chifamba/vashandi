package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	feedbackSchemaVersion              = "paperclip-feedback-envelope-v2"
	feedbackBundleVersion              = "paperclip-feedback-bundle-v2"
	feedbackPayloadVersion             = "paperclip-feedback-v1"
	feedbackDestination                = "paperclip_labs_feedback_v1"
	defaultFeedbackSharingTermsVersion = "feedback-data-sharing-v1"
	feedbackMaxExcerptChars            = 200
	feedbackMaxTextChars               = 8000
)

var (
	ErrFeedbackUnsupportedTarget = errors.New("unsupported feedback target type")
	ErrFeedbackTargetNotFound    = errors.New("feedback target not found")
	ErrFeedbackVoteNotAllowed    = errors.New("feedback vote not allowed for target")
	ErrFeedbackInvalidVote       = errors.New("invalid feedback vote")
)

type FeedbackVote = models.FeedbackVote

type FeedbackTraceFilters struct {
	CompanyID      string
	IssueID        string
	ProjectID      string
	TargetType     string
	Vote           string
	Status         string
	From           *time.Time
	To             *time.Time
	SharedOnly     bool
	IncludePayload bool
}

type FeedbackVoteInput struct {
	IssueID      string
	TargetType   string
	TargetID     string
	Vote         string
	Reason       *string
	AuthorUserID string
	AllowSharing bool
}

type FeedbackTraceTargetSummary struct {
	Label          string     `json:"label"`
	Excerpt        *string    `json:"excerpt"`
	AuthorAgentID  *string    `json:"authorAgentId"`
	AuthorUserID   *string    `json:"authorUserId"`
	CreatedAt      *time.Time `json:"createdAt"`
	DocumentKey    *string    `json:"documentKey"`
	DocumentTitle  *string    `json:"documentTitle"`
	RevisionNumber *int       `json:"revisionNumber"`
}

type FeedbackTrace struct {
	ID               string                     `json:"id"`
	CompanyID        string                     `json:"companyId"`
	FeedbackVoteID   string                     `json:"feedbackVoteId"`
	IssueID          string                     `json:"issueId"`
	ProjectID        *string                    `json:"projectId"`
	IssueIdentifier  *string                    `json:"issueIdentifier"`
	IssueTitle       string                     `json:"issueTitle"`
	AuthorUserID     string                     `json:"authorUserId"`
	TargetType       string                     `json:"targetType"`
	TargetID         string                     `json:"targetId"`
	Vote             string                     `json:"vote"`
	Status           string                     `json:"status"`
	Destination      *string                    `json:"destination"`
	ExportID         *string                    `json:"exportId"`
	ConsentVersion   *string                    `json:"consentVersion"`
	SchemaVersion    string                     `json:"schemaVersion"`
	BundleVersion    string                     `json:"bundleVersion"`
	PayloadVersion   string                     `json:"payloadVersion"`
	PayloadDigest    *string                    `json:"payloadDigest"`
	PayloadSnapshot  interface{}                `json:"payloadSnapshot"`
	TargetSummary    FeedbackTraceTargetSummary `json:"targetSummary"`
	RedactionSummary interface{}                `json:"redactionSummary"`
	AttemptCount     int                        `json:"attemptCount"`
	LastAttemptedAt  *time.Time                 `json:"lastAttemptedAt"`
	ExportedAt       *time.Time                 `json:"exportedAt"`
	FailureReason    *string                    `json:"failureReason"`
	CreatedAt        time.Time                  `json:"createdAt"`
	UpdatedAt        time.Time                  `json:"updatedAt"`
}

type FeedbackTraceBundleFile struct {
	Path        string `json:"path"`
	ContentType string `json:"contentType"`
	Encoding    string `json:"encoding"`
	ByteLength  int    `json:"byteLength"`
	SHA256      string `json:"sha256"`
	Source      string `json:"source"`
	Contents    string `json:"contents"`
}

type FeedbackService struct {
	db *gorm.DB
}

type feedbackTraceRow struct {
	models.FeedbackExport
	IssueIdentifier *string `gorm:"column:issue_identifier"`
	IssueTitle      string  `gorm:"column:issue_title"`
}

type feedbackTargetRecord struct {
	Label          string
	Body           string
	CreatedAt      time.Time
	AuthorAgentID  *string
	AuthorUserID   *string
	CreatedByRunID *string
	DocumentKey    *string
	DocumentTitle  *string
	RevisionNumber *int
	PayloadTarget  map[string]interface{}
}

func NewFeedbackService(db *gorm.DB) *FeedbackService {
	return &FeedbackService{db: db}
}

func (s *FeedbackService) ListTraces(ctx context.Context, filters FeedbackTraceFilters) ([]FeedbackTrace, error) {
	resolved, err := s.resolveFilters(ctx, filters)
	if err != nil {
		return nil, err
	}
	rows, err := s.queryTraceRows(ctx, resolved)
	if err != nil {
		return nil, err
	}
	traces := make([]FeedbackTrace, 0, len(rows))
	for _, row := range rows {
		traces = append(traces, mapFeedbackTraceRow(row, resolved.IncludePayload))
	}
	return traces, nil
}

func (s *FeedbackService) GetTraceByID(ctx context.Context, traceID string, includePayload bool) (*FeedbackTrace, error) {
	row, err := s.getTraceRowByID(ctx, traceID)
	if err != nil {
		return nil, err
	}
	trace := mapFeedbackTraceRow(*row, includePayload)
	return &trace, nil
}

func (s *FeedbackService) GetTraceBundle(ctx context.Context, traceID string) (*FeedbackTraceBundle, error) {
	row, err := s.getTraceRowByID(ctx, traceID)
	if err != nil {
		return nil, err
	}
	trace := mapFeedbackTraceRow(*row, true)
	payload := asRecord(trace.PayloadSnapshot)
	state := NewFeedbackRedactionState()
	files := []FeedbackTraceBundleFile{}
	notes := []string{}

	if payload != nil {
		files = append(files, makeBundleFile(
			"feedback/payload.json",
			"application/json",
			"feedback_payload",
			mustJSON(sanitizeFeedbackValue(payload, state, "bundle.payload")),
		))
	}

	captureStatus := "unavailable"
	var adapterType *string
	var paperclipRun interface{}
	var rawAdapterTrace interface{}
	var normalizedAdapterTrace interface{}
	if runID := resolveSourceRunID(payload); runID != "" {
		runBundle, found, err := s.loadRunBundle(ctx, row.CompanyID, runID, state)
		if err != nil {
			return nil, err
		}
		if found {
			paperclipRun = runBundle
			rawAdapterTrace = map[string]interface{}{"sourceRunId": runID}
			normalizedAdapterTrace = map[string]interface{}{"sourceRunId": runID}
			files = append(files, makeBundleFile(
				"paperclip/run-bundle.json",
				"application/json",
				"paperclip_run",
				mustJSON(runBundle),
			))
			captureStatus = "partial"
			if runMap, ok := runBundle.(map[string]interface{}); ok {
				if rawAdapterType, ok := runMap["adapterType"].(string); ok && strings.TrimSpace(rawAdapterType) != "" {
					adapterType = &rawAdapterType
				}
			}
		} else {
			notes = appendUnique(notes, "source_run_unavailable")
		}
	} else {
		notes = appendUnique(notes, "source_run_missing")
	}
	if captureStatus == "partial" && payload != nil {
		captureStatus = "full"
	}
	if captureStatus != "full" && len(files) > 0 {
		notes = appendUnique(notes, "adapter_trace_partial")
	}

	envelope := sanitizeFeedbackValue(map[string]interface{}{
		"traceId":         trace.ID,
		"exportId":        trace.ExportID,
		"companyId":       trace.CompanyID,
		"feedbackVoteId":  trace.FeedbackVoteID,
		"issueId":         trace.IssueID,
		"issueIdentifier": trace.IssueIdentifier,
		"issueTitle":      trace.IssueTitle,
		"projectId":       trace.ProjectID,
		"authorUserId":    trace.AuthorUserID,
		"targetType":      trace.TargetType,
		"targetId":        trace.TargetID,
		"vote":            trace.Vote,
		"status":          trace.Status,
		"destination":     trace.Destination,
		"consentVersion":  trace.ConsentVersion,
		"schemaVersion":   trace.SchemaVersion,
		"bundleVersion":   trace.BundleVersion,
		"payloadVersion":  trace.PayloadVersion,
		"payloadDigest":   trace.PayloadDigest,
		"createdAt":       trace.CreatedAt.UTC().Format(time.RFC3339),
		"exportedAt":      formatTimePtr(trace.ExportedAt),
	}, state, "bundle.envelope")

	surface := sanitizeFeedbackValue(map[string]interface{}{
		"target":  payload["target"],
		"summary": trace.TargetSummary,
		"payload": payload,
	}, state, "bundle.surface")

	return &FeedbackTraceBundle{
		TraceID:                trace.ID,
		ExportID:               trace.ExportID,
		CompanyID:              trace.CompanyID,
		IssueID:                trace.IssueID,
		IssueIdentifier:        trace.IssueIdentifier,
		AdapterType:            adapterType,
		CaptureStatus:          captureStatus,
		Notes:                  notes,
		Envelope:               envelope,
		Surface:                surface,
		PaperclipRun:           paperclipRun,
		RawAdapterTrace:        rawAdapterTrace,
		NormalizedAdapterTrace: normalizedAdapterTrace,
		Privacy: map[string]interface{}{
			"traceRedactionSummary":  trace.RedactionSummary,
			"bundleRedactionSummary": FinalizeFeedbackRedactionSummary(state),
		},
		Integrity: map[string]interface{}{
			"payloadDigest": trace.PayloadDigest,
			"bundleDigest": SHA256Digest(map[string]interface{}{
				"traceId":       trace.ID,
				"files":         fileDigests(files),
				"captureStatus": captureStatus,
			}),
		},
		Files: files,
		Data:  payload,
	}, nil
}

func (s *FeedbackService) SaveVote(ctx context.Context, input FeedbackVoteInput) (*FeedbackVote, error) {
	if strings.TrimSpace(input.AuthorUserID) == "" {
		return nil, fmt.Errorf("author user id is required")
	}
	if input.TargetType != "issue_comment" && input.TargetType != "issue_document_revision" {
		return nil, ErrFeedbackUnsupportedTarget
	}
	if input.Vote != "up" && input.Vote != "down" {
		return nil, ErrFeedbackInvalidVote
	}

	issue, err := s.getIssueByID(ctx, input.IssueID)
	if err != nil {
		return nil, err
	}
	company, err := s.getCompanyByID(ctx, issue.CompanyID)
	if err != nil {
		return nil, err
	}
	target, err := s.resolveTarget(ctx, issue, input.TargetType, input.TargetID)
	if err != nil {
		return nil, err
	}
	var agent *models.Agent
	if target.AuthorAgentID != nil {
		candidate := &models.Agent{}
		if err := s.db.WithContext(ctx).First(candidate, "id = ?", *target.AuthorAgentID).Error; err == nil {
			agent = candidate
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}

	now := time.Now().UTC()
	state := NewFeedbackRedactionState()
	reason := sanitizeNullableText(normalizeFeedbackReason(input.Vote, input.Reason), state, "vote.reason")
	issueTitle := SanitizeFeedbackText(issue.Title, state, "bundle.issueContext.title", feedbackMaxTextChars)
	issueDescription := sanitizeNullableText(issue.Description, state, "bundle.issueContext.description")
	targetBody := SanitizeFeedbackText(target.Body, state, "bundle.primaryContent.body", feedbackMaxTextChars)
	excerpt := truncateExcerpt(targetBody, feedbackMaxExcerptChars)
	targetCreatedAt := target.CreatedAt.UTC()
	targetSummary := FeedbackTraceTargetSummary{
		Label:          target.Label,
		Excerpt:        excerpt,
		AuthorAgentID:  target.AuthorAgentID,
		AuthorUserID:   target.AuthorUserID,
		CreatedAt:      &targetCreatedAt,
		DocumentKey:    target.DocumentKey,
		DocumentTitle:  target.DocumentTitle,
		RevisionNumber: target.RevisionNumber,
	}

	payload := map[string]interface{}{
		"vote": map[string]interface{}{
			"value":        input.Vote,
			"reason":       reason,
			"authorUserId": input.AuthorUserID,
		},
		"target": target.PayloadTarget,
		"bundle": map[string]interface{}{
			"primaryContent": map[string]interface{}{
				"label": target.Label,
				"body":  targetBody,
			},
			"issueContext": map[string]interface{}{
				"id":          issue.ID,
				"companyId":   issue.CompanyID,
				"projectId":   issue.ProjectID,
				"identifier":  issue.Identifier,
				"title":       issueTitle,
				"description": issueDescription,
			},
			"agentContext": map[string]interface{}{
				"agent": map[string]interface{}{
					"id":          feedbackStringPtrValue(target.AuthorAgentID),
					"name":        agentString(agent, func(a *models.Agent) string { return a.Name }),
					"role":        agentString(agent, func(a *models.Agent) string { return a.Role }),
					"title":       agentPtr(agent, func(a *models.Agent) *string { return a.Title }),
					"adapterType": agentString(agent, func(a *models.Agent) string { return a.AdapterType }),
				},
				"runtime": map[string]interface{}{
					"sourceRun": map[string]interface{}{
						"id": feedbackStringPtrValue(target.CreatedByRunID),
					},
				},
			},
		},
	}
	redactionSummary := FinalizeFeedbackRedactionSummary(state)
	payload["redactionSummary"] = redactionSummary
	payloadDigest := SHA256Digest(payload)
	sharedWithLabs := input.AllowSharing && company.FeedbackDataSharingEnabled
	consentVersion := company.FeedbackDataSharingTermsVersion
	if sharedWithLabs && (consentVersion == nil || strings.TrimSpace(*consentVersion) == "") {
		defaultTerms := defaultFeedbackSharingTermsVersion
		consentVersion = &defaultTerms
	}

	var savedVote models.FeedbackVote
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var vote models.FeedbackVote
		err := tx.Where(
			"company_id = ? AND target_type = ? AND target_id = ? AND author_user_id = ?",
			issue.CompanyID, input.TargetType, input.TargetID, input.AuthorUserID,
		).First(&vote).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		creatingVote := errors.Is(err, gorm.ErrRecordNotFound)
		if creatingVote {
			vote = models.FeedbackVote{ID: uuid.NewString(), CreatedAt: now}
		}
		vote.CompanyID = issue.CompanyID
		vote.IssueID = issue.ID
		vote.TargetType = input.TargetType
		vote.TargetID = input.TargetID
		vote.AuthorUserID = input.AuthorUserID
		vote.Vote = input.Vote
		vote.Reason = reason
		vote.SharedWithLabs = sharedWithLabs
		if sharedWithLabs {
			vote.SharedAt = &now
		} else {
			vote.SharedAt = nil
		}
		vote.ConsentVersion = consentVersion
		vote.RedactionSummary = mustJSONBytes(redactionSummary)
		vote.UpdatedAt = now
		if creatingVote {
			if err := tx.Create(&vote).Error; err != nil {
				return err
			}
		} else if err := tx.Save(&vote).Error; err != nil {
			return err
		}

		var export models.FeedbackExport
		err = tx.Where("feedback_vote_id = ?", vote.ID).First(&export).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		creatingExport := errors.Is(err, gorm.ErrRecordNotFound)
		if creatingExport {
			export = models.FeedbackExport{ID: uuid.NewString(), CreatedAt: now}
		}
		export.CompanyID = issue.CompanyID
		export.FeedbackVoteID = vote.ID
		export.IssueID = issue.ID
		export.ProjectID = issue.ProjectID
		export.AuthorUserID = input.AuthorUserID
		export.TargetType = input.TargetType
		export.TargetID = input.TargetID
		export.Vote = input.Vote
		if sharedWithLabs {
			export.Status = "pending"
			export.Destination = stringPtr(feedbackDestination)
			export.ExportID = stringPtr(buildExportID(vote.ID, now))
			export.ConsentVersion = consentVersion
		} else {
			export.Status = "local_only"
			export.Destination = nil
			export.ExportID = nil
			export.ConsentVersion = nil
		}
		export.SchemaVersion = feedbackSchemaVersion
		export.BundleVersion = feedbackBundleVersion
		export.PayloadVersion = feedbackPayloadVersion
		export.PayloadDigest = stringPtr(payloadDigest)
		export.PayloadSnapshot = mustJSONBytes(payload)
		export.TargetSummary = mustJSONBytes(targetSummary)
		export.RedactionSummary = mustJSONBytes(redactionSummary)
		export.FailureReason = nil
		export.UpdatedAt = now
		if creatingExport {
			if err := tx.Create(&export).Error; err != nil {
				return err
			}
		} else if err := tx.Save(&export).Error; err != nil {
			return err
		}
		savedVote = vote
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &savedVote, nil
}

func (s *FeedbackService) ListVotesForUser(ctx context.Context, issueID, userID string) ([]FeedbackVote, error) {
	var votes []models.FeedbackVote
	if err := s.db.WithContext(ctx).
		Where("issue_id = ? AND author_user_id = ?", issueID, userID).
		Order("created_at ASC").
		Find(&votes).Error; err != nil {
		return nil, err
	}
	return votes, nil
}

func (s *FeedbackService) resolveFilters(ctx context.Context, filters FeedbackTraceFilters) (FeedbackTraceFilters, error) {
	resolved := filters
	if strings.TrimSpace(resolved.IssueID) != "" && strings.TrimSpace(resolved.CompanyID) == "" {
		issue, err := s.getIssueByID(ctx, resolved.IssueID)
		if err != nil {
			return FeedbackTraceFilters{}, err
		}
		resolved.CompanyID = issue.CompanyID
	}
	return resolved, nil
}

func (s *FeedbackService) queryTraceRows(ctx context.Context, filters FeedbackTraceFilters) ([]feedbackTraceRow, error) {
	query := s.db.WithContext(ctx).
		Table("feedback_exports fe").
		Select("fe.*, i.identifier AS issue_identifier, i.title AS issue_title").
		Joins("INNER JOIN issues i ON i.id = fe.issue_id")
	if filters.CompanyID != "" {
		query = query.Where("fe.company_id = ?", filters.CompanyID)
	}
	if filters.IssueID != "" {
		query = query.Where("fe.issue_id = ?", filters.IssueID)
	}
	if filters.ProjectID != "" {
		query = query.Where("fe.project_id = ?", filters.ProjectID)
	}
	if filters.TargetType != "" {
		query = query.Where("fe.target_type = ?", filters.TargetType)
	}
	if filters.Vote != "" {
		query = query.Where("fe.vote = ?", filters.Vote)
	}
	if filters.Status != "" {
		query = query.Where("fe.status = ?", filters.Status)
	}
	if filters.SharedOnly {
		query = query.Where("fe.status <> ?", "local_only")
	}
	if filters.From != nil {
		query = query.Where("fe.created_at >= ?", *filters.From)
	}
	if filters.To != nil {
		query = query.Where("fe.created_at <= ?", *filters.To)
	}
	var rows []feedbackTraceRow
	if err := query.Order("fe.created_at DESC").Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *FeedbackService) getTraceRowByID(ctx context.Context, traceID string) (*feedbackTraceRow, error) {
	var row feedbackTraceRow
	if err := s.db.WithContext(ctx).
		Table("feedback_exports fe").
		Select("fe.*, i.identifier AS issue_identifier, i.title AS issue_title").
		Joins("INNER JOIN issues i ON i.id = fe.issue_id").
		Where("fe.id = ?", traceID).
		Scan(&row).Error; err != nil {
		return nil, err
	}
	if row.ID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	return &row, nil
}

func (s *FeedbackService) getIssueByID(ctx context.Context, issueID string) (*models.Issue, error) {
	var issue models.Issue
	if err := s.db.WithContext(ctx).First(&issue, "id = ?", issueID).Error; err != nil {
		return nil, err
	}
	return &issue, nil
}

func (s *FeedbackService) getCompanyByID(ctx context.Context, companyID string) (*models.Company, error) {
	var company models.Company
	if err := s.db.WithContext(ctx).First(&company, "id = ?", companyID).Error; err != nil {
		return nil, err
	}
	return &company, nil
}

func (s *FeedbackService) resolveTarget(ctx context.Context, issue *models.Issue, targetType, targetID string) (*feedbackTargetRecord, error) {
	issuePath := buildIssuePath(issue.Identifier)
	switch targetType {
	case "issue_comment":
		var comment models.IssueComment
		if err := s.db.WithContext(ctx).First(&comment, "id = ?", targetID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrFeedbackTargetNotFound
			}
			return nil, err
		}
		if comment.IssueID != issue.ID || comment.CompanyID != issue.CompanyID {
			return nil, ErrFeedbackTargetNotFound
		}
		if comment.AuthorAgentID == nil || strings.TrimSpace(*comment.AuthorAgentID) == "" {
			return nil, ErrFeedbackVoteNotAllowed
		}
		return &feedbackTargetRecord{
			Label:          "Comment",
			Body:           comment.Body,
			CreatedAt:      comment.CreatedAt.UTC(),
			AuthorAgentID:  comment.AuthorAgentID,
			AuthorUserID:   comment.AuthorUserID,
			CreatedByRunID: comment.CreatedByRunID,
			PayloadTarget: map[string]interface{}{
				"type":           targetType,
				"id":             comment.ID,
				"createdAt":      comment.CreatedAt.UTC().Format(time.RFC3339),
				"authorAgentId":  comment.AuthorAgentID,
				"authorUserId":   comment.AuthorUserID,
				"createdByRunId": comment.CreatedByRunID,
				"issuePath":      issuePath,
				"targetPath":     joinAnchor(issuePath, "comment-"+comment.ID),
			},
		}, nil
	case "issue_document_revision":
		var revision models.DocumentRevision
		if err := s.db.WithContext(ctx).First(&revision, "id = ?", targetID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrFeedbackTargetNotFound
			}
			return nil, err
		}
		var issueDoc models.IssueDocument
		if err := s.db.WithContext(ctx).
			Where("document_id = ? AND issue_id = ? AND company_id = ?", revision.DocumentID, issue.ID, issue.CompanyID).
			First(&issueDoc).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrFeedbackTargetNotFound
			}
			return nil, err
		}
		if revision.CreatedByAgentID == nil || strings.TrimSpace(*revision.CreatedByAgentID) == "" {
			return nil, ErrFeedbackVoteNotAllowed
		}
		var doc models.Document
		if err := s.db.WithContext(ctx).First(&doc, "id = ?", revision.DocumentID).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		label := fmt.Sprintf("%s rev %d", issueDoc.Key, revision.RevisionNumber)
		return &feedbackTargetRecord{
			Label:          label,
			Body:           revision.Body,
			CreatedAt:      revision.CreatedAt.UTC(),
			AuthorAgentID:  revision.CreatedByAgentID,
			AuthorUserID:   revision.CreatedByUserID,
			CreatedByRunID: revision.CreatedByRunID,
			DocumentKey:    &issueDoc.Key,
			DocumentTitle:  doc.Title,
			RevisionNumber: &revision.RevisionNumber,
			PayloadTarget: map[string]interface{}{
				"type":           targetType,
				"id":             revision.ID,
				"documentId":     revision.DocumentID,
				"documentKey":    issueDoc.Key,
				"documentTitle":  doc.Title,
				"revisionNumber": revision.RevisionNumber,
				"createdAt":      revision.CreatedAt.UTC().Format(time.RFC3339),
				"authorAgentId":  revision.CreatedByAgentID,
				"authorUserId":   revision.CreatedByUserID,
				"createdByRunId": revision.CreatedByRunID,
				"issuePath":      issuePath,
				"targetPath":     joinAnchor(issuePath, "document-"+issueDoc.Key),
			},
		}, nil
	default:
		return nil, ErrFeedbackUnsupportedTarget
	}
}

func (s *FeedbackService) loadRunBundle(ctx context.Context, companyID, runID string, state *FeedbackRedactionState) (interface{}, bool, error) {
	var run models.HeartbeatRun
	if err := s.db.WithContext(ctx).First(&run, "id = ? AND company_id = ?", runID, companyID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	var agent models.Agent
	if err := s.db.WithContext(ctx).First(&agent, "id = ?", run.AgentID).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}
	var events []models.HeartbeatRunEvent
	if err := s.db.WithContext(ctx).Where("run_id = ?", run.ID).Order("seq ASC").Find(&events).Error; err != nil {
		return nil, false, err
	}
	sanitizedEvents := make([]map[string]interface{}, 0, len(events))
	for _, event := range events {
		sanitizedEvents = append(sanitizedEvents, map[string]interface{}{
			"id":        event.ID,
			"seq":       event.Seq,
			"eventType": event.EventType,
			"stream":    event.Stream,
			"level":     event.Level,
			"color":     event.Color,
			"message":   sanitizeNullableText(event.Message, state, fmt.Sprintf("bundle.runEvents.%d.message", event.Seq)),
			"payload":   sanitizeFeedbackValue(jsonInterface(event.Payload), state, fmt.Sprintf("bundle.runEvents.%d.payload", event.Seq)),
			"createdAt": event.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	return sanitizeFeedbackValue(map[string]interface{}{
		"id":               run.ID,
		"companyId":        run.CompanyID,
		"agentId":          run.AgentID,
		"adapterType":      agent.AdapterType,
		"invocationSource": run.InvocationSource,
		"status":           run.Status,
		"startedAt":        formatTimePtr(run.StartedAt),
		"finishedAt":       formatTimePtr(run.FinishedAt),
		"createdAt":        run.CreatedAt.UTC().Format(time.RFC3339),
		"updatedAt":        run.UpdatedAt.UTC().Format(time.RFC3339),
		"error":            sanitizeNullableText(run.Error, state, "bundle.run.error"),
		"errorCode":        run.ErrorCode,
		"usage":            sanitizeFeedbackValue(jsonInterface(run.UsageJSON), state, "bundle.run.usage"),
		"result":           sanitizeFeedbackValue(jsonInterface(run.ResultJSON), state, "bundle.run.result"),
		"sessionIdBefore":  run.SessionIDBefore,
		"sessionIdAfter":   run.SessionIDAfter,
		"externalRunId":    run.ExternalRunID,
		"contextSnapshot":  sanitizeFeedbackValue(jsonInterface(run.ContextSnapshot), state, "bundle.run.contextSnapshot"),
		"agent": map[string]interface{}{
			"id":          agent.ID,
			"name":        agent.Name,
			"role":        agent.Role,
			"title":       agent.Title,
			"adapterType": agent.AdapterType,
		},
		"events": sanitizedEvents,
	}, state, "bundle.paperclipRun"), true, nil
}

func mapFeedbackTraceRow(row feedbackTraceRow, includePayload bool) FeedbackTrace {
	trace := FeedbackTrace{
		ID:              row.ID,
		CompanyID:       row.CompanyID,
		FeedbackVoteID:  row.FeedbackVoteID,
		IssueID:         row.IssueID,
		ProjectID:       row.ProjectID,
		IssueIdentifier: row.IssueIdentifier,
		IssueTitle:      row.IssueTitle,
		AuthorUserID:    row.AuthorUserID,
		TargetType:      row.TargetType,
		TargetID:        row.TargetID,
		Vote:            row.Vote,
		Status:          row.Status,
		Destination:     row.Destination,
		ExportID:        row.ExportID,
		ConsentVersion:  row.ConsentVersion,
		SchemaVersion:   row.SchemaVersion,
		BundleVersion:   row.BundleVersion,
		PayloadVersion:  row.PayloadVersion,
		PayloadDigest:   row.PayloadDigest,
		AttemptCount:    row.AttemptCount,
		LastAttemptedAt: row.LastAttemptedAt,
		ExportedAt:      row.ExportedAt,
		FailureReason:   row.FailureReason,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
	if includePayload {
		trace.PayloadSnapshot = jsonInterface(row.PayloadSnapshot)
	}
	trace.TargetSummary = parseTargetSummary(row.TargetSummary, row.TargetType)
	trace.RedactionSummary = jsonInterface(row.RedactionSummary)
	return trace
}

func parseTargetSummary(raw datatypes.JSON, fallbackLabel string) FeedbackTraceTargetSummary {
	var summary FeedbackTraceTargetSummary
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &summary)
	}
	if strings.TrimSpace(summary.Label) == "" {
		summary.Label = fallbackLabel
	}
	return summary
}

func jsonInterface(raw datatypes.JSON) interface{} {
	if len(raw) == 0 {
		return nil
	}
	var value interface{}
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil
	}
	return value
}

func mustJSON(value interface{}) string {
	body, _ := json.MarshalIndent(value, "", "  ")
	return string(body) + "\n"
}

func mustJSONBytes(value interface{}) datatypes.JSON {
	body, _ := json.Marshal(value)
	return datatypes.JSON(body)
}

func formatTimePtr(value *time.Time) interface{} {
	if value == nil {
		return nil
	}
	return value.UTC().Format(time.RFC3339)
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func truncateExcerpt(text string, max int) *string {
	normalized := strings.Join(strings.Fields(text), " ")
	if normalized == "" {
		return nil
	}
	if max < 4 {
		return &normalized
	}
	if len(normalized) > max {
		normalized = normalized[:max-3] + "..."
	}
	return &normalized
}

func normalizeFeedbackReason(vote string, reason *string) *string {
	if vote != "down" || reason == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*reason)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func sanitizeNullableText(value *string, state *FeedbackRedactionState, fieldPath string) *string {
	if value == nil {
		return nil
	}
	sanitized := SanitizeFeedbackText(*value, state, fieldPath, feedbackMaxTextChars)
	return &sanitized
}

func sanitizeFeedbackValue(value interface{}, state *FeedbackRedactionState, fieldPath string) interface{} {
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		return SanitizeFeedbackText(v, state, fieldPath, feedbackMaxTextChars)
	case *string:
		return sanitizeNullableText(v, state, fieldPath)
	case bool:
		return v
	case float64:
		return v
	case float32:
		return v
	case int:
		return v
	case int8:
		return v
	case int16:
		return v
	case int32:
		return v
	case int64:
		return v
	case uint:
		return v
	case uint8:
		return v
	case uint16:
		return v
	case uint32:
		return v
	case uint64:
		return v
	case json.Number:
		return v
	case map[string]interface{}:
		result := make(map[string]interface{}, len(v))
		for key, item := range v {
			result[key] = sanitizeFeedbackValue(item, state, fieldPath+"."+key)
		}
		return result
	case []interface{}:
		result := make([]interface{}, 0, len(v))
		for i, item := range v {
			result = append(result, sanitizeFeedbackValue(item, state, fmt.Sprintf("%s[%d]", fieldPath, i)))
		}
		return result
	case FeedbackTraceTargetSummary:
		copy := v
		copy.Label = SanitizeFeedbackText(copy.Label, state, fieldPath+".label", feedbackMaxTextChars)
		copy.Excerpt = sanitizeNullableText(copy.Excerpt, state, fieldPath+".excerpt")
		copy.DocumentTitle = sanitizeNullableText(copy.DocumentTitle, state, fieldPath+".documentTitle")
		return copy
	default:
		body, err := json.Marshal(v)
		if err != nil {
			return v
		}
		var generic interface{}
		if err := json.Unmarshal(body, &generic); err != nil {
			return v
		}
		return sanitizeFeedbackValue(generic, state, fieldPath)
	}
}

func buildIssuePath(identifier *string) *string {
	if identifier == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*identifier)
	if trimmed == "" {
		return nil
	}
	hyphen := strings.Index(trimmed, "-")
	if hyphen <= 0 {
		return nil
	}
	prefix := trimmed[:hyphen]
	path := fmt.Sprintf("/%s/issues/%s", prefix, trimmed)
	return &path
}

func joinAnchor(base *string, suffix string) *string {
	if base == nil || strings.TrimSpace(*base) == "" {
		return nil
	}
	joined := *base + "#" + suffix
	return &joined
}

func resolveSourceRunID(payload map[string]interface{}) string {
	if payload == nil {
		return ""
	}
	if target := asRecord(payload["target"]); target != nil {
		if runID, _ := target["createdByRunId"].(string); strings.TrimSpace(runID) != "" {
			return runID
		}
	}
	bundle := asRecord(payload["bundle"])
	agentContext := asRecord(bundle["agentContext"])
	runtime := asRecord(agentContext["runtime"])
	sourceRun := asRecord(runtime["sourceRun"])
	runID, _ := sourceRun["id"].(string)
	return strings.TrimSpace(runID)
}

func asRecord(value interface{}) map[string]interface{} {
	if value == nil {
		return nil
	}
	if typed, ok := value.(map[string]interface{}); ok {
		return typed
	}
	body, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil
	}
	return result
}

func makeBundleFile(path, contentType, source, contents string) FeedbackTraceBundleFile {
	checksum := sha256.Sum256([]byte(contents))
	return FeedbackTraceBundleFile{
		Path:        path,
		ContentType: contentType,
		Encoding:    "utf8",
		ByteLength:  len([]byte(contents)),
		SHA256:      hex.EncodeToString(checksum[:]),
		Source:      source,
		Contents:    contents,
	}
}

func fileDigests(files []FeedbackTraceBundleFile) []map[string]string {
	out := make([]map[string]string, 0, len(files))
	for _, file := range files {
		out = append(out, map[string]string{
			"path":   file.Path,
			"source": file.Source,
			"sha256": file.SHA256,
		})
	}
	return out
}

func buildExportID(feedbackVoteID string, sharedAt time.Time) string {
	return "fbexp_" + SHA256Digest(feedbackVoteID + ":" + sharedAt.UTC().Format(time.RFC3339))[:24]
}

func stringPtr(value string) *string {
	copy := value
	return &copy
}

func feedbackStringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func agentString(agent *models.Agent, selector func(*models.Agent) string) interface{} {
	if agent == nil {
		return nil
	}
	value := strings.TrimSpace(selector(agent))
	if value == "" {
		return nil
	}
	return value
}

func agentPtr(agent *models.Agent, selector func(*models.Agent) *string) interface{} {
	if agent == nil {
		return nil
	}
	return selector(agent)
}
