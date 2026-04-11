package models

import (
	"time"

	"gorm.io/datatypes"
)

type AgentWakeupRequest struct {
	ID                   string         `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID            string         `gorm:"column:company_id;type:uuid;not null;index:agent_wakeup_requests_company_agent_status_idx;index:agent_wakeup_requests_company_requested_idx"`
	AgentID              string         `gorm:"column:agent_id;type:uuid;not null;index:agent_wakeup_requests_company_agent_status_idx;index:agent_wakeup_requests_agent_requested_idx"`
	Source               string         `gorm:"column:source;not null"`
	TriggerDetail        *string        `gorm:"column:trigger_detail"`
	Reason               *string        `gorm:"column:reason"`
	Payload              datatypes.JSON `gorm:"column:payload;type:jsonb"`
	Status               string         `gorm:"column:status;not null;default:queued;index:agent_wakeup_requests_company_agent_status_idx"`
	CoalescedCount       int            `gorm:"column:coalesced_count;not null;default:0"`
	RequestedByActorType *string        `gorm:"column:requested_by_actor_type"`
	RequestedByActorID   *string        `gorm:"column:requested_by_actor_id"`
	IdempotencyKey       *string        `gorm:"column:idempotency_key"`
	RunID                *string        `gorm:"column:run_id;type:uuid"`
	RequestedAt          time.Time      `gorm:"column:requested_at;type:timestamptz;not null;default:now();index:agent_wakeup_requests_company_requested_idx;index:agent_wakeup_requests_agent_requested_idx"`
	ClaimedAt            *time.Time     `gorm:"column:claimed_at;type:timestamptz"`
	FinishedAt           *time.Time     `gorm:"column:finished_at;type:timestamptz"`
	Error                *string        `gorm:"column:error"`
	CreatedAt            time.Time      `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt            time.Time      `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	Company Company `gorm:"foreignKey:CompanyID"`
	Agent   Agent   `gorm:"foreignKey:AgentID"`
}

func (AgentWakeupRequest) TableName() string {
	return "agent_wakeup_requests"
}
