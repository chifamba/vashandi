package services

import (
	"strings"
	"testing"
)

func TestInjectContextIntoPrompt_EmptyXML(t *testing.T) {
	prompt := "You are a helpful assistant."
	result := InjectContextIntoPrompt(prompt, "")
	if result != prompt {
		t.Errorf("expected original prompt when memoryXML is empty, got %q", result)
	}
}

func TestInjectContextIntoPrompt_WithXML(t *testing.T) {
	prompt := "You are a helpful assistant."
	xml := "<memory_context><memories count=\"1\"><memory title=\"test\" tier=\"1\" relevance=\"0.95\">some context</memory></memories></memory_context>"

	result := InjectContextIntoPrompt(prompt, xml)

	if result == prompt {
		t.Error("expected modified prompt when memoryXML is provided")
	}

	// Should contain the agent_memory wrapper
	if !strings.Contains(result, "<agent_memory>") {
		t.Error("expected result to contain <agent_memory> tag")
	}
	if !strings.Contains(result, "</agent_memory>") {
		t.Error("expected result to contain </agent_memory> closing tag")
	}
	// Should contain original prompt at the end
	if !strings.Contains(result, prompt) {
		t.Error("expected result to contain original prompt")
	}
	// Should contain the memory XML
	if !strings.Contains(result, xml) {
		t.Error("expected result to contain the memory XML")
	}
}

func TestXmlEscape(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"no escaping needed", "hello world", "hello world"},
		{"ampersand", "a & b", "a &amp; b"},
		{"less than", "a < b", "a &lt; b"},
		{"greater than", "a > b", "a &gt; b"},
		{"double quote", `he said "hello"`, "he said &quot;hello&quot;"},
		{"single quote", "it's fine", "it&#39;s fine"},
		{"all together", `<a & "b" > 'c'`, "&lt;a &amp; &quot;b&quot; &gt; &#39;c&#39;"},
		{"empty", "", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := xmlEscape(tc.input)
			if got != tc.expected {
				t.Errorf("xmlEscape(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestStringMapToAny(t *testing.T) {
	input := map[string]string{
		"key1": "val1",
		"key2": "val2",
	}
	result := stringMapToAny(input)
	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result))
	}
	if result["key1"] != "val1" {
		t.Errorf("expected key1=val1, got %v", result["key1"])
	}
	if result["key2"] != "val2" {
		t.Errorf("expected key2=val2, got %v", result["key2"])
	}
}

func TestStringMapToAny_Empty(t *testing.T) {
	result := stringMapToAny(map[string]string{})
	if len(result) != 0 {
		t.Fatalf("expected 0 entries for empty input, got %d", len(result))
	}
}
