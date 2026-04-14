package shared

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolvePaperclipHomeDir_Default(t *testing.T) {
	// Clear env to test default
	os.Unsetenv("PAPERCLIP_HOME")

	result := ResolvePaperclipHomeDir()
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".paperclip")
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func TestResolvePaperclipHomeDir_EnvOverride(t *testing.T) {
	os.Setenv("PAPERCLIP_HOME", "/tmp/test-paperclip")
	defer os.Unsetenv("PAPERCLIP_HOME")

	result := ResolvePaperclipHomeDir()
	if result != "/tmp/test-paperclip" {
		t.Errorf("expected /tmp/test-paperclip, got %s", result)
	}
}

func TestResolvePaperclipHomeDir_TildeExpansion(t *testing.T) {
	os.Setenv("PAPERCLIP_HOME", "~/custom-paperclip")
	defer os.Unsetenv("PAPERCLIP_HOME")

	result := ResolvePaperclipHomeDir()
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, "custom-paperclip")
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func TestResolvePaperclipInstanceID_Default(t *testing.T) {
	os.Unsetenv("PAPERCLIP_INSTANCE_ID")

	result := ResolvePaperclipInstanceID()
	if result != DefaultInstanceID {
		t.Errorf("expected %s, got %s", DefaultInstanceID, result)
	}
}

func TestResolvePaperclipInstanceID_EnvOverride(t *testing.T) {
	os.Setenv("PAPERCLIP_INSTANCE_ID", "prod-1")
	defer os.Unsetenv("PAPERCLIP_INSTANCE_ID")

	result := ResolvePaperclipInstanceID()
	if result != "prod-1" {
		t.Errorf("expected prod-1, got %s", result)
	}
}

func TestResolvePaperclipInstanceRoot(t *testing.T) {
	os.Setenv("PAPERCLIP_HOME", "/tmp/test-pclip")
	os.Setenv("PAPERCLIP_INSTANCE_ID", "dev")
	defer os.Unsetenv("PAPERCLIP_HOME")
	defer os.Unsetenv("PAPERCLIP_INSTANCE_ID")

	result := ResolvePaperclipInstanceRoot()
	expected := filepath.Join("/tmp/test-pclip", "instances", "dev")
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func TestSanitizeFriendlyPathSegment(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		fallback string
		expected string
	}{
		{"normal", "hello-world", "fb", "hello-world"},
		{"with spaces", "hello world", "fb", "hello-world"},
		{"with special chars", "hello@#$world", "fb", "hello-world"},
		{"empty string", "", "fb", "fb"},
		{"whitespace only", "   ", "fb", "fb"},
		{"all special", "@#$", "fb", "fb"},
		{"dots allowed", "my.file", "fb", "my.file"},
		{"underscores allowed", "my_file", "fb", "my_file"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SanitizeFriendlyPathSegment(tc.input, tc.fallback)
			if got != tc.expected {
				t.Errorf("SanitizeFriendlyPathSegment(%q, %q) = %q, want %q", tc.input, tc.fallback, got, tc.expected)
			}
		})
	}
}

func TestResolveManagedProjectWorkspaceDir(t *testing.T) {
	os.Setenv("PAPERCLIP_HOME", "/tmp/test-pclip")
	os.Setenv("PAPERCLIP_INSTANCE_ID", "dev")
	defer os.Unsetenv("PAPERCLIP_HOME")
	defer os.Unsetenv("PAPERCLIP_INSTANCE_ID")

	result := ResolveManagedProjectWorkspaceDir("comp-1", "proj-1", "my-repo")
	if !strings.Contains(result, "projects") {
		t.Errorf("expected path to contain 'projects', got %s", result)
	}
	if !strings.Contains(result, "comp-1") {
		t.Errorf("expected path to contain 'comp-1', got %s", result)
	}
	if !strings.Contains(result, "proj-1") {
		t.Errorf("expected path to contain 'proj-1', got %s", result)
	}
	if !strings.Contains(result, "my-repo") {
		t.Errorf("expected path to contain 'my-repo', got %s", result)
	}
}

func TestResolveDefaultAgentWorkspaceDir(t *testing.T) {
	os.Setenv("PAPERCLIP_HOME", "/tmp/test-pclip")
	os.Setenv("PAPERCLIP_INSTANCE_ID", "dev")
	defer os.Unsetenv("PAPERCLIP_HOME")
	defer os.Unsetenv("PAPERCLIP_INSTANCE_ID")

	result := ResolveDefaultAgentWorkspaceDir("agent-123")
	if !strings.Contains(result, "workspaces") {
		t.Errorf("expected path to contain 'workspaces', got %s", result)
	}
	if !strings.Contains(result, "agent-123") {
		t.Errorf("expected path to contain 'agent-123', got %s", result)
	}
}

func TestPathSegmentRegex(t *testing.T) {
	valid := []string{"abc", "abc-def", "abc_def", "abc123"}
	for _, v := range valid {
		if !PathSegmentRegex.MatchString(v) {
			t.Errorf("expected %q to match path segment regex", v)
		}
	}

	invalid := []string{"abc def", "abc/def", "abc@def", ""}
	for _, v := range invalid {
		if PathSegmentRegex.MatchString(v) {
			t.Errorf("expected %q to NOT match path segment regex", v)
		}
	}
}

func TestExpandHomePrefix(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"tilde only", "~", home},
		{"tilde prefix", "~/dir", filepath.Join(home, "dir")},
		{"no tilde", "/usr/local", "/usr/local"},
		{"empty", "", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := expandHomePrefix(tc.input)
			if got != tc.expected {
				t.Errorf("expandHomePrefix(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}
