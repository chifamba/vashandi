package pilocal

import (
	"encoding/json"
	"strings"
	"testing"
)

func jsonLine(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestParsePiJsonl_AgentLifecycleAndMessages(t *testing.T) {
	stdout := strings.Join([]string{
		jsonLine(map[string]interface{}{"type": "agent_start"}),
		jsonLine(map[string]interface{}{
			"type": "turn_end",
			"message": map[string]interface{}{
				"role":    "assistant",
				"content": []interface{}{map[string]interface{}{"type": "text", "text": "Hello from Pi"}},
			},
		}),
		jsonLine(map[string]interface{}{"type": "agent_end", "messages": []interface{}{}}),
	}, "\n")

	parsed := ParsePiJsonl(stdout)
	if len(parsed.Messages) == 0 || !strings.Contains(parsed.Messages[0], "Hello from Pi") {
		t.Fatalf("expected message to contain 'Hello from Pi', got %v", parsed.Messages)
	}
	if parsed.FinalMessage == nil || *parsed.FinalMessage != "Hello from Pi" {
		t.Fatalf("expected finalMessage to be 'Hello from Pi', got %v", parsed.FinalMessage)
	}
}

func TestParsePiJsonl_StreamingTextDeltas(t *testing.T) {
	stdout := strings.Join([]string{
		jsonLine(map[string]interface{}{
			"type": "message_update",
			"assistantMessageEvent": map[string]interface{}{"type": "text_delta", "delta": "Hello "},
		}),
		jsonLine(map[string]interface{}{
			"type": "message_update",
			"assistantMessageEvent": map[string]interface{}{"type": "text_delta", "delta": "World"},
		}),
		jsonLine(map[string]interface{}{
			"type":    "turn_end",
			"message": map[string]interface{}{"role": "assistant", "content": "Hello World"},
		}),
	}, "\n")

	parsed := ParsePiJsonl(stdout)
	found := false
	for _, m := range parsed.Messages {
		if strings.Contains(m, "Hello World") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected messages to contain 'Hello World', got %v", parsed.Messages)
	}
}

func TestParsePiJsonl_ToolExecution(t *testing.T) {
	stdout := strings.Join([]string{
		jsonLine(map[string]interface{}{
			"type":       "tool_execution_start",
			"toolCallId": "tool_1",
			"toolName":   "read",
			"args":       map[string]interface{}{"path": "/tmp/test.txt"},
		}),
		jsonLine(map[string]interface{}{
			"type":       "tool_execution_end",
			"toolCallId": "tool_1",
			"toolName":   "read",
			"result":     "file contents",
			"isError":    false,
		}),
		jsonLine(map[string]interface{}{
			"type":    "turn_end",
			"message": map[string]interface{}{"role": "assistant", "content": "Done"},
			"toolResults": []interface{}{
				map[string]interface{}{"toolCallId": "tool_1", "content": "file contents", "isError": false},
			},
		}),
	}, "\n")

	parsed := ParsePiJsonl(stdout)
	if len(parsed.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(parsed.ToolCalls))
	}
	if parsed.ToolCalls[0].ToolName != "read" {
		t.Fatalf("expected toolName 'read', got %q", parsed.ToolCalls[0].ToolName)
	}
	if parsed.ToolCalls[0].Result == nil || *parsed.ToolCalls[0].Result != "file contents" {
		t.Fatalf("expected result 'file contents', got %v", parsed.ToolCalls[0].Result)
	}
	if parsed.ToolCalls[0].IsError {
		t.Fatal("expected isError to be false")
	}
}

func TestParsePiJsonl_ToolExecutionError(t *testing.T) {
	stdout := strings.Join([]string{
		jsonLine(map[string]interface{}{
			"type":       "tool_execution_start",
			"toolCallId": "tool_1",
			"toolName":   "read",
			"args":       map[string]interface{}{"path": "/missing.txt"},
		}),
		jsonLine(map[string]interface{}{
			"type":       "tool_execution_end",
			"toolCallId": "tool_1",
			"toolName":   "read",
			"result":     "File not found",
			"isError":    true,
		}),
	}, "\n")

	parsed := ParsePiJsonl(stdout)
	if len(parsed.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(parsed.ToolCalls))
	}
	if !parsed.ToolCalls[0].IsError {
		t.Fatal("expected isError to be true")
	}
	if parsed.ToolCalls[0].Result == nil || *parsed.ToolCalls[0].Result != "File not found" {
		t.Fatalf("expected result 'File not found', got %v", parsed.ToolCalls[0].Result)
	}
}

