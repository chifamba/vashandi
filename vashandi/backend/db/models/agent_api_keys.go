package models

import (
	"time"
)

type AgentAPIKey struct {
	ID         string     `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	AgentID    string     `gorm:"column:agent_id;type:uuid;not null;index:agent_api_keys_company_agent_idx"`
	CompanyID  string     `gorm:"column:company_id;type:uuid;not null;index:agent_api_keys_company_agent_idx"`
	Name       string     `gorm:"column:name;not null"`
	KeyHash    string     `gorm:"column:key_hash;not null;index:agent_api_keys_key_hash_idx"`
	LastUsedAt *time.Time `gorm:"column:last_used_at;type:timestamptz"`
	RevokedAt  *time.Time `gorm:"column:revoked_at;type:timestamptz"`
	CreatedAt  time.Time  `gorm:"column:created_at;type:timestamptz;not null;default:now()"`

	Agent   Agent   `gorm:"foreignKey:AgentID"`
	Company Company `gorm:"foreignKey:CompanyID"`
}

func (AgentAPIKey) TableName() string {
	return "agent_api_keys"
}
