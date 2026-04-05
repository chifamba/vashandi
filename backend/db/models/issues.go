package models

import (
	"time"

	"gorm.io/datatypes"
)

type Issue struct {
	ID                           string         `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID                    string         `gorm:"column:company_id;type:uuid;not null;index:issues_company_status_idx;index:issues_company_assignee_status_idx;index:issues_company_assignee_user_status_idx;index:issues_company_parent_idx;index:issues_company_project_idx;index:issues_company_origin_idx;index:issues_company_project_workspace_idx;index:issues_company_execution_workspace_idx;index:issues_open_routine_execution_uq"`
	ProjectID                    *string        `gorm:"column:project_id;type:uuid;index:issues_company_project_idx"`
	ProjectWorkspaceID           *string        `gorm:"column:project_workspace_id;type:uuid;index:issues_company_project_workspace_idx"`
	GoalID                       *string        `gorm:"column:goal_id;type:uuid"`
	ParentID                     *string        `gorm:"column:parent_id;type:uuid;index:issues_company_parent_idx"`
	Title                        string         `gorm:"column:title;not null"`
	Description                  *string        `gorm:"column:description"`
	Status                       string         `gorm:"column:status;not null;default:backlog;index:issues_company_status_idx;index:issues_company_assignee_status_idx;index:issues_company_assignee_user_status_idx"`
	Priority                     string         `gorm:"column:priority;not null;default:medium"`
	AssigneeAgentID              *string        `gorm:"column:assignee_agent_id;type:uuid;index:issues_company_assignee_status_idx"`
	AssigneeUserID               *string        `gorm:"column:assignee_user_id;index:issues_company_assignee_user_status_idx"`
	CheckoutRunID                *string        `gorm:"column:checkout_run_id;type:uuid"`
	ExecutionRunID               *string        `gorm:"column:execution_run_id;type:uuid"`
	ExecutionAgentNameKey        *string        `gorm:"column:execution_agent_name_key"`
	ExecutionLockedAt            *time.Time     `gorm:"column:execution_locked_at;type:timestamptz"`
	CreatedByAgentID             *string        `gorm:"column:created_by_agent_id;type:uuid"`
	CreatedByUserID              *string        `gorm:"column:created_by_user_id"`
	IssueNumber                  *int           `gorm:"column:issue_number"`
	Identifier                   *string        `gorm:"column:identifier;uniqueIndex:issues_identifier_idx"`
	OriginKind                   string         `gorm:"column:origin_kind;not null;default:manual;index:issues_company_origin_idx;index:issues_open_routine_execution_uq"`
	OriginID                     *string        `gorm:"column:origin_id;index:issues_company_origin_idx;index:issues_open_routine_execution_uq"`
	OriginRunID                  *string        `gorm:"column:origin_run_id"`
	RequestDepth                 int            `gorm:"column:request_depth;not null;default:0"`
	BillingCode                  *string        `gorm:"column:billing_code"`
	AssigneeAdapterOverrides     datatypes.JSON `gorm:"column:assignee_adapter_overrides;type:jsonb"`
	ExecutionWorkspaceID         *string        `gorm:"column:execution_workspace_id;type:uuid;index:issues_company_execution_workspace_idx"`
	ExecutionWorkspacePreference *string        `gorm:"column:execution_workspace_preference"`
	ExecutionWorkspaceSettings   datatypes.JSON `gorm:"column:execution_workspace_settings;type:jsonb"`
	StartedAt                    *time.Time     `gorm:"column:started_at;type:timestamptz"`
	CompletedAt                  *time.Time     `gorm:"column:completed_at;type:timestamptz"`
	CancelledAt                  *time.Time     `gorm:"column:cancelled_at;type:timestamptz"`
	HiddenAt                     *time.Time     `gorm:"column:hidden_at;type:timestamptz"`
	CreatedAt                    time.Time      `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt                    time.Time      `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	Company          Company       `gorm:"foreignKey:CompanyID"`
	Project          *Project      `gorm:"foreignKey:ProjectID"`
	Parent           *Issue        `gorm:"foreignKey:ParentID"`
	AssigneeAgent    *Agent        `gorm:"foreignKey:AssigneeAgentID"`
	CheckoutRun      *HeartbeatRun `gorm:"foreignKey:CheckoutRunID;constraint:OnDelete:SET NULL"`
	ExecutionRun     *HeartbeatRun `gorm:"foreignKey:ExecutionRunID;constraint:OnDelete:SET NULL"`
	CreatedByAgent   *Agent        `gorm:"foreignKey:CreatedByAgentID"`
}

func (Issue) TableName() string {
	return "issues"
}
