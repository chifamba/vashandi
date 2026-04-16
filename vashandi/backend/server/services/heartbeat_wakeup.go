package services

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type HeartbeatPolicy struct {
	Enabled           bool
	IntervalSec       int
	WakeOnDemand      bool
	MaxConcurrentRuns int
}

type HeartbeatTimerTickResult struct {
	Checked  int
	Enqueued int
	Skipped  int
}

func parseHeartbeatPolicy(agent *models.Agent) HeartbeatPolicy {
	runtimeConfig := parseJSONObject(agent.RuntimeConfig)
	heartbeat := nestedObject(runtimeConfig, "heartbeat")
	policy := HeartbeatPolicy{
		Enabled:           false,
		IntervalSec:       0,
		WakeOnDemand:      true,
		MaxConcurrentRuns: 1,
	}
	if enabled, ok := readBool(heartbeat["enabled"]); ok {
		policy.Enabled = enabled
	}
	if interval, ok := readInt(heartbeat["intervalSec"]); ok && interval > 0 {
		policy.IntervalSec = interval
	}
	if wakeOnDemand, ok := readBool(firstNonNil(heartbeat["wakeOnDemand"], heartbeat["wakeOnAssignment"], heartbeat["wakeOnOnDemand"], heartbeat["wakeOnAutomation"])); ok {
		policy.WakeOnDemand = wakeOnDemand
	}
	if maxConcurrentRuns, ok := readInt(heartbeat["maxConcurrentRuns"]); ok && maxConcurrentRuns > 0 {
		policy.MaxConcurrentRuns = maxConcurrentRuns
	}
	return policy
}

