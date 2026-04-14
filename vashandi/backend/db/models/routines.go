package models

import (
	"time"

	"gorm.io/datatypes"
)

type RoutineTrigger struct {
	ID              string     `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID       string     `gorm:"column:company_id;type:uuid;not null;index:routine_triggers_company_routine_idx;index:routine_triggers_company_kind_idx"`
	RoutineID       string     `gorm:"column:routine_id;type:uuid;not null;index:routine_triggers_company_routine_idx"`
	Kind            string     `gorm:"column:kind;not null;index:routine_triggers_company_kind_idx"`
	Label           *string    `gorm:"column:label"`
	Enabled         bool       `gorm:"column:enabled;not null;default:true"`
	CronExpression  *string    `gorm:"column:cron_expression"`
	Timezone        *string    `gorm:"column:timezone"`
	NextRunAt       *time.Time `gorm:"column:next_run_at;type:timestamptz;index:routine_triggers_next_run_idx"`
	LastFiredAt     *time.Time `gorm:"column:last_fired_at;type:timestamptz"`
	PublicID        *string    `gorm:"column:public_id;uniqueIndex:routine_triggers_public_id_uq;index:routine_triggers_public_id_idx"`
	SecretID        *string    `gorm:"column:secret_id;type:uuid"`
	SigningMode     *string    `gorm:"column:signing_mode"`
	ReplayWindowSec *int       `gorm:"column:replay_window_sec"`
	LastRotatedAt   *time.Time `gorm:"column:last_rotated_at;type:timestamptz"`
	LastResult      *string    `gorm:"column:last_result"`
	CreatedByAgentID *string   `gorm:"column:created_by_agent_id;type:uuid"`
	CreatedByUserID  *string   `gorm:"column:created_by_user_id"`
	UpdatedByAgentID *string   `gorm:"column:updated_by_agent_id;type:uuid"`
	UpdatedByUserID  *string   `gorm:"column:updated_by_user_id"`
	CreatedAt       time.Time  `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt       time.Time  `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	Company Company `gorm:"foreignKey:CompanyID;constraint:OnDelete:CASCADE"`
	Routine Routine `gorm:"foreignKey:RoutineID;constraint:OnDelete:CASCADE"`
}

func (RoutineTrigger) TableName() string {
	return "routine_triggers"
}

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
