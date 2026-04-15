package pilocal

import (
	"strings"
	"testing"
)

func TestParseModelsOutput_ColumnarFormat(t *testing.T) {
	// Pi outputs a columnar table with 2+ space separators.
	stdout := strings.Join([]string{
		"provider   model           context  max-out  thinking  images",
		"anthropic  claude-3-5-sonnet  200000   8192     on        yes",
		"xai        grok-4             128000   16384    on        no",
		"openai     gpt-4o             128000   4096     off       yes",
	}, "\n")

	models := parseModelsOutput(stdout)
	if len(models) != 3 {
		t.Fatalf("expected 3 models, got %d: %v", len(models), models)
	}
	if models[0].ID != "anthropic/claude-3-5-sonnet" {
		t.Errorf("unexpected first model: %q", models[0].ID)
	}
	if models[1].ID != "xai/grok-4" {
		t.Errorf("unexpected second model: %q", models[1].ID)
	}
	if models[2].ID != "openai/gpt-4o" {
		t.Errorf("unexpected third model: %q", models[2].ID)
	}
}

func TestParseModelsOutput_DeduplicatesAndSorts(t *testing.T) {
	stdout := strings.Join([]string{
		"xai    grok-4    128000",
		"xai    grok-4    128000", // duplicate
		"anthropic  claude-3-5-sonnet  200000",
	}, "\n")

	models := sortModels(dedupeModels(parseModelsOutput(stdout)))
	if len(models) != 2 {
		t.Fatalf("expected 2 unique models, got %d", len(models))
	}
	// Sorted alphabetically: anthropic < xai
	if models[0].ID != "anthropic/claude-3-5-sonnet" {
		t.Errorf("expected anthropic first, got %q", models[0].ID)
	}
}

func TestParseModelsOutput_SkipsHeaderRow(t *testing.T) {
	stdout := strings.Join([]string{
		"provider   model",
		"xai        grok-4",
	}, "\n")

	models := parseModelsOutput(stdout)
	if len(models) != 1 {
		t.Fatalf("expected 1 model (header skipped), got %d", len(models))
	}
	if models[0].ID != "xai/grok-4" {
		t.Errorf("unexpected model: %q", models[0].ID)
	}
}

func TestParseModelsOutput_EmptyOutput(t *testing.T) {
	models := parseModelsOutput("")
	if len(models) != 0 {
		t.Fatalf("expected empty slice for empty output, got %v", models)
	}
}

func TestDiscoverPiModelsCached_ReturnsMissingOnUnknownCommand(t *testing.T) {
	ResetModelsCacheForTests()
	defer ResetModelsCacheForTests()

	_, err := DiscoverPiModels(DiscoverInput{Command: "__paperclip_missing_pi_command__"})
	if err == nil {
		t.Fatal("expected error for unknown command, got nil")
	}
}

func TestListPiModels_ReturnsEmptyOnMissingCommand(t *testing.T) {
	ResetModelsCacheForTests()
	defer ResetModelsCacheForTests()

	// Temporarily set PAPERCLIP_PI_COMMAND to a non-existent binary.
	t.Setenv("PAPERCLIP_PI_COMMAND", "__paperclip_missing_pi_command__")
	models := ListPiModels()
	if models != nil && len(models) != 0 {
		t.Fatalf("expected empty/nil slice, got %v", models)
	}
}

func TestEnsurePiModelConfiguredAndAvailable_RejectsEmptyModel(t *testing.T) {
	ResetModelsCacheForTests()
	defer ResetModelsCacheForTests()

	_, err := EnsurePiModelConfiguredAndAvailable("", DiscoverInput{})
	if err == nil {
		t.Fatal("expected error for empty model")
	}
	if !strings.Contains(err.Error(), "adapterConfig.model") {
		t.Errorf("expected error about adapterConfig.model, got: %v", err)
	}
}

func TestEnsurePiModelConfiguredAndAvailable_RejectsWhenDiscoveryFails(t *testing.T) {
	ResetModelsCacheForTests()
	defer ResetModelsCacheForTests()

	_, err := EnsurePiModelConfiguredAndAvailable("xai/grok-4", DiscoverInput{
		Command: "__paperclip_missing_pi_command__",
	})
	if err == nil {
		t.Fatal("expected error when discovery command fails")
	}
}

func TestResolvePiBiller_PrefersAPIKeyProvider(t *testing.T) {
	env := map[string]string{"ANTHROPIC_API_KEY": "sk-test"}
	biller := resolvePiBiller(env, "xai")
	if biller != "anthropic" {
		t.Errorf("expected 'anthropic', got %q", biller)
	}
}

func TestResolvePiBiller_FallsBackToProvider(t *testing.T) {
	env := map[string]string{}
	biller := resolvePiBiller(env, "xai")
	if biller != "xai" {
		t.Errorf("expected 'xai', got %q", biller)
	}
}

func TestResolvePiBiller_FallsBackToUnknown(t *testing.T) {
	env := map[string]string{}
	biller := resolvePiBiller(env, "")
	if biller != "unknown" {
		t.Errorf("expected 'unknown', got %q", biller)
	}
}

func TestParseModelProvider(t *testing.T) {
	tests := []struct{ in, want string }{
		{"xai/grok-4", "xai"},
		{"anthropic/claude-3-5-sonnet", "anthropic"},
		{"gpt-4o", ""},
		{"", ""},
		{"/gpt-4o", ""},
	}
	for _, tt := range tests {
		got := parseModelProvider(tt.in)
		if got != tt.want {
			t.Errorf("parseModelProvider(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseModelID(t *testing.T) {
	tests := []struct{ in, want string }{
		{"xai/grok-4", "grok-4"},
		{"anthropic/claude-3-5-sonnet", "claude-3-5-sonnet"},
		{"gpt-4o", "gpt-4o"},
		{"", ""},
	}
	for _, tt := range tests {
		got := parseModelID(tt.in)
		if got != tt.want {
			t.Errorf("parseModelID(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRenderTemplate(t *testing.T) {
	agent := AgentInfo{ID: "agent-1", Name: "My Agent", CompanyID: "company-1"}
	tmpl := "Hello {{agent.id}} ({{agent.name}}) run={{run.id}}"
	result := renderTemplate(tmpl, agent, "run-42")
	want := "Hello agent-1 (My Agent) run=run-42"
	if result != want {
		t.Errorf("renderTemplate = %q, want %q", result, want)
	}
}

func TestJoinPromptSections(t *testing.T) {
	sections := []string{"", "Section A", "  ", "Section B", ""}
	result := joinPromptSections(sections)
	want := "Section A\n\nSection B"
	if result != want {
		t.Errorf("joinPromptSections = %q, want %q", result, want)
	}
}
