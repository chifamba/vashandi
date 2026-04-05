package models

import (
	"time"

	"gorm.io/datatypes"
)

type PluginConfig struct {
	ID         string         `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	PluginID   string         `gorm:"column:plugin_id;type:uuid;not null;uniqueIndex:plugin_config_plugin_id_idx"`
	ConfigJSON datatypes.JSON `gorm:"column:config_json;type:jsonb;not null;default:'{}'"`
	LastError  *string        `gorm:"column:last_error;type:text"`
	CreatedAt  time.Time      `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt  time.Time      `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	Plugin Plugin `gorm:"foreignKey:PluginID;constraint:OnDelete:CASCADE"`
}

func (PluginConfig) TableName() string {
	return "plugin_config"
}
