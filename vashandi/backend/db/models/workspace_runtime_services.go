package models

import (
	"time"

	"gorm.io/datatypes"
)

type WorkspaceRuntimeService struct {
	ID                   string         `gorm:"column:id;type:uuid;primaryKey"`
	CompanyID            string         `gorm:"column:company_id;type:uuid;not null;index:workspace_runtime_services_company_scope_idx;index:workspace_runtime_services_company_issue_idx;index:workspace_runtime_services_company_project_idx;index:workspace_runtime_services_company_status_idx"`
	ProjectID            *string        `gorm:"column:project_id;type:uuid;index:workspace_runtime_services_company_project_idx"`
	ProjectWorkspaceID   *string        `gorm:"column:project_workspace_id;type:uuid"`
	ExecutionWorkspaceID *string        `gorm:"column:execution_workspace_id;type:uuid"`
	IssueID              *string        `gorm:"column:issue_id;type:uuid;index:workspace_runtime_services_company_issue_idx"`
	ScopeType            string         `gorm:"column:scope_type;not null;index:workspace_runtime_services_company_scope_idx"`
	ScopeID              *string        `gorm:"column:scope_id;index:workspace_runtime_services_company_scope_idx"`
	ServiceName          string         `gorm:"column:service_name;not null"`
	Status               string         `gorm:"column:status;not null;index:workspace_runtime_services_company_status_idx"`
	Lifecycle            string         `gorm:"column:lifecycle;not null"`
	ReuseKey             *string        `gorm:"column:reuse_key"`
	Command              *string        `gorm:"column:command"`
	Cwd                  *string        `gorm:"column:cwd"`
	Port                 *int           `gorm:"column:port"`
	URL                  *string        `gorm:"column:url"`
	Provider             string         `gorm:"column:provider;not null"`
	ProviderRef          *string        `gorm:"column:provider_ref"`
	OwnerAgentID         *string        `gorm:"column:owner_agent_id;type:uuid"`
	StartedByRunID       *string        `gorm:"column:started_by_run_id;type:uuid"`
	LastUsedAt           time.Time      `gorm:"column:last_used_at;type:timestamptz;not null;default:now()"`
	StartedAt            time.Time      `gorm:"column:started_at;type:timestamptz;not null;default:now()"`
	StoppedAt            *time.Time     `gorm:"column:stopped_at;type:timestamptz"`
	StopPolicy           datatypes.JSON `gorm:"column:stop_policy;type:jsonb"`
	HealthStatus         string         `gorm:"column:health_status;not null;default:unknown"`
	CreatedAt            time.Time      `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt            time.Time      `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	Company            Company            `gorm:"foreignKey:CompanyID"`
	Project            *Project           `gorm:"foreignKey:ProjectID;constraint:OnDelete:SET NULL"`
	ProjectWorkspace   *ProjectWorkspace  `gorm:"foreignKey:ProjectWorkspaceID;constraint:OnDelete:SET NULL"`
	ExecutionWorkspace *ExecutionWorkspace `gorm:"foreignKey:ExecutionWorkspaceID;constraint:OnDelete:SET NULL"`
	Issue              *Issue             `gorm:"foreignKey:IssueID;constraint:OnDelete:SET NULL"`
	OwnerAgent         *Agent             `gorm:"foreignKey:OwnerAgentID;constraint:OnDelete:SET NULL"`
	StartedByRun       *HeartbeatRun      `gorm:"foreignKey:StartedByRunID;constraint:OnDelete:SET NULL"`
}

func (WorkspaceRuntimeService) TableName() string {
	return "workspace_runtime_services"
}
