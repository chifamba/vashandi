package models

import (
	"time"

	"gorm.io/datatypes"
)

type ProjectGoal struct {
	CompanyID string    `gorm:"column:company_id;type:uuid;not null;index:project_goals_company_idx"`
	ProjectID string    `gorm:"column:project_id;type:uuid;not null;primaryKey;index:project_goals_project_idx"`
	GoalID    string    `gorm:"column:goal_id;type:uuid;not null;primaryKey;index:project_goals_goal_idx"`
	CreatedAt time.Time `gorm:"column:created_at;type:timestamptz;not null;default:now()"`

	Company Company `gorm:"foreignKey:CompanyID"`
	Project Project `gorm:"foreignKey:ProjectID;constraint:OnDelete:CASCADE"`
	Goal    Goal    `gorm:"foreignKey:GoalID;constraint:OnDelete:CASCADE"`
}

func (ProjectGoal) TableName() string {
	return "project_goals"
}

type ProjectWorkspace struct {
	ID                 string         `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID          string         `gorm:"column:company_id;type:uuid;not null;index:project_workspaces_company_project_idx;index:project_workspaces_company_shared_key_idx"`
	ProjectID          string         `gorm:"column:project_id;type:uuid;not null;index:project_workspaces_company_project_idx;index:project_workspaces_project_primary_idx;index:project_workspaces_project_source_type_idx;uniqueIndex:project_workspaces_project_remote_ref_idx,composite:remote_ref"`
	Name               string         `gorm:"column:name;not null"`
	SourceType         string         `gorm:"column:source_type;not null;default:local_path;index:project_workspaces_project_source_type_idx"`
	Cwd                *string        `gorm:"column:cwd"`
	RepoUrl            *string        `gorm:"column:repo_url"`
	RepoRef            *string        `gorm:"column:repo_ref"`
	DefaultRef         *string        `gorm:"column:default_ref"`
	Visibility         string         `gorm:"column:visibility;not null;default:default"`
	SetupCommand       *string        `gorm:"column:setup_command"`
	CleanupCommand     *string        `gorm:"column:cleanup_command"`
	RemoteProvider     *string        `gorm:"column:remote_provider;uniqueIndex:project_workspaces_project_remote_ref_idx,composite:remote_ref"`
	RemoteWorkspaceRef *string        `gorm:"column:remote_workspace_ref;uniqueIndex:project_workspaces_project_remote_ref_idx,composite:remote_ref"`
	SharedWorkspaceKey *string        `gorm:"column:shared_workspace_key;index:project_workspaces_company_shared_key_idx"`
	Metadata           datatypes.JSON `gorm:"column:metadata;type:jsonb"`
	IsPrimary          bool           `gorm:"column:is_primary;not null;default:false;index:project_workspaces_project_primary_idx"`
	CreatedAt          time.Time      `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt          time.Time      `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	Company Company `gorm:"foreignKey:CompanyID"`
	Project Project `gorm:"foreignKey:ProjectID;constraint:OnDelete:CASCADE"`
}

func (ProjectWorkspace) TableName() string {
	return "project_workspaces"
}
