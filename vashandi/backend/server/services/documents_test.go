package services

import (
	"testing"
)

func TestExtractLegacyPlanBody(t *testing.T) {
	tests := []struct {
		name        string
		description *string
		want        *string
	}{
		{
			name:        "nil description",
			description: nil,
			want:        nil,
		},
		{
			name:        "empty description",
			description: docStringPtr(""),
			want:        nil,
		},
		{
			name:        "no plan tag",
			description: docStringPtr("This is a regular description without plan tags"),
			want:        nil,
		},
		{
			name:        "plan tag with content",
			description: docStringPtr("Some intro text <plan>This is the plan content</plan> and more text"),
			want:        docStringPtr("This is the plan content"),
		},
		{
			name:        "plan tag with whitespace",
			description: docStringPtr("<plan>   Plan with leading and trailing whitespace   </plan>"),
			want:        docStringPtr("Plan with leading and trailing whitespace"),
		},
		{
			name:        "plan tag multiline",
			description: docStringPtr("<plan>\nLine 1\nLine 2\nLine 3\n</plan>"),
			want:        docStringPtr("Line 1\nLine 2\nLine 3"),
		},
		{
			name:        "plan tag case insensitive",
			description: docStringPtr("<PLAN>Uppercase plan tag</PLAN>"),
			want:        docStringPtr("Uppercase plan tag"),
		},
		{
			name:        "empty plan tag",
			description: docStringPtr("<plan></plan>"),
			want:        nil,
		},
		{
			name:        "plan tag with only whitespace",
			description: docStringPtr("<plan>   </plan>"),
			want:        nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractLegacyPlanBody(tt.description)
			if tt.want == nil {
				if got != nil {
					t.Errorf("ExtractLegacyPlanBody() = %v, want nil", *got)
				}
			} else {
				if got == nil {
					t.Errorf("ExtractLegacyPlanBody() = nil, want %v", *tt.want)
				} else if *got != *tt.want {
					t.Errorf("ExtractLegacyPlanBody() = %v, want %v", *got, *tt.want)
				}
			}
		})
	}
}

func TestNormalizeDocumentKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		want    string
		wantErr bool
	}{
		{
			name: "valid lowercase key",
			key:  "plan",
			want: "plan",
		},
		{
			name: "uppercase to lowercase",
			key:  "PLAN",
			want: "plan",
		},
		{
			name: "with hyphen",
			key:  "my-plan",
			want: "my-plan",
		},
		{
			name: "with underscore",
			key:  "my_plan",
			want: "my_plan",
		},
		{
			name: "with numbers",
			key:  "plan123",
			want: "plan123",
		},
		{
			name: "starts with number",
			key:  "123plan",
			want: "123plan",
		},
		{
			name:    "empty string",
			key:     "",
			wantErr: true,
		},
		{
			name:    "only whitespace",
			key:     "   ",
			wantErr: true,
		},
		{
			name:    "invalid characters",
			key:     "plan@special",
			wantErr: true,
		},
		{
			name:    "starts with hyphen",
			key:     "-plan",
			wantErr: true,
		},
		{
			name:    "starts with underscore",
			key:     "_plan",
			wantErr: true,
		},
		{
			name: "with leading/trailing whitespace",
			key:  "  plan  ",
			want: "plan",
		},
		{
			name:    "too long",
			key:     "this_is_a_very_long_document_key_that_exceeds_the_maximum_allowed_length_of_64_characters",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeDocumentKey(tt.key)
			if tt.wantErr {
				if err == nil {
					t.Errorf("normalizeDocumentKey() error = nil, wantErr = true")
				}
			} else {
				if err != nil {
					t.Errorf("normalizeDocumentKey() error = %v, wantErr = false", err)
				} else if got != tt.want {
					t.Errorf("normalizeDocumentKey() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestIntToString(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{10, "10"},
		{123, "123"},
		{-1, "-1"},
		{-123, "-123"},
	}

	for _, tt := range tests {
		got := intToString(tt.n)
		if got != tt.want {
			t.Errorf("intToString(%d) = %v, want %v", tt.n, got, tt.want)
		}
	}
}

// helper to create string pointer
func docStringPtr(s string) *string {
	return &s
}
