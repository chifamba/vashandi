package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const heartbeatTaskKey = "__heartbeat__"

const (
	defaultSessionCompactionMaxRuns        = 200
	defaultSessionCompactionMaxInputTokens = 2_000_000
	defaultSessionCompactionMaxAgeHours    = 72
)

type SessionCompactionPolicy struct {
	Enabled            bool
	MaxSessionRuns     int
	MaxRawInputTokens  int
	MaxSessionAgeHours int
}

type CompactionDecision struct {
	Rotate          bool
	Reason          string
	HandoffMarkdown string
	PreviousRunID   string
}

type taskSessionUpsertInput struct {
	CompanyID         string
	AgentID           string
	AdapterType       string
	TaskKey           string
	SessionParamsJSON json.RawMessage
	SessionDisplayID  string
	LastRunID         *string
	LastError         *string
}

func (s *HeartbeatService) getTaskSession(ctx context.Context, companyID, agentID, adapterType, taskKey string) (*models.AgentTaskSession, error) {
	var session models.AgentTaskSession
	err := s.DB.WithContext(ctx).
		Where("company_id = ? AND agent_id = ? AND adapter_type = ? AND task_key = ?", companyID, agentID, adapterType, taskKey).
		First(&session).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &session, nil
}

func (s *HeartbeatService) listTaskSessions(ctx context.Context, companyID, agentID string) ([]models.AgentTaskSession, error) {
	var sessions []models.AgentTaskSession
	err := s.DB.WithContext(ctx).
		Where("company_id = ? AND agent_id = ?", companyID, agentID).
		Order("updated_at desc").
		Find(&sessions).Error
	return sessions, err
}

func (s *HeartbeatService) upsertTaskSession(ctx context.Context, input taskSessionUpsertInput) error {
	existing, err := s.getTaskSession(ctx, input.CompanyID, input.AgentID, input.AdapterType, input.TaskKey)
	if err != nil {
		return err
	}
	values := map[string]interface{}{
		"company_id":          input.CompanyID,
		"agent_id":            input.AgentID,
		"adapter_type":        input.AdapterType,
		"task_key":            input.TaskKey,
		"session_params_json": datatypes.JSON(input.SessionParamsJSON),
		"session_display_id":  stringOrNil(input.SessionDisplayID),
		"last_run_id":         input.LastRunID,
		"last_error":          input.LastError,
		"updated_at":          time.Now(),
	}
	if existing == nil {
		return s.DB.WithContext(ctx).Create(&models.AgentTaskSession{
			CompanyID:         input.CompanyID,
			AgentID:           input.AgentID,
			AdapterType:       input.AdapterType,
			TaskKey:           input.TaskKey,
			SessionParamsJSON: datatypes.JSON(input.SessionParamsJSON),
			SessionDisplayID:  stringOrNil(input.SessionDisplayID),
			LastRunID:         input.LastRunID,
			LastError:         input.LastError,
		}).Error
	}
	return s.DB.WithContext(ctx).Model(&models.AgentTaskSession{}).Where("id = ?", existing.ID).Updates(values).Error
}

func (s *HeartbeatService) clearTaskSessions(ctx context.Context, companyID, agentID string, taskKey, adapterType *string) (int64, error) {
	query := s.DB.WithContext(ctx).Model(&models.AgentTaskSession{}).Where("company_id = ? AND agent_id = ?", companyID, agentID)
	if taskKey != nil && *taskKey != "" {
		query = query.Where("task_key = ?", *taskKey)
	}
	if adapterType != nil && *adapterType != "" {
		query = query.Where("adapter_type = ?", *adapterType)
	}
	result := query.Delete(&models.AgentTaskSession{})
	return result.RowsAffected, result.Error
}

func (s *HeartbeatService) getRuntimeState(ctx context.Context, agentID string) (*models.AgentRuntimeState, error) {
	var state models.AgentRuntimeState
	err := s.DB.WithContext(ctx).Where("agent_id = ?", agentID).First(&state).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &state, nil
}

