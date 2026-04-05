package models

import (
	"time"
)

type CostEvent struct {
	ID                string    `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID         string    `gorm:"column:company_id;type:uuid;not null;index:cost_events_company_occurred_idx;index:cost_events_company_agent_occurred_idx;index:cost_events_company_provider_occurred_idx;index:cost_events_company_biller_occurred_idx;index:cost_events_company_heartbeat_run_idx"`
	AgentID           string    `gorm:"column:agent_id;type:uuid;not null;index:cost_events_company_agent_occurred_idx"`
	IssueID           *string   `gorm:"column:issue_id;type:uuid"`
	ProjectID         *string   `gorm:"column:project_id;type:uuid"`
	GoalID            *string   `gorm:"column:goal_id;type:uuid"`
	HeartbeatRunID    *string   `gorm:"column:heartbeat_run_id;type:uuid;index:cost_events_company_heartbeat_run_idx"`
	BillingCode       *string   `gorm:"column:billing_code"`
	Provider          string    `gorm:"column:provider;not null;index:cost_events_company_provider_occurred_idx"`
	Biller            string    `gorm:"column:biller;not null;default:unknown;index:cost_events_company_biller_occurred_idx"`
	BillingType       string    `gorm:"column:billing_type;not null;default:unknown"`
	Model             string    `gorm:"column:model;not null"`
	InputTokens       int       `gorm:"column:input_tokens;not null;default:0"`
	CachedInputTokens int       `gorm:"column:cached_input_tokens;not null;default:0"`
	OutputTokens      int       `gorm:"column:output_tokens;not null;default:0"`
	CostCents         int       `gorm:"column:cost_cents;not null"`
	OccurredAt        time.Time `gorm:"column:occurred_at;type:timestamptz;not null;index:cost_events_company_occurred_idx;index:cost_events_company_agent_occurred_idx;index:cost_events_company_provider_occurred_idx;index:cost_events_company_biller_occurred_idx"`
	CreatedAt         time.Time `gorm:"column:created_at;type:timestamptz;not null;default:now()"`

	Company      Company       `gorm:"foreignKey:CompanyID"`
	Agent        Agent         `gorm:"foreignKey:AgentID"`
	Issue        *Issue        `gorm:"foreignKey:IssueID"`
	Project      *Project      `gorm:"foreignKey:ProjectID"`
	Goal         *Goal         `gorm:"foreignKey:GoalID"`
	HeartbeatRun *HeartbeatRun `gorm:"foreignKey:HeartbeatRunID"`
}

func (CostEvent) TableName() string {
	return "cost_events"
}
