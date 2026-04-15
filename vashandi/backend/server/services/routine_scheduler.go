package services

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"gorm.io/gorm"
)

const (
	maxCatchUpRuns = 25

	concurrencyPolicyAlwaysEnqueue   = "always_enqueue"
	concurrencyPolicySkipIfActive    = "skip_if_active"
	concurrencyPolicyCoalesceIfActive = "coalesce_if_active"

	catchUpPolicySkipMissed          = "skip_missed"
	catchUpPolicyEnqueueMissedWithCap = "enqueue_missed_with_cap"

	openIssueSQLStatuses = "'backlog','todo','in_progress','in_review','blocked'"
)

// RoutineSchedulerService drives cron-triggered routine runs.
// It is designed to be ticked on a regular interval (e.g. every 60 s).
type RoutineSchedulerService struct {
	DB        *gorm.DB
	Heartbeat *HeartbeatService
	Issues    *IssueService
	Activity  *ActivityService
}

// NewRoutineSchedulerService creates a RoutineSchedulerService wired to the
// supplied dependencies.
func NewRoutineSchedulerService(db *gorm.DB, heartbeat *HeartbeatService, issues *IssueService, activity *ActivityService) *RoutineSchedulerService {
	return &RoutineSchedulerService{
		DB:        db,
		Heartbeat: heartbeat,
		Issues:    issues,
		Activity:  activity,
	}
}

// TickResult reports how many routine runs were dispatched in the tick.
type TickResult struct {
	Triggered int
}

// TickScheduledTriggers finds every cron trigger that is due as of `now`,
// claims it atomically, and dispatches the appropriate number of routine runs.
func (s *RoutineSchedulerService) TickScheduledTriggers(ctx context.Context, now time.Time) (TickResult, error) {
	type triggerRow struct {
		Trigger models.RoutineTrigger
		Routine models.Routine
	}

	// Fetch due triggers: kind=schedule, enabled, routine active, nextRunAt <= now.
	rows, err := s.fetchDueTriggers(ctx, now)
	if err != nil {
		return TickResult{}, fmt.Errorf("fetch due triggers: %w", err)
	}

	var result TickResult
	for _, row := range rows {
		triggered, err := s.processTrigger(ctx, row.Trigger, row.Routine, now)
		if err != nil {
			slog.Error("routine scheduler: failed to process trigger",
				"triggerId", row.Trigger.ID,
				"routineId", row.Routine.ID,
				"error", err)
			continue
		}
		result.Triggered += triggered
	}
	return result, nil
}

type dueTriggerRow struct {
	Trigger models.RoutineTrigger
	Routine models.Routine
}

func (s *RoutineSchedulerService) fetchDueTriggers(ctx context.Context, now time.Time) ([]dueTriggerRow, error) {
	type rawRow struct {
		models.RoutineTrigger `gorm:"embedded"`
		RoutineID2            string `gorm:"column:r_id"`
		RoutineCompanyID      string `gorm:"column:r_company_id"`
		RoutineProjectID      string `gorm:"column:r_project_id"`
		RoutineTitle          string `gorm:"column:r_title"`
		RoutineAssigneeAgent  string `gorm:"column:r_assignee_agent_id"`
		RoutinePriority       string `gorm:"column:r_priority"`
		RoutineStatus         string `gorm:"column:r_status"`
		RoutineConcurrency    string `gorm:"column:r_concurrency_policy"`
		RoutineCatchUp        string `gorm:"column:r_catch_up_policy"`
		RoutineGoalID         *string `gorm:"column:r_goal_id"`
		RoutineParentIssueID  *string `gorm:"column:r_parent_issue_id"`
		RoutineDescription    *string `gorm:"column:r_description"`
	}

	// Use raw SQL for the join to avoid GORM embedded struct conflicts.
	type joinRow struct {
		Trigger models.RoutineTrigger
		Routine models.Routine
	}

	var triggers []models.RoutineTrigger
	if err := s.DB.WithContext(ctx).
		Joins("JOIN routines ON routines.id = routine_triggers.routine_id").
		Where("routine_triggers.kind = ? AND routine_triggers.enabled = ? AND routines.status = ? AND routine_triggers.next_run_at IS NOT NULL AND routine_triggers.next_run_at <= ?",
			"schedule", true, "active", now).
		Order("routine_triggers.next_run_at ASC, routine_triggers.created_at ASC").
		Find(&triggers).Error; err != nil {
		return nil, err
	}

	if len(triggers) == 0 {
		return nil, nil
	}

	// Fetch routines for the triggers in one query.
	routineIDs := make([]string, len(triggers))
	for i, t := range triggers {
		routineIDs[i] = t.RoutineID
	}
	var routines []models.Routine
	if err := s.DB.WithContext(ctx).Where("id IN ?", routineIDs).Find(&routines).Error; err != nil {
		return nil, err
	}
	routineByID := make(map[string]models.Routine, len(routines))
	for _, r := range routines {
		routineByID[r.ID] = r
	}

	rows := make([]dueTriggerRow, 0, len(triggers))
	for _, t := range triggers {
		r, ok := routineByID[t.RoutineID]
		if !ok {
			continue
		}
		rows = append(rows, dueTriggerRow{Trigger: t, Routine: r})
	}
	return rows, nil
}

