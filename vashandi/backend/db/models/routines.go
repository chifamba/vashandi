package models

import (
	"time"

	"gorm.io/datatypes"
)

type Routine struct {
	ID                string         `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID         string         `gorm:"column:company_id;type:uuid;not null;index:routines_company_idx"`
	ProjectID         string         `gorm:"column:project_id;type:uuid;not null"`
	GoalID            *string        `gorm:"column:goal_id;type:uuid"`
	ParentIssueID     *string        `gorm:"column:parent_issue_id;type:uuid"`
	Title             string         `gorm:"column:title;not null"`
	Description       *string        `gorm:"column:description"`
	AssigneeAgentID   string         `gorm:"column:assignee_agent_id;type:uuid;not null"`
	Priority          string         `gorm:"column:priority;not null;default:medium"`
	Status            string         `gorm:"column:status;not null;default:active"`
	ConcurrencyPolicy string         `gorm:"column:concurrency_policy;not null;default:coalesce_if_active"`
	CatchUpPolicy     string         `gorm:"column:catch_up_policy;not null;default:skip_missed"`
	Variables         datatypes.JSON `gorm:"column:variables;type:jsonb;not null;default:'[]'"`
	CreatedByAgentID  *string        `gorm:"column:created_by_agent_id;type:uuid"`
	CreatedByUserID   *string        `gorm:"column:created_by_user_id"`
	UpdatedByAgentID  *string        `gorm:"column:updated_by_agent_id;type:uuid"`
	UpdatedByUserID   *string        `gorm:"column:updated_by_user_id"`
	LastTriggeredAt   *time.Time     `gorm:"column:last_triggered_at;type:timestamptz"`
	LastEnqueuedAt    *time.Time     `gorm:"column:last_enqueued_at;type:timestamptz"`
	CreatedAt         time.Time      `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt         time.Time      `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	Company       Company `gorm:"foreignKey:CompanyID;constraint:OnDelete:CASCADE"`
	Project       Project `gorm:"foreignKey:ProjectID;constraint:OnDelete:CASCADE"`
	AssigneeAgent Agent   `gorm:"foreignKey:AssigneeAgentID"`
}

func (Routine) TableName() string {
	return "routines"
}
