package models

import (
	"time"

	"gorm.io/datatypes"
)

type AgentConfigRevision struct {
	ID                       string         `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID                string         `gorm:"column:company_id;type:uuid;not null;index:agent_config_revisions_company_agent_created_idx"`
	AgentID                  string         `gorm:"column:agent_id;type:uuid;not null;index:agent_config_revisions_company_agent_created_idx;index:agent_config_revisions_agent_created_idx"`
	CreatedByAgentID         *string        `gorm:"column:created_by_agent_id;type:uuid"`
	CreatedByUserID          *string        `gorm:"column:created_by_user_id"`
	Source                   string         `gorm:"column:source;not null;default:patch"`
	RolledBackFromRevisionID *string        `gorm:"column:rolled_back_from_revision_id;type:uuid"`
	ChangedKeys              datatypes.JSON `gorm:"column:changed_keys;type:jsonb;not null;default:'[]'"`
	BeforeConfig             datatypes.JSON `gorm:"column:before_config;type:jsonb;not null"`
	AfterConfig              datatypes.JSON `gorm:"column:after_config;type:jsonb;not null"`
	CreatedAt                time.Time      `gorm:"column:created_at;type:timestamptz;not null;default:now();index:agent_config_revisions_company_agent_created_idx;index:agent_config_revisions_agent_created_idx"`

	Company        Company `gorm:"foreignKey:CompanyID"`
	Agent          Agent   `gorm:"foreignKey:AgentID;constraint:OnDelete:CASCADE"`
	CreatedByAgent *Agent  `gorm:"foreignKey:CreatedByAgentID;constraint:OnDelete:SET NULL"`
}

func (AgentConfigRevision) TableName() string {
	return "agent_config_revisions"
}
