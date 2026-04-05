package models

import (
	"time"

	"gorm.io/datatypes"
)

type PluginEntity struct {
	ID         string         `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	PluginID   string         `gorm:"column:plugin_id;type:uuid;not null;index:plugin_entities_plugin_idx;uniqueIndex:plugin_entities_external_idx"`
	EntityType string         `gorm:"column:entity_type;type:text;not null;index:plugin_entities_type_idx;uniqueIndex:plugin_entities_external_idx"`
	ScopeKind  string         `gorm:"column:scope_kind;type:text;not null;index:plugin_entities_scope_idx"`
	ScopeID    *string        `gorm:"column:scope_id;type:text;index:plugin_entities_scope_idx"`
	ExternalID *string        `gorm:"column:external_id;type:text;uniqueIndex:plugin_entities_external_idx"`
	Title      *string        `gorm:"column:title;type:text"`
	Status     *string        `gorm:"column:status;type:text"`
	Data       datatypes.JSON `gorm:"column:data;type:jsonb;not null;default:'{}'"`
	CreatedAt  time.Time      `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt  time.Time      `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	Plugin Plugin `gorm:"foreignKey:PluginID;constraint:OnDelete:CASCADE"`
}

func (PluginEntity) TableName() string {
	return "plugin_entities"
}
