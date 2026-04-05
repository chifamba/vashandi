package models

import (
	"time"
)

type CompanyLogo struct {
	ID        string    `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID string    `gorm:"column:company_id;type:uuid;not null;uniqueIndex:company_logos_company_uq"`
	AssetID   string    `gorm:"column:asset_id;type:uuid;not null;uniqueIndex:company_logos_asset_uq"`
	CreatedAt time.Time `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt time.Time `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	Company Company `gorm:"foreignKey:CompanyID;constraint:OnDelete:CASCADE"`
	Asset   Asset   `gorm:"foreignKey:AssetID;constraint:OnDelete:CASCADE"`
}

func (CompanyLogo) TableName() string {
	return "company_logos"
}

type CompanyMembership struct {
	ID             string    `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID      string    `gorm:"column:company_id;type:uuid;not null;uniqueIndex:company_memberships_company_principal_unique_idx;index:company_memberships_company_status_idx"`
	PrincipalType  string    `gorm:"column:principal_type;not null;uniqueIndex:company_memberships_company_principal_unique_idx;index:company_memberships_principal_status_idx"`
	PrincipalID    string    `gorm:"column:principal_id;not null;uniqueIndex:company_memberships_company_principal_unique_idx;index:company_memberships_principal_status_idx"`
	Status         string    `gorm:"column:status;not null;default:active;index:company_memberships_principal_status_idx;index:company_memberships_company_status_idx"`
	MembershipRole *string   `gorm:"column:membership_role"`
	CreatedAt      time.Time `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt      time.Time `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	Company Company `gorm:"foreignKey:CompanyID"`
}

func (CompanyMembership) TableName() string {
	return "company_memberships"
}
