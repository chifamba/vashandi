package shared

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type RoutineVariableType string

const (
	RoutineVariableTypeText    RoutineVariableType = "text"
	RoutineVariableTypeNumber  RoutineVariableType = "number"
	RoutineVariableTypeBoolean RoutineVariableType = "boolean"
)

// Represents string | number | boolean | null
type RoutineVariableDefaultValue struct {
	Value interface{}
}

func (r *RoutineVariableDefaultValue) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.Value)
}

func (r *RoutineVariableDefaultValue) UnmarshalJSON(data []byte) error {
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	r.Value = v
	return nil
}

type RoutineVariable struct {
	Name         string                      `json:"name"`
	Label        *string                     `json:"label"`
	Type         RoutineVariableType         `json:"type"`
	DefaultValue RoutineVariableDefaultValue `json:"defaultValue"`
	Required     bool                        `json:"required"`
	Options      []string                    `json:"options"`
}

type RoutineProjectSummary struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	Status      string  `json:"status"`
	GoalID      *string `json:"goalId,omitempty"`
}

type RoutineAgentSummary struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Role   string  `json:"role"`
	Title  *string `json:"title"`
	URLKey *string `json:"urlKey,omitempty"`
}

type RoutineIssueSummary struct {
	ID         string    `json:"id"`
	Identifier *string   `json:"identifier"`
	Title      string    `json:"title"`
	Status     string    `json:"status"`
	Priority   string    `json:"priority"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type Routine struct {
	ID                string            `json:"id"`
	CompanyID         string            `json:"companyId"`
	ProjectID         string            `json:"projectId"`
	GoalID            *string           `json:"goalId"`
	ParentIssueID     *string           `json:"parentIssueId"`
	Title             string            `json:"title"`
	Description       *string           `json:"description"`
	AssigneeAgentID   string            `json:"assigneeAgentId"`
	Priority          string            `json:"priority"`
	Status            string            `json:"status"`
	ConcurrencyPolicy string            `json:"concurrencyPolicy"`
	CatchUpPolicy     string            `json:"catchUpPolicy"`
	Variables         []RoutineVariable `json:"variables"`
	CreatedByAgentID  *string           `json:"createdByAgentId"`
	CreatedByUserID   *string           `json:"createdByUserId"`
	UpdatedByAgentID  *string           `json:"updatedByAgentId"`
	UpdatedByUserID   *string           `json:"updatedByUserId"`
	LastTriggeredAt   *time.Time        `json:"lastTriggeredAt"`
	LastEnqueuedAt    *time.Time        `json:"lastEnqueuedAt"`
	CreatedAt         time.Time         `json:"createdAt"`
	UpdatedAt         time.Time         `json:"updatedAt"`
}

// RoutineVariableMatcher matches variable placeholders like {{variableName}} in templates.
var RoutineVariableMatcher = regexp.MustCompile(`\{\{\s*([A-Za-z][A-Za-z0-9_]*)\s*\}\}`)

// BuiltinRoutineVariableNames contains the set of automatically available variables.
var BuiltinRoutineVariableNames = map[string]bool{
	"date": true,
}

// IsBuiltinRoutineVariable returns true if the variable name is a built-in.
func IsBuiltinRoutineVariable(name string) bool {
	return BuiltinRoutineVariableNames[name]
}

// GetBuiltinRoutineVariableValues returns current values for all built-in routine variables.
// `date` expands to the current date in YYYY-MM-DD format (UTC).
func GetBuiltinRoutineVariableValues() map[string]interface{} {
	return map[string]interface{}{
		"date": time.Now().UTC().Format("2006-01-02"),
	}
}

// IsValidRoutineVariableName validates a routine variable name.
func IsValidRoutineVariableName(name string) bool {
	matched, _ := regexp.MatchString(`^[A-Za-z][A-Za-z0-9_]*$`, name)
	return matched
}

// ExtractRoutineVariableNames extracts all variable names from templates.
func ExtractRoutineVariableNames(templates ...string) []string {
	found := make(map[string]bool)
	var result []string

	for _, template := range templates {
		if template == "" {
			continue
		}
		matches := RoutineVariableMatcher.FindAllStringSubmatch(template, -1)
		for _, match := range matches {
			if len(match) > 1 {
				name := match[1]
				if !found[name] {
					found[name] = true
					result = append(result, name)
				}
			}
		}
	}
	return result
}

// DefaultRoutineVariable creates a default variable definition for a name.
func DefaultRoutineVariable(name string) RoutineVariable {
	return RoutineVariable{
		Name:         name,
		Label:        nil,
		Type:         RoutineVariableTypeText,
		DefaultValue: RoutineVariableDefaultValue{Value: nil},
		Required:     true,
		Options:      []string{},
	}
}

// SyncRoutineVariablesWithTemplate synchronizes variables with template placeholders.
func SyncRoutineVariablesWithTemplate(templates []string, existing []RoutineVariable) []RoutineVariable {
	names := ExtractRoutineVariableNames(templates...)

	// Filter out built-in variables
	var filteredNames []string
	for _, name := range names {
		if !IsBuiltinRoutineVariable(name) {
			filteredNames = append(filteredNames, name)
		}
	}

	// Build lookup for existing variables
	existingByName := make(map[string]RoutineVariable)
	for _, v := range existing {
		existingByName[v.Name] = v
	}

	// Build result, preserving existing definitions
	result := make([]RoutineVariable, 0, len(filteredNames))
	for _, name := range filteredNames {
		if v, ok := existingByName[name]; ok {
			result = append(result, v)
		} else {
			result = append(result, DefaultRoutineVariable(name))
		}
	}
	return result
}

// StringifyRoutineVariableValue converts a variable value to string.
func StringifyRoutineVariableValue(value interface{}) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(data)
	}
}

// InterpolateRoutineTemplate replaces variable placeholders with values.
func InterpolateRoutineTemplate(template string, values map[string]interface{}) string {
	if template == "" || len(values) == 0 {
		return template
	}

	return RoutineVariableMatcher.ReplaceAllStringFunc(template, func(match string) string {
		submatch := RoutineVariableMatcher.FindStringSubmatch(match)
		if len(submatch) < 2 {
			return match
		}
		name := submatch[1]
		if val, ok := values[name]; ok {
			return StringifyRoutineVariableValue(val)
		}
		return match
	})
}

// InterpolateRoutineTemplatePtr handles nullable template strings.
func InterpolateRoutineTemplatePtr(template *string, values map[string]interface{}) *string {
	if template == nil {
		return nil
	}
	result := InterpolateRoutineTemplate(*template, values)
	return &result
}

// NormalizeRoutineVariableValue normalizes a raw value based on variable type.
func NormalizeRoutineVariableValue(variable RoutineVariable, raw interface{}) (interface{}, error) {
	if raw == nil {
		return nil, nil
	}

	switch variable.Type {
	case RoutineVariableTypeBoolean:
		return ParseBooleanVariableValue(variable.Name, raw)
	case RoutineVariableTypeNumber:
		return ParseNumberVariableValue(variable.Name, raw)
	default:
		normalized := StringifyRoutineVariableValue(raw)
		if variable.Type == "select" && len(variable.Options) > 0 {
			found := false
			for _, opt := range variable.Options {
				if opt == normalized {
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("variable %q must match one of: %s", variable.Name, strings.Join(variable.Options, ", "))
			}
		}
		return normalized, nil
	}
}

// ParseBooleanVariableValue parses a value as boolean.
func ParseBooleanVariableValue(name string, raw interface{}) (bool, error) {
	switch v := raw.(type) {
	case bool:
		return v, nil
	case float64:
		if v == 0 || v == 1 {
			return v == 1, nil
		}
	case int:
		if v == 0 || v == 1 {
			return v == 1, nil
		}
	case string:
		normalized := strings.ToLower(strings.TrimSpace(v))
		switch normalized {
		case "true", "1", "yes", "y", "on":
			return true, nil
		case "false", "0", "no", "n", "off":
			return false, nil
		}
	}
	return false, fmt.Errorf("variable %q must be a boolean", name)
}

// ParseNumberVariableValue parses a value as number.
func ParseNumberVariableValue(name string, raw interface{}) (float64, error) {
	switch v := raw.(type) {
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case string:
		if strings.TrimSpace(v) != "" {
			parsed, err := strconv.ParseFloat(v, 64)
			if err == nil {
				return parsed, nil
			}
		}
	}
	return 0, fmt.Errorf("variable %q must be a number", name)
}

// IsMissingRoutineVariableValue checks if a value is considered missing.
func IsMissingRoutineVariableValue(value interface{}) bool {
	if value == nil {
		return true
	}
	if s, ok := value.(string); ok {
		return strings.TrimSpace(s) == ""
	}
	return false
}

// ResolveRoutineVariableValues resolves variable values from provided inputs.
func ResolveRoutineVariableValues(
	variables []RoutineVariable,
	source string,
	payload map[string]interface{},
	providedVars map[string]interface{},
) (map[string]interface{}, error) {
	if len(variables) == 0 {
		return map[string]interface{}{}, nil
	}

	// Collect provided values
	provided := make(map[string]interface{})
	if source == "webhook" && payload != nil {
		for k, v := range payload {
			if k != "variables" {
				provided[k] = v
			}
		}
	}
	if nestedVars, ok := payload["variables"].(map[string]interface{}); ok {
		for k, v := range nestedVars {
			provided[k] = v
		}
	}
	for k, v := range providedVars {
		provided[k] = v
	}

	// Resolve each variable
	resolved := make(map[string]interface{})
	var missing []string

	for _, variable := range variables {
		candidate := provided[variable.Name]
		if candidate == nil {
			candidate = variable.DefaultValue.Value
		}

		normalized, err := NormalizeRoutineVariableValue(variable, candidate)
		if err != nil {
			return nil, err
		}

		if IsMissingRoutineVariableValue(normalized) {
			if variable.Required {
				missing = append(missing, variable.Name)
			}
			continue
		}
		resolved[variable.Name] = normalized
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("missing routine variables: %s", strings.Join(missing, ", "))
	}

	return resolved, nil
}

// MergeRoutineRunPayload merges payload with resolved variables.
func MergeRoutineRunPayload(payload map[string]interface{}, variables map[string]interface{}) map[string]interface{} {
	if len(variables) == 0 {
		return payload
	}
	if payload == nil {
		return map[string]interface{}{"variables": variables}
	}

	result := make(map[string]interface{})
	for k, v := range payload {
		result[k] = v
	}

	existingVars := map[string]interface{}{}
	if ev, ok := result["variables"].(map[string]interface{}); ok {
		existingVars = ev
	}
	mergedVars := make(map[string]interface{})
	for k, v := range existingVars {
		mergedVars[k] = v
	}
	for k, v := range variables {
		mergedVars[k] = v
	}
	result["variables"] = mergedVars
	return result
}
