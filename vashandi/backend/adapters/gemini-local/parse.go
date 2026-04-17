package geminilocal

import (
	"encoding/json"
	"regexp"
	"strings"
)

type StreamEvent struct {
	Type        string                 `json:"type"`
	Subtype     string                 `json:"subtype,omitempty"`
	SessionID   *string                `json:"session_id,omitempty"`
	SessionID2  *string                `json:"sessionId,omitempty"`
	SessionID3  *string                `json:"sessionID,omitempty"`
	Message     interface{}            `json:"message,omitempty"`
	Result      interface{}            `json:"result,omitempty"`
	Error       interface{}            `json:"error,omitempty"`
	Detail      interface{}            `json:"detail,omitempty"`
	Part        map[string]interface{} `json:"part,omitempty"`
	Usage       map[string]interface{} `json:"usage,omitempty"`
	IsError     bool                   `json:"is_error,omitempty"`
	CostUsd     *float64               `json:"cost_usd,omitempty"`
	TotalCostUsd *float64              `json:"total_cost_usd,omitempty"`
}

type UsageSummary struct {
	InputTokens       int `json:"inputTokens"`
	CachedInputTokens int `json:"cachedInputTokens"`
	OutputTokens      int `json:"outputTokens"`
}

type ParseResult struct {
	SessionID    *string
	Summary      string
	ErrorMessage *string
	CostUsd      *float64
	Usage        *UsageSummary
}


func asErrorText(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	if m, ok := v.(map[string]interface{}); ok {
		if msg := asString(m["message"]); msg != "" {
			return msg
		}
		if err := asString(m["error"]); err != "" {
			return err
		}
		if code := asString(m["code"]); code != "" {
			return code
		}
		if det := asString(m["detail"]); det != "" {
			return det
		}
		b, _ := json.Marshal(m)
		return string(b)
	}
	return ""
}

func collectAssistantText(v interface{}) []string {
	var texts []string
	if s, ok := v.(string); ok {
		if t := strings.TrimSpace(s); t != "" {
			texts = append(texts, t)
		}
		return texts
	}
	if m, ok := v.(map[string]interface{}); ok {
		if text := asString(m["text"]); text != "" {
			texts = append(texts, text)
		}
		if arr, ok := m["content"].([]interface{}); ok {
			for _, partRaw := range arr {
				if part, ok := partRaw.(map[string]interface{}); ok {
					ptype := asString(part["type"])
					if ptype == "output_text" || ptype == "text" {
						if text := asString(part["text"]); text != "" {
							texts = append(texts, text)
						}
					}
				}
			}
		}
	}
	return texts
}

func ParseGeminiJsonl(stdout string) ParseResult {
	var res ParseResult
	res.Usage = &UsageSummary{}
	var cost float64
	var texts []string
	var errStr string

	lines := strings.Split(stdout, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Unmarshal the event
		var event StreamEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}

		if event.SessionID != nil {
			res.SessionID = event.SessionID
		} else if event.SessionID2 != nil {
			res.SessionID = event.SessionID2
		} else if event.SessionID3 != nil {
			res.SessionID = event.SessionID3
		}

		switch event.Type {
		case "assistant":
			texts = append(texts, collectAssistantText(event.Message)...)

		case "result":
			if event.Usage != nil {
				res.Usage.InputTokens += asInt(event.Usage["input_tokens"], event.Usage["inputTokens"])
				res.Usage.OutputTokens += asInt(event.Usage["output_tokens"], event.Usage["outputTokens"])
				res.Usage.CachedInputTokens += asInt(event.Usage["cached_input_tokens"], event.Usage["cachedInputTokens"], event.Usage["cache_read_input_tokens"])
			}

			if event.TotalCostUsd != nil {
				cost += *event.TotalCostUsd
			} else if event.CostUsd != nil {
				cost += *event.CostUsd
			}

			isErr := event.IsError || strings.ToLower(event.Subtype) == "error"
			resText := asString(event.Result)
			if resText != "" && len(texts) == 0 {
				texts = append(texts, resText)
			}
			if isErr {
				if errText := asErrorText(firstNonNil(event.Error, event.Message, event.Result)); errText != "" {
					errStr = errText
				}
			}
		case "error":
			if errText := asErrorText(firstNonNil(event.Message, event.Error, event.Detail)); errText != "" {
				errStr = errText
			}
		case "system":
			if strings.ToLower(event.Subtype) == "error" {
				if errText := asErrorText(firstNonNil(event.Message, event.Error, event.Detail)); errText != "" {
					errStr = errText
				}
			}
		case "text":
			if event.Part != nil {
				if t := asString(event.Part["text"]); t != "" {
					texts = append(texts, t)
				}
			}
		case "step_finish":
			if event.Part != nil && event.Part["tokens"] != nil {
				if toks, ok := event.Part["tokens"].(map[string]interface{}); ok {
					res.Usage.InputTokens += asInt(toks["input"])
					res.Usage.OutputTokens += asInt(toks["output"])
					if c, ok := toks["cache"].(map[string]interface{}); ok {
						res.Usage.CachedInputTokens += asInt(c["read"])
					}
				}
				if c, ok := event.Part["cost"].(float64); ok {
					cost += c
				}
			}
		}
	}

	res.Summary = strings.TrimSpace(strings.Join(texts, "\n\n"))
	if errStr != "" {
		res.ErrorMessage = &errStr
	}
	if cost > 0 {
		res.CostUsd = &cost
	}

	return res
}

func firstNonNil(items ...interface{}) interface{} {
	for _, item := range items {
		if item != nil {
			return item
		}
	}
	return nil
}

func asInt(vals ...interface{}) int {
	for _, v := range vals {
		if f, ok := v.(float64); ok && f != 0 {
			return int(f)
		}
		if i, ok := v.(int); ok && i != 0 {
			return i
		}
	}
	return 0
}

func IsGeminiUnknownSessionError(stdout, stderr string) bool {
	re := regexp.MustCompile(`(?i)(unknown\s+(session|chat)|session\s+.*\s+not\s+found|chat\s+.*\s+not\s+found|resume\s+.*\s+not\s+found|could\s+not\s+resume)`)
	return re.MatchString(stdout) || re.MatchString(stderr)
}
