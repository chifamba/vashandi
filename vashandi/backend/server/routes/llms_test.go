package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListAgentConfigurationHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/llms/configs", nil)
	w := httptest.NewRecorder()

	ListAgentConfigurationHandler()(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	ct := w.Header().Get("Content-Type")
	if ct != "text/plain" {
		t.Errorf("expected Content-Type text/plain, got %q", ct)
	}

	body := w.Body.String()
	expectedParts := []string{"claude", "codex", "gemini", "cursor", "windsurf", "aider"}
	for _, part := range expectedParts {
		if !strings.Contains(body, part) {
			t.Errorf("expected body to contain %q", part)
		}
	}
}

func TestListAgentIconsHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/llms/icons", nil)
	w := httptest.NewRecorder()

	ListAgentIconsHandler()(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	expectedIcons := []string{"claude", "gpt", "gemini", "cursor", "windsurf", "aider", "robot", "brain"}
	for _, icon := range expectedIcons {
		if !strings.Contains(body, icon) {
			t.Errorf("expected body to contain icon %q", icon)
		}
	}
}

func TestListAgentConfigurationHandler_ContentType(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/llms/configs", nil)
	w := httptest.NewRecorder()

	ListAgentConfigurationHandler()(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "text/plain" {
		t.Errorf("expected Content-Type text/plain, got %q", ct)
	}
}

func TestListAgentIconsHandler_NonEmpty(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/llms/icons", nil)
	w := httptest.NewRecorder()

	ListAgentIconsHandler()(w, req)

	body := w.Body.String()
	lines := strings.Split(strings.TrimSpace(body), "\n")
	if len(lines) < 5 {
		t.Errorf("expected at least 5 icons, got %d lines", len(lines))
	}
}
