package models

import (
	"time"

	"gorm.io/datatypes"
)

type Plugin struct {
	ID           string         `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	PluginKey    string         `gorm:"column:plugin_key;type:text;not null;uniqueIndex:plugins_plugin_key_idx"`
	PackageName  string         `gorm:"column:package_name;type:text;not null"`
	Version      string         `gorm:"column:version;type:text;not null"`
	APIVersion   int            `gorm:"column:api_version;type:integer;not null;default:1"`
	Categories   datatypes.JSON `gorm:"column:categories;type:jsonb;not null;default:'[]'"`
	ManifestJSON datatypes.JSON `gorm:"column:manifest_json;type:jsonb;not null"`
	Status       string         `gorm:"column:status;type:text;not null;default:'installed';index:plugins_status_idx"`
	InstallOrder *int           `gorm:"column:install_order;type:integer"`
	PackagePath  *string        `gorm:"column:package_path;type:text"`
	LastError    *string        `gorm:"column:last_error;type:text"`
	InstalledAt  time.Time      `gorm:"column:installed_at;type:timestamptz;not null;default:now()"`
	UpdatedAt    time.Time      `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`
}

func (Plugin) TableName() string {
	return "plugins"
}