// processTrigger claims the trigger atomically, calculates the number of runs
// to dispatch (respecting catchUpPolicy), and dispatches each run.
// Returns the number of runs dispatched.
func (s *RoutineSchedulerService) processTrigger(ctx context.Context, trigger models.RoutineTrigger, routine models.Routine, now time.Time) (int, error) {
	if trigger.CronExpression == nil || trigger.Timezone == nil || trigger.NextRunAt == nil {
		return 0, nil
	}

	expr := *trigger.CronExpression
	tz := *trigger.Timezone
	nextRunAt := *trigger.NextRunAt

	// Calculate the next run time after now.
	claimedNextRunAt, err := nextCronTickInTimezone(expr, tz, now)
	if err != nil {
		return 0, fmt.Errorf("next cron tick: %w", err)
	}

	// Determine how many missed runs to dispatch.
	runCount := 1
	if routine.CatchUpPolicy == catchUpPolicyEnqueueMissedWithCap {
		runCount = 0
		cursor := &nextRunAt
		for cursor != nil && !cursor.After(now) && runCount < maxCatchUpRuns {
			runCount++
			cursor, err = nextCronTickInTimezone(expr, tz, *cursor)
			if err != nil {
				break
			}
		}
		if runCount == 0 {
			runCount = 1
		}
	}

	// Atomically claim the trigger by updating nextRunAt.
	// The WHERE clause includes the current nextRunAt to detect races.
	updateResult := s.DB.WithContext(ctx).Model(&models.RoutineTrigger{}).
		Where("id = ? AND enabled = ? AND next_run_at = ?", trigger.ID, true, nextRunAt).
		Updates(map[string]interface{}{
			"next_run_at": claimedNextRunAt,
			"updated_at":  now,
		})
	if updateResult.Error != nil {
		return 0, updateResult.Error
	}
	if updateResult.RowsAffected == 0 {
		// Another instance claimed this trigger first; skip.
		return 0, nil
	}

	// Dispatch routine runs.
	dispatched := 0
	for i := 0; i < runCount; i++ {
		if err := s.dispatchRoutineRun(ctx, &routine, &trigger); err != nil {
			slog.Error("routine scheduler: dispatch failed",
				"routineId", routine.ID,
				"triggerId", trigger.ID,
				"error", err)
		} else {
			dispatched++
		}
	}
	return dispatched, nil
}

