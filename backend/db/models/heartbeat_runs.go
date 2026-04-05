package models

import (
	"time"

	"gorm.io/datatypes"
)

type HeartbeatRun struct {
	ID                    string         `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID             string         `gorm:"column:company_id;type:uuid;not null;index:heartbeat_runs_company_agent_started_idx"`
	AgentID               string         `gorm:"column:agent_id;type:uuid;not null;index:heartbeat_runs_company_agent_started_idx"`
	InvocationSource      string         `gorm:"column:invocation_source;not null;default:on_demand"`
	TriggerDetail         *string        `gorm:"column:trigger_detail"`
	Status                string         `gorm:"column:status;not null;default:queued"`
	StartedAt             *time.Time     `gorm:"column:started_at;type:timestamptz;index:heartbeat_runs_company_agent_started_idx"`
	FinishedAt            *time.Time     `gorm:"column:finished_at;type:timestamptz"`
	Error                 *string        `gorm:"column:error"`
	WakeupRequestID       *string        `gorm:"column:wakeup_request_id;type:uuid"`
	ExitCode              *int           `gorm:"column:exit_code"`
	Signal                *string        `gorm:"column:signal"`
	UsageJSON             datatypes.JSON `gorm:"column:usage_json;type:jsonb"`
	ResultJSON            datatypes.JSON `gorm:"column:result_json;type:jsonb"`
	SessionIDBefore       *string        `gorm:"column:session_id_before"`
	SessionIDAfter        *string        `gorm:"column:session_id_after"`
	LogStore              *string        `gorm:"column:log_store"`
	LogRef                *string        `gorm:"column:log_ref"`
	LogBytes              *int64         `gorm:"column:log_bytes"`
	LogSha256             *string        `gorm:"column:log_sha256"`
	LogCompressed         bool           `gorm:"column:log_compressed;not null;default:false"`
	StdoutExcerpt         *string        `gorm:"column:stdout_excerpt"`
	StderrExcerpt         *string        `gorm:"column:stderr_excerpt"`
	ErrorCode             *string        `gorm:"column:error_code"`
	ExternalRunID         *string        `gorm:"column:external_run_id"`
	ProcessPid            *int           `gorm:"column:process_pid"`
	ProcessStartedAt      *time.Time     `gorm:"column:process_started_at;type:timestamptz"`
	RetryOfRunID          *string        `gorm:"column:retry_of_run_id;type:uuid"`
	ProcessLossRetryCount int            `gorm:"column:process_loss_retry_count;not null;default:0"`
	ContextSnapshot       datatypes.JSON `gorm:"column:context_snapshot;type:jsonb"`
	CreatedAt             time.Time      `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt             time.Time      `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	Company    Company       `gorm:"foreignKey:CompanyID"`
	Agent      Agent         `gorm:"foreignKey:AgentID"`
	RetryOfRun *HeartbeatRun `gorm:"foreignKey:RetryOfRunID;constraint:OnDelete:SET NULL"`
}

func (HeartbeatRun) TableName() string {
	return "heartbeat_runs"
}
