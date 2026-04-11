package models

import (
	"time"
)

type Goal struct {
	ID           string    `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID    string    `gorm:"column:company_id;type:uuid;not null;index:goals_company_idx"`
	Title        string    `gorm:"column:title;not null"`
	Description  *string   `gorm:"column:description"`
	Level        string    `gorm:"column:level;not null;default:task"`
	Status       string    `gorm:"column:status;not null;default:planned"`
	ParentID     *string   `gorm:"column:parent_id;type:uuid"`
	OwnerAgentID *string   `gorm:"column:owner_agent_id;type:uuid"`
	CreatedAt    time.Time `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt    time.Time `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	Company    Company `gorm:"foreignKey:CompanyID"`
	Parent     *Goal   `gorm:"foreignKey:ParentID"`
	OwnerAgent *Agent  `gorm:"foreignKey:OwnerAgentID"`
}

func (Goal) TableName() string {
	return "goals"
}
