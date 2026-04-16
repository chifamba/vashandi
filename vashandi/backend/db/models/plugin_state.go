package models

import (
	"time"

	"gorm.io/datatypes"
)

// PluginState stores namespaced key-value data for plugins.
// It supports multiple scopes: instance, company, project, agent.
type PluginState struct {
	ID        string         `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	PluginID  string         `gorm:"column:plugin_id;type:uuid;not null;index:plugin_state_plugin_scope_idx"`
	ScopeKind string         `gorm:"column:scope_kind;not null;index:plugin_state_plugin_scope_idx"`
	ScopeID   *string        `gorm:"column:scope_id"`
	Namespace string         `gorm:"column:namespace;not null;default:'default'"`
	StateKey  string         `gorm:"column:state_key;not null"`
	ValueJSON datatypes.JSON `gorm:"column:value_json;type:jsonb;not null;default:'null'"`
	UpdatedAt time.Time      `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	Plugin Plugin `gorm:"foreignKey:PluginID"`
}

func (PluginState) TableName() string {
	return "plugin_state"
}
