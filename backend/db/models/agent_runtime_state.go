package models

import (
	"time"

	"gorm.io/datatypes"
)

type AgentRuntimeState struct {
	AgentID                string         `gorm:"column:agent_id;type:uuid;primaryKey;index:agent_runtime_state_company_agent_idx"`
	CompanyID              string         `gorm:"column:company_id;type:uuid;not null;index:agent_runtime_state_company_agent_idx;index:agent_runtime_state_company_updated_idx"`
	AdapterType            string         `gorm:"column:adapter_type;not null"`
	SessionID              *string        `gorm:"column:session_id"`
	StateJSON              datatypes.JSON `gorm:"column:state_json;type:jsonb;not null;default:'{}'"`
	LastRunID              *string        `gorm:"column:last_run_id;type:uuid"`
	LastRunStatus          *string        `gorm:"column:last_run_status"`
	TotalInputTokens       int64          `gorm:"column:total_input_tokens;not null;default:0"`
	TotalOutputTokens      int64          `gorm:"column:total_output_tokens;not null;default:0"`
	TotalCachedInputTokens int64          `gorm:"column:total_cached_input_tokens;not null;default:0"`
	TotalCostCents         int64          `gorm:"column:total_cost_cents;not null;default:0"`
	LastError              *string        `gorm:"column:last_error"`
	CreatedAt              time.Time      `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt              time.Time      `gorm:"column:updated_at;type:timestamptz;not null;default:now();index:agent_runtime_state_company_updated_idx"`

	Agent   Agent   `gorm:"foreignKey:AgentID"`
	Company Company `gorm:"foreignKey:CompanyID"`
}

func (AgentRuntimeState) TableName() string {
	return "agent_runtime_state"
}
