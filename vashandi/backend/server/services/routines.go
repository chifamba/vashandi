// Package services provides the routines service for comprehensive routine management,
// mirroring the Node.js implementation in server/src/services/routines.ts.
//
// This file uses several functions and constants defined in routine_scheduler.go:
//   - validateCron(), nextCronTickInTimezone() for cron expression handling
//   - isUniqueConstraintError() for database constraint detection
//   - buildLastResult() for trigger result formatting
//   - concurrencyPolicy* constants for concurrency handling
package services

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"github.com/chifamba/vashandi/vashandi/backend/shared"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Routine status and policy constants
const (
	RoutineStatusActive   = "active"
	RoutineStatusPaused   = "paused"
	RoutineStatusArchived = "archived"

	TriggerKindSchedule = "schedule"
	TriggerKindWebhook  = "webhook"
	TriggerKindManual   = "manual"

	SourceSchedule = "schedule"
	SourceManual   = "manual"
	SourceAPI      = "api"
	SourceWebhook  = "webhook"

	SigningModeNone       = "none"
	SigningModeBearer     = "bearer"
	SigningModeGitHubHMAC = "github_hmac"
	SigningModeTimestamp  = "timestamp"

	RunStatusReceived     = "received"
	RunStatusIssueCreated = "issue_created"
	RunStatusCompleted    = "completed"
	RunStatusFailed       = "failed"
	RunStatusSkipped      = "skipped"
	RunStatusCoalesced    = "coalesced"
)

var (
	openIssueStatuses        = []string{"backlog", "todo", "in_progress", "in_review", "blocked"}
	liveHeartbeatRunStatuses = []string{"queued", "running"}
)

// RoutineService provides comprehensive routine management.
type RoutineService struct {
	DB        *gorm.DB
	Issues    *IssueService
	Activity  *ActivityService
	Heartbeat *HeartbeatService
	Secrets   *SecretsService
}

// NewRoutineService creates a new RoutineService instance.
func NewRoutineService(db *gorm.DB, issues *IssueService, activity *ActivityService, heartbeat *HeartbeatService, secrets *SecretsService) *RoutineService {
	return &RoutineService{
		DB:        db,
		Issues:    issues,
		Activity:  activity,
		Heartbeat: heartbeat,
		Secrets:   secrets,
	}
}

// RoutineListItem represents a routine with summary information for list views.
type RoutineListItem struct {
	models.Routine
	Triggers    []RoutineTriggerSummary `json:"triggers"`
	LastRun     *RoutineRunSummary      `json:"lastRun"`
	ActiveIssue *RoutineIssueSummary    `json:"activeIssue"`
}

// RoutineTriggerSummary is a summary of a trigger for list views.
type RoutineTriggerSummary struct {
	ID          string     `json:"id"`
	Kind        string     `json:"kind"`
	Label       *string    `json:"label"`
	Enabled     bool       `json:"enabled"`
	NextRunAt   *time.Time `json:"nextRunAt"`
	LastFiredAt *time.Time `json:"lastFiredAt"`
	LastResult  *string    `json:"lastResult"`
}

// RoutineRunSummary represents a routine run with additional context.
type RoutineRunSummary struct {
	models.RoutineRun
	LinkedIssue *RoutineIssueSummary          `json:"linkedIssue"`
	Trigger     *RoutineTriggerRunSummary     `json:"trigger"`
}

// RoutineTriggerRunSummary is minimal trigger info for run summaries.
type RoutineTriggerRunSummary struct {
	ID    string  `json:"id"`
	Kind  string  `json:"kind"`
	Label *string `json:"label"`
}

