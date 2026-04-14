package services

import (
	"testing"
)

func TestDeriveRepoNameFromURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"https URL with .git", "https://github.com/user/repo.git", "repo"},
		{"https URL without .git", "https://github.com/user/repo", "repo"},
		{"ssh URL with .git", "git@github.com:user/repo.git", "repo"},
		{"bare name", "my-repo", "my-repo"},
		{"bare name with .git", "my-repo.git", "my-repo"},
		{"empty string", "", ""},
		{"nested path", "https://github.com/org/sub/repo.git", "repo"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := deriveRepoNameFromURL(tc.input)
			if got != tc.expected {
				t.Errorf("deriveRepoNameFromURL(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}
