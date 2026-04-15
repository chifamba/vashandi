// Package pilocal implements the pi-local adapter for Go,
// mirroring the behaviour of packages/adapters/pi-local/src/server/.
package pilocal

import (
	"encoding/json"
	"regexp"
	"strings"
)

// ToolCall records one tool invocation and its result.
type ToolCall struct {
	ToolCallID string
	ToolName   string
	Args       interface{}
	Result     *string
	IsError    bool
}

// UsageSummary holds token counts and cost aggregated across all turns.
type UsageSummary struct {
	InputTokens       int
	OutputTokens      int
	CachedInputTokens int
	CostUsd           float64
}

// ParsedPiOutput is the structured output of ParsePiJsonl.
type ParsedPiOutput struct {
	SessionID    *string
	Messages     []string
	Errors       []string
	Usage        UsageSummary
	FinalMessage *string
	ToolCalls    []ToolCall
}

// extractTextContent converts a Pi content field (either a plain string or a
// JSON array of content blocks) into a plain string.
func extractTextContent(raw interface{}) string {
	switch v := raw.(type) {
	case string:
		return v
	case []interface{}:
		var parts []string
		for _, block := range v {
			if m, ok := block.(map[string]interface{}); ok {
				if t, _ := m["type"].(string); t == "text" {
					if text, _ := m["text"].(string); text != "" {
						parts = append(parts, text)
					}
				}
			}
		}
		return strings.Join(parts, "")
	default:
		return ""
	}
}

// asRecord converts an interface{} to map[string]interface{} or returns nil.
func asRecord(v interface{}) map[string]interface{} {
	if m, ok := v.(map[string]interface{}); ok {
		return m
	}
	return nil
}

// asString returns the string value or the fallback.
func asString(v interface{}, fallback string) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fallback
}

// asFloat returns the float64 value or the fallback.
func asFloat(v interface{}, fallback float64) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			return fallback
		}
		return f
	}
	return fallback
}

