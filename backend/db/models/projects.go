package models

import (
	"time"

	"gorm.io/datatypes"
)

type Project struct {
	ID                       string         `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID                string         `gorm:"column:company_id;type:uuid;not null;index:projects_company_idx"`
	GoalID                   *string        `gorm:"column:goal_id;type:uuid"`
	Name                     string         `gorm:"column:name;not null"`
	Description              *string        `gorm:"column:description"`
	Status                   string         `gorm:"column:status;not null;default:backlog"`
	LeadAgentID              *string        `gorm:"column:lead_agent_id;type:uuid"`
	TargetDate               *time.Time     `gorm:"column:target_date;type:date"`
	Color                    *string        `gorm:"column:color"`
	PauseReason              *string        `gorm:"column:pause_reason"`
	PausedAt                 *time.Time     `gorm:"column:paused_at;type:timestamptz"`
	ExecutionWorkspacePolicy datatypes.JSON `gorm:"column:execution_workspace_policy;type:jsonb"`
	ArchivedAt               *time.Time     `gorm:"column:archived_at;type:timestamptz"`
	CreatedAt                time.Time      `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt                time.Time      `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	Company   Company `gorm:"foreignKey:CompanyID"`
	LeadAgent *Agent  `gorm:"foreignKey:LeadAgentID"`
}

func (Project) TableName() string {
	return "projects"
}