func (s *HeartbeatService) enqueueWakeup(ctx context.Context, companyID, agentID string, opts WakeupOptions) (*models.HeartbeatRun, error) {
	var agent models.Agent
	if err := s.DB.WithContext(ctx).Where("id = ? AND company_id = ?", agentID, companyID).First(&agent).Error; err != nil {
		return nil, err
	}
	source := firstNonEmpty(opts.Source, "on_demand")
	triggerDetail := firstNonEmpty(opts.TriggerDetail, "manual")
	contextSnapshot := parseJSONObject(opts.Context)
	payload := parseJSONObject(opts.Payload)
	if reason := strings.TrimSpace(opts.Reason); reason != "" && readNonEmptyString(contextSnapshot["wakeReason"]) == "" {
		contextSnapshot["wakeReason"] = reason
	}
	if readNonEmptyString(contextSnapshot["wakeSource"]) == "" {
		contextSnapshot["wakeSource"] = source
	}
	if readNonEmptyString(contextSnapshot["wakeTriggerDetail"]) == "" {
		contextSnapshot["wakeTriggerDetail"] = triggerDetail
	}
	if issueID := firstNonEmpty(readNonEmptyString(contextSnapshot["issueId"]), readNonEmptyString(payload["issueId"])); issueID != "" {
		contextSnapshot["issueId"] = issueID
	}
	taskKey := deriveTaskKeyWithHeartbeatFallback(contextSnapshot, payload)
	if taskKey != "" && readNonEmptyString(contextSnapshot["taskKey"]) == "" {
		contextSnapshot["taskKey"] = taskKey
	}

	if opts.IdempotencyKey != "" {
		var existing models.AgentWakeupRequest
		err := s.DB.WithContext(ctx).
			Where("company_id = ? AND agent_id = ? AND idempotency_key = ?", companyID, agentID, opts.IdempotencyKey).
			Order("requested_at desc").
			First(&existing).Error
		if err == nil {
			if existing.RunID == nil {
				return nil, nil
			}
			var run models.HeartbeatRun
			if loadErr := s.DB.WithContext(ctx).Where("id = ?", *existing.RunID).First(&run).Error; loadErr == nil {
				return &run, nil
			}
			return nil, nil
		}
	}

	resetTaskSession := shouldResetTaskSessionForWake(contextSnapshot)
	sessionBefore := ""
	if !resetTaskSession {
		if taskKey != "" {
			if taskSession, err := s.getTaskSession(ctx, companyID, agentID, agent.AdapterType, taskKey); err == nil && taskSession != nil {
				sessionBefore = firstNonEmpty(derefString(taskSession.SessionDisplayID), sessionIDFromRaw(json.RawMessage(taskSession.SessionParamsJSON)))
			}
		}
		if sessionBefore == "" {
			if runtimeState, err := s.getRuntimeState(ctx, agentID); err == nil && runtimeState != nil && runtimeState.SessionID != nil {
				sessionBefore = *runtimeState.SessionID
			}
		}
	}

	var run *models.HeartbeatRun
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var activeRuns []models.HeartbeatRun
		if err := tx.Where("agent_id = ? AND status IN ?", agentID, []string{"queued", "running"}).Order("created_at desc").Find(&activeRuns).Error; err != nil {
			return err
		}
		issueID := readNonEmptyString(contextSnapshot["issueId"])
		taskID := firstNonEmpty(readNonEmptyString(contextSnapshot["taskId"]), issueID)
		for _, candidate := range activeRuns {
			if !isSameTaskScope(&candidate, taskKey, taskID, issueID) {
				continue
			}
			merged := mergeCoalescedContextSnapshot(candidate.ContextSnapshot, contextSnapshot)
			payloadJSON, _ := json.Marshal(merged)
			if err := tx.Model(&models.HeartbeatRun{}).Where("id = ?", candidate.ID).Updates(map[string]interface{}{
				"context_snapshot": datatypes.JSON(payloadJSON),
				"updated_at":       time.Now(),
			}).Error; err != nil {
				return err
			}
			request := &models.AgentWakeupRequest{
				CompanyID:            companyID,
				AgentID:              agentID,
				Source:               source,
				TriggerDetail:        stringOrNil(triggerDetail),
				Reason:               stringOrNil(opts.Reason),
				Payload:              marshalJSON(payload),
				Status:               "coalesced",
				CoalescedCount:       1,
				RequestedByActorType: stringOrNil(opts.RequestedByActorType),
				RequestedByActorID:   stringOrNil(opts.RequestedByActorID),
				IdempotencyKey:       stringOrNil(opts.IdempotencyKey),
				RunID:                &candidate.ID,
				FinishedAt:           timePtr(time.Now()),
			}
			if err := tx.Create(request).Error; err != nil {
				return err
			}
			run = &candidate
			run.ContextSnapshot = datatypes.JSON(payloadJSON)
			return nil
		}

		request := &models.AgentWakeupRequest{
			CompanyID:            companyID,
			AgentID:              agentID,
			Source:               source,
			TriggerDetail:        stringOrNil(triggerDetail),
			Reason:               stringOrNil(opts.Reason),
			Payload:              marshalJSON(payload),
			Status:               "queued",
			RequestedByActorType: stringOrNil(opts.RequestedByActorType),
			RequestedByActorID:   stringOrNil(opts.RequestedByActorID),
			IdempotencyKey:       stringOrNil(opts.IdempotencyKey),
		}
		if err := tx.Create(request).Error; err != nil {
			return err
		}
		contextJSON, _ := json.Marshal(contextSnapshot)
		newRun := &models.HeartbeatRun{
			CompanyID:        companyID,
			AgentID:          agentID,
			InvocationSource: source,
			TriggerDetail:    stringOrNil(triggerDetail),
			Status:           "queued",
			WakeupRequestID:  &request.ID,
			ContextSnapshot:  datatypes.JSON(contextJSON),
			SessionIDBefore:  stringOrNil(sessionBefore),
			TaskID:           firstNonEmpty(readNonEmptyString(contextSnapshot["taskId"]), readNonEmptyString(contextSnapshot["issueId"]), taskKey),
		}
		if err := tx.Create(newRun).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.AgentWakeupRequest{}).Where("id = ?", request.ID).Update("run_id", newRun.ID).Error; err != nil {
			return err
		}
		run = newRun
		return nil
	})
	if err != nil {
		return nil, err
	}
	if run != nil && run.Status == "queued" {
		go s.ResumeQueuedRuns(context.Background(), agentID)
	}
	return run, nil
}

func mergeCoalescedContextSnapshot(existingRaw interface{}, incoming map[string]interface{}) map[string]interface{} {
	merged := parseJSONObject(existingRaw)
	for key, value := range incoming {
		merged[key] = value
	}
	commentIDs := uniqueStrings(extractWakeCommentIDs(merged), extractWakeCommentIDs(incoming), []string{readNonEmptyString(merged["commentId"]), readNonEmptyString(incoming["commentId"])})
	if len(commentIDs) > 0 {
		merged["wakeCommentIds"] = commentIDs
		merged["commentId"] = commentIDs[len(commentIDs)-1]
		merged["wakeCommentId"] = commentIDs[len(commentIDs)-1]
	}
	return merged
}

func (s *HeartbeatService) updateWakeupRequestStatus(ctx context.Context, wakeupRequestID, status, errMsg string, finishedAt time.Time, runID *string) error {
	updates := map[string]interface{}{
		"status":      status,
		"updated_at":  time.Now(),
		"finished_at": finishedAt,
	}
	if errMsg != "" {
		updates["error"] = errMsg
	}
	if runID != nil && *runID != "" {
		updates["run_id"] = *runID
	}
	return s.DB.WithContext(ctx).Model(&models.AgentWakeupRequest{}).Where("id = ?", wakeupRequestID).Updates(updates).Error
}