// ParsePiJsonl parses the JSONL stdout produced by `pi --mode json` and returns
// a structured summary.  It mirrors the logic in parse.ts exactly.
func ParsePiJsonl(stdout string) ParsedPiOutput {
	result := ParsedPiOutput{}

	var currentToolCallID string

	for _, rawLine := range strings.Split(stdout, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}

		var event map[string]interface{}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}

		eventType := asString(event["type"], "")

		// RPC protocol messages – skip (internal implementation details)
		switch eventType {
		case "response", "extension_ui_request", "extension_ui_response", "extension_error":
			continue
		}

		// Agent lifecycle
		if eventType == "agent_start" {
			continue
		}

		if eventType == "agent_end" {
			if messages, ok := event["messages"].([]interface{}); ok && len(messages) > 0 {
				if last, ok := messages[len(messages)-1].(map[string]interface{}); ok {
					if role, _ := last["role"].(string); role == "assistant" {
						text := extractTextContent(last["content"])
						result.FinalMessage = &text
					}
				}
			}
			continue
		}

		if eventType == "auto_retry_end" {
			if event["success"] != true {
				finalError := strings.TrimSpace(asString(event["finalError"], ""))
				if finalError == "" {
					finalError = "Pi exhausted automatic retries without producing a response."
				}
				result.Errors = append(result.Errors, finalError)
			}
			continue
		}

		// Turn lifecycle
		if eventType == "turn_start" {
			continue
		}

		if eventType == "turn_end" {
			message := asRecord(event["message"])
			if message != nil {
				text := extractTextContent(message["content"])
				if text != "" {
					result.FinalMessage = &text
					result.Messages = append(result.Messages, text)
				}

				// Extract usage from the assistant message (Pi format)
				usage := asRecord(message["usage"])
				if usage != nil {
					result.Usage.InputTokens += int(asFloat(usage["input"], 0))
					result.Usage.OutputTokens += int(asFloat(usage["output"], 0))
					result.Usage.CachedInputTokens += int(asFloat(usage["cacheRead"], 0))

					cost := asRecord(usage["cost"])
					if cost != nil {
						result.Usage.CostUsd += asFloat(cost["total"], 0)
					}
				}
			}

			// Tool results inside turn_end
			if toolResults, ok := event["toolResults"].([]interface{}); ok {
				for _, tr := range toolResults {
					trMap := asRecord(tr)
					if trMap == nil {
						continue
					}
					tcID := asString(trMap["toolCallId"], "")
					isError, _ := trMap["isError"].(bool)

					for i := range result.ToolCalls {
						if result.ToolCalls[i].ToolCallID == tcID {
							var resultStr string
							if s, ok := trMap["content"].(string); ok {
								resultStr = s
							} else if trMap["content"] != nil {
								b, _ := json.Marshal(trMap["content"])
								resultStr = string(b)
							}
							result.ToolCalls[i].Result = &resultStr
							result.ToolCalls[i].IsError = isError
							break
						}
					}
				}
			}
			continue
		}

		// Streaming message updates
		if eventType == "message_update" {
			assistantEvent := asRecord(event["assistantMessageEvent"])
			if assistantEvent != nil {
				msgType := asString(assistantEvent["type"], "")
				if msgType == "text_delta" {
					delta := asString(assistantEvent["delta"], "")
					if delta != "" {
						if len(result.Messages) == 0 {
							result.Messages = append(result.Messages, delta)
						} else {
							result.Messages[len(result.Messages)-1] += delta
						}
					}
				}
			}
			continue
		}

		if eventType == "error" {
			msg := strings.TrimSpace(asString(event["message"], ""))
			if msg != "" {
				result.Errors = append(result.Errors, msg)
			}
			continue
		}

		// Tool execution
		if eventType == "tool_execution_start" {
			tcID := asString(event["toolCallId"], "")
			toolName := asString(event["toolName"], "")
			currentToolCallID = tcID
			result.ToolCalls = append(result.ToolCalls, ToolCall{
				ToolCallID: tcID,
				ToolName:   toolName,
				Args:       event["args"],
			})
			continue
		}

		if eventType == "tool_execution_end" {
			tcID := asString(event["toolCallId"], "")
			isError, _ := event["isError"].(bool)
			_ = currentToolCallID

			var resultStr string
			if s, ok := event["result"].(string); ok {
				resultStr = s
			} else if event["result"] != nil {
				b, _ := json.Marshal(event["result"])
				resultStr = string(b)
			}

			for i := range result.ToolCalls {
				if result.ToolCalls[i].ToolCallID == tcID {
					result.ToolCalls[i].Result = &resultStr
					result.ToolCalls[i].IsError = isError
					break
				}
			}
			currentToolCallID = ""
			continue
		}

		// Standalone usage event or event with usage field (fallback)
		if eventType == "usage" || event["usage"] != nil {
			usage := asRecord(event["usage"])
			if usage != nil {
				// Support Pi format (input/output/cacheRead) and generic format
				inp := asFloat(firstNonNil(usage["inputTokens"], usage["input"]), 0)
				out := asFloat(firstNonNil(usage["outputTokens"], usage["output"]), 0)
				cached := asFloat(firstNonNil(usage["cachedInputTokens"], usage["cacheRead"]), 0)
				result.Usage.InputTokens += int(inp)
				result.Usage.OutputTokens += int(out)
				result.Usage.CachedInputTokens += int(cached)

				cost := asRecord(usage["cost"])
				if cost != nil {
					result.Usage.CostUsd += asFloat(firstNonNil(cost["total"], usage["costUsd"]), 0)
				} else {
					result.Usage.CostUsd += asFloat(usage["costUsd"], 0)
				}
			}
		}
	}

	return result
}

// firstNonNil returns the first non-nil value from the arguments.
func firstNonNil(vals ...interface{}) interface{} {
	for _, v := range vals {
		if v != nil {
			return v
		}
	}
	return nil
}

var unknownSessionRe = regexp.MustCompile(
	`(?i)unknown\s+session|session\s+not\s+found|session\s+.*\s+not\s+found|no\s+session`,
)

// IsPiUnknownSessionError returns true when the combined stdout+stderr output
// contains a Pi "unknown session" error message.
func IsPiUnknownSessionError(stdout, stderr string) bool {
	combined := stdout + "\n" + stderr
	lines := strings.Split(combined, "\n")
	filtered := make([]string, 0, len(lines))
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if t != "" {
			filtered = append(filtered, t)
		}
	}
	haystack := strings.Join(filtered, "\n")
	return unknownSessionRe.MatchString(haystack)
}
