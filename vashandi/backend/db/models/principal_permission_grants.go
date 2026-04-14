package models

import (
	"time"

	"gorm.io/datatypes"
)

type PrincipalPermissionGrant struct {
	ID              string         `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID       string         `gorm:"column:company_id;type:uuid;not null;index:principal_permission_grants_company_principal_idx;index:principal_permission_grants_company_key_idx"`
	PrincipalType   string         `gorm:"column:principal_type;not null;index:principal_permission_grants_company_principal_idx"`
	PrincipalID     string         `gorm:"column:principal_id;not null;index:principal_permission_grants_company_principal_idx"`
	PermissionKey   string         `gorm:"column:permission_key;not null;index:principal_permission_grants_company_key_idx"`
	Scope           datatypes.JSON `gorm:"column:scope;type:jsonb"`
	GrantedByUserID *string        `gorm:"column:granted_by_user_id"`
	CreatedAt       time.Time      `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt       time.Time      `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	Company Company `gorm:"foreignKey:CompanyID"`
}

func (PrincipalPermissionGrant) TableName() string {
	return "principal_permission_grants"
}
