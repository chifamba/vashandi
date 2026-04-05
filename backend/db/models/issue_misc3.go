package models

import (
	"time"

	"gorm.io/datatypes"
)

type IssueReadState struct {
	ID         string    `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID  string    `gorm:"column:company_id;type:uuid;not null;index:issue_read_states_company_issue_idx;index:issue_read_states_company_user_idx;uniqueIndex:issue_read_states_company_issue_user_idx"`
	IssueID    string    `gorm:"column:issue_id;type:uuid;not null;index:issue_read_states_company_issue_idx;uniqueIndex:issue_read_states_company_issue_user_idx"`
	UserID     string    `gorm:"column:user_id;not null;index:issue_read_states_company_user_idx;uniqueIndex:issue_read_states_company_issue_user_idx"`
	LastReadAt time.Time `gorm:"column:last_read_at;type:timestamptz;not null;default:now()"`
	CreatedAt  time.Time `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt  time.Time `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	Company Company `gorm:"foreignKey:CompanyID"`
	Issue   Issue   `gorm:"foreignKey:IssueID"`
	User    User    `gorm:"foreignKey:UserID"`
}

func (IssueReadState) TableName() string {
	return "issue_read_states"
}

type IssueWorkProduct struct {
	ID                   string         `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID            string         `gorm:"column:company_id;type:uuid;not null;index:issue_work_products_company_issue_type_idx;index:issue_work_products_company_execution_workspace_type_idx;index:issue_work_products_company_provider_external_id_idx;index:issue_work_products_company_updated_idx"`
	ProjectID            *string        `gorm:"column:project_id;type:uuid"`
	IssueID              string         `gorm:"column:issue_id;type:uuid;not null;index:issue_work_products_company_issue_type_idx"`
	ExecutionWorkspaceID *string        `gorm:"column:execution_workspace_id;type:uuid;index:issue_work_products_company_execution_workspace_type_idx"`
	RuntimeServiceID     *string        `gorm:"column:runtime_service_id;type:uuid"`
	Type                 string         `gorm:"column:type;not null;index:issue_work_products_company_issue_type_idx;index:issue_work_products_company_execution_workspace_type_idx"`
	Provider             string         `gorm:"column:provider;not null;index:issue_work_products_company_provider_external_id_idx"`
	ExternalID           *string        `gorm:"column:external_id;index:issue_work_products_company_provider_external_id_idx"`
	Title                string         `gorm:"column:title;not null"`
	URL                  *string        `gorm:"column:url"`
	Status               string         `gorm:"column:status;not null"`
	ReviewState          string         `gorm:"column:review_state;not null;default:none"`
	IsPrimary            bool           `gorm:"column:is_primary;not null;default:false"`
	HealthStatus         string         `gorm:"column:health_status;not null;default:unknown"`
	Summary              *string        `gorm:"column:summary"`
	Metadata             datatypes.JSON `gorm:"column:metadata;type:jsonb"`
	CreatedByRunID       *string        `gorm:"column:created_by_run_id;type:uuid"`
	CreatedAt            time.Time      `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt            time.Time      `gorm:"column:updated_at;type:timestamptz;not null;default:now();index:issue_work_products_company_updated_idx"`

	Company        Company       `gorm:"foreignKey:CompanyID"`
	Project        *Project      `gorm:"foreignKey:ProjectID"`
	Issue          Issue         `gorm:"foreignKey:IssueID;constraint:OnDelete:CASCADE"`
	// ExecutionWorkspace *ExecutionWorkspace `gorm:"foreignKey:ExecutionWorkspaceID"` // Ported shortly
	// RuntimeService *WorkspaceRuntimeService `gorm:"foreignKey:RuntimeServiceID"` // Ported shortly
	CreatedByRun   *HeartbeatRun `gorm:"foreignKey:CreatedByRunID;constraint:OnDelete:SET NULL"`
}

func (IssueWorkProduct) TableName() string {
	return "issue_work_products"
}
