package claudelocal

import (
	"encoding/json"
	"regexp"
	"strings"
)

var (
	claudeAuthRequiredRE = regexp.MustCompile(`(?i)(?:not\s+logged\s+in|please\s+log\s+in|please\s+run\s+` + "`" + `?claude\s+login` + "`" + `?|login\s+required|requires\s+login|unauthorized|authentication\s+required)`)
	urlRE                = regexp.MustCompile(`(?i)(https?://[^\s'"` + "`" + `<>()[\]{};,!?]+[^\s'"` + "`" + `<>()[\]{};,!.?:]+)`)
)

type StreamEvent struct {
	Type      string                 `json:"type"`
	Subtype   string                 `json:"subtype,omitempty"`
	SessionID string                 `json:"session_id,omitempty"`
	Model     string                 `json:"model,omitempty"`
	Message   map[string]interface{} `json:"message,omitempty"`
	Result    string                 `json:"result,omitempty"`
	Usage     map[string]interface{} `json:"usage,omitempty"`
	CostUsd   float64                `json:"total_cost_usd,omitempty"`
	Errors    []interface{}          `json:"errors,omitempty"`
}

type UsageSummary struct {
	InputTokens       int `json:"inputTokens"`
	CachedInputTokens int `json:"cachedInputTokens"`
	OutputTokens      int `json:"outputTokens"`
}

type ParseResult struct {
	SessionID  string                 `json:"sessionId"`
	Model      string                 `json:"model"`
	CostUsd    *float64               `json:"costUsd"`
	Usage      *UsageSummary          `json:"usage"`
	Summary    string                 `json:"summary"`
	ResultJSON map[string]interface{} `json:"resultJson"`
}

func ParseClaudeStreamJson(stdout string) ParseResult {
	var sessionId string
	var model string
	var finalResult map[string]interface{}
	var assistantTexts []string

	lines := strings.Split(stdout, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var event StreamEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}

		if event.SessionID != "" {
			sessionId = event.SessionID
		}
		if event.Model != "" {
			model = event.Model
		}

		switch event.Type {
		case "assistant":
			if msg := event.Message; msg != nil {
				if content, ok := msg["content"].([]interface{}); ok {
					for _, blockRaw := range content {
						if block, ok := blockRaw.(map[string]interface{}); ok {
							if block["type"] == "text" {
								if text, ok := block["text"].(string); ok && text != "" {
									assistantTexts = append(assistantTexts, text)
								}
							}
						}
					}
				}
			}
		case "result":
			// Capture the raw event as results
			var raw map[string]interface{}
			json.Unmarshal([]byte(line), &raw)
			finalResult = raw
		}
	}

	res := ParseResult{
		SessionID: sessionId,
		Model:     model,
		Summary:   strings.TrimSpace(strings.Join(assistantTexts, "\n\n")),
	}

	if finalResult != nil {
		res.ResultJSON = finalResult
		if usage, ok := finalResult["usage"].(map[string]interface{}); ok {
			res.Usage = &UsageSummary{
				InputTokens:       asInt(usage["input_tokens"]),
				CachedInputTokens: asInt(usage["cache_read_input_tokens"]),
				OutputTokens:      asInt(usage["output_tokens"]),
			}
		}
		if cost, ok := finalResult["total_cost_usd"].(float64); ok {
			res.CostUsd = &cost
		}
		if summary, ok := finalResult["result"].(string); ok && summary != "" {
			res.Summary = summary
		}
	}

	return res
}

func asInt(v interface{}) int {
	if f, ok := v.(float64); ok {
		return int(f)
	}
	if i, ok := v.(int); ok {
		return i
	}
	return 0
}

func ExtractClaudeLoginUrl(text string) string {
	matches := urlRE.FindAllString(text, -1)
	if len(matches) == 0 {
		return ""
	}
	for _, raw := range matches {
		cleaned := strings.TrimRight(raw, "])}.!,?;:'\"")
		lower := strings.ToLower(cleaned)
		if strings.Contains(lower, "claude") || strings.Contains(lower, "anthropic") || strings.Contains(lower, "auth") {
			return cleaned
		}
	}
	return strings.TrimRight(matches[0], "])}.!,?;:'\"")
}

func IsClaudeMaxTurnsResult(res map[string]interface{}) bool {
	if res == nil {
		return false
	}
	subtype := strings.ToLower(asString(res["subtype"]))
	if subtype == "error_max_turns" {
		return true
	}
	stopReason := strings.ToLower(asString(res["stop_reason"]))
	if stopReason == "max_turns" {
		return true
	}
	resultText := asString(res["result"])
	return regexp.MustCompile(`(?i)max(imum)?\s+turns?`).MatchString(resultText)
}

func asString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
