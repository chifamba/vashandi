package models

import (
	"time"

	"gorm.io/datatypes"
)

type PluginState struct {
	ID        string         `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	PluginID  string         `gorm:"column:plugin_id;type:uuid;not null;index:plugin_state_plugin_scope_idx;uniqueIndex:plugin_state_unique_entry_idx"`
	ScopeKind string         `gorm:"column:scope_kind;type:text;not null;index:plugin_state_plugin_scope_idx;uniqueIndex:plugin_state_unique_entry_idx"`
	ScopeID   *string        `gorm:"column:scope_id;type:text;uniqueIndex:plugin_state_unique_entry_idx"` // gorm does not support NULLS NOT DISTINCT natively yet without raw SQL, but we map the struct field.
	Namespace string         `gorm:"column:namespace;type:text;not null;default:'default';uniqueIndex:plugin_state_unique_entry_idx"`
	StateKey  string         `gorm:"column:state_key;type:text;not null;uniqueIndex:plugin_state_unique_entry_idx"`
	ValueJSON datatypes.JSON `gorm:"column:value_json;type:jsonb;not null"`
	UpdatedAt time.Time      `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	Plugin Plugin `gorm:"foreignKey:PluginID;constraint:OnDelete:CASCADE"`
}

func (PluginState) TableName() string {
	return "plugin_state"
}
