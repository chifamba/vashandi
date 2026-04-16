package services

import (
	"encoding/json"
	"math"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"gorm.io/datatypes"
)

type AgentRunResult struct {
	ExitCode          int
	CostUsd           float64
	UsageJSON         json.RawMessage
	ResultJSON        json.RawMessage
	SessionParamsJSON json.RawMessage
}

type UsageTotals struct {
	InputTokens       int
	CachedInputTokens int
	OutputTokens      int
}

// parseAgentRunResultLine extracts structured result fields from a single JSON
// line emitted by the local runner subprocess.
func parseAgentRunResultLine(line string) (*AgentRunResult, bool) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &payload); err != nil {
		return nil, false
	}
	result := &AgentRunResult{}
	recognized := false
	if raw := firstJSONField(payload, "exitCode", "exit_code"); len(raw) > 0 {
		if err := json.Unmarshal(raw, &result.ExitCode); err == nil {
			recognized = true
		}
	}
	if raw := firstJSONField(payload, "costUsd", "cost_usd", "total_cost_usd"); len(raw) > 0 {
		if err := json.Unmarshal(raw, &result.CostUsd); err == nil {
			recognized = true
		}
	}
	if raw := firstJSONField(payload, "usageJson", "usage"); len(raw) > 0 {
		result.UsageJSON = append(json.RawMessage(nil), raw...)
		recognized = true
	}
	if raw := firstJSONField(payload, "resultJson", "result"); len(raw) > 0 {
		result.ResultJSON = append(json.RawMessage(nil), raw...)
		recognized = true
	}
	if raw := firstJSONField(payload, "sessionParamsJson", "sessionParams", "session"); len(raw) > 0 {
		result.SessionParamsJSON = append(json.RawMessage(nil), raw...)
		recognized = true
	}
	return result, recognized
}

func mergeAgentRunResult(dst, src *AgentRunResult) {
	if dst == nil || src == nil {
		return
	}
	if src.ExitCode != 0 {
		dst.ExitCode = src.ExitCode
	}
	if src.CostUsd != 0 {
		dst.CostUsd = src.CostUsd
	}
	if len(src.UsageJSON) > 0 {
		dst.UsageJSON = append(json.RawMessage(nil), src.UsageJSON...)
	}
	if len(src.ResultJSON) > 0 {
		dst.ResultJSON = append(json.RawMessage(nil), src.ResultJSON...)
	}
	if len(src.SessionParamsJSON) > 0 {
		dst.SessionParamsJSON = append(json.RawMessage(nil), src.SessionParamsJSON...)
	}
}

func normalizeUsageJSON(result *AgentRunResult) (datatypes.JSON, UsageTotals) {
	if result == nil || len(result.UsageJSON) == 0 {
		return nil, UsageTotals{}
	}
	payload := parseJSONObject(result.UsageJSON)
	totals := UsageTotals{
		InputTokens:       readIntValue(payload["inputTokens"], payload["input_tokens"], payload["promptTokens"], payload["prompt_tokens"]),
		CachedInputTokens: readIntValue(payload["cachedInputTokens"], payload["cached_input_tokens"]),
		OutputTokens:      readIntValue(payload["outputTokens"], payload["output_tokens"], payload["completionTokens"], payload["completion_tokens"]),
	}
	if totals.InputTokens != 0 {
		payload["inputTokens"] = totals.InputTokens
	}
	if totals.CachedInputTokens != 0 {
		payload["cachedInputTokens"] = totals.CachedInputTokens
	}
	if totals.OutputTokens != 0 {
		payload["outputTokens"] = totals.OutputTokens
	}
	normalized, _ := json.Marshal(payload)
	return datatypes.JSON(normalized), totals
}

func buildCostEvent(run *models.HeartbeatRun, result *AgentRunResult, usage UsageTotals, occurredAt time.Time) *models.CostEvent {
	contextSnapshot := parseJSONObject(run.ContextSnapshot)
	projectID := firstNonEmpty(
		readNonEmptyString(contextSnapshot["projectId"]),
		readNonEmptyString(nestedObject(contextSnapshot, "paperclipWorkspace")["projectId"]),
	)
	issueID := readNonEmptyString(contextSnapshot["issueId"])
	event := &models.CostEvent{
		AgentID:           run.AgentID,
		HeartbeatRunID:    &run.ID,
		Provider:          run.Agent.AdapterType,
		Biller:            "unknown",
		BillingType:       "unknown",
		Model:             "default",
		InputTokens:       usage.InputTokens,
		CachedInputTokens: usage.CachedInputTokens,
		OutputTokens:      usage.OutputTokens,
		OccurredAt:        occurredAt,
	}
	if projectID != "" {
		event.ProjectID = &projectID
	}
	if issueID != "" {
		event.IssueID = &issueID
	}
	if result != nil {
		event.CostCents = costUSDToCents(result.CostUsd)
		resultPayload := parseJSONObject(result.ResultJSON)
		event.Provider = firstNonEmpty(readNonEmptyString(resultPayload["provider"]), event.Provider)
		event.Biller = firstNonEmpty(readNonEmptyString(resultPayload["biller"]), event.Biller)
		event.BillingType = firstNonEmpty(readNonEmptyString(resultPayload["billingType"]), readNonEmptyString(resultPayload["billing_type"]), event.BillingType)
		event.Model = firstNonEmpty(readNonEmptyString(resultPayload["model"]), event.Model)
	}
	return event
}

func costUSDToCents(value float64) int {
	if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return int(math.Round(value * 100))
}

func firstJSONField(payload map[string]json.RawMessage, keys ...string) json.RawMessage {
	for _, key := range keys {
		if raw, ok := payload[key]; ok && len(raw) > 0 && string(raw) != "null" {
			return raw
		}
	}
	return nil
}
