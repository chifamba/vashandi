package services

import "strings"

// summarizeTextValue truncates a string to maxLength characters.
// Returns empty string if the value is empty.
func summarizeTextValue(value string, maxLength int) string {
	if len(value) > maxLength {
		return value[:maxLength]
	}
	return value
}

// SummarizeHeartbeatRunResultJSON extracts a concise summary map from a heartbeat
// run result JSON object. Returns nil if no relevant fields are found.
// Mirrors the TypeScript summarizeHeartbeatRunResultJson function.
func SummarizeHeartbeatRunResultJSON(resultJSON map[string]interface{}) map[string]interface{} {
	if resultJSON == nil {
		return nil
	}

	const maxTextLength = 500
	summary := make(map[string]interface{})

	for _, key := range []string{"summary", "result", "message", "error"} {
		if v, ok := resultJSON[key]; ok {
			if s, isStr := v.(string); isStr && s != "" {
				summary[key] = summarizeTextValue(s, maxTextLength)
			}
		}
	}

	for _, key := range []string{"total_cost_usd", "cost_usd", "costUsd"} {
		if v, ok := resultJSON[key]; ok && v != nil {
			summary[key] = v
		}
	}

	if len(summary) == 0 {
		return nil
	}
	return summary
}

// BuildHeartbeatRunIssueComment extracts a comment string from a heartbeat run
// result JSON object. Returns empty string if no comment-worthy field is found.
// Mirrors the TypeScript buildHeartbeatRunIssueComment function.
func BuildHeartbeatRunIssueComment(resultJSON map[string]interface{}) string {
	if resultJSON == nil {
		return ""
	}
	for _, key := range []string{"summary", "result", "message"} {
		if v, ok := resultJSON[key]; ok {
			if s, isStr := v.(string); isStr {
				s = strings.TrimSpace(s)
				if s != "" {
					return s
				}
			}
		}
	}
	return ""
}
