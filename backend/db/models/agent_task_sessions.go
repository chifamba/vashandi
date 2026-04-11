package models

import (
	"time"

	"gorm.io/datatypes"
)

type AgentTaskSession struct {
	ID                string         `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID         string         `gorm:"column:company_id;type:uuid;not null;uniqueIndex:agent_task_sessions_company_agent_adapter_task_uniq;index:agent_task_sessions_company_agent_updated_idx;index:agent_task_sessions_company_task_updated_idx"`
	AgentID           string         `gorm:"column:agent_id;type:uuid;not null;uniqueIndex:agent_task_sessions_company_agent_adapter_task_uniq;index:agent_task_sessions_company_agent_updated_idx"`
	AdapterType       string         `gorm:"column:adapter_type;not null;uniqueIndex:agent_task_sessions_company_agent_adapter_task_uniq"`
	TaskKey           string         `gorm:"column:task_key;not null;uniqueIndex:agent_task_sessions_company_agent_adapter_task_uniq;index:agent_task_sessions_company_task_updated_idx"`
	SessionParamsJSON datatypes.JSON `gorm:"column:session_params_json;type:jsonb"`
	SessionDisplayID  *string        `gorm:"column:session_display_id"`
	LastRunID         *string        `gorm:"column:last_run_id;type:uuid"`
	LastError         *string        `gorm:"column:last_error"`
	CreatedAt         time.Time      `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt         time.Time      `gorm:"column:updated_at;type:timestamptz;not null;default:now();index:agent_task_sessions_company_agent_updated_idx;index:agent_task_sessions_company_task_updated_idx"`

	Company Company       `gorm:"foreignKey:CompanyID"`
	Agent   Agent         `gorm:"foreignKey:AgentID"`
	LastRun *HeartbeatRun `gorm:"foreignKey:LastRunID"`
}

func (AgentTaskSession) TableName() string {
	return "agent_task_sessions"
}
