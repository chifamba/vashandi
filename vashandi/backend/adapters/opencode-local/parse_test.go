package opencodelocal

import (
	"encoding/json"
	"testing"
)

func jsonLine(t *testing.T, v map[string]interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return string(b)
}

func TestParseOpenCodeJsonl_TextUsageCostError(t *testing.T) {
	lines := []string{
		jsonLine(t, map[string]interface{}{
			"type":      "text",
			"sessionID": "session_123",
			"part":      map[string]interface{}{"text": "Hello from OpenCode"},
		}),
		jsonLine(t, map[string]interface{}{
			"type":      "step_finish",
			"sessionID": "session_123",
			"part": map[string]interface{}{
				"reason": "done",
				"cost":   0.0025,
				"tokens": map[string]interface{}{
					"input":     120,
					"output":    40,
					"reasoning": 10,
					"cache":     map[string]interface{}{"read": 20, "write": 0},
				},
			},
		}),
		jsonLine(t, map[string]interface{}{
			"type":      "error",
			"sessionID": "session_123",
			"error":     map[string]interface{}{"message": "model unavailable"},
		}),
	}
	stdout := ""
	for _, l := range lines {
		stdout += l + "\n"
	}

	parsed := ParseOpenCodeJsonl(stdout)

	if parsed.SessionID == nil || *parsed.SessionID != "session_123" {
		t.Fatalf("expected sessionID session_123, got %v", parsed.SessionID)
	}
	if parsed.Summary != "Hello from OpenCode" {
		t.Fatalf("expected summary 'Hello from OpenCode', got %q", parsed.Summary)
	}
	if parsed.Usage.InputTokens != 120 {
		t.Fatalf("expected inputTokens=120, got %d", parsed.Usage.InputTokens)
	}
	if parsed.Usage.CachedInputTokens != 20 {
		t.Fatalf("expected cachedInputTokens=20, got %d", parsed.Usage.CachedInputTokens)
	}
	if parsed.Usage.OutputTokens != 50 { // 40 + 10
		t.Fatalf("expected outputTokens=50, got %d", parsed.Usage.OutputTokens)
	}
	if parsed.CostUsd < 0.0024 || parsed.CostUsd > 0.0026 {
		t.Fatalf("expected costUsd~0.0025, got %f", parsed.CostUsd)
	}
	if parsed.ErrorMessage == nil || *parsed.ErrorMessage != "model unavailable" {
		t.Fatalf("expected errorMessage 'model unavailable', got %v", parsed.ErrorMessage)
	}
}

func TestParseOpenCodeJsonl_EmptyInput(t *testing.T) {
	parsed := ParseOpenCodeJsonl("")
	if parsed.SessionID != nil {
		t.Errorf("expected nil sessionID, got %v", parsed.SessionID)
	}
	if parsed.Summary != "" {
		t.Errorf("expected empty summary, got %q", parsed.Summary)
	}
	if parsed.ErrorMessage != nil {
		t.Errorf("expected nil error, got %v", parsed.ErrorMessage)
	}
}

func TestParseOpenCodeJsonl_MultipleTextBlocks(t *testing.T) {
	lines := []string{
		jsonLine(t, map[string]interface{}{"type": "text", "sessionID": "s1", "part": map[string]interface{}{"text": "Hello"}}),
		jsonLine(t, map[string]interface{}{"type": "text", "sessionID": "s1", "part": map[string]interface{}{"text": "World"}}),
	}
	stdout := lines[0] + "\n" + lines[1] + "\n"
	parsed := ParseOpenCodeJsonl(stdout)
	if parsed.Summary != "Hello\n\nWorld" {
		t.Fatalf("expected joined summary, got %q", parsed.Summary)
	}
}

func TestParseOpenCodeJsonl_ToolUseError(t *testing.T) {
	line := jsonLine(t, map[string]interface{}{
		"type": "tool_use",
		"part": map[string]interface{}{
			"tool": "bash",
			"state": map[string]interface{}{
				"status": "error",
				"error":  "permission denied",
			},
		},
	})
	parsed := ParseOpenCodeJsonl(line + "\n")
	if parsed.ErrorMessage == nil || *parsed.ErrorMessage != "permission denied" {
		t.Fatalf("expected tool_use error, got %v", parsed.ErrorMessage)
	}
}

func TestIsOpenCodeUnknownSessionError(t *testing.T) {
	cases := []struct {
		stdout, stderr string
		want           bool
	}{
		{"Session not found: s_123", "", true},
		{"", "unknown session id", true},
		{"resource not found: /path/session/abc.json", "", true},
		{"NotFoundError: no session", "", true},
		{"all good", "", false},
		{"", "all good", false},
	}
	for _, tc := range cases {
		got := IsOpenCodeUnknownSessionError(tc.stdout, tc.stderr)
		if got != tc.want {
			t.Errorf("IsOpenCodeUnknownSessionError(%q, %q) = %v, want %v",
				tc.stdout, tc.stderr, got, tc.want)
		}
	}
}
