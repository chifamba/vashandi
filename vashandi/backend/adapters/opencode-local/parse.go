package opencodelocal

import (
	"encoding/json"
	"regexp"
	"strings"
)

type StreamEvent struct {
	Type      string                 `json:"type"`
	SessionID *string                `json:"sessionID,omitempty"`
	Message   interface{}            `json:"message,omitempty"`
	Error     interface{}            `json:"error,omitempty"`
	Name      string                 `json:"name,omitempty"`
	Code      string                 `json:"code,omitempty"`
	Part      map[string]interface{} `json:"part,omitempty"`
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

func asString(v interface{}) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
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

func errorText(value interface{}) string {
	if s, ok := value.(string); ok {
		return strings.TrimSpace(s)
	}
	if m, ok := value.(map[string]interface{}); ok {
		if msg := asString(m["message"]); msg != "" {
			return msg
		}
		if data, ok := m["data"].(map[string]interface{}); ok {
			if nested := asString(data["message"]); nested != "" {
				return nested
			}
		}
		if name := asString(m["name"]); name != "" {
			return name
		}
		if code := asString(m["code"]); code != "" {
			return code
		}
		b, _ := json.Marshal(m)
		return string(b)
	}
	return ""
}

func ParseOpenCodeJsonl(stdout string) ParseResult {
	var res ParseResult
	res.Usage = &UsageSummary{}
	var cost float64
	var texts []string
	var errors []string

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

		if event.SessionID != nil && strings.TrimSpace(*event.SessionID) != "" {
			res.SessionID = event.SessionID
		}

		switch event.Type {
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
					res.Usage.OutputTokens += asInt(toks["output"], toks["reasoning"])
					if c, ok := toks["cache"].(map[string]interface{}); ok {
						res.Usage.CachedInputTokens += asInt(c["read"])
					}
				}
				if c, ok := event.Part["cost"].(float64); ok {
					cost += c
				}
			}
		case "tool_use":
			if event.Part != nil && event.Part["state"] != nil {
				if state, ok := event.Part["state"].(map[string]interface{}); ok {
					if asString(state["status"]) == "error" {
						if errTxt := asString(state["error"]); errTxt != "" {
							errors = append(errors, errTxt)
						}
					}
				}
			}
		case "error":
			target := event.Error
			if target == nil {
				target = event.Message
			}
			if errTxt := errorText(target); errTxt != "" {
				errors = append(errors, errTxt)
			}
		}
	}

	res.Summary = strings.TrimSpace(strings.Join(texts, "\n\n"))
	if len(errors) > 0 {
		e := strings.Join(errors, "\n")
		res.ErrorMessage = &e
	}
	if cost > 0 {
		res.CostUsd = &cost
	}

	return res
}

func IsOpenCodeUnknownSessionError(stdout, stderr string) bool {
	re := regexp.MustCompile(`(?i)(unknown\s+session|session\b.*\bnot\s+found|resource\s+not\s+found:.*[\\/]session[\\/].*\.json|notfounderror|no session)`)
	return re.MatchString(stdout) || re.MatchString(stderr)
}
