package models

import (
	"time"

	"gorm.io/datatypes"
)

type WorkspaceOperation struct {
	ID                   string         `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID            string         `gorm:"column:company_id;type:uuid;not null;index:workspace_operations_company_run_started_idx;index:workspace_operations_company_workspace_started_idx"`
	ExecutionWorkspaceID *string        `gorm:"column:execution_workspace_id;type:uuid;index:workspace_operations_company_workspace_started_idx"`
	HeartbeatRunID       *string        `gorm:"column:heartbeat_run_id;type:uuid;index:workspace_operations_company_run_started_idx"`
	Phase                string         `gorm:"column:phase;type:text;not null"`
	Command              *string        `gorm:"column:command;type:text"`
	CWD                  *string        `gorm:"column:cwd;type:text"`
	Status               string         `gorm:"column:status;type:text;not null;default:'running'"`
	ExitCode             *int           `gorm:"column:exit_code;type:integer"`
	LogStore             *string        `gorm:"column:log_store;type:text"`
	LogRef               *string        `gorm:"column:log_ref;type:text"`
	LogBytes             *int64         `gorm:"column:log_bytes;type:bigint"`
	LogSha256            *string        `gorm:"column:log_sha256;type:text"`
	LogCompressed        bool           `gorm:"column:log_compressed;type:boolean;not null;default:false"`
	StdoutExcerpt        *string        `gorm:"column:stdout_excerpt;type:text"`
	StderrExcerpt        *string        `gorm:"column:stderr_excerpt;type:text"`
	Metadata             datatypes.JSON `gorm:"column:metadata;type:jsonb"`
	StartedAt            time.Time      `gorm:"column:started_at;type:timestamptz;not null;default:now();index:workspace_operations_company_run_started_idx;index:workspace_operations_company_workspace_started_idx"`
	FinishedAt           *time.Time     `gorm:"column:finished_at;type:timestamptz"`
	CreatedAt            time.Time      `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt            time.Time      `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	Company            Company             `gorm:"foreignKey:CompanyID"`
	ExecutionWorkspace *ExecutionWorkspace `gorm:"foreignKey:ExecutionWorkspaceID;constraint:OnDelete:SET NULL"`
	HeartbeatRun       *HeartbeatRun       `gorm:"foreignKey:HeartbeatRunID;constraint:OnDelete:SET NULL"`
}

func (WorkspaceOperation) TableName() string {
	return "workspace_operations"
}
