package models

import (
	"time"

	"gorm.io/datatypes"
)

type HeartbeatRunEvent struct {
	ID        int64          `gorm:"column:id;type:bigserial;primaryKey"`
	CompanyID string         `gorm:"column:company_id;type:uuid;not null;index:heartbeat_run_events_company_run_idx;index:heartbeat_run_events_company_created_idx"`
	RunID     string         `gorm:"column:run_id;type:uuid;not null;index:heartbeat_run_events_run_seq_idx;index:heartbeat_run_events_company_run_idx"`
	AgentID   string         `gorm:"column:agent_id;type:uuid;not null"`
	Seq       int            `gorm:"column:seq;not null;index:heartbeat_run_events_run_seq_idx"`
	EventType string         `gorm:"column:event_type;not null"`
	Stream    *string        `gorm:"column:stream"`
	Level     *string        `gorm:"column:level"`
	Color     *string        `gorm:"column:color"`
	Message   *string        `gorm:"column:message"`
	Payload   datatypes.JSON `gorm:"column:payload;type:jsonb"`
	CreatedAt time.Time      `gorm:"column:created_at;type:timestamptz;not null;default:now();index:heartbeat_run_events_company_created_idx"`

	Company Company      `gorm:"foreignKey:CompanyID"`
	Run     HeartbeatRun `gorm:"foreignKey:RunID"`
	Agent   Agent        `gorm:"foreignKey:AgentID"`
}

func (HeartbeatRunEvent) TableName() string {
	return "heartbeat_run_events"
}
