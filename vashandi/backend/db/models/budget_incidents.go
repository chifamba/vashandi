package models

import (
	"time"
)

type BudgetIncident struct {
	ID             string     `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID      string     `gorm:"column:company_id;type:uuid;not null;index:budget_incidents_company_status_idx;index:budget_incidents_company_scope_idx"`
	PolicyID       string     `gorm:"column:policy_id;type:uuid;not null;index:budget_incidents_policy_window_threshold_idx"`
	ScopeType      string     `gorm:"column:scope_type;not null;index:budget_incidents_company_scope_idx"`
	ScopeID        string     `gorm:"column:scope_id;type:uuid;not null;index:budget_incidents_company_scope_idx"`
	Metric         string     `gorm:"column:metric;not null"`
	WindowKind     string     `gorm:"column:window_kind;not null"`
	WindowStart    time.Time  `gorm:"column:window_start;type:timestamptz;not null;index:budget_incidents_policy_window_threshold_idx"`
	WindowEnd      time.Time  `gorm:"column:window_end;type:timestamptz;not null"`
	ThresholdType  string     `gorm:"column:threshold_type;not null;index:budget_incidents_policy_window_threshold_idx"`
	AmountLimit    int        `gorm:"column:amount_limit;not null"`
	AmountObserved int        `gorm:"column:amount_observed;not null"`
	Status         string     `gorm:"column:status;not null;default:open;index:budget_incidents_company_status_idx;index:budget_incidents_company_scope_idx"`
	ApprovalID     *string    `gorm:"column:approval_id;type:uuid"`
	ResolvedAt     *time.Time `gorm:"column:resolved_at;type:timestamptz"`
	CreatedAt      time.Time  `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt      time.Time  `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	Company  Company      `gorm:"foreignKey:CompanyID"`
	Policy   BudgetPolicy `gorm:"foreignKey:PolicyID"` // Not porting budget_policies right this second to avoid circular deps if they exist, but will link later. Wait, let's just make sure it compiles. We don't necessarily need the strict foreign key mapped if the model isn't here yet, but we will add it.
	Approval *Approval    `gorm:"foreignKey:ApprovalID"`
}

func (BudgetIncident) TableName() string {
	return "budget_incidents"
}