func (s *HeartbeatService) upsertRuntimeState(ctx context.Context, agent *models.Agent, run *models.HeartbeatRun, result *AgentRunResult, usage UsageTotals, status string) error {
	if agent == nil {
		return nil
	}
	existing, err := s.getRuntimeState(ctx, agent.ID)
	if err != nil {
		return err
	}
	sessionID := ""
	if result != nil {
		sessionID = sessionIDFromRaw(result.SessionParamsJSON)
	}
	updates := map[string]interface{}{
		"company_id":                agent.CompanyID,
		"adapter_type":              agent.AdapterType,
		"last_run_id":               &run.ID,
		"last_run_status":           status,
		"last_error":                run.Error,
		"total_input_tokens":        int64(usage.InputTokens),
		"total_cached_input_tokens": int64(usage.CachedInputTokens),
		"total_output_tokens":       int64(usage.OutputTokens),
		"total_cost_cents":          int64(costUSDToCents(0)),
		"updated_at":                time.Now(),
	}
	if result != nil {
		updates["total_cost_cents"] = int64(costUSDToCents(result.CostUsd))
	}
	if sessionID != "" {
		updates["session_id"] = sessionID
	}
	if existing == nil {
		record := &models.AgentRuntimeState{
			AgentID:                agent.ID,
			CompanyID:              agent.CompanyID,
			AdapterType:            agent.AdapterType,
			SessionID:              stringOrNil(sessionID),
			LastRunID:              &run.ID,
			LastRunStatus:          stringOrNil(status),
			TotalInputTokens:       int64(usage.InputTokens),
			TotalCachedInputTokens: int64(usage.CachedInputTokens),
			TotalOutputTokens:      int64(usage.OutputTokens),
			TotalCostCents:         int64(costUSDToCents(0)),
			LastError:              run.Error,
		}
		if result != nil {
			record.TotalCostCents = int64(costUSDToCents(result.CostUsd))
		}
		return s.DB.WithContext(ctx).Create(record).Error
	}
	return s.DB.WithContext(ctx).Model(&models.AgentRuntimeState{}).Where("agent_id = ?", agent.ID).Updates(updates).Error
}

func (s *HeartbeatService) evaluateSessionCompaction(ctx context.Context, agentID, sessionID string) (*CompactionDecision, error) {
	if sessionID == "" {
		return &CompactionDecision{}, nil
	}
	var agent models.Agent
	if err := s.DB.WithContext(ctx).Where("id = ?", agentID).First(&agent).Error; err != nil {
		return nil, err
	}
	policy := parseSessionCompactionPolicy(&agent)
	if !policy.Enabled || (policy.MaxSessionRuns <= 0 && policy.MaxRawInputTokens <= 0 && policy.MaxSessionAgeHours <= 0) {
		return &CompactionDecision{}, nil
	}

	var taskSessions []models.AgentTaskSession
	_ = s.DB.WithContext(ctx).Where("agent_id = ? AND session_display_id = ?", agentID, sessionID).Order("updated_at desc").Limit(1).Find(&taskSessions).Error

	fetchLimit := 4
	if policy.MaxSessionRuns > 0 && policy.MaxSessionRuns+1 > fetchLimit {
		fetchLimit = policy.MaxSessionRuns + 1
	}
	var runs []models.HeartbeatRun
	if err := s.DB.WithContext(ctx).
		Where("agent_id = ? AND session_id_after = ?", agentID, sessionID).
		Order("created_at desc").
		Limit(fetchLimit).
		Find(&runs).Error; err != nil {
		return nil, err
	}
	if len(runs) == 0 {
		return &CompactionDecision{}, nil
	}
	latestRun := runs[0]
	oldestRun := runs[len(runs)-1]
	if policy.MaxSessionAgeHours > 0 {
		var oldest models.HeartbeatRun
		if err := s.DB.WithContext(ctx).
			Select("id, created_at").
			Where("agent_id = ? AND session_id_after = ?", agentID, sessionID).
			Order("created_at asc, id asc").
			First(&oldest).Error; err == nil {
			oldestRun = oldest
		}
	}
	usage := parseJSONObject(latestRun.UsageJSON)
	reason := ""
	switch {
	case policy.MaxSessionRuns > 0 && len(runs) > policy.MaxSessionRuns:
		reason = fmt.Sprintf("session exceeded %d runs", policy.MaxSessionRuns)
	case policy.MaxRawInputTokens > 0 && readIntValue(usage["rawInputTokens"], usage["inputTokens"]) >= policy.MaxRawInputTokens:
		reason = fmt.Sprintf("session raw input reached %d tokens (threshold %d)", readIntValue(usage["rawInputTokens"], usage["inputTokens"]), policy.MaxRawInputTokens)
	case policy.MaxSessionAgeHours > 0:
		ageHours := int(latestRun.CreatedAt.Sub(oldestRun.CreatedAt).Hours())
		if ageHours >= policy.MaxSessionAgeHours {
			reason = fmt.Sprintf("session age reached %d hours", ageHours)
		}
	}
	if reason == "" {
		return &CompactionDecision{PreviousRunID: latestRun.ID}, nil
	}
	summary := SummarizeHeartbeatRunResultJSON(parseJSONObject(latestRun.ResultJSON))
	lastSummary := firstNonEmpty(
		readNonEmptyString(summary["summary"]),
		readNonEmptyString(summary["result"]),
		readNonEmptyString(summary["message"]),
		derefString(latestRun.Error),
	)
	handoff := []string{
		"Paperclip session handoff:",
		fmt.Sprintf("- Previous session: %s", sessionID),
		fmt.Sprintf("- Rotation reason: %s", reason),
	}
	if len(taskSessions) > 0 && taskSessions[0].TaskKey != "" {
		handoff = append(handoff, fmt.Sprintf("- Task key: %s", taskSessions[0].TaskKey))
	}
	if lastSummary != "" {
		handoff = append(handoff, fmt.Sprintf("- Last run summary: %s", lastSummary))
	}
	handoff = append(handoff, "Continue from the current task state. Rebuild only the minimum context you need.")
	return &CompactionDecision{
		Rotate:          true,
		Reason:          reason,
		HandoffMarkdown: strings.Join(handoff, "\n"),
		PreviousRunID:   latestRun.ID,
	}, nil
}

