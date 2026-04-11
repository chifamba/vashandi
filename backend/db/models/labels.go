package models

import (
	"time"
)

type Label struct {
	ID        string    `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID string    `gorm:"column:company_id;type:uuid;not null;index:labels_company_idx;uniqueIndex:labels_company_name_idx"`
	Name      string    `gorm:"column:name;not null;uniqueIndex:labels_company_name_idx"`
	Color     string    `gorm:"column:color;not null"`
	CreatedAt time.Time `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt time.Time `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	Company Company `gorm:"foreignKey:CompanyID;constraint:OnDelete:CASCADE"`
}

func (Label) TableName() string {
	return "labels"
}
