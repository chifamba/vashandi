package models

import (
	"time"

	"gorm.io/datatypes"
)

type ExecutionWorkspace struct {
	ID                              string         `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID                       string         `gorm:"column:company_id;type:uuid;not null;index:execution_workspaces_company_project_status_idx;index:execution_workspaces_company_project_workspace_status_idx;index:execution_workspaces_company_source_issue_idx;index:execution_workspaces_company_last_used_idx;index:execution_workspaces_company_branch_idx"`
	ProjectID                       string         `gorm:"column:project_id;type:uuid;not null;index:execution_workspaces_company_project_status_idx"`
	ProjectWorkspaceID              *string        `gorm:"column:project_workspace_id;type:uuid;index:execution_workspaces_company_project_workspace_status_idx"`
	SourceIssueID                   *string        `gorm:"column:source_issue_id;type:uuid;index:execution_workspaces_company_source_issue_idx"`
	Mode                            string         `gorm:"column:mode;not null"`
	StrategyType                    string         `gorm:"column:strategy_type;not null"`
	Name                            string         `gorm:"column:name;not null"`
	Status                          string         `gorm:"column:status;not null;default:active;index:execution_workspaces_company_project_status_idx;index:execution_workspaces_company_project_workspace_status_idx"`
	Cwd                             *string        `gorm:"column:cwd"`
	RepoURL                         *string        `gorm:"column:repo_url"`
	BaseRef                         *string        `gorm:"column:base_ref"`
	BranchName                      *string        `gorm:"column:branch_name;index:execution_workspaces_company_branch_idx"`
	ProviderType                    string         `gorm:"column:provider_type;not null;default:local_fs"`
	ProviderRef                     *string        `gorm:"column:provider_ref"`
	DerivedFromExecutionWorkspaceID *string        `gorm:"column:derived_from_execution_workspace_id;type:uuid"`
	LastUsedAt                      time.Time      `gorm:"column:last_used_at;type:timestamptz;not null;default:now();index:execution_workspaces_company_last_used_idx"`
	OpenedAt                        time.Time      `gorm:"column:opened_at;type:timestamptz;not null;default:now()"`
	ClosedAt                        *time.Time     `gorm:"column:closed_at;type:timestamptz"`
	CleanupEligibleAt               *time.Time     `gorm:"column:cleanup_eligible_at;type:timestamptz"`
	CleanupReason                   *string        `gorm:"column:cleanup_reason"`
	Metadata                        datatypes.JSON `gorm:"column:metadata;type:jsonb"`
	CreatedAt                       time.Time      `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt                       time.Time      `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	Company Company `gorm:"foreignKey:CompanyID"`
	Project Project `gorm:"foreignKey:ProjectID;constraint:OnDelete:CASCADE"`
	ProjectWorkspace *ProjectWorkspace `gorm:"foreignKey:ProjectWorkspaceID;constraint:OnDelete:SET NULL"`
	SourceIssue *Issue `gorm:"foreignKey:SourceIssueID;constraint:OnDelete:SET NULL"`
	DerivedFrom *ExecutionWorkspace `gorm:"foreignKey:DerivedFromExecutionWorkspaceID;constraint:OnDelete:SET NULL"`
}

func (ExecutionWorkspace) TableName() string {
	return "execution_workspaces"
}
