package models

import (
	"time"

	"gorm.io/datatypes"
)

type WorkspaceOperation struct {
	ID                   string         `gorm:"primaryKey;type:uuid;default:uuid_generate_v4()"`
	CompanyID            string         `gorm:"type:uuid;not null;index:workspace_operations_company_run_started_idx;index:workspace_operations_company_workspace_started_idx"`
	ExecutionWorkspaceID *string        `gorm:"type:uuid;index:workspace_operations_company_workspace_started_idx"`
	HeartbeatRunID       *string        `gorm:"type:uuid;index:workspace_operations_company_run_started_idx"`
	Phase                string         `gorm:"not null"`
	Command              *string
	Cwd                  *string
	Status               string         `gorm:"not null;default:running"`
	ExitCode             *int
	LogStore             *string
	LogRef               *string
	LogBytes             *int64
	LogSha256            *string
	LogCompressed        bool           `gorm:"not null;default:false"`
	StdoutExcerpt        *string
	StderrExcerpt        *string
	Metadata             datatypes.JSON `gorm:"type:jsonb"`
	StartedAt            time.Time      `gorm:"not null;default:now();index:workspace_operations_company_run_started_idx;index:workspace_operations_company_workspace_started_idx"`
	FinishedAt           *time.Time
	CreatedAt            time.Time      `gorm:"not null;default:now()"`
	UpdatedAt            time.Time      `gorm:"not null;default:now()"`
}
