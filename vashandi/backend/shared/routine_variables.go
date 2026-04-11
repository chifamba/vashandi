package shared

import (
	"encoding/json"
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