// dispatchRoutineRun creates a RoutineRun record, enforces the concurrency
// policy, and (when appropriate) creates an Issue and queues an agent wakeup.
func (s *RoutineSchedulerService) dispatchRoutineRun(ctx context.Context, routine *models.Routine, trigger *models.RoutineTrigger) error {
	now := time.Now()

	// Create the RoutineRun record.
	triggerID := ""
	if trigger != nil {
		triggerID = trigger.ID
	}
	run := &models.RoutineRun{
		CompanyID:   routine.CompanyID,
		RoutineID:   routine.ID,
		Source:      "schedule",
		Status:      "received",
		TriggeredAt: now,
	}
	if triggerID != "" {
		run.TriggerID = &triggerID
	}
	if err := s.DB.WithContext(ctx).Create(run).Error; err != nil {
		return fmt.Errorf("create routine run: %w", err)
	}

	// Recalculate the next run time for persistence.
	var nextRunAt *time.Time
	if trigger != nil && trigger.CronExpression != nil && trigger.Timezone != nil {
		nt, _ := nextCronTickInTimezone(*trigger.CronExpression, *trigger.Timezone, now)
		nextRunAt = nt
	}

	// Check concurrency policy.
	if routine.ConcurrencyPolicy != concurrencyPolicyAlwaysEnqueue {
		activeIssue, err := s.findLiveExecutionIssue(ctx, routine)
		if err != nil {
			return fmt.Errorf("check live execution issue: %w", err)
		}
		if activeIssue != nil {
			status := "coalesced"
			if routine.ConcurrencyPolicy == concurrencyPolicySkipIfActive {
				status = "skipped"
			}
			coalescedRunID := activeIssue.OriginRunID
			updates := map[string]interface{}{
				"status":       status,
				"completed_at": now,
				"updated_at":   now,
			}
			if coalescedRunID != nil {
				updates["coalesced_into_run_id"] = *coalescedRunID
			}
			s.DB.WithContext(ctx).Model(run).Updates(updates)
			s.updateRoutineTouchedState(ctx, routine.ID, triggerID, now, status, activeIssue.ID, nextRunAt)
			return nil
		}
	}

	// Create the execution issue.
	issue := &models.Issue{
		CompanyID:       routine.CompanyID,
		ProjectID:       &routine.ProjectID,
		GoalID:          routine.GoalID,
		ParentID:        routine.ParentIssueID,
		Title:           routine.Title,
		Description:     routine.Description,
		Status:          "todo",
		Priority:        routine.Priority,
		AssigneeAgentID: &routine.AssigneeAgentID,
		OriginKind:      "routine_execution",
		OriginID:        &routine.ID,
		OriginRunID:     &run.ID,
	}

	if s.Issues != nil {
		if _, err := s.Issues.CreateIssue(ctx, issue); err != nil {
			// Check for unique constraint violation (concurrent dispatch).
			if isUniqueConstraintError(err) && routine.ConcurrencyPolicy != concurrencyPolicyAlwaysEnqueue {
				existingIssue, _ := s.findLiveExecutionIssue(ctx, routine)
				if existingIssue != nil {
					status := "coalesced"
					if routine.ConcurrencyPolicy == concurrencyPolicySkipIfActive {
						status = "skipped"
					}
					coalescedRunID := existingIssue.OriginRunID
					updates := map[string]interface{}{
						"status":       status,
						"completed_at": now,
						"updated_at":   now,
					}
					if coalescedRunID != nil {
						updates["coalesced_into_run_id"] = *coalescedRunID
					}
					s.DB.WithContext(ctx).Model(run).Updates(updates)
					s.updateRoutineTouchedState(ctx, routine.ID, triggerID, now, status, existingIssue.ID, nextRunAt)
					return nil
				}
			}
			// Mark the run as failed.
			reason := err.Error()
			s.DB.WithContext(ctx).Model(run).Updates(map[string]interface{}{
				"status":         "failed",
				"failure_reason": reason,
				"completed_at":   now,
				"updated_at":     now,
			})
			s.updateRoutineTouchedState(ctx, routine.ID, triggerID, now, "failed", "", nextRunAt)
			return fmt.Errorf("create issue: %w", err)
		}
	}

	// Link the run to the issue and mark it as issue_created.
	s.DB.WithContext(ctx).Model(run).Updates(map[string]interface{}{
		"status":          "issue_created",
		"linked_issue_id": issue.ID,
		"updated_at":      now,
	})
	s.updateRoutineTouchedState(ctx, routine.ID, triggerID, now, "issue_created", issue.ID, nextRunAt)

	// Wake the assignee agent to work on the newly created issue.
	if s.Heartbeat != nil && routine.AssigneeAgentID != "" {
		contextMap := map[string]interface{}{
			"issueId": issue.ID,
			"source":  "routine.dispatch",
		}
		_, _ = s.Heartbeat.Wakeup(ctx, routine.CompanyID, routine.AssigneeAgentID, WakeupOptions{
			Source:        "assignment",
			TriggerDetail: "system",
			Context:       contextMap,
		})
	}

	// Log activity.
	s.logRunActivity(ctx, routine, run, triggerID)
	return nil
}

// findLiveExecutionIssue returns the most-recently-updated open issue for the
// routine that has an active heartbeat run, or nil if none exists.
func (s *RoutineSchedulerService) findLiveExecutionIssue(ctx context.Context, routine *models.Routine) (*models.Issue, error) {
	type minimalIssue struct {
		ID          string  `gorm:"column:id"`
		OriginRunID *string `gorm:"column:origin_run_id"`
	}

	liveStatuses := []string{"queued", "running"}
	openStatuses := []string{"backlog", "todo", "in_progress", "in_review", "blocked"}

	// Primary check: issues whose execution_run_id maps to a live heartbeat run.
	var row minimalIssue
	err := s.DB.WithContext(ctx).Raw(`
		SELECT i.id, i.origin_run_id
		FROM issues i
		JOIN heartbeat_runs hr ON hr.id = i.execution_run_id AND hr.status IN ?
		WHERE i.company_id = ?
		  AND i.origin_kind = 'routine_execution'
		  AND i.origin_id = ?
		  AND i.status IN ?
		  AND i.hidden_at IS NULL
		ORDER BY i.updated_at DESC, i.created_at DESC
		LIMIT 1
	`, liveStatuses, routine.CompanyID, routine.ID, openStatuses).Scan(&row).Error

	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	if row.ID != "" {
		return &models.Issue{ID: row.ID, OriginRunID: row.OriginRunID}, nil
	}

	// Fallback: check for open issues where a live heartbeat run holds a
	// reference to the issue in its context_snapshot.
	// The JSON extraction syntax differs between PostgreSQL and SQLite; we
	// build it here based on the active driver and treat any query error as a
	// non-fatal miss (the primary check above covers the common case).
	jsonExtractExpr := jsonExtractIssueID(s.DB)
	if jsonExtractExpr != "" {
		row = minimalIssue{} // reset
		_ = s.DB.WithContext(ctx).Raw(`
			SELECT i.id, i.origin_run_id
			FROM issues i
			JOIN heartbeat_runs hr ON hr.company_id = i.company_id
			  AND hr.status IN ?
			  AND `+jsonExtractExpr+` = i.id
			WHERE i.company_id = ?
			  AND i.origin_kind = 'routine_execution'
			  AND i.origin_id = ?
			  AND i.status IN ?
			  AND i.hidden_at IS NULL
			ORDER BY i.updated_at DESC, i.created_at DESC
			LIMIT 1
		`, liveStatuses, routine.CompanyID, routine.ID, openStatuses).Scan(&row).Error

		if row.ID != "" {
			return &models.Issue{ID: row.ID, OriginRunID: row.OriginRunID}, nil
		}
	}

	return nil, nil
}

