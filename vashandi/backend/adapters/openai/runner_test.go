package openai

import (
	"strings"
	"testing"
)

func TestParseSSE_AggregatesContentAndToolCalls(t *testing.T) {
	reader := strings.NewReader(strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"Hello "}}]}`,
		`data: {"choices":[{"delta":{"content":"world","tool_calls":[{"index":0,"id":"call_1","function":{"name":"lookup","arguments":"{\"a\":"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"1}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":3,"completion_tokens":4}}`,
		`data: [DONE]`,
	}, "\n"))

	result, err := (&Runner{}).parseSSE(reader)
	if err != nil {
		t.Fatalf("parseSSE returned error: %v", err)
	}

	if result.Content != "Hello world" {
		t.Fatalf("expected concatenated content, got %q", result.Content)
	}
	if result.StopReason != "tool_calls" {
		t.Fatalf("expected stop reason tool_calls, got %q", result.StopReason)
	}
	if result.Usage.PromptTokens != 3 || result.Usage.CompletionTokens != 4 {
		t.Fatalf("unexpected usage: %+v", result.Usage)
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(result.ToolCalls))
	}
	if result.ToolCalls[0].ID != "call_1" {
		t.Fatalf("expected tool call id call_1, got %q", result.ToolCalls[0].ID)
	}
	if result.ToolCalls[0].Function.Name != "lookup" {
		t.Fatalf("expected tool name lookup, got %q", result.ToolCalls[0].Function.Name)
	}
	if result.ToolCalls[0].Function.Arguments != `{"a":1}` {
		t.Fatalf("expected merged tool arguments, got %q", result.ToolCalls[0].Function.Arguments)
	}
}
