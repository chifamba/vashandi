package models

import (
	"time"

	"gorm.io/datatypes"
)

type Plugin struct {
	ID           string         `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	PluginKey    string         `gorm:"column:plugin_key;notNull;uniqueIndex:plugins_plugin_key_idx"`
	PackageName  string         `gorm:"column:package_name;notNull"`
	Version      string         `gorm:"column:version;notNull"`
	ApiVersion   int            `gorm:"column:api_version;notNull;default:1"`
	Categories   datatypes.JSON `gorm:"column:categories;type:jsonb;notNull;default:'[]'"`
	ManifestJSON datatypes.JSON `gorm:"column:manifest_json;type:jsonb;notNull"`
	Status       string         `gorm:"column:status;notNull;default:installed;index:plugins_status_idx"`
	InstallOrder *int           `gorm:"column:install_order"`
	PackagePath  *string        `gorm:"column:package_path"`
	LastError    *string        `gorm:"column:last_error"`
	InstalledAt  time.Time      `gorm:"column:installed_at;type:timestamptz;notNull;default:now()"`
	UpdatedAt    time.Time      `gorm:"column:updated_at;type:timestamptz;notNull;default:now()"`
}

func (Plugin) TableName() string {
	return "plugins"
}

type PluginConfig struct {
	ID        string         `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID string         `gorm:"column:company_id;type:uuid;notNull;uniqueIndex:plugin_config_company_plugin_idx"`
	PluginKey string         `gorm:"column:plugin_key;notNull;uniqueIndex:plugin_config_company_plugin_idx"`
	Config    datatypes.JSON `gorm:"column:config;type:jsonb;notNull;default:'{}'"`
	CreatedAt time.Time      `gorm:"column:created_at;type:timestamptz;notNull;default:now()"`
	UpdatedAt time.Time      `gorm:"column:updated_at;type:timestamptz;notNull;default:now()"`
}

func (PluginConfig) TableName() string {
	return "plugin_config"
}