// RoutineIssueSummary represents minimal issue information.
type RoutineIssueSummary struct {
	ID         string    `json:"id"`
	Identifier *string   `json:"identifier"`
	Title      string    `json:"title"`
	Status     string    `json:"status"`
	Priority   string    `json:"priority"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// RoutineDetail contains full routine details including related entities.
type RoutineDetail struct {
	models.Routine
	Project     *RoutineProjectSummary `json:"project"`
	Assignee    *RoutineAgentSummary   `json:"assignee"`
	ParentIssue *RoutineIssueSummary   `json:"parentIssue"`
	Triggers    []models.RoutineTrigger `json:"triggers"`
	RecentRuns  []RoutineRunSummary    `json:"recentRuns"`
	ActiveIssue *RoutineIssueSummary   `json:"activeIssue"`
}

// RoutineProjectSummary is minimal project info.
type RoutineProjectSummary struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	Status      string  `json:"status"`
}

// RoutineAgentSummary is minimal agent info.
type RoutineAgentSummary struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Role   string  `json:"role"`
	Title  *string `json:"title"`
	URLKey *string `json:"urlKey,omitempty"`
}

// CreateRoutineInput is the input for creating a routine.
type CreateRoutineInput struct {
	ProjectID         *string                  `json:"projectId"`
	GoalID            *string                  `json:"goalId"`
	ParentIssueID     *string                  `json:"parentIssueId"`
	Title             string                   `json:"title"`
	Description       *string                  `json:"description"`
	AssigneeAgentID   *string                  `json:"assigneeAgentId"`
	Priority          string                   `json:"priority"`
	Status            string                   `json:"status"`
	ConcurrencyPolicy string                   `json:"concurrencyPolicy"`
	CatchUpPolicy     string                   `json:"catchUpPolicy"`
	Variables         []shared.RoutineVariable `json:"variables"`
}

// UpdateRoutineInput is the input for updating a routine.
type UpdateRoutineInput struct {
	ProjectID         *string                  `json:"projectId,omitempty"`
	GoalID            *string                  `json:"goalId,omitempty"`
	ParentIssueID     *string                  `json:"parentIssueId,omitempty"`
	Title             *string                  `json:"title,omitempty"`
	Description       *string                  `json:"description,omitempty"`
	AssigneeAgentID   *string                  `json:"assigneeAgentId,omitempty"`
	Priority          *string                  `json:"priority,omitempty"`
	Status            *string                  `json:"status,omitempty"`
	ConcurrencyPolicy *string                  `json:"concurrencyPolicy,omitempty"`
	CatchUpPolicy     *string                  `json:"catchUpPolicy,omitempty"`
	Variables         []shared.RoutineVariable `json:"variables,omitempty"`
}

// CreateTriggerInput is the input for creating a trigger.
type CreateTriggerInput struct {
	Kind            string  `json:"kind"`
	Label           *string `json:"label"`
	Enabled         *bool   `json:"enabled"`
	CronExpression  string  `json:"cronExpression"`
	Timezone        string  `json:"timezone"`
	SigningMode     *string `json:"signingMode"`
	ReplayWindowSec *int    `json:"replayWindowSec"`
}

// UpdateTriggerInput is the input for updating a trigger.
type UpdateTriggerInput struct {
	Label           *string `json:"label,omitempty"`
	Enabled         *bool   `json:"enabled,omitempty"`
	CronExpression  *string `json:"cronExpression,omitempty"`
	Timezone        *string `json:"timezone,omitempty"`
	SigningMode     *string `json:"signingMode,omitempty"`
	ReplayWindowSec *int    `json:"replayWindowSec,omitempty"`
}

// TriggerSecretMaterial contains webhook authentication material.
type TriggerSecretMaterial struct {
	WebhookURL    string `json:"webhookUrl"`
	WebhookSecret string `json:"webhookSecret"`
}

// RunRoutineInput is the input for running a routine.
type RunRoutineInput struct {
	Source                       string                 `json:"source"`
	TriggerId                    *string                `json:"triggerId"`
	Payload                      map[string]interface{} `json:"payload"`
	Variables                    map[string]interface{} `json:"variables"`
	ProjectID                    *string                `json:"projectId"`
	AssigneeAgentID              *string                `json:"assigneeAgentId"`
	IdempotencyKey               *string                `json:"idempotencyKey"`
	ExecutionWorkspaceID         *string                `json:"executionWorkspaceId"`
	ExecutionWorkspacePreference *string                `json:"executionWorkspacePreference"`
	ExecutionWorkspaceSettings   map[string]interface{} `json:"executionWorkspaceSettings"`
}

// FirePublicTriggerInput is the input for firing a public webhook trigger.
type FirePublicTriggerInput struct {
	AuthorizationHeader string                 `json:"authorizationHeader"`
	SignatureHeader     string                 `json:"signatureHeader"`
	HubSignatureHeader  string                 `json:"hubSignatureHeader"`
	TimestampHeader     string                 `json:"timestampHeader"`
	IdempotencyKey      *string                `json:"idempotencyKey"`
	RawBody             []byte                 `json:"rawBody"`
	Payload             map[string]interface{} `json:"payload"`
}

// Get retrieves a routine by ID.
func (s *RoutineService) Get(ctx context.Context, id string) (*models.Routine, error) {
	var routine models.Routine
	if err := s.DB.WithContext(ctx).First(&routine, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &routine, nil
}

// GetTrigger retrieves a trigger by ID.
func (s *RoutineService) GetTrigger(ctx context.Context, id string) (*models.RoutineTrigger, error) {
	var trigger models.RoutineTrigger
	if err := s.DB.WithContext(ctx).First(&trigger, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &trigger, nil
}

// List returns all routines for a company.
func (s *RoutineService) List(ctx context.Context, companyID string) ([]RoutineListItem, error) {
	var routines []models.Routine
	if err := s.DB.WithContext(ctx).
		Where("company_id = ?", companyID).
		Order("updated_at DESC, title ASC").
		Find(&routines).Error; err != nil {
		return nil, err
	}

	if len(routines) == 0 {
		return []RoutineListItem{}, nil
	}

	routineIDs := make([]string, len(routines))
	for i, r := range routines {
		routineIDs[i] = r.ID
	}

	// Fetch triggers
	triggersByRoutine := s.listTriggersForRoutineIDs(ctx, companyID, routineIDs)
	latestRunByRoutine := s.listLatestRunByRoutineIDs(ctx, companyID, routineIDs)
	activeIssueByRoutine := s.listLiveIssueByRoutineIDs(ctx, companyID, routineIDs)

	result := make([]RoutineListItem, len(routines))
	for i, routine := range routines {
		triggers := triggersByRoutine[routine.ID]
		triggerSummaries := make([]RoutineTriggerSummary, len(triggers))
		for j, t := range triggers {
			triggerSummaries[j] = RoutineTriggerSummary{
				ID:          t.ID,
				Kind:        t.Kind,
				Label:       t.Label,
				Enabled:     t.Enabled,
				NextRunAt:   t.NextRunAt,
				LastFiredAt: t.LastFiredAt,
				LastResult:  t.LastResult,
			}
		}

		result[i] = RoutineListItem{
			Routine:     routine,
			Triggers:    triggerSummaries,
			LastRun:     latestRunByRoutine[routine.ID],
			ActiveIssue: activeIssueByRoutine[routine.ID],
		}
	}

	return result, nil
}

// GetDetail returns full routine details.
func (s *RoutineService) GetDetail(ctx context.Context, id string) (*RoutineDetail, error) {
	routine, err := s.Get(ctx, id)
	if err != nil || routine == nil {
		return nil, err
	}

	var project *RoutineProjectSummary
	if routine.ProjectID != "" {
		var p models.Project
		if err := s.DB.WithContext(ctx).First(&p, "id = ?", routine.ProjectID).Error; err == nil {
			project = &RoutineProjectSummary{
				ID:          p.ID,
				Name:        p.Name,
				Description: p.Description,
				Status:      p.Status,
			}
		}
	}

	var assignee *RoutineAgentSummary
	if routine.AssigneeAgentID != "" {
		var a models.Agent
		if err := s.DB.WithContext(ctx).First(&a, "id = ?", routine.AssigneeAgentID).Error; err == nil {
			assignee = &RoutineAgentSummary{
				ID:    a.ID,
				Name:  a.Name,
				Role:  a.Role,
				Title: a.Title,
			}
		}
	}

	var parentIssue *RoutineIssueSummary
	if routine.ParentIssueID != nil {
		var issue models.Issue
		if err := s.DB.WithContext(ctx).First(&issue, "id = ?", *routine.ParentIssueID).Error; err == nil {
			parentIssue = &RoutineIssueSummary{
				ID:         issue.ID,
				Identifier: issue.Identifier,
				Title:      issue.Title,
				Status:     issue.Status,
				Priority:   issue.Priority,
				UpdatedAt:  issue.UpdatedAt,
			}
		}
	}

	var triggers []models.RoutineTrigger
	s.DB.WithContext(ctx).
		Where("routine_id = ?", routine.ID).
		Order("created_at ASC").
		Find(&triggers)

	recentRuns := s.getRecentRuns(ctx, routine.ID, 25)
	activeIssue := s.findLiveExecutionIssue(ctx, routine)

	return &RoutineDetail{
		Routine:     *routine,
		Project:     project,
		Assignee:    assignee,
		ParentIssue: parentIssue,
		Triggers:    triggers,
		RecentRuns:  recentRuns,
		ActiveIssue: activeIssue,
	}, nil
}

// Create creates a new routine.
func (s *RoutineService) Create(ctx context.Context, companyID string, input CreateRoutineInput, actor Actor) (*models.Routine, error) {
	// Validate references
	if input.ProjectID != nil {
		if err := s.assertProject(ctx, companyID, *input.ProjectID); err != nil {
			return nil, err
		}
	}
	if input.AssigneeAgentID != nil {
		if err := s.assertAssignableAgent(ctx, companyID, *input.AssigneeAgentID); err != nil {
			return nil, err
		}
	}
	if input.GoalID != nil {
		if err := s.assertGoal(ctx, companyID, *input.GoalID); err != nil {
			return nil, err
		}
	}
	if input.ParentIssueID != nil {
		if err := s.assertParentIssue(ctx, companyID, *input.ParentIssueID); err != nil {
			return nil, err
		}
	}

	// Sync variables with template
	templates := []string{input.Title}
	if input.Description != nil {
		templates = append(templates, *input.Description)
	}
	variables := shared.SyncRoutineVariablesWithTemplate(templates, input.Variables)

	// Normalize status
	status := input.Status
	if status == RoutineStatusActive && input.AssigneeAgentID == nil {
		status = RoutineStatusPaused
	}

	variablesJSON, _ := json.Marshal(variables)

	projectID := ""
	if input.ProjectID != nil {
		projectID = *input.ProjectID
	}
	assigneeID := ""
	if input.AssigneeAgentID != nil {
		assigneeID = *input.AssigneeAgentID
	}

	routine := &models.Routine{
		CompanyID:         companyID,
		ProjectID:         projectID,
		GoalID:            input.GoalID,
		ParentIssueID:     input.ParentIssueID,
		Title:             input.Title,
		Description:       input.Description,
		AssigneeAgentID:   assigneeID,
		Priority:          input.Priority,
		Status:            status,
		ConcurrencyPolicy: input.ConcurrencyPolicy,
		CatchUpPolicy:     input.CatchUpPolicy,
		Variables:         datatypes.JSON(variablesJSON),
		CreatedByAgentID:  actor.AgentID,
		CreatedByUserID:   actor.UserID,
		UpdatedByAgentID:  actor.AgentID,
		UpdatedByUserID:   actor.UserID,
	}

	if err := s.DB.WithContext(ctx).Create(routine).Error; err != nil {
		return nil, err
	}

	return routine, nil
}

// Update updates a routine.
func (s *RoutineService) Update(ctx context.Context, id string, input UpdateRoutineInput, actor Actor) (*models.Routine, error) {
	routine, err := s.Get(ctx, id)
	if err != nil || routine == nil {
		return nil, err
	}

	updates := make(map[string]interface{})

	if input.ProjectID != nil {
		if err := s.assertProject(ctx, routine.CompanyID, *input.ProjectID); err != nil {
			return nil, err
		}
		updates["project_id"] = *input.ProjectID
	}
	if input.GoalID != nil {
		if err := s.assertGoal(ctx, routine.CompanyID, *input.GoalID); err != nil {
			return nil, err
		}
		updates["goal_id"] = *input.GoalID
	}
	if input.ParentIssueID != nil {
		if err := s.assertParentIssue(ctx, routine.CompanyID, *input.ParentIssueID); err != nil {
			return nil, err
		}
		updates["parent_issue_id"] = *input.ParentIssueID
	}
	if input.AssigneeAgentID != nil {
		if err := s.assertAssignableAgent(ctx, routine.CompanyID, *input.AssigneeAgentID); err != nil {
			return nil, err
		}
		updates["assignee_agent_id"] = *input.AssigneeAgentID
	}
	if input.Title != nil {
		updates["title"] = *input.Title
	}
	if input.Description != nil {
		updates["description"] = *input.Description
	}
	if input.Priority != nil {
		updates["priority"] = *input.Priority
	}
	if input.Status != nil {
		assigneeID := routine.AssigneeAgentID
		if input.AssigneeAgentID != nil {
			assigneeID = *input.AssigneeAgentID
		}
		status := *input.Status
		if status == RoutineStatusActive && assigneeID == "" {
			status = RoutineStatusPaused
		}
		updates["status"] = status
	}
	if input.ConcurrencyPolicy != nil {
		updates["concurrency_policy"] = *input.ConcurrencyPolicy
	}
	if input.CatchUpPolicy != nil {
		updates["catch_up_policy"] = *input.CatchUpPolicy
	}

	// Sync variables
	if input.Variables != nil || input.Title != nil || input.Description != nil {
		title := routine.Title
		if input.Title != nil {
			title = *input.Title
		}
		desc := routine.Description
		if input.Description != nil {
			desc = input.Description
		}
		templates := []string{title}
		if desc != nil {
			templates = append(templates, *desc)
		}

		existingVars := input.Variables
		if existingVars == nil {
			var v []shared.RoutineVariable
			_ = json.Unmarshal(routine.Variables, &v)
			existingVars = v
		}
		variables := shared.SyncRoutineVariablesWithTemplate(templates, existingVars)
		variablesJSON, _ := json.Marshal(variables)
		updates["variables"] = datatypes.JSON(variablesJSON)
	}

	updates["updated_by_agent_id"] = actor.AgentID
	updates["updated_by_user_id"] = actor.UserID
	updates["updated_at"] = time.Now()

	if err := s.DB.WithContext(ctx).Model(routine).Updates(updates).Error; err != nil {
		return nil, err
	}

	return s.Get(ctx, id)
}

// Delete removes a routine.
func (s *RoutineService) Delete(ctx context.Context, id string) error {
	return s.DB.WithContext(ctx).Delete(&models.Routine{}, "id = ?", id).Error
}

// CreateTrigger creates a new trigger for a routine.
func (s *RoutineService) CreateTrigger(ctx context.Context, routineID string, input CreateTriggerInput, actor Actor, apiURL string) (*models.RoutineTrigger, *TriggerSecretMaterial, error) {
	routine, err := s.Get(ctx, routineID)
	if err != nil || routine == nil {
		return nil, nil, errors.New("routine not found")
	}

	var secretMaterial *TriggerSecretMaterial
	var secretID *string
	var publicID *string
	var nextRunAt *time.Time

	if input.Kind == TriggerKindSchedule {
		tz := input.Timezone
		if tz == "" {
			tz = "UTC"
		}
		cronErr := validateCron(input.CronExpression)
		if cronErr != "" {
			return nil, nil, errors.New(cronErr)
		}
		nextRunAt, _ = nextCronTickInTimezone(input.CronExpression, tz, time.Now())
	}

	if input.Kind == TriggerKindWebhook {
		pubID := generateRandomHex(12)
		publicID = &pubID

		secretValue := generateRandomHex(24)
		if s.Secrets != nil {
			secret, err := s.Secrets.Create(ctx, routine.CompanyID, CreateSecretInput{
				Name:        fmt.Sprintf("routine-%s-%s", routineID, generateRandomHex(6)),
				Provider:    "local_encrypted",
				Value:       secretValue,
				Description: fmt.Sprintf("Webhook auth for routine %s", routineID),
			}, actor)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to create webhook secret: %w", err)
			}
			secretID = &secret.ID
		}

		webhookURL := fmt.Sprintf("%s/api/routine-triggers/public/%s/fire", apiURL, pubID)
		secretMaterial = &TriggerSecretMaterial{
			WebhookURL:    webhookURL,
			WebhookSecret: secretValue,
		}
	}

	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}

	var cronExpr *string
	var timezone *string
	if input.Kind == TriggerKindSchedule {
		cronExpr = &input.CronExpression
		tz := input.Timezone
		if tz == "" {
			tz = "UTC"
		}
		timezone = &tz
	}

	trigger := &models.RoutineTrigger{
		CompanyID:        routine.CompanyID,
		RoutineID:        routineID,
		Kind:             input.Kind,
		Label:            input.Label,
		Enabled:          enabled,
		CronExpression:   cronExpr,
		Timezone:         timezone,
		NextRunAt:        nextRunAt,
		PublicID:         publicID,
		SecretID:         secretID,
		SigningMode:      input.SigningMode,
		ReplayWindowSec:  input.ReplayWindowSec,
		CreatedByAgentID: actor.AgentID,
		CreatedByUserID:  actor.UserID,
		UpdatedByAgentID: actor.AgentID,
		UpdatedByUserID:  actor.UserID,
	}

	if input.Kind == TriggerKindWebhook {
		now := time.Now()
		trigger.LastRotatedAt = &now
	}

	if err := s.DB.WithContext(ctx).Create(trigger).Error; err != nil {
		return nil, nil, err
	}

	return trigger, secretMaterial, nil
}

// UpdateTrigger updates a trigger.
func (s *RoutineService) UpdateTrigger(ctx context.Context, id string, input UpdateTriggerInput, actor Actor) (*models.RoutineTrigger, error) {
	trigger, err := s.GetTrigger(ctx, id)
	if err != nil || trigger == nil {
		return nil, err
	}

	updates := make(map[string]interface{})

	if input.Label != nil {
		updates["label"] = *input.Label
	}
	if input.Enabled != nil {
		updates["enabled"] = *input.Enabled
	}

	if trigger.Kind == TriggerKindSchedule {
		cronExpr := trigger.CronExpression
		tz := trigger.Timezone

		if input.CronExpression != nil {
			cronErr := validateCron(*input.CronExpression)
			if cronErr != "" {
				return nil, errors.New(cronErr)
			}
			cronExpr = input.CronExpression
			updates["cron_expression"] = *input.CronExpression
		}
		if input.Timezone != nil {
			tz = input.Timezone
			updates["timezone"] = *input.Timezone
		}

		if cronExpr != nil && tz != nil {
			nextRunAt, _ := nextCronTickInTimezone(*cronExpr, *tz, time.Now())
			updates["next_run_at"] = nextRunAt
		}
	}

	if input.SigningMode != nil {
		updates["signing_mode"] = *input.SigningMode
	}
	if input.ReplayWindowSec != nil {
		updates["replay_window_sec"] = *input.ReplayWindowSec
	}

	updates["updated_by_agent_id"] = actor.AgentID
	updates["updated_by_user_id"] = actor.UserID
	updates["updated_at"] = time.Now()

	if err := s.DB.WithContext(ctx).Model(trigger).Updates(updates).Error; err != nil {
		return nil, err
	}

	return s.GetTrigger(ctx, id)
}

// DeleteTrigger removes a trigger.
func (s *RoutineService) DeleteTrigger(ctx context.Context, id string) error {
	return s.DB.WithContext(ctx).Delete(&models.RoutineTrigger{}, "id = ?", id).Error
}

// RotateTriggerSecret rotates the secret for a webhook trigger.
func (s *RoutineService) RotateTriggerSecret(ctx context.Context, id string, actor Actor, apiURL string) (*models.RoutineTrigger, *TriggerSecretMaterial, error) {
	trigger, err := s.GetTrigger(ctx, id)
	if err != nil || trigger == nil {
		return nil, nil, errors.New("routine trigger not found")
	}

	if trigger.Kind != TriggerKindWebhook || trigger.PublicID == nil || trigger.SecretID == nil {
		return nil, nil, errors.New("only webhook triggers can rotate secrets")
	}

	secretValue := generateRandomHex(24)
	if s.Secrets != nil {
		_, err := s.Secrets.Rotate(ctx, *trigger.SecretID, secretValue, actor)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to rotate secret: %w", err)
		}
	}

	now := time.Now()
	s.DB.WithContext(ctx).Model(trigger).Updates(map[string]interface{}{
		"last_rotated_at":     now,
		"updated_by_agent_id": actor.AgentID,
		"updated_by_user_id":  actor.UserID,
		"updated_at":          now,
	})

	trigger, _ = s.GetTrigger(ctx, id)

	return trigger, &TriggerSecretMaterial{
		WebhookURL:    fmt.Sprintf("%s/api/routine-triggers/public/%s/fire", apiURL, *trigger.PublicID),
		WebhookSecret: secretValue,
	}, nil
}

// RunRoutine manually runs a routine.
func (s *RoutineService) RunRoutine(ctx context.Context, id string, input RunRoutineInput) (*models.RoutineRun, error) {
	routine, err := s.Get(ctx, id)
	if err != nil || routine == nil {
		return nil, errors.New("routine not found")
	}

	if routine.Status == RoutineStatusArchived {
		return nil, errors.New("routine is archived")
	}

	var trigger *models.RoutineTrigger
	if input.TriggerId != nil {
		trigger, err = s.GetTrigger(ctx, *input.TriggerId)
		if err != nil || trigger == nil {
			return nil, errors.New("trigger not found")
		}
		if trigger.RoutineID != routine.ID {
			return nil, errors.New("trigger does not belong to routine")
		}
		if !trigger.Enabled {
			return nil, errors.New("trigger is not enabled")
		}
	}

	return s.dispatchRoutineRun(ctx, dispatchInput{
		Routine:                      routine,
		Trigger:                      trigger,
		Source:                       input.Source,
		Payload:                      input.Payload,
		Variables:                    input.Variables,
		ProjectID:                    input.ProjectID,
		AssigneeAgentID:              input.AssigneeAgentID,
		IdempotencyKey:               input.IdempotencyKey,
		ExecutionWorkspaceID:         input.ExecutionWorkspaceID,
		ExecutionWorkspacePreference: input.ExecutionWorkspacePreference,
		ExecutionWorkspaceSettings:   input.ExecutionWorkspaceSettings,
	})
}

// FirePublicTrigger handles an incoming webhook trigger request.
func (s *RoutineService) FirePublicTrigger(ctx context.Context, publicID string, input FirePublicTriggerInput) (*models.RoutineRun, error) {
	var trigger models.RoutineTrigger
	if err := s.DB.WithContext(ctx).
		Where("public_id = ? AND kind = ?", publicID, TriggerKindWebhook).
		First(&trigger).Error; err != nil {
		return nil, errors.New("routine trigger not found")
	}

	routine, err := s.Get(ctx, trigger.RoutineID)
	if err != nil || routine == nil {
		return nil, errors.New("routine not found")
	}

	if !trigger.Enabled || routine.Status != RoutineStatusActive {
		return nil, errors.New("routine trigger is not active")
	}

	// Verify signature based on signing mode
	signingMode := SigningModeNone
	if trigger.SigningMode != nil {
		signingMode = *trigger.SigningMode
	}

	if signingMode != SigningModeNone && s.Secrets != nil && trigger.SecretID != nil {
		secretValue, err := s.Secrets.ResolveValue(ctx, routine.CompanyID, *trigger.SecretID)
		if err != nil {
			return nil, errors.New("failed to resolve trigger secret")
		}

		switch signingMode {
		case SigningModeGitHubHMAC:
			rawBody := input.RawBody
			if rawBody == nil {
				rawBody, _ = json.Marshal(input.Payload)
			}
			providedSig := strings.TrimSpace(input.HubSignatureHeader)
			if providedSig == "" {
				providedSig = strings.TrimSpace(input.SignatureHeader)
			}
			if providedSig == "" {
				return nil, errors.New("unauthorized")
			}
			if !verifyHMACSHA256(secretValue, rawBody, providedSig) {
				return nil, errors.New("unauthorized")
			}

		case SigningModeBearer:
			expected := "Bearer " + secretValue
			if !secureCompare(expected, strings.TrimSpace(input.AuthorizationHeader)) {
				return nil, errors.New("unauthorized")
			}

		default: // timestamp mode
			rawBody := input.RawBody
			if rawBody == nil {
				rawBody, _ = json.Marshal(input.Payload)
			}
			providedSig := strings.TrimSpace(input.SignatureHeader)
			providedTs := strings.TrimSpace(input.TimestampHeader)
			if providedSig == "" || providedTs == "" {
				return nil, errors.New("unauthorized")
			}
			tsMillis := normalizeWebhookTimestamp(providedTs)
			if tsMillis == 0 {
				return nil, errors.New("unauthorized")
			}
			replayWindow := 300
			if trigger.ReplayWindowSec != nil {
				replayWindow = *trigger.ReplayWindowSec
			}
			if abs(time.Now().UnixMilli()-tsMillis) > int64(replayWindow*1000) {
				return nil, errors.New("unauthorized")
			}
			if !verifyTimestampHMAC(secretValue, providedTs, rawBody, providedSig) {
				return nil, errors.New("unauthorized")
			}
		}
	}

	// Extract variables from payload
	var payloadVars map[string]interface{}
	if input.Payload != nil {
		if vars, ok := input.Payload["variables"].(map[string]interface{}); ok {
			payloadVars = vars
		}
	}

	return s.dispatchRoutineRun(ctx, dispatchInput{
		Routine:        routine,
		Trigger:        &trigger,
		Source:         SourceWebhook,
		Payload:        input.Payload,
		Variables:      payloadVars,
		IdempotencyKey: input.IdempotencyKey,
	})
}

// ListRuns returns recent runs for a routine.
func (s *RoutineService) ListRuns(ctx context.Context, routineID string, limit int) ([]RoutineRunSummary, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	return s.getRecentRuns(ctx, routineID, limit), nil
}

// SyncRunStatusForIssue updates a routine run status based on issue completion.
func (s *RoutineService) SyncRunStatusForIssue(ctx context.Context, issueID string) (*models.RoutineRun, error) {
	var issue models.Issue
	if err := s.DB.WithContext(ctx).First(&issue, "id = ?", issueID).Error; err != nil {
		return nil, nil
	}

	if issue.OriginKind != "routine_execution" || issue.OriginRunID == nil {
		return nil, nil
	}

	var run models.RoutineRun
	if err := s.DB.WithContext(ctx).First(&run, "id = ?", *issue.OriginRunID).Error; err != nil {
		return nil, nil
	}

	now := time.Now()
	var updates map[string]interface{}

	switch issue.Status {
	case "done":
		updates = map[string]interface{}{
			"status":       RunStatusCompleted,
			"completed_at": now,
			"updated_at":   now,
		}
	case "blocked", "cancelled":
		reason := fmt.Sprintf("Execution issue moved to %s", issue.Status)
		updates = map[string]interface{}{
			"status":         RunStatusFailed,
			"failure_reason": reason,
			"completed_at":   now,
			"updated_at":     now,
		}
	default:
		return nil, nil
	}

	if err := s.DB.WithContext(ctx).Model(&run).Updates(updates).Error; err != nil {
		return nil, err
	}

	return &run, nil
}

// dispatchInput contains all parameters for dispatching a routine run.
type dispatchInput struct {
	Routine                      *models.Routine
	Trigger                      *models.RoutineTrigger
	Source                       string
	Payload                      map[string]interface{}
	Variables                    map[string]interface{}
	ProjectID                    *string
	AssigneeAgentID              *string
	IdempotencyKey               *string
	ExecutionWorkspaceID         *string
	ExecutionWorkspacePreference *string
	ExecutionWorkspaceSettings   map[string]interface{}
}

// dispatchRoutineRun creates and processes a routine run.
func (s *RoutineService) dispatchRoutineRun(ctx context.Context, input dispatchInput) (*models.RoutineRun, error) {
	routine := input.Routine

	projectID := routine.ProjectID
	if input.ProjectID != nil {
		projectID = *input.ProjectID
	}

	assigneeAgentID := routine.AssigneeAgentID
	if input.AssigneeAgentID != nil {
		assigneeAgentID = *input.AssigneeAgentID
	}
	if assigneeAgentID == "" {
		return nil, errors.New("default agent required")
	}

	// Parse routine variables
	var routineVars []shared.RoutineVariable
	_ = json.Unmarshal(routine.Variables, &routineVars)

	// Resolve variable values
	resolvedVars, err := shared.ResolveRoutineVariableValues(routineVars, input.Source, input.Payload, input.Variables)
	if err != nil {
		return nil, err
	}

	// Merge with built-in variables for interpolation
	allVars := shared.GetBuiltinRoutineVariableValues()
	for k, v := range resolvedVars {
		allVars[k] = v
	}

	// Interpolate title and description
	title := shared.InterpolateRoutineTemplate(routine.Title, allVars)
	description := shared.InterpolateRoutineTemplatePtr(routine.Description, allVars)

	// Merge payload with resolved variables
	triggerPayload := shared.MergeRoutineRunPayload(input.Payload, resolvedVars)
	triggerPayloadJSON, _ := json.Marshal(triggerPayload)

	now := time.Now()

	// Check for idempotent run
	if input.IdempotencyKey != nil {
		var existing models.RoutineRun
		query := s.DB.WithContext(ctx).
			Where("company_id = ? AND routine_id = ? AND source = ? AND idempotency_key = ?",
				routine.CompanyID, routine.ID, input.Source, *input.IdempotencyKey)
		if input.Trigger != nil {
			query = query.Where("trigger_id = ?", input.Trigger.ID)
		} else {
			query = query.Where("trigger_id IS NULL")
		}
		if err := query.Order("created_at DESC").First(&existing).Error; err == nil {
			return &existing, nil
		}
	}

	// Create the run record
	var triggerID *string
	if input.Trigger != nil {
		triggerID = &input.Trigger.ID
	}

	run := &models.RoutineRun{
		CompanyID:      routine.CompanyID,
		RoutineID:      routine.ID,
		TriggerID:      triggerID,
		Source:         input.Source,
		Status:         RunStatusReceived,
		TriggeredAt:    now,
		IdempotencyKey: input.IdempotencyKey,
		TriggerPayload: datatypes.JSON(triggerPayloadJSON),
	}

	if err := s.DB.WithContext(ctx).Create(run).Error; err != nil {
		return nil, err
	}

	// Calculate next run time for schedule triggers
	var nextRunAt *time.Time
	if input.Trigger != nil && input.Trigger.Kind == TriggerKindSchedule &&
		input.Trigger.CronExpression != nil && input.Trigger.Timezone != nil {
		nextRunAt, _ = nextCronTickInTimezone(*input.Trigger.CronExpression, *input.Trigger.Timezone, now)
	}

	// Check concurrency policy
	if routine.ConcurrencyPolicy != concurrencyPolicyAlwaysEnqueue {
		activeIssue := s.findLiveExecutionIssue(ctx, routine)
		if activeIssue != nil {
			status := RunStatusCoalesced
			if routine.ConcurrencyPolicy == concurrencyPolicySkipIfActive {
				status = RunStatusSkipped
			}
			s.finalizeRun(ctx, run.ID, status, activeIssue.ID, nil, nil)
			s.updateRoutineTouchedState(ctx, routine.ID, triggerID, now, status, activeIssue.ID, nextRunAt)
			run, _ = s.getRunByID(ctx, run.ID)
			return run, nil
		}
	}

	// Create execution issue
	issue := &models.Issue{
		CompanyID:       routine.CompanyID,
		ProjectID:       &projectID,
		GoalID:          routine.GoalID,
		ParentID:        routine.ParentIssueID,
		Title:           title,
		Description:     description,
		Status:          "todo",
		Priority:        routine.Priority,
		AssigneeAgentID: &assigneeAgentID,
		OriginKind:      "routine_execution",
		OriginID:        &routine.ID,
		OriginRunID:     &run.ID,
	}

	if s.Issues != nil {
		created, err := s.Issues.CreateIssue(ctx, issue)
		if err != nil {
			// Handle concurrent creation conflict
			if isUniqueConstraintError(err) && routine.ConcurrencyPolicy != concurrencyPolicyAlwaysEnqueue {
				activeIssue := s.findLiveExecutionIssue(ctx, routine)
				if activeIssue != nil {
					status := RunStatusCoalesced
					if routine.ConcurrencyPolicy == concurrencyPolicySkipIfActive {
						status = RunStatusSkipped
					}
					s.finalizeRun(ctx, run.ID, status, activeIssue.ID, nil, nil)
					s.updateRoutineTouchedState(ctx, routine.ID, triggerID, now, status, activeIssue.ID, nextRunAt)
					run, _ = s.getRunByID(ctx, run.ID)
					return run, nil
				}
			}
			reason := err.Error()
			s.finalizeRun(ctx, run.ID, RunStatusFailed, "", &now, &reason)
			s.updateRoutineTouchedState(ctx, routine.ID, triggerID, now, RunStatusFailed, "", nextRunAt)
			run, _ = s.getRunByID(ctx, run.ID)
			return run, nil
		}
		issue = created
	} else {
		if err := s.DB.WithContext(ctx).Create(issue).Error; err != nil {
			reason := err.Error()
			s.finalizeRun(ctx, run.ID, RunStatusFailed, "", &now, &reason)
			s.updateRoutineTouchedState(ctx, routine.ID, triggerID, now, RunStatusFailed, "", nextRunAt)
			run, _ = s.getRunByID(ctx, run.ID)
			return run, nil
		}
	}

	// Link run to issue
	s.finalizeRun(ctx, run.ID, RunStatusIssueCreated, issue.ID, nil, nil)
	s.updateRoutineTouchedState(ctx, routine.ID, triggerID, now, RunStatusIssueCreated, issue.ID, nextRunAt)

	// Wake up the assignee agent
	if s.Heartbeat != nil && assigneeAgentID != "" {
		contextMap := map[string]interface{}{
			"issueId": issue.ID,
			"source":  "routine.dispatch",
		}
		_, _ = s.Heartbeat.Wakeup(ctx, routine.CompanyID, assigneeAgentID, WakeupOptions{
			Source:        "assignment",
			TriggerDetail: "system",
			Context:       contextMap,
		})
	}

	// Log activity
	if s.Activity != nil && (input.Source == SourceSchedule || input.Source == SourceWebhook) {
		actorID := "routine-scheduler"
		if input.Source == SourceWebhook {
			actorID = "routine-webhook"
		}
		details := map[string]interface{}{
			"routineId": routine.ID,
			"source":    run.Source,
			"status":    run.Status,
		}
		if triggerID != nil {
			details["triggerId"] = *triggerID
		}
		_, _ = s.Activity.Log(ctx, LogEntry{
			CompanyID:  routine.CompanyID,
			ActorType:  "system",
			ActorID:    actorID,
			Action:     "routine.run_triggered",
			EntityType: "routine_run",
			EntityID:   run.ID,
			Details:    details,
		})
	}

	run, _ = s.getRunByID(ctx, run.ID)
	return run, nil
}

// Helper methods

func (s *RoutineService) getRunByID(ctx context.Context, id string) (*models.RoutineRun, error) {
	var run models.RoutineRun
	if err := s.DB.WithContext(ctx).First(&run, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &run, nil
}

func (s *RoutineService) finalizeRun(ctx context.Context, runID, status, issueID string, completedAt *time.Time, failureReason *string) {
	updates := map[string]interface{}{
		"status":     status,
		"updated_at": time.Now(),
	}
	if issueID != "" {
		updates["linked_issue_id"] = issueID
	}
	if completedAt != nil {
		updates["completed_at"] = *completedAt
	}
	if failureReason != nil {
		updates["failure_reason"] = *failureReason
	}
	s.DB.WithContext(ctx).Model(&models.RoutineRun{}).Where("id = ?", runID).Updates(updates)
}

func (s *RoutineService) updateRoutineTouchedState(ctx context.Context, routineID string, triggerID *string, triggeredAt time.Time, status, issueID string, nextRunAt *time.Time) {
	routineUpdates := map[string]interface{}{
		"last_triggered_at": triggeredAt,
		"updated_at":        time.Now(),
	}
	if issueID != "" {
		routineUpdates["last_enqueued_at"] = triggeredAt
	}
	s.DB.WithContext(ctx).Model(&models.Routine{}).Where("id = ?", routineID).Updates(routineUpdates)

	if triggerID != nil {
		triggerUpdates := map[string]interface{}{
			"last_fired_at": triggeredAt,
			"last_result":   buildLastResult(status, issueID),
			"updated_at":    time.Now(),
		}
		if nextRunAt != nil {
			triggerUpdates["next_run_at"] = *nextRunAt
		}
		s.DB.WithContext(ctx).Model(&models.RoutineTrigger{}).Where("id = ?", *triggerID).Updates(triggerUpdates)
	}
}

func (s *RoutineService) findLiveExecutionIssue(ctx context.Context, routine *models.Routine) *RoutineIssueSummary {
	var issue models.Issue
	err := s.DB.WithContext(ctx).Raw(`
		SELECT i.*
		FROM issues i
		JOIN heartbeat_runs hr ON hr.id = i.execution_run_id AND hr.status IN ?
		WHERE i.company_id = ?
		  AND i.origin_kind = 'routine_execution'
		  AND i.origin_id = ?
		  AND i.status IN ?
		  AND i.hidden_at IS NULL
		ORDER BY i.updated_at DESC, i.created_at DESC
		LIMIT 1
	`, liveHeartbeatRunStatuses, routine.CompanyID, routine.ID, openIssueStatuses).Scan(&issue).Error

	if err != nil || issue.ID == "" {
		return nil
	}

	return &RoutineIssueSummary{
		ID:         issue.ID,
		Identifier: issue.Identifier,
		Title:      issue.Title,
		Status:     issue.Status,
		Priority:   issue.Priority,
		UpdatedAt:  issue.UpdatedAt,
	}
}

func (s *RoutineService) listTriggersForRoutineIDs(ctx context.Context, companyID string, routineIDs []string) map[string][]models.RoutineTrigger {
	result := make(map[string][]models.RoutineTrigger)
	if len(routineIDs) == 0 {
		return result
	}

	var triggers []models.RoutineTrigger
	s.DB.WithContext(ctx).
		Where("company_id = ? AND routine_id IN ?", companyID, routineIDs).
		Order("created_at ASC, id ASC").
		Find(&triggers)

	for _, t := range triggers {
		result[t.RoutineID] = append(result[t.RoutineID], t)
	}
	return result
}

func (s *RoutineService) listLatestRunByRoutineIDs(ctx context.Context, companyID string, routineIDs []string) map[string]*RoutineRunSummary {
	result := make(map[string]*RoutineRunSummary)
	if len(routineIDs) == 0 {
		return result
	}

	for _, routineID := range routineIDs {
		var run models.RoutineRun
		if err := s.DB.WithContext(ctx).
			Where("company_id = ? AND routine_id = ?", companyID, routineID).
			Order("created_at DESC").
			First(&run).Error; err == nil {
			summary := &RoutineRunSummary{RoutineRun: run}

			// Get linked issue
			if run.LinkedIssueID != nil {
				var issue models.Issue
				if err := s.DB.WithContext(ctx).First(&issue, "id = ?", *run.LinkedIssueID).Error; err == nil {
					summary.LinkedIssue = &RoutineIssueSummary{
						ID:         issue.ID,
						Identifier: issue.Identifier,
						Title:      issue.Title,
						Status:     issue.Status,
						Priority:   issue.Priority,
						UpdatedAt:  issue.UpdatedAt,
					}
				}
			}

			// Get trigger info
			if run.TriggerID != nil {
				var trigger models.RoutineTrigger
				if err := s.DB.WithContext(ctx).First(&trigger, "id = ?", *run.TriggerID).Error; err == nil {
					summary.Trigger = &RoutineTriggerRunSummary{
						ID:    trigger.ID,
						Kind:  trigger.Kind,
						Label: trigger.Label,
					}
				}
			}

			result[routineID] = summary
		}
	}
	return result
}

func (s *RoutineService) listLiveIssueByRoutineIDs(ctx context.Context, companyID string, routineIDs []string) map[string]*RoutineIssueSummary {
	result := make(map[string]*RoutineIssueSummary)
	if len(routineIDs) == 0 {
		return result
	}

	for _, routineID := range routineIDs {
		var issue models.Issue
		err := s.DB.WithContext(ctx).Raw(`
			SELECT i.*
			FROM issues i
			JOIN heartbeat_runs hr ON hr.id = i.execution_run_id AND hr.status IN ?
			WHERE i.company_id = ?
			  AND i.origin_kind = 'routine_execution'
			  AND i.origin_id = ?
			  AND i.status IN ?
			  AND i.hidden_at IS NULL
			ORDER BY i.updated_at DESC, i.created_at DESC
			LIMIT 1
		`, liveHeartbeatRunStatuses, companyID, routineID, openIssueStatuses).Scan(&issue).Error

		if err == nil && issue.ID != "" {
			result[routineID] = &RoutineIssueSummary{
				ID:         issue.ID,
				Identifier: issue.Identifier,
				Title:      issue.Title,
				Status:     issue.Status,
				Priority:   issue.Priority,
				UpdatedAt:  issue.UpdatedAt,
			}
		}
	}
	return result
}

func (s *RoutineService) getRecentRuns(ctx context.Context, routineID string, limit int) []RoutineRunSummary {
	var runs []models.RoutineRun
	s.DB.WithContext(ctx).
		Where("routine_id = ?", routineID).
		Order("created_at DESC").
		Limit(limit).
		Find(&runs)

	result := make([]RoutineRunSummary, len(runs))
	for i, run := range runs {
		summary := RoutineRunSummary{RoutineRun: run}

		if run.LinkedIssueID != nil {
			var issue models.Issue
			if err := s.DB.WithContext(ctx).First(&issue, "id = ?", *run.LinkedIssueID).Error; err == nil {
				summary.LinkedIssue = &RoutineIssueSummary{
					ID:         issue.ID,
					Identifier: issue.Identifier,
					Title:      issue.Title,
					Status:     issue.Status,
					Priority:   issue.Priority,
					UpdatedAt:  issue.UpdatedAt,
				}
			}
		}

		if run.TriggerID != nil {
			var trigger models.RoutineTrigger
			if err := s.DB.WithContext(ctx).First(&trigger, "id = ?", *run.TriggerID).Error; err == nil {
				summary.Trigger = &RoutineTriggerRunSummary{
					ID:    trigger.ID,
					Kind:  trigger.Kind,
					Label: trigger.Label,
				}
			}
		}

		result[i] = summary
	}
	return result
}

func (s *RoutineService) assertProject(ctx context.Context, companyID, projectID string) error {
	if projectID == "" {
		return nil
	}
	var project models.Project
	if err := s.DB.WithContext(ctx).First(&project, "id = ?", projectID).Error; err != nil {
		return errors.New("project not found")
	}
	if project.CompanyID != companyID {
		return errors.New("project must belong to same company")
	}
	return nil
}

func (s *RoutineService) assertAssignableAgent(ctx context.Context, companyID, agentID string) error {
	if agentID == "" {
		return nil
	}
	var agent models.Agent
	if err := s.DB.WithContext(ctx).First(&agent, "id = ?", agentID).Error; err != nil {
		return errors.New("assignee agent not found")
	}
	if agent.CompanyID != companyID {
		return errors.New("assignee must belong to same company")
	}
	if agent.Status == "pending_approval" {
		return errors.New("cannot assign routines to pending approval agents")
	}
	if agent.Status == "terminated" {
		return errors.New("cannot assign routines to terminated agents")
	}
	return nil
}

func (s *RoutineService) assertGoal(ctx context.Context, companyID, goalID string) error {
	var goal models.Goal
	if err := s.DB.WithContext(ctx).First(&goal, "id = ?", goalID).Error; err != nil {
		return errors.New("goal not found")
	}
	if goal.CompanyID != companyID {
		return errors.New("goal must belong to same company")
	}
	return nil
}

func (s *RoutineService) assertParentIssue(ctx context.Context, companyID, issueID string) error {
	var issue models.Issue
	if err := s.DB.WithContext(ctx).First(&issue, "id = ?", issueID).Error; err != nil {
		return errors.New("parent issue not found")
	}
	if issue.CompanyID != companyID {
		return errors.New("parent issue must belong to same company")
	}
	return nil
}

// Utility functions

func generateRandomHex(n int) string {
	bytes := make([]byte, n)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

func verifyHMACSHA256(secret string, body []byte, signature string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))

	normalized := strings.TrimPrefix(signature, "sha256=")
	return secureCompare(expected, normalized)
}

func verifyTimestampHMAC(secret, timestamp string, body []byte, signature string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "."))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))

	normalized := strings.TrimPrefix(signature, "sha256=")
	return secureCompare(expected, normalized)
}

func secureCompare(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return hmac.Equal([]byte(a), []byte(b))
}

func normalizeWebhookTimestamp(raw string) int64 {
	parsed, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0
	}
	if parsed > 1e12 {
		return int64(parsed)
	}
	return int64(parsed * 1000)
}

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}
