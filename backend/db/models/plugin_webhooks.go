package models

import (
	"time"

	"gorm.io/datatypes"
)

type PluginWebhookDelivery struct {
	ID         string         `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	PluginID   string         `gorm:"column:plugin_id;type:uuid;not null;index:plugin_webhook_deliveries_plugin_idx"`
	WebhookKey string         `gorm:"column:webhook_key;type:text;not null;index:plugin_webhook_deliveries_key_idx"`
	ExternalID *string        `gorm:"column:external_id;type:text"`
	Status     string         `gorm:"column:status;type:text;not null;default:'pending';index:plugin_webhook_deliveries_status_idx"`
	DurationMs *int           `gorm:"column:duration_ms;type:integer"`
	Error      *string        `gorm:"column:error;type:text"`
	Payload    datatypes.JSON `gorm:"column:payload;type:jsonb;not null"`
	Headers    datatypes.JSON `gorm:"column:headers;type:jsonb;not null;default:'{}'"`
	StartedAt  *time.Time     `gorm:"column:started_at;type:timestamptz"`
	FinishedAt *time.Time     `gorm:"column:finished_at;type:timestamptz"`
	CreatedAt  time.Time      `gorm:"column:created_at;type:timestamptz;not null;default:now()"`

	Plugin Plugin `gorm:"foreignKey:PluginID;constraint:OnDelete:CASCADE"`
}

func (PluginWebhookDelivery) TableName() string {
	return "plugin_webhook_deliveries"
}
