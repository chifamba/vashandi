package models

import (
	"time"

	"gorm.io/datatypes"
)

type InstanceSetting struct {
	ID           string         `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	SingletonKey string         `gorm:"column:singleton_key;not null;default:default;uniqueIndex:instance_settings_singleton_key_idx"`
	General      datatypes.JSON `gorm:"column:general;type:jsonb;not null;default:'{}'"`
	Experimental datatypes.JSON `gorm:"column:experimental;type:jsonb;not null;default:'{}'"`
	CreatedAt    time.Time      `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt    time.Time      `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`
}

func (InstanceSetting) TableName() string {
	return "instance_settings"
}
