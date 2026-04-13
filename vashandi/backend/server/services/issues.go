package services

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"gorm.io/gorm"
)

var ALL_ISSUE_STATUSES = []string{"backlog", "todo", "in_progress", "in_review", "blocked", "done", "cancelled"}

// IssueService manages control-plane logic for issues and tasks
type IssueService struct {
	db       *gorm.DB
	activity *ActivityService
}

// NewIssueService creates a new IssueService
func NewIssueService(db *gorm.DB, activity *ActivityService) *IssueService {
	return &IssueService{
		db:       db,
		activity: activity,
	}
}

// ListIssues filters and retrieves issues
func (s *IssueService) ListIssues(ctx context.Context, companyID string, filters map[string]interface{}) ([]models.Issue, error) {
	var issues []models.Issue
	query := s.db.WithContext(ctx).Where("company_id = ?", companyID)

	if status, ok := filters["status"].(string); ok && status != "" {
		query = query.Where("status = ?", status)
	}
	if assignee, ok := filters["assigneeAgentId"].(string); ok && assignee != "" {
		query = query.Where("assignee_agent_id = ?", assignee)
	}
	if project, ok := filters["projectId"].(string); ok && project != "" {
		query = query.Where("project_id = ?", project)
	}

	err := query.Order("updated_at DESC").Find(&issues).Error
	return issues, err
}

// CreateIssue creates a new issue with standardized metadata
func (s *IssueService) CreateIssue(ctx context.Context, issue *models.Issue) (*models.Issue, error) {
	if issue.Status == "" {
		issue.Status = "backlog"
	}

	// Generate identifier if project is present
	if issue.ProjectID != nil && issue.Identifier == nil {
		id, err := s.generateIdentifier(ctx, issue.CompanyID, *issue.ProjectID)
		if err == nil {
			issue.Identifier = &id
		}
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(issue).Error; err != nil {
			return err
		}

		// Log activity
		if s.activity != nil {
			_, _ = s.activity.Log(ctx, LogEntry{
				CompanyID:  issue.CompanyID,
				ActorType:  "system", // Could be from context if we had Auth
				ActorID:    "system",
				Action:     "issue.created",
				EntityType: "issue",
				EntityID:   issue.ID,
				Details:    map[string]interface{}{"title": issue.Title},
			})
		}
		return nil
	})

	return issue, err
}

// TransitionStatus moves an issue to a new status with side effects
func (s *IssueService) TransitionStatus(ctx context.Context, issueID string, companyID string, newStatus string) (*models.Issue, error) {
	var issue models.Issue
	if err := s.db.WithContext(ctx).Where("id = ? AND company_id = ?", issueID, companyID).First(&issue).Error; err != nil {
		return nil, err
	}

	if issue.Status == newStatus {
		return &issue, nil
	}

	valid := false
	for _, s := range ALL_ISSUE_STATUSES {
		if s == newStatus {
			valid = true
			break
		}
	}
	if !valid {
		return nil, fmt.Errorf("invalid status: %s", newStatus)
	}

	oldStatus := issue.Status
	issue.Status = newStatus

	// Side effects
	now := time.Now()
	if newStatus == "in_progress" && issue.StartedAt == nil {
		issue.StartedAt = &now
	}
	if newStatus == "done" && issue.CompletedAt == nil {
		issue.CompletedAt = &now
	}
	if newStatus == "cancelled" && issue.CancelledAt == nil {
		issue.CancelledAt = &now
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&issue).Error; err != nil {
			return err
		}

		if s.activity != nil {
			_, _ = s.activity.Log(ctx, LogEntry{
				CompanyID:  companyID,
				ActorType:  "system",
				ActorID:    "system",
				Action:     "issue.status_changed",
				EntityType: "issue",
				EntityID:   issue.ID,
				Details: map[string]interface{}{
					"from": oldStatus,
					"to":   newStatus,
				},
			})
		}
		return nil
	})

	return &issue, err
}

// Checkout atomically locks an issue for a specific run
func (s *IssueService) Checkout(ctx context.Context, issueID string, companyID string, runID string) error {
	now := time.Now()
	result := s.db.WithContext(ctx).Model(&models.Issue{}).
		Where("id = ? AND company_id = ? AND (checkout_run_id IS NULL OR checkout_run_id = ?)", issueID, companyID, runID).
		Updates(map[string]interface{}{
			"checkout_run_id":     runID,
			"execution_locked_at": &now,
			"status":              "in_progress",
		})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("issue already checked out by another run")
	}

	return nil
}

func (s *IssueService) generateIdentifier(ctx context.Context, companyID string, projectID string) (string, error) {
	var project models.Project
	if err := s.db.WithContext(ctx).First(&project, "id = ?", projectID).Error; err != nil {
		return "", err
	}

	prefix := project.Name
	if len(prefix) > 4 {
		prefix = prefix[:4]
	}
	prefix = strings.ToUpper(prefix)

	var count int64
	s.db.WithContext(ctx).Model(&models.Issue{}).Where("project_id = ?", projectID).Count(&count)

	return fmt.Sprintf("%s-%d", prefix, count+1), nil
}

// NormalizeAgentMentionToken decodes HTML entities in raw mentions
// Replication of Node.js logic for parity
func NormalizeAgentMentionToken(raw string) string {
	// Simple unescape for common entities
	s := strings.ReplaceAll(raw, "&amp;", "&")
	s = strings.ReplaceAll(s, "&quot;", "\"")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&nbsp;", " ")

	re := regexp.MustCompile("&#x([0-9a-fA-F]+);")
	s = re.ReplaceAllStringFunc(s, func(m string) string {
		var hex int
		fmt.Sscanf(m, "&#x%x;", &hex)
		return string(rune(hex))
	})

	return strings.TrimSpace(s)
}