func TestParsePiJsonl_UsageAndCostFromTurnEnd(t *testing.T) {
	stdout := jsonLine(map[string]interface{}{
		"type": "turn_end",
		"message": map[string]interface{}{
			"role":    "assistant",
			"content": "Response with usage",
			"usage": map[string]interface{}{
				"input":     100,
				"output":    50,
				"cacheRead": 20,
				"cost": map[string]interface{}{
					"input":      0.001,
					"output":     0.0015,
					"cacheRead":  0.0001,
					"cacheWrite": 0,
					"total":      0.0026,
				},
			},
		},
		"toolResults": []interface{}{},
	})

	parsed := ParsePiJsonl(stdout)
	if parsed.Usage.InputTokens != 100 {
		t.Fatalf("expected inputTokens=100, got %d", parsed.Usage.InputTokens)
	}
	if parsed.Usage.OutputTokens != 50 {
		t.Fatalf("expected outputTokens=50, got %d", parsed.Usage.OutputTokens)
	}
	if parsed.Usage.CachedInputTokens != 20 {
		t.Fatalf("expected cachedInputTokens=20, got %d", parsed.Usage.CachedInputTokens)
	}
	const eps = 0.0001
	if diff := parsed.Usage.CostUsd - 0.0026; diff > eps || diff < -eps {
		t.Fatalf("expected costUsd≈0.0026, got %f", parsed.Usage.CostUsd)
	}
}

func TestParsePiJsonl_AccumulatesUsageMultipleTurns(t *testing.T) {
	stdout := strings.Join([]string{
		jsonLine(map[string]interface{}{
			"type": "turn_end",
			"message": map[string]interface{}{
				"role": "assistant", "content": "First",
				"usage": map[string]interface{}{
					"input": 50, "output": 25, "cacheRead": 0,
					"cost": map[string]interface{}{"total": 0.001},
				},
			},
		}),
		jsonLine(map[string]interface{}{
			"type": "turn_end",
			"message": map[string]interface{}{
				"role": "assistant", "content": "Second",
				"usage": map[string]interface{}{
					"input": 30, "output": 20, "cacheRead": 10,
					"cost": map[string]interface{}{"total": 0.0015},
				},
			},
		}),
	}, "\n")

	parsed := ParsePiJsonl(stdout)
	if parsed.Usage.InputTokens != 80 {
		t.Fatalf("expected inputTokens=80, got %d", parsed.Usage.InputTokens)
	}
	if parsed.Usage.OutputTokens != 45 {
		t.Fatalf("expected outputTokens=45, got %d", parsed.Usage.OutputTokens)
	}
	if parsed.Usage.CachedInputTokens != 10 {
		t.Fatalf("expected cachedInputTokens=10, got %d", parsed.Usage.CachedInputTokens)
	}
	const eps = 0.0001
	if diff := parsed.Usage.CostUsd - 0.0025; diff > eps || diff < -eps {
		t.Fatalf("expected costUsd≈0.0025, got %f", parsed.Usage.CostUsd)
	}
}

func TestParsePiJsonl_StandaloneUsagePiFormat(t *testing.T) {
	stdout := jsonLine(map[string]interface{}{
		"type": "usage",
		"usage": map[string]interface{}{
			"input":     200,
			"output":    100,
			"cacheRead": 50,
			"cost":      map[string]interface{}{"total": 0.005},
		},
	})

	parsed := ParsePiJsonl(stdout)
	if parsed.Usage.InputTokens != 200 {
		t.Fatalf("expected inputTokens=200, got %d", parsed.Usage.InputTokens)
	}
	if parsed.Usage.OutputTokens != 100 {
		t.Fatalf("expected outputTokens=100, got %d", parsed.Usage.OutputTokens)
	}
	if parsed.Usage.CachedInputTokens != 50 {
		t.Fatalf("expected cachedInputTokens=50, got %d", parsed.Usage.CachedInputTokens)
	}
	const eps = 0.0001
	if diff := parsed.Usage.CostUsd - 0.005; diff > eps || diff < -eps {
		t.Fatalf("expected costUsd=0.005, got %f", parsed.Usage.CostUsd)
	}
}

