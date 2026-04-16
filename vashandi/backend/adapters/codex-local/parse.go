package codexlocal

import (
	"encoding/json"
	"regexp"
	"strings"
)

type StreamEvent struct {
	Type      string                 `json:"type"`
	SessionID *string                `json:"sessionId,omitempty"`
	Message   map[string]interface{} `json:"message,omitempty"`
	Summary   *string                `json:"summary,omitempty"`
	Usage     *UsageSummary          `json:"usage,omitempty"`
	Status    *string                `json:"status,omitempty"`
	Error     *string                `json:"error,omitempty"`
}

type UsageSummary struct {
	InputTokens  int `json:"inputTokens"`
	OutputTokens int `json:"outputTokens"`
}

type ParseResult struct {
	SessionID    *string
	Summary      string
	ErrorMessage *string
	Usage        *UsageSummary
}

func ParseCodexJsonl(stdout string) ParseResult {
	var res ParseResult
	var texts []string

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

		if event.SessionID != nil {
			res.SessionID = event.SessionID
		}

		switch event.Type {
		case "content":
			if msg := event.Message; msg != nil {
				if text, ok := msg["text"].(string); ok && text != "" {
					texts = append(texts, text)
				}
			}
		case "finish":
			if event.Summary != nil {
				res.Summary = *event.Summary
			}
			if event.Usage != nil {
				res.Usage = event.Usage
			}
		case "error":
			if event.Error != nil {
				res.ErrorMessage = event.Error
			}
		}
	}

	if res.Summary == "" && len(texts) > 0 {
		res.Summary = strings.Join(texts, "\n\n")
	}

	return res
}

func IsCodexUnknownSessionError(stdout, stderr string) bool {
	re := regexp.MustCompile(`(?i)(unknown session|session not found|invalid session|session [0-9a-zA-Z-]+ not found)`)
	return re.MatchString(stdout) || re.MatchString(stderr)
}
