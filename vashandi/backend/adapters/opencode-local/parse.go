package opencodelocal

import (
	"encoding/json"
	"regexp"
	"strings"
)

// UsageInfo accumulates token usage across all step_finish events.
type UsageInfo struct {
	InputTokens       int
	CachedInputTokens int
	OutputTokens      int
}

// ParsedOutput is the result of parsing an OpenCode JSONL stdout stream.
type ParsedOutput struct {
	SessionID    *string
	Summary      string
	Usage        UsageInfo
	CostUsd      float64
	ErrorMessage *string
}

// errorText extracts a human-readable error string from an arbitrary JSON value.
func errorText(value interface{}) string {
	if s, ok := value.(string); ok {
		return s
	}
	rec, ok := value.(map[string]interface{})
	if !ok {
		return ""
	}
	if msg, ok := rec["message"].(string); ok && msg != "" {
		return msg
	}
	if data, ok := rec["data"].(map[string]interface{}); ok {
		if msg, ok := data["message"].(string); ok && msg != "" {
			return msg
		}
	}
	if name, ok := rec["name"].(string); ok && name != "" {
		return name
	}
	if code, ok := rec["code"].(string); ok && code != "" {
		return code
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return ""
	}
	return string(b)
}

func asFloat64(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	}
	return 0
}

func asInt(v interface{}) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}

func asString(v interface{}) string {
	s, _ := v.(string)
	return s
}

func asMap(v interface{}) map[string]interface{} {
	m, _ := v.(map[string]interface{})
	return m
}

// ParseOpenCodeJsonl parses JSONL output from `opencode run --format json`.
func ParseOpenCodeJsonl(stdout string) ParsedOutput {
	var out ParsedOutput
	var messages []string
	var errors []string

	for _, rawLine := range strings.Split(stdout, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}

		var event map[string]interface{}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}

		if sid := strings.TrimSpace(asString(event["sessionID"])); sid != "" {
			s := sid
			out.SessionID = &s
		}

		evType := asString(event["type"])

		switch evType {
		case "text":
			part := asMap(event["part"])
			text := strings.TrimSpace(asString(part["text"]))
			if text != "" {
				messages = append(messages, text)
			}

		case "step_finish":
			part := asMap(event["part"])
			tokens := asMap(part["tokens"])
			cache := asMap(tokens["cache"])
			out.Usage.InputTokens += asInt(tokens["input"])
			out.Usage.CachedInputTokens += asInt(cache["read"])
			out.Usage.OutputTokens += asInt(tokens["output"]) + asInt(tokens["reasoning"])
			out.CostUsd += asFloat64(part["cost"])

		case "tool_use":
			part := asMap(event["part"])
			state := asMap(part["state"])
			if asString(state["status"]) == "error" {
				text := strings.TrimSpace(asString(state["error"]))
				if text != "" {
					errors = append(errors, text)
				}
			}

		case "error":
			var errVal interface{}
			if e, ok := event["error"]; ok {
				errVal = e
			} else {
				errVal = event["message"]
			}
			text := strings.TrimSpace(errorText(errVal))
			if text != "" {
				errors = append(errors, text)
			}
		}
	}

	out.Summary = strings.TrimSpace(strings.Join(messages, "\n\n"))
	if len(errors) > 0 {
		msg := strings.Join(errors, "\n")
		out.ErrorMessage = &msg
	}
	return out
}

var unknownSessionRe = regexp.MustCompile(
	`(?i)unknown\s+session|session\b.*\bnot\s+found|resource\s+not\s+found:.*[/\\]session[/\\].*\.json|notfounderror|no session`,
)

// IsOpenCodeUnknownSessionError returns true when the stdout/stderr indicates
// that the requested session no longer exists in OpenCode.
func IsOpenCodeUnknownSessionError(stdout, stderr string) bool {
	var lines []string
	for _, line := range strings.Split(stdout+"\n"+stderr, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	haystack := strings.Join(lines, "\n")
	return unknownSessionRe.MatchString(haystack)
}