// jsonExtractIssueID returns a SQL fragment that extracts the "issueId" field
// from the heartbeat_runs.context_snapshot column, dialect-aware.
func jsonExtractIssueID(db *gorm.DB) string {
	switch db.Name() {
	case "postgres":
		return "hr.context_snapshot::jsonb->>'issueId'"
	case "sqlite":
		return "json_extract(hr.context_snapshot, '$.issueId')"
	default:
		return ""
	}
}

// updateRoutineTouchedState updates last_triggered_at and last_fired_at on the
// routine and trigger respectively, and advances next_run_at on the trigger.
func (s *RoutineSchedulerService) updateRoutineTouchedState(
	ctx context.Context,
	routineID, triggerID string,
	now time.Time,
	status, issueID string,
	nextRunAt *time.Time,
) {
	lastResult := buildLastResult(status, issueID)

	s.DB.WithContext(ctx).Model(&models.Routine{}).
		Where("id = ?", routineID).
		Updates(map[string]interface{}{
			"last_triggered_at": now,
			"updated_at":        now,
		})

	if triggerID != "" {
		updates := map[string]interface{}{
			"last_fired_at": now,
			"last_result":   lastResult,
			"updated_at":    now,
		}
		if nextRunAt != nil {
			updates["next_run_at"] = *nextRunAt
		}
		s.DB.WithContext(ctx).Model(&models.RoutineTrigger{}).
			Where("id = ?", triggerID).
			Updates(updates)
	}
}

func buildLastResult(status, issueID string) string {
	switch status {
	case "issue_created":
		if issueID != "" {
			return "Created execution issue " + issueID
		}
		return "Created execution issue"
	case "coalesced":
		return "Coalesced into an existing live execution issue"
	case "skipped":
		return "Skipped because a live execution issue already exists"
	case "failed":
		return "Execution failed"
	default:
		return status
	}
}

func (s *RoutineSchedulerService) logRunActivity(ctx context.Context, routine *models.Routine, run *models.RoutineRun, triggerID string) {
	if s.Activity == nil {
		return
	}
	details := map[string]interface{}{
		"routineId": routine.ID,
		"source":    run.Source,
		"status":    run.Status,
	}
	if triggerID != "" {
		details["triggerId"] = triggerID
	}
	_, _ = s.Activity.Log(ctx, LogEntry{
		CompanyID:  routine.CompanyID,
		ActorType:  "system",
		ActorID:    "routine-scheduler",
		Action:     "routine.run_triggered",
		EntityType: "routine_run",
		EntityID:   run.ID,
		Details:    details,
	})
}

// isUniqueConstraintError returns true when err is a PostgreSQL unique
// constraint violation (SQLSTATE 23505).
func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "23505") ||
		strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "UNIQUE constraint")
}

// StartRoutineScheduler launches the routine scheduler as a background goroutine that
// ticks every intervalMs milliseconds.  The goroutine stops when ctx is cancelled.
func StartRoutineScheduler(ctx context.Context, svc *RoutineSchedulerService, intervalMs int) {
	if intervalMs <= 0 {
		intervalMs = 60_000 // default: 60 s
	}
	ticker := time.NewTicker(time.Duration(intervalMs) * time.Millisecond)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case t := <-ticker.C:
				result, err := svc.TickScheduledTriggers(ctx, t)
				if err != nil {
					slog.Error("routine scheduler tick failed", "error", err)
				} else if result.Triggered > 0 {
					slog.Info("routine scheduler tick", "triggered", result.Triggered)
				}
			}
		}
	}()
}
