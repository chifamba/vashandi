package models

import (
	"time"
)

type CLIAuthChallenge struct {
	ID                 string     `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	SecretHash         string     `gorm:"column:secret_hash;not null;index:cli_auth_challenges_secret_hash_idx"`
	Command            string     `gorm:"column:command;not null"`
	ClientName         *string    `gorm:"column:client_name"`
	RequestedAccess    string     `gorm:"column:requested_access;not null;default:board"`
	RequestedCompanyID *string    `gorm:"column:requested_company_id;type:uuid;index:cli_auth_challenges_requested_company_idx"`
	PendingKeyHash     string     `gorm:"column:pending_key_hash;not null"`
	PendingKeyName     string     `gorm:"column:pending_key_name;not null"`
	ApprovedByUserID   *string    `gorm:"column:approved_by_user_id;index:cli_auth_challenges_approved_by_idx"`
	BoardAPIKeyID      *string    `gorm:"column:board_api_key_id;type:uuid"`
	ApprovedAt         *time.Time `gorm:"column:approved_at;type:timestamptz"`
	CancelledAt        *time.Time `gorm:"column:cancelled_at;type:timestamptz"`
	ExpiresAt          time.Time  `gorm:"column:expires_at;type:timestamptz;not null"`
	CreatedAt          time.Time  `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt          time.Time  `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	RequestedCompany *Company     `gorm:"foreignKey:RequestedCompanyID;constraint:OnDelete:SET NULL"`
	ApprovedByUser   *User        `gorm:"foreignKey:ApprovedByUserID;constraint:OnDelete:SET NULL"`
	BoardAPIKey      *BoardAPIKey `gorm:"foreignKey:BoardAPIKeyID;constraint:OnDelete:SET NULL"`
}

func (CLIAuthChallenge) TableName() string {
	return "cli_auth_challenges"
}
