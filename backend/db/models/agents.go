package models

import (
	"time"

	"gorm.io/datatypes"
)

type Agent struct {
	ID                 string         `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID          string         `gorm:"column:company_id;type:uuid;not null;index:agents_company_status_idx;index:agents_company_reports_to_idx"`
	Name               string         `gorm:"column:name;not null"`
	Role               string         `gorm:"column:role;not null;default:general"`
	Title              *string        `gorm:"column:title"`
	Icon               *string        `gorm:"column:icon"`
	Status             string         `gorm:"column:status;not null;default:idle;index:agents_company_status_idx"`
	ReportsTo          *string        `gorm:"column:reports_to;type:uuid;index:agents_company_reports_to_idx"`
	Capabilities       *string        `gorm:"column:capabilities"`
	AdapterType        string         `gorm:"column:adapter_type;not null;default:process"`
	AdapterConfig      datatypes.JSON `gorm:"column:adapter_config;type:jsonb;not null;default:'{}'"`
	RuntimeConfig      datatypes.JSON `gorm:"column:runtime_config;type:jsonb;not null;default:'{}'"`
	BudgetMonthlyCents int            `gorm:"column:budget_monthly_cents;not null;default:0"`
	SpentMonthlyCents  int            `gorm:"column:spent_monthly_cents;not null;default:0"`
	PauseReason        *string        `gorm:"column:pause_reason"`
	PausedAt           *time.Time     `gorm:"column:paused_at;type:timestamptz"`
	Permissions        datatypes.JSON `gorm:"column:permissions;type:jsonb;not null;default:'{}'"`
	LastHeartbeatAt    *time.Time     `gorm:"column:last_heartbeat_at;type:timestamptz"`
	Metadata           datatypes.JSON `gorm:"column:metadata;type:jsonb"`
	CreatedAt          time.Time      `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt          time.Time      `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	ParentAgent *Agent `gorm:"foreignKey:ReportsTo;constraint:OnDelete:SET NULL"`
}

func (Agent) TableName() string {
	return "agents"
}
