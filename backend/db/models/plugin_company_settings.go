package models

import (
	"time"

	"gorm.io/datatypes"
)

type PluginCompanySetting struct {
	ID           string         `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID    string         `gorm:"column:company_id;type:uuid;not null;index:plugin_company_settings_company_idx;uniqueIndex:plugin_company_settings_company_plugin_uq"`
	PluginID     string         `gorm:"column:plugin_id;type:uuid;not null;index:plugin_company_settings_plugin_idx;uniqueIndex:plugin_company_settings_company_plugin_uq"`
	Enabled      bool           `gorm:"column:enabled;type:boolean;not null;default:true"`
	SettingsJSON datatypes.JSON `gorm:"column:settings_json;type:jsonb;not null;default:'{}'"`
	LastError    *string        `gorm:"column:last_error;type:text"`
	CreatedAt    time.Time      `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt    time.Time      `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	Company Company `gorm:"foreignKey:CompanyID;constraint:OnDelete:CASCADE"`
	Plugin  Plugin  `gorm:"foreignKey:PluginID;constraint:OnDelete:CASCADE"`
}

func (PluginCompanySetting) TableName() string {
	return "plugin_company_settings"
}
