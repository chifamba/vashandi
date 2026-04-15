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

// PluginConfig stores instance-level configuration for a plugin.
// The plugin_id is a unique FK to plugins.id (one config record per plugin).
type PluginConfig struct {
	ID         string         `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	PluginID   string         `gorm:"column:plugin_id;type:uuid;notNull;uniqueIndex:plugin_config_plugin_id_idx"`
	ConfigJSON datatypes.JSON `gorm:"column:config_json;type:jsonb;notNull;default:'{}'"`
	LastError  *string        `gorm:"column:last_error"`
	CreatedAt  time.Time      `gorm:"column:created_at;type:timestamptz;notNull;default:now()"`
	UpdatedAt  time.Time      `gorm:"column:updated_at;type:timestamptz;notNull;default:now()"`
}

func (PluginConfig) TableName() string {
	return "plugin_config"
}

// PluginLog stores log entries emitted by a plugin worker.
type PluginLog struct {
	ID        string         `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	PluginID  string         `gorm:"column:plugin_id;type:uuid;notNull;index:plugin_logs_plugin_time_idx"`
	Level     string         `gorm:"column:level;notNull;default:info;index:plugin_logs_level_idx"`
	Message   string         `gorm:"column:message;notNull"`
	Meta      datatypes.JSON `gorm:"column:meta;type:jsonb"`
	CreatedAt time.Time      `gorm:"column:created_at;type:timestamptz;notNull;default:now();index:plugin_logs_plugin_time_idx"`
}

func (PluginLog) TableName() string {
	return "plugin_logs"
}

// PluginJob represents a scheduled job declared by a plugin in its manifest.
type PluginJob struct {
	ID        string     `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	PluginID  string     `gorm:"column:plugin_id;type:uuid;notNull;index:plugin_jobs_plugin_idx;uniqueIndex:plugin_jobs_unique_idx"`
	JobKey    string     `gorm:"column:job_key;notNull;uniqueIndex:plugin_jobs_unique_idx"`
	Schedule  string     `gorm:"column:schedule;notNull"`
	Status    string     `gorm:"column:status;notNull;default:active"`
	LastRunAt *time.Time `gorm:"column:last_run_at;type:timestamptz"`
	NextRunAt *time.Time `gorm:"column:next_run_at;type:timestamptz;index:plugin_jobs_next_run_idx"`
	CreatedAt time.Time  `gorm:"column:created_at;type:timestamptz;notNull;default:now()"`
	UpdatedAt time.Time  `gorm:"column:updated_at;type:timestamptz;notNull;default:now()"`
}

func (PluginJob) TableName() string {
	return "plugin_jobs"
}

// PluginJobRun records a single execution of a plugin job.
type PluginJobRun struct {
	ID         string         `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	JobID      string         `gorm:"column:job_id;type:uuid;notNull;index:plugin_job_runs_job_idx"`
	PluginID   string         `gorm:"column:plugin_id;type:uuid;notNull;index:plugin_job_runs_plugin_idx"`
	Trigger    string         `gorm:"column:trigger;notNull"`
	Status     string         `gorm:"column:status;notNull;default:pending;index:plugin_job_runs_status_idx"`
	DurationMs *int           `gorm:"column:duration_ms"`
	Error      *string        `gorm:"column:error"`
	Logs       datatypes.JSON `gorm:"column:logs;type:jsonb;notNull;default:'[]'"`
	StartedAt  *time.Time     `gorm:"column:started_at;type:timestamptz"`
	FinishedAt *time.Time     `gorm:"column:finished_at;type:timestamptz"`
	CreatedAt  time.Time      `gorm:"column:created_at;type:timestamptz;notNull;default:now()"`
}

func (PluginJobRun) TableName() string {
	return "plugin_job_runs"
}

// PluginWebhookDelivery records an inbound webhook delivery attempt for a plugin.
type PluginWebhookDelivery struct {
	ID         string         `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	PluginID   string         `gorm:"column:plugin_id;type:uuid;notNull;index:plugin_webhook_deliveries_plugin_idx"`
	WebhookKey string         `gorm:"column:webhook_key;notNull;index:plugin_webhook_deliveries_key_idx"`
	ExternalID *string        `gorm:"column:external_id"`
	Status     string         `gorm:"column:status;notNull;default:pending;index:plugin_webhook_deliveries_status_idx"`
	DurationMs *int           `gorm:"column:duration_ms"`
	Error      *string        `gorm:"column:error"`
	Payload    datatypes.JSON `gorm:"column:payload;type:jsonb;notNull"`
	Headers    datatypes.JSON `gorm:"column:headers;type:jsonb;notNull;default:'{}'"`
	StartedAt  *time.Time     `gorm:"column:started_at;type:timestamptz"`
	FinishedAt *time.Time     `gorm:"column:finished_at;type:timestamptz"`
	CreatedAt  time.Time      `gorm:"column:created_at;type:timestamptz;notNull;default:now()"`
}

func (PluginWebhookDelivery) TableName() string {
	return "plugin_webhook_deliveries"
}
