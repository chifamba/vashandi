package shared

import (
	"testing"
)

func TestExtractRoutineVariableNames(t *testing.T) {
	tests := []struct {
		name      string
		templates []string
		want      []string
	}{
		{
			name:      "empty templates",
			templates: []string{},
			want:      nil,
		},
		{
			name:      "no variables",
			templates: []string{"Hello world"},
			want:      nil,
		},
		{
			name:      "single variable",
			templates: []string{"Hello {{name}}"},
			want:      []string{"name"},
		},
		{
			name:      "multiple variables",
			templates: []string{"{{greeting}} {{name}}, it is {{date}}"},
			want:      []string{"greeting", "name", "date"},
		},
		{
			name:      "variables with spaces",
			templates: []string{"{{ name }} and {{  spaced  }}"},
			want:      []string{"name", "spaced"},
		},
		{
			name:      "duplicate variables",
			templates: []string{"{{name}} and {{name}} again"},
			want:      []string{"name"},
		},
		{
			name:      "variables across templates",
			templates: []string{"{{first}}", "{{second}}"},
			want:      []string{"first", "second"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractRoutineVariableNames(tt.templates...)
			if len(got) != len(tt.want) {
				t.Errorf("ExtractRoutineVariableNames() = %v, want %v", got, tt.want)
				return
			}
			for i, v := range got {
				if v != tt.want[i] {
					t.Errorf("ExtractRoutineVariableNames()[%d] = %v, want %v", i, v, tt.want[i])
				}
			}
		})
	}
}

func TestInterpolateRoutineTemplate(t *testing.T) {
	tests := []struct {
		name     string
		template string
		values   map[string]interface{}
		want     string
	}{
		{
			name:     "empty template",
			template: "",
			values:   map[string]interface{}{"name": "test"},
			want:     "",
		},
		{
			name:     "no variables in template",
			template: "Hello world",
			values:   map[string]interface{}{"name": "test"},
			want:     "Hello world",
		},
		{
			name:     "single variable",
			template: "Hello {{name}}",
			values:   map[string]interface{}{"name": "World"},
			want:     "Hello World",
		},
		{
			name:     "multiple variables",
			template: "{{greeting}} {{name}}!",
			values:   map[string]interface{}{"greeting": "Hello", "name": "World"},
			want:     "Hello World!",
		},
		{
			name:     "missing variable kept",
			template: "Hello {{name}}",
			values:   map[string]interface{}{},
			want:     "Hello {{name}}",
		},
		{
			name:     "number value",
			template: "Count: {{count}}",
			values:   map[string]interface{}{"count": 42.0},
			want:     "Count: 42",
		},
		{
			name:     "boolean value",
			template: "Enabled: {{enabled}}",
			values:   map[string]interface{}{"enabled": true},
			want:     "Enabled: true",
		},
		{
			name:     "nil value",
			template: "Value: {{value}}",
			values:   map[string]interface{}{"value": nil},
			want:     "Value: ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InterpolateRoutineTemplate(tt.template, tt.values)
			if got != tt.want {
				t.Errorf("InterpolateRoutineTemplate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStringifyRoutineVariableValue(t *testing.T) {
	tests := []struct {
		name  string
		value interface{}
		want  string
	}{
		{"nil", nil, ""},
		{"string", "hello", "hello"},
		{"int", 42, "42"},
		{"float no decimal", 42.0, "42"},
		{"float with decimal", 42.5, "42.5"},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StringifyRoutineVariableValue(tt.value)
			if got != tt.want {
				t.Errorf("StringifyRoutineVariableValue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsBuiltinRoutineVariable(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"date", true},
		{"time", false},
		{"name", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsBuiltinRoutineVariable(tt.name); got != tt.want {
				t.Errorf("IsBuiltinRoutineVariable(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestSyncRoutineVariablesWithTemplate(t *testing.T) {
	existing := []RoutineVariable{
		{Name: "name", Type: RoutineVariableTypeText, Required: true, Options: []string{}},
	}

	t.Run("adds new variables", func(t *testing.T) {
		result := SyncRoutineVariablesWithTemplate([]string{"{{name}} {{age}}"}, existing)
		if len(result) != 2 {
			t.Errorf("expected 2 variables, got %d", len(result))
			return
		}
		if result[0].Name != "name" {
			t.Errorf("expected first variable to be 'name', got %q", result[0].Name)
		}
		if result[1].Name != "age" {
			t.Errorf("expected second variable to be 'age', got %q", result[1].Name)
		}
	})

	t.Run("preserves existing variable config", func(t *testing.T) {
		result := SyncRoutineVariablesWithTemplate([]string{"{{name}}"}, existing)
		if len(result) != 1 {
			t.Errorf("expected 1 variable, got %d", len(result))
			return
		}
		if result[0].Type != RoutineVariableTypeText {
			t.Errorf("expected type 'text', got %q", result[0].Type)
		}
	})

	t.Run("removes unused variables", func(t *testing.T) {
		result := SyncRoutineVariablesWithTemplate([]string{"{{other}}"}, existing)
		if len(result) != 1 {
			t.Errorf("expected 1 variable, got %d", len(result))
			return
		}
		if result[0].Name != "other" {
			t.Errorf("expected variable 'other', got %q", result[0].Name)
		}
	})

	t.Run("excludes builtin variables", func(t *testing.T) {
		result := SyncRoutineVariablesWithTemplate([]string{"{{date}} {{name}}"}, existing)
		if len(result) != 1 {
			t.Errorf("expected 1 variable (date should be excluded), got %d", len(result))
			return
		}
		if result[0].Name != "name" {
			t.Errorf("expected variable 'name', got %q", result[0].Name)
		}
	})
}

func TestGetBuiltinRoutineVariableValues(t *testing.T) {
	values := GetBuiltinRoutineVariableValues()
	if values == nil {
		t.Error("expected non-nil values")
		return
	}
	if _, ok := values["date"]; !ok {
		t.Error("expected 'date' builtin variable")
	}
	if dateVal, ok := values["date"].(string); !ok || len(dateVal) != 10 {
		t.Errorf("expected date to be 10 char string (YYYY-MM-DD), got %v", values["date"])
	}
}
