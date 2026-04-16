package models

import (
	"time"

	"gorm.io/datatypes"
)

// PluginEntity stores persistent mappings between Paperclip objects and external entities.
type PluginEntity struct {
	ID         string         `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	PluginID   string         `gorm:"column:plugin_id;type:uuid;not null;index:plugin_entities_plugin_idx"`
	EntityType string         `gorm:"column:entity_type;not null;index:plugin_entities_type_idx"`
	ScopeKind  string         `gorm:"column:scope_kind;not null;index:plugin_entities_scope_idx"`
	ScopeID    *string        `gorm:"column:scope_id;index:plugin_entities_scope_idx"`
	ExternalID *string        `gorm:"column:external_id;index:plugin_entities_external_idx"`
	Title      *string        `gorm:"column:title"`
	Status     *string        `gorm:"column:status"`
	Data       datatypes.JSON `gorm:"column:data;type:jsonb;not null;default:'{}'"`
	CreatedAt  time.Time      `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt  time.Time      `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	Plugin Plugin `gorm:"foreignKey:PluginID"`
}

func (PluginEntity) TableName() string {
	return "plugin_entities"
}
