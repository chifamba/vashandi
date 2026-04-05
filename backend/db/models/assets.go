package models

import (
	"time"
)

type Asset struct {
	ID               string    `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID        string    `gorm:"column:company_id;type:uuid;not null;index:assets_company_created_idx;index:assets_company_provider_idx;uniqueIndex:assets_company_object_key_uq"`
	Provider         string    `gorm:"column:provider;not null;index:assets_company_provider_idx"`
	ObjectKey        string    `gorm:"column:object_key;not null;uniqueIndex:assets_company_object_key_uq"`
	ContentType      string    `gorm:"column:content_type;not null"`
	ByteSize         int       `gorm:"column:byte_size;not null"`
	Sha256           string    `gorm:"column:sha256;not null"`
	OriginalFilename *string   `gorm:"column:original_filename"`
	CreatedByAgentID *string   `gorm:"column:created_by_agent_id;type:uuid"`
	CreatedByUserID  *string   `gorm:"column:created_by_user_id"`
	CreatedAt        time.Time `gorm:"column:created_at;type:timestamptz;not null;default:now();index:assets_company_created_idx"`
	UpdatedAt        time.Time `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	Company        Company `gorm:"foreignKey:CompanyID"`
	CreatedByAgent *Agent  `gorm:"foreignKey:CreatedByAgentID"`
}

func (Asset) TableName() string {
	return "assets"
}
