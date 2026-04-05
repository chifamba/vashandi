package models

import (
	"time"

	"gorm.io/datatypes"
)

type PluginLog struct {
	ID        string         `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	PluginID  string         `gorm:"column:plugin_id;type:uuid;not null;index:plugin_logs_plugin_time_idx"`
	Level     string         `gorm:"column:level;type:text;not null;default:'info';index:plugin_logs_level_idx"`
	Message   string         `gorm:"column:message;type:text;not null"`
	Meta      datatypes.JSON `gorm:"column:meta;type:jsonb"`
	CreatedAt time.Time      `gorm:"column:created_at;type:timestamptz;not null;default:now();index:plugin_logs_plugin_time_idx"`

	Plugin Plugin `gorm:"foreignKey:PluginID;constraint:OnDelete:CASCADE"`
}

func (PluginLog) TableName() string {
	return "plugin_logs"
}