func (s *HeartbeatService) finalizeAgentStatus(ctx context.Context, agentID, runStatus string) error {
	var agent models.Agent
	if err := s.DB.WithContext(ctx).Where("id = ?", agentID).First(&agent).Error; err != nil {
		return err
	}
	if agent.Status == "paused" || agent.Status == "terminated" {
		return nil
	}
	var runningCount int64
	if err := s.DB.WithContext(ctx).Model(&models.HeartbeatRun{}).Where("agent_id = ? AND status = ?", agentID, "running").Count(&runningCount).Error; err != nil {
		return err
	}
	nextStatus := "error"
	if runningCount > 0 {
		nextStatus = "running"
	} else if runStatus == "completed" {
		nextStatus = "idle"
	}
	return s.DB.WithContext(ctx).Model(&models.Agent{}).Where("id = ?", agentID).Updates(map[string]interface{}{
		"status":            nextStatus,
		"last_heartbeat_at": time.Now(),
		"updated_at":        time.Now(),
	}).Error
}

func (s *HeartbeatService) TickTimers(ctx context.Context, now time.Time) (HeartbeatTimerTickResult, error) {
	var agents []models.Agent
	if err := s.DB.WithContext(ctx).Where("status NOT IN ?", []string{"paused", "terminated", "pending_approval"}).Find(&agents).Error; err != nil {
		return HeartbeatTimerTickResult{}, err
	}
	result := HeartbeatTimerTickResult{}
	for _, agent := range agents {
		policy := parseHeartbeatPolicy(&agent)
		if !policy.Enabled || policy.IntervalSec <= 0 {
			continue
		}
		result.Checked++
		baseline := agent.CreatedAt
		if agent.LastHeartbeatAt != nil {
			baseline = *agent.LastHeartbeatAt
		}
		if now.Sub(baseline) < time.Duration(policy.IntervalSec)*time.Second {
			continue
		}
		run, err := s.enqueueWakeup(ctx, agent.CompanyID, agent.ID, WakeupOptions{
			Source:               "timer",
			TriggerDetail:        "system",
			Reason:               "heartbeat_timer",
			RequestedByActorType: "system",
			RequestedByActorID:   "heartbeat_scheduler",
			Context: map[string]interface{}{
				"source": "scheduler",
				"reason": "interval_elapsed",
				"now":    now.Format(time.RFC3339),
			},
		})
		if err != nil || run == nil {
			result.Skipped++
			continue
		}
		result.Enqueued++
	}
	return result, nil
}

func StartHeartbeatScheduler(ctx context.Context, svc *HeartbeatService, intervalMs int) {
	if svc == nil {
		return
	}
	if intervalMs <= 0 {
		intervalMs = 60_000
	}
	ticker := time.NewTicker(time.Duration(intervalMs) * time.Millisecond)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case tickAt := <-ticker.C:
				_, _ = svc.TickTimers(ctx, tickAt)
			}
		}
	}()
}

func isSameTaskScope(run *models.HeartbeatRun, taskKey, taskID, issueID string) bool {
	contextSnapshot := parseJSONObject(run.ContextSnapshot)
	runTaskKey := deriveTaskKeyWithHeartbeatFallback(contextSnapshot, nil)
	if runTaskKey != "" || taskKey != "" {
		return runTaskKey == taskKey && taskKey != ""
	}
	if issueID != "" {
		return readNonEmptyString(contextSnapshot["issueId"]) == issueID
	}
	if taskID != "" {
		return firstNonEmpty(readNonEmptyString(contextSnapshot["taskId"]), readNonEmptyString(contextSnapshot["issueId"])) == taskID
	}
	return false
}

func extractWakeCommentIDs(contextSnapshot map[string]interface{}) []string {
	raw, ok := contextSnapshot["wakeCommentIds"].([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, entry := range raw {
		if value := readNonEmptyString(entry); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func uniqueStrings(groups ...[]string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, group := range groups {
		for _, value := range group {
			if value == "" {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			out = append(out, value)
		}
	}
	return out
}

func marshalJSON(value map[string]interface{}) datatypes.JSON {
	if len(value) == 0 {
		return nil
	}
	encoded, _ := json.Marshal(value)
	return datatypes.JSON(encoded)
}

func timePtr(value time.Time) *time.Time {
	return &value
}

func firstNonNil(values ...interface{}) interface{} {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}
