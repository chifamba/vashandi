package services

import (
	"testing"
)

func TestTruncateExcerpt(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		max      int
		expected *string
	}{
		{"empty string", "", 10, nil},
		{"whitespace only", "   \n\t  ", 10, nil},
		{"short string", "hello", 10, stringPtr("hello")},
		{"exact length string", "1234567890", 10, stringPtr("1234567890")},
		{"long string", "this is a very long string", 10, stringPtr("this is...")},
		{"max less than 4", "hello", 3, stringPtr("hello")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateExcerpt(tt.input, tt.max)
			if got == nil && tt.expected == nil {
				return
			}
			if got == nil || tt.expected == nil {
				t.Fatalf("expected %v, got %v", tt.expected, got)
			}
			if *got != *tt.expected {
				t.Errorf("expected %q, got %q", *tt.expected, *got)
			}
		})
	}
}

func TestNormalizeFeedbackReason(t *testing.T) {
	tests := []struct {
		name     string
		vote     string
		reason   *string
		expected *string
	}{
		{"up vote with reason", "up", stringPtr("good"), nil},
		{"up vote no reason", "up", nil, nil},
		{"down vote no reason", "down", nil, nil},
		{"down vote empty reason", "down", stringPtr("   "), nil},
		{"down vote with reason", "down", stringPtr("  bad  "), stringPtr("bad")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeFeedbackReason(tt.vote, tt.reason)
			if got == nil && tt.expected == nil {
				return
			}
			if got == nil || tt.expected == nil {
				t.Fatalf("expected %v, got %v", tt.expected, got)
			}
			if *got != *tt.expected {
				t.Errorf("expected %q, got %q", *tt.expected, *got)
			}
		})
	}
}
