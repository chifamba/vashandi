package models

import (
	"time"

	"gorm.io/datatypes"
)

type ActivityLog struct {
	ID         string         `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID  string         `gorm:"column:company_id;type:uuid;not null;index:activity_log_company_created_idx"`
	ActorType  string         `gorm:"column:actor_type;not null;default:system"`
	ActorID    string         `gorm:"column:actor_id;not null"`
	Action     string         `gorm:"column:action;not null"`
	EntityType string         `gorm:"column:entity_type;not null;index:activity_log_entity_type_id_idx"`
	EntityID   string         `gorm:"column:entity_id;not null;index:activity_log_entity_type_id_idx"`
	AgentID    *string        `gorm:"column:agent_id;type:uuid"`
	RunID      *string        `gorm:"column:run_id;type:uuid;index:activity_log_run_id_idx"`
	Details    datatypes.JSON `gorm:"column:details;type:jsonb"`
	CreatedAt  time.Time      `gorm:"column:created_at;type:timestamptz;not null;default:now();index:activity_log_company_created_idx"`

	Agent *Agent `gorm:"foreignKey:AgentID;constraint:OnDelete:SET NULL"`
}

func (ActivityLog) TableName() string {
	return "activity_log"
}
