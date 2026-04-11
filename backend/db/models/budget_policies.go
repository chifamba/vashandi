package models

import (
	"time"
)

type BudgetPolicy struct {
	ID              string    `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID       string    `gorm:"column:company_id;type:uuid;not null;index:budget_policies_company_scope_active_idx;index:budget_policies_company_window_idx;uniqueIndex:budget_policies_company_scope_metric_unique_idx"`
	ScopeType       string    `gorm:"column:scope_type;not null;index:budget_policies_company_scope_active_idx;uniqueIndex:budget_policies_company_scope_metric_unique_idx"`
	ScopeID         string    `gorm:"column:scope_id;type:uuid;not null;index:budget_policies_company_scope_active_idx;uniqueIndex:budget_policies_company_scope_metric_unique_idx"`
	Metric          string    `gorm:"column:metric;not null;default:billed_cents;index:budget_policies_company_window_idx;uniqueIndex:budget_policies_company_scope_metric_unique_idx"`
	WindowKind      string    `gorm:"column:window_kind;not null;index:budget_policies_company_window_idx;uniqueIndex:budget_policies_company_scope_metric_unique_idx"`
	Amount          int       `gorm:"column:amount;not null;default:0"`
	WarnPercent     int       `gorm:"column:warn_percent;not null;default:80"`
	HardStopEnabled bool      `gorm:"column:hard_stop_enabled;not null;default:true"`
	NotifyEnabled   bool      `gorm:"column:notify_enabled;not null;default:true"`
	IsActive        bool      `gorm:"column:is_active;not null;default:true;index:budget_policies_company_scope_active_idx"`
	CreatedByUserID *string   `gorm:"column:created_by_user_id"`
	UpdatedByUserID *string   `gorm:"column:updated_by_user_id"`
	CreatedAt       time.Time `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt       time.Time `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	Company Company `gorm:"foreignKey:CompanyID"`
}

func (BudgetPolicy) TableName() string {
	return "budget_policies"
}
