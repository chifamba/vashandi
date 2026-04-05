package models

import (
	"time"

	"gorm.io/datatypes"
)

type WorkspaceRuntimeService struct {
	ID                   string         `gorm:"column:id;type:uuid;primaryKey"`
	CompanyID            string         `gorm:"column:company_id;type:uuid;not null;index:workspace_runtime_services_company_workspace_status_idx;index:workspace_runtime_services_company_execution_workspace_status_idx;index:workspace_runtime_services_company_project_status_idx;index:workspace_runtime_services_company_updated_idx"`
	ProjectID            *string        `gorm:"column:project_id;type:uuid;index:workspace_runtime_services_company_project_status_idx"`
	ProjectWorkspaceID   *string        `gorm:"column:project_workspace_id;type:uuid;index:workspace_runtime_services_company_workspace_status_idx"`
	ExecutionWorkspaceID *string        `gorm:"column:execution_workspace_id;type:uuid;index:workspace_runtime_services_company_execution_workspace_status_idx"`
	IssueID              *string        `gorm:"column:issue_id;type:uuid"`
	ScopeType            string         `gorm:"column:scope_type;type:text;not null"`
	ScopeID              *string        `gorm:"column:scope_id;type:text"`
	ServiceName          string         `gorm:"column:service_name;type:text;not null"`
	Status               string         `gorm:"column:status;type:text;not null;index:workspace_runtime_services_company_workspace_status_idx;index:workspace_runtime_services_company_execution_workspace_status_idx;index:workspace_runtime_services_company_project_status_idx"`
	Lifecycle            string         `gorm:"column:lifecycle;type:text;not null"`
	ReuseKey             *string        `gorm:"column:reuse_key;type:text"`
	Command              *string        `gorm:"column:command;type:text"`
	CWD                  *string        `gorm:"column:cwd;type:text"`
	Port                 *int           `gorm:"column:port;type:integer"`
	URL                  *string        `gorm:"column:url;type:text"`
	Provider             string         `gorm:"column:provider;type:text;not null"`
	ProviderRef          *string        `gorm:"column:provider_ref;type:text"`
	OwnerAgentID         *string        `gorm:"column:owner_agent_id;type:uuid"`
	StartedByRunID       *string        `gorm:"column:started_by_run_id;type:uuid;index:workspace_runtime_services_run_idx"`
	LastUsedAt           time.Time      `gorm:"column:last_used_at;type:timestamptz;not null;default:now()"`
	StartedAt            time.Time      `gorm:"column:started_at;type:timestamptz;not null;default:now()"`
	StoppedAt            *time.Time     `gorm:"column:stopped_at;type:timestamptz"`
	StopPolicy           datatypes.JSON `gorm:"column:stop_policy;type:jsonb"`
	HealthStatus         string         `gorm:"column:health_status;type:text;not null;default:'unknown'"`
	CreatedAt            time.Time      `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt            time.Time      `gorm:"column:updated_at;type:timestamptz;not null;default:now();index:workspace_runtime_services_company_updated_idx"`

	Company            Company             `gorm:"foreignKey:CompanyID"`
	Project            *Project            `gorm:"foreignKey:ProjectID;constraint:OnDelete:SET NULL"`
	ProjectWorkspace   *ProjectWorkspace   `gorm:"foreignKey:ProjectWorkspaceID;constraint:OnDelete:SET NULL"`
	ExecutionWorkspace *ExecutionWorkspace `gorm:"foreignKey:ExecutionWorkspaceID;constraint:OnDelete:SET NULL"`
	Issue              *Issue              `gorm:"foreignKey:IssueID;constraint:OnDelete:SET NULL"`
	OwnerAgent         *Agent              `gorm:"foreignKey:OwnerAgentID;constraint:OnDelete:SET NULL"`
	StartedByRun       *HeartbeatRun       `gorm:"foreignKey:StartedByRunID;constraint:OnDelete:SET NULL"`
}

func (WorkspaceRuntimeService) TableName() string {
	return "workspace_runtime_services"
}
