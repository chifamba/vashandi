package models

import (
	"time"
)

type BoardAPIKey struct {
	ID         string     `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID     string     `gorm:"column:user_id;not null;index:board_api_keys_user_idx"`
	Name       string     `gorm:"column:name;not null"`
	KeyHash    string     `gorm:"column:key_hash;not null;uniqueIndex:board_api_keys_key_hash_idx"`
	LastUsedAt *time.Time `gorm:"column:last_used_at;type:timestamptz"`
	RevokedAt  *time.Time `gorm:"column:revoked_at;type:timestamptz"`
	ExpiresAt  *time.Time `gorm:"column:expires_at;type:timestamptz"`
	CreatedAt  time.Time  `gorm:"column:created_at;type:timestamptz;not null;default:now()"`

	User User `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`
}

func (BoardAPIKey) TableName() string {
	return "board_api_keys"
}
