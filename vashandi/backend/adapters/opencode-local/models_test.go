package opencodelocal

import (
	"context"
	"testing"
)

func TestListOpenCodeModels_EmptyWhenCommandMissing(t *testing.T) {
	ResetModelsCacheForTests()
	// Use a command that does not exist so discovery fails gracefully.
	t.Setenv("PATH", "/nonexistent")
	models := ListOpenCodeModels(context.Background())
	// ListOpenCodeModels swallows the error and returns an empty/nil slice.
	if len(models) != 0 {
		t.Errorf("expected empty models, got %v", models)
	}
}

func TestEnsureOpenCodeModelConfiguredAndAvailable_RejectsEmpty(t *testing.T) {
	ResetModelsCacheForTests()
	_, err := EnsureOpenCodeModelConfiguredAndAvailable(context.Background(), "", "opencode", "/tmp", nil)
	if err == nil {
		t.Fatal("expected error for empty model, got nil")
	}
	if err.Error() == "" {
		t.Error("expected non-empty error message")
	}
}

func TestParseModelsOutput(t *testing.T) {
	stdout := `
anthropic/claude-sonnet-4-5
openai/gpt-5.2-codex
openai/gpt-5.4
gemini/gemini-2.5-pro
`
	models := parseModelsOutput(stdout)
	if len(models) != 4 {
		t.Fatalf("expected 4 models, got %d: %v", len(models), models)
	}
	// Verify all have the provider/model format.
	for _, m := range models {
		if m.ID == "" {
			t.Errorf("model has empty ID: %+v", m)
		}
		if m.Label == "" {
			t.Errorf("model has empty label: %+v", m)
		}
	}
}

func TestParseModelsOutput_IgnoresLinesWithoutSlash(t *testing.T) {
	stdout := "notamodel\nanthropiconly\nopenai/gpt-5.2-codex\n"
	models := parseModelsOutput(stdout)
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d: %v", len(models), models)
	}
	if models[0].ID != "openai/gpt-5.2-codex" {
		t.Errorf("unexpected model ID: %q", models[0].ID)
	}
}

func TestParseModelsOutput_Deduplication(t *testing.T) {
	stdout := "openai/gpt-5.2\nopenai/gpt-5.2\nopenai/gpt-5.4\n"
	models := parseModelsOutput(stdout)
	if len(models) != 2 {
		t.Fatalf("expected 2 unique models, got %d", len(models))
	}
}

func TestDefaultModels(t *testing.T) {
	models := DefaultModels()
	if len(models) == 0 {
		t.Fatal("expected at least one default model")
	}
	for _, m := range models {
		if !containsSlash(m.ID) {
			t.Errorf("default model ID %q is not in provider/model format", m.ID)
		}
	}
}

func containsSlash(s string) bool {
	for _, r := range s {
		if r == '/' {
			return true
		}
	}
	return false
}