func parseSessionCompactionPolicy(agent *models.Agent) SessionCompactionPolicy {
	base := SessionCompactionPolicy{
		Enabled:            false,
		MaxSessionRuns:     defaultSessionCompactionMaxRuns,
		MaxRawInputTokens:  defaultSessionCompactionMaxInputTokens,
		MaxSessionAgeHours: defaultSessionCompactionMaxAgeHours,
	}
	switch agent.AdapterType {
	case "claude_local", "codex_local", "cursor", "gemini_local", "hermes_local", "opencode_local", "pi_local":
		base.Enabled = true
	}
	switch agent.AdapterType {
	case "claude_local", "codex_local", "hermes_local":
		base.MaxSessionRuns = 0
		base.MaxRawInputTokens = 0
		base.MaxSessionAgeHours = 0
	}
	runtime := parseJSONObject(agent.RuntimeConfig)
	heartbeat := nestedObject(runtime, "heartbeat")
	compaction := firstObject(heartbeat["sessionCompaction"], heartbeat["sessionRotation"], runtime["sessionCompaction"])
	if enabled, ok := readBool(compaction["enabled"]); ok {
		base.Enabled = enabled
	}
	if value, ok := readInt(compaction["maxSessionRuns"]); ok {
		base.MaxSessionRuns = value
	}
	if value, ok := readInt(compaction["maxRawInputTokens"]); ok {
		base.MaxRawInputTokens = value
	}
	if value, ok := readInt(compaction["maxSessionAgeHours"]); ok {
		base.MaxSessionAgeHours = value
	}
	return base
}

func deriveTaskKeyWithHeartbeatFallback(contextSnapshot, payload map[string]interface{}) string {
	if key := firstNonEmpty(
		readNonEmptyString(contextSnapshot["taskKey"]),
		readNonEmptyString(contextSnapshot["taskId"]),
		readNonEmptyString(contextSnapshot["issueId"]),
		readNonEmptyString(payload["taskKey"]),
		readNonEmptyString(payload["taskId"]),
		readNonEmptyString(payload["issueId"]),
	); key != "" {
		return key
	}
	if readNonEmptyString(contextSnapshot["wakeSource"]) == "timer" {
		return heartbeatTaskKey
	}
	return ""
}

func shouldResetTaskSessionForWake(contextSnapshot map[string]interface{}) bool {
	if value, ok := contextSnapshot["forceFreshSession"].(bool); ok && value {
		return true
	}
	switch readNonEmptyString(contextSnapshot["wakeReason"]) {
	case "issue_assigned", "execution_review_requested", "execution_approval_requested", "execution_changes_requested":
		return true
	default:
		return false
	}
}

func sessionIDFromRaw(raw json.RawMessage) string {
	return sessionIDFromParams(parseJSONObject(raw))
}

func sessionIDFromParams(params map[string]interface{}) string {
	return firstNonEmpty(readNonEmptyString(params["sessionDisplayId"]), readNonEmptyString(params["sessionId"]))
}

func parseJSONObject(raw interface{}) map[string]interface{} {
	switch value := raw.(type) {
	case nil:
		return map[string]interface{}{}
	case map[string]interface{}:
		cloned := make(map[string]interface{}, len(value))
		for k, v := range value {
			cloned[k] = v
		}
		return cloned
	case datatypes.JSON:
		if len(value) == 0 {
			return map[string]interface{}{}
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal(value, &parsed); err != nil || parsed == nil {
			return map[string]interface{}{}
		}
		return parsed
	case json.RawMessage:
		if len(value) == 0 {
			return map[string]interface{}{}
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal(value, &parsed); err != nil || parsed == nil {
			return map[string]interface{}{}
		}
		return parsed
	case []byte:
		if len(value) == 0 {
			return map[string]interface{}{}
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal(value, &parsed); err != nil || parsed == nil {
			return map[string]interface{}{}
		}
		return parsed
	default:
		return map[string]interface{}{}
	}
}

func nestedObject(parent map[string]interface{}, key string) map[string]interface{} {
	return firstObject(parent[key])
}

func firstObject(values ...interface{}) map[string]interface{} {
	for _, value := range values {
		if object, ok := value.(map[string]interface{}); ok && object != nil {
			return object
		}
	}
	return map[string]interface{}{}
}

func readNonEmptyString(value interface{}) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func readBool(value interface{}) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "1", "yes", "on":
			return true, true
		case "false", "0", "no", "off":
			return false, true
		}
	}
	return false, false
}

func readInt(value interface{}) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return int(parsed), true
		}
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return parsed, true
		}
	}
	return 0, false
}

func readIntValue(values ...interface{}) int {
	for _, value := range values {
		if parsed, ok := readInt(value); ok {
			return parsed
		}
	}
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func stringOrNil(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
