package services

import (
	"path/filepath"
	"testing"
)

func TestResolveServiceCommand(t *testing.T) {
	tests := []struct {
		name     string
		service  map[string]interface{}
		expected string
	}{
		{
			name: "has command",
			service: map[string]interface{}{
				"command": "npm start",
			},
			expected: "npm start",
		},
		{
			name:     "no command",
			service:  map[string]interface{}{},
			expected: "",
		},
		{
			name: "command is not string",
			service: map[string]interface{}{
				"command": 123,
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveServiceCommand(tt.service)
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestResolveServiceCwd(t *testing.T) {
	tests := []struct {
		name         string
		service      map[string]interface{}
		workspaceCwd string
		expected     string
	}{
		{
			name: "absolute cwd",
			service: map[string]interface{}{
				"cwd": "/absolute/path",
			},
			workspaceCwd: "/workspace",
			expected:     "/absolute/path",
		},
		{
			name: "relative cwd",
			service: map[string]interface{}{
				"cwd": "relative/path",
			},
			workspaceCwd: "/workspace",
			expected:     filepath.Join("/workspace", "relative/path"),
		},
		{
			name:         "empty cwd",
			service:      map[string]interface{}{"cwd": ""},
			workspaceCwd: "/workspace",
			expected:     "/workspace",
		},
		{
			name:         "no cwd",
			service:      map[string]interface{}{},
			workspaceCwd: "/workspace",
			expected:     "/workspace",
		},
		{
			name: "cwd is not string",
			service: map[string]interface{}{
				"cwd": 123,
			},
			workspaceCwd: "/workspace",
			expected:     "/workspace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveServiceCwd(tt.service, tt.workspaceCwd)
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}
