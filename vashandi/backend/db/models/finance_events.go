package models

import (
	"time"

	"gorm.io/datatypes"
)

type FinanceEvent struct {
	ID                   string         `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID            string         `gorm:"column:company_id;type:uuid;not null;index:finance_events_company_occurred_idx;index:finance_events_company_biller_occurred_idx;index:finance_events_company_kind_occurred_idx;index:finance_events_company_direction_occurred_idx;index:finance_events_company_heartbeat_run_idx;index:finance_events_company_cost_event_idx"`
	AgentID              *string        `gorm:"column:agent_id;type:uuid"`
	IssueID              *string        `gorm:"column:issue_id;type:uuid"`
	ProjectID            *string        `gorm:"column:project_id;type:uuid"`
	GoalID               *string        `gorm:"column:goal_id;type:uuid"`
	HeartbeatRunID       *string        `gorm:"column:heartbeat_run_id;type:uuid;index:finance_events_company_heartbeat_run_idx"`
	CostEventID          *string        `gorm:"column:cost_event_id;type:uuid;index:finance_events_company_cost_event_idx"`
	BillingCode          *string        `gorm:"column:billing_code"`
	Description          *string        `gorm:"column:description"`
	EventKind            string         `gorm:"column:event_kind;not null;index:finance_events_company_kind_occurred_idx"`
	Direction            string         `gorm:"column:direction;not null;default:debit;index:finance_events_company_direction_occurred_idx"`
	Biller               string         `gorm:"column:biller;not null;index:finance_events_company_biller_occurred_idx"`
	Provider             *string        `gorm:"column:provider"`
	ExecutionAdapterType *string        `gorm:"column:execution_adapter_type"`
	PricingTier          *string        `gorm:"column:pricing_tier"`
	Region               *string        `gorm:"column:region"`
	Model                *string        `gorm:"column:model"`
	Quantity             *int           `gorm:"column:quantity"`
	Unit                 *string        `gorm:"column:unit"`
	AmountCents          int            `gorm:"column:amount_cents;not null"`
	Currency             string         `gorm:"column:currency;not null;default:USD"`
	Estimated            bool           `gorm:"column:estimated;not null;default:false"`
	ExternalInvoiceID    *string        `gorm:"column:external_invoice_id"`
	MetadataJSON         datatypes.JSON `gorm:"column:metadata_json;type:jsonb"`
	OccurredAt           time.Time      `gorm:"column:occurred_at;type:timestamptz;not null;index:finance_events_company_occurred_idx;index:finance_events_company_biller_occurred_idx;index:finance_events_company_kind_occurred_idx;index:finance_events_company_direction_occurred_idx"`
	CreatedAt            time.Time      `gorm:"column:created_at;type:timestamptz;not null;default:now()"`

	Company      Company       `gorm:"foreignKey:CompanyID"`
	Agent        *Agent        `gorm:"foreignKey:AgentID"`
	Issue        *Issue        `gorm:"foreignKey:IssueID"`
	Project      *Project      `gorm:"foreignKey:ProjectID"`
	Goal         *Goal         `gorm:"foreignKey:GoalID"`
	HeartbeatRun *HeartbeatRun `gorm:"foreignKey:HeartbeatRunID"`
	CostEvent    *CostEvent    `gorm:"foreignKey:CostEventID"`
}

func (FinanceEvent) TableName() string {
	return "finance_events"
}