func TestParsePiJsonl_StandaloneUsageGenericFormat(t *testing.T) {
	stdout := jsonLine(map[string]interface{}{
		"type": "usage",
		"usage": map[string]interface{}{
			"inputTokens":       150,
			"outputTokens":      75,
			"cachedInputTokens": 25,
			"costUsd":           0.003,
		},
	})

	parsed := ParsePiJsonl(stdout)
	if parsed.Usage.InputTokens != 150 {
		t.Fatalf("expected inputTokens=150, got %d", parsed.Usage.InputTokens)
	}
	if parsed.Usage.OutputTokens != 75 {
		t.Fatalf("expected outputTokens=75, got %d", parsed.Usage.OutputTokens)
	}
	if parsed.Usage.CachedInputTokens != 25 {
		t.Fatalf("expected cachedInputTokens=25, got %d", parsed.Usage.CachedInputTokens)
	}
	const eps = 0.0001
	if diff := parsed.Usage.CostUsd - 0.003; diff > eps || diff < -eps {
		t.Fatalf("expected costUsd=0.003, got %f", parsed.Usage.CostUsd)
	}
}

func TestParsePiJsonl_AutoRetryExhausted(t *testing.T) {
	stdout := jsonLine(map[string]interface{}{
		"type":       "auto_retry_end",
		"success":    false,
		"attempt":    3,
		"finalError": "Cloud Code Assist API error (429): RESOURCE_EXHAUSTED",
	})

	parsed := ParsePiJsonl(stdout)
	if len(parsed.Errors) != 1 || parsed.Errors[0] != "Cloud Code Assist API error (429): RESOURCE_EXHAUSTED" {
		t.Fatalf("expected specific error, got %v", parsed.Errors)
	}
}

func TestParsePiJsonl_AutoRetrySucceeded(t *testing.T) {
	stdout := jsonLine(map[string]interface{}{
		"type":    "auto_retry_end",
		"success": true,
		"attempt": 2,
	})

	parsed := ParsePiJsonl(stdout)
	if len(parsed.Errors) != 0 {
		t.Fatalf("expected no errors, got %v", parsed.Errors)
	}
}

func TestParsePiJsonl_StandaloneErrorEvent(t *testing.T) {
	stdout := jsonLine(map[string]interface{}{
		"type":    "error",
		"message": "Connection to model provider lost",
	})

	parsed := ParsePiJsonl(stdout)
	if len(parsed.Errors) != 1 || parsed.Errors[0] != "Connection to model provider lost" {
		t.Fatalf("expected specific error, got %v", parsed.Errors)
	}
}

func TestParsePiJsonl_EmptyErrorMessageIgnored(t *testing.T) {
	stdout := jsonLine(map[string]interface{}{
		"type":    "error",
		"message": "",
	})

	parsed := ParsePiJsonl(stdout)
	if len(parsed.Errors) != 0 {
		t.Fatalf("expected no errors, got %v", parsed.Errors)
	}
}

func TestParsePiJsonl_SkipsRPCMessages(t *testing.T) {
	stdout := strings.Join([]string{
		jsonLine(map[string]interface{}{"type": "response", "data": "something"}),
		jsonLine(map[string]interface{}{"type": "extension_ui_request"}),
		jsonLine(map[string]interface{}{"type": "extension_ui_response"}),
		jsonLine(map[string]interface{}{"type": "extension_error", "message": "some ext error"}),
	}, "\n")

	parsed := ParsePiJsonl(stdout)
	if len(parsed.Errors) != 0 || len(parsed.Messages) != 0 {
		t.Fatalf("expected empty result for RPC messages, got errors=%v messages=%v", parsed.Errors, parsed.Messages)
	}
}

func TestIsPiUnknownSessionError(t *testing.T) {
	tests := []struct {
		stdout, stderr string
		want           bool
	}{
		{"session not found: s_123", "", true},
		{"", "unknown session id", true},
		{"", "no session available", true},
		{"all good", "", false},
		{"working fine", "no errors", false},
	}
	for _, tt := range tests {
		got := IsPiUnknownSessionError(tt.stdout, tt.stderr)
		if got != tt.want {
			t.Errorf("IsPiUnknownSessionError(%q, %q) = %v, want %v", tt.stdout, tt.stderr, got, tt.want)
		}
	}
}
