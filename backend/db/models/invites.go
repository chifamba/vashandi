package models

import (
	"time"

	"gorm.io/datatypes"
)

type Invite struct {
	ID               string         `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID        *string        `gorm:"column:company_id;type:uuid;index:invites_company_invite_state_idx"`
	InviteType       string         `gorm:"column:invite_type;not null;default:company_join;index:invites_company_invite_state_idx"`
	TokenHash        string         `gorm:"column:token_hash;not null;uniqueIndex:invites_token_hash_unique_idx"`
	AllowedJoinTypes string         `gorm:"column:allowed_join_types;not null;default:both"`
	DefaultsPayload  datatypes.JSON `gorm:"column:defaults_payload;type:jsonb"`
	ExpiresAt        time.Time      `gorm:"column:expires_at;type:timestamptz;not null;index:invites_company_invite_state_idx"`
	InvitedByUserID  *string        `gorm:"column:invited_by_user_id"`
	RevokedAt        *time.Time     `gorm:"column:revoked_at;type:timestamptz;index:invites_company_invite_state_idx"`
	AcceptedAt       *time.Time     `gorm:"column:accepted_at;type:timestamptz"`
	CreatedAt        time.Time      `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt        time.Time      `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	Company       *Company `gorm:"foreignKey:CompanyID"`
	InvitedByUser *User    `gorm:"foreignKey:InvitedByUserID"`
}

func (Invite) TableName() string {
	return "invites"
}
