package models

import (
	"time"

	"gorm.io/datatypes"
)

type PluginJob struct {
	ID        string     `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	PluginID  string     `gorm:"column:plugin_id;type:uuid;not null;index:plugin_jobs_plugin_idx;uniqueIndex:plugin_jobs_unique_idx"`
	JobKey    string     `gorm:"column:job_key;type:text;not null;uniqueIndex:plugin_jobs_unique_idx"`
	Schedule  string     `gorm:"column:schedule;type:text;not null"`
	Status    string     `gorm:"column:status;type:text;not null;default:'active'"`
	LastRunAt *time.Time `gorm:"column:last_run_at;type:timestamptz"`
	NextRunAt *time.Time `gorm:"column:next_run_at;type:timestamptz;index:plugin_jobs_next_run_idx"`
	CreatedAt time.Time  `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt time.Time  `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	Plugin Plugin `gorm:"foreignKey:PluginID;constraint:OnDelete:CASCADE"`
}

func (PluginJob) TableName() string {
	return "plugin_jobs"
}

type PluginJobRun struct {
	ID         string         `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	JobID      string         `gorm:"column:job_id;type:uuid;not null;index:plugin_job_runs_job_idx"`
	PluginID   string         `gorm:"column:plugin_id;type:uuid;not null;index:plugin_job_runs_plugin_idx"`
	Trigger    string         `gorm:"column:trigger;type:text;not null"`
	Status     string         `gorm:"column:status;type:text;not null;default:'pending';index:plugin_job_runs_status_idx"`
	DurationMs *int           `gorm:"column:duration_ms;type:integer"`
	Error      *string        `gorm:"column:error;type:text"`
	Logs       datatypes.JSON `gorm:"column:logs;type:jsonb;not null;default:'[]'"`
	StartedAt  *time.Time     `gorm:"column:started_at;type:timestamptz"`
	FinishedAt *time.Time     `gorm:"column:finished_at;type:timestamptz"`
	CreatedAt  time.Time      `gorm:"column:created_at;type:timestamptz;not null;default:now()"`

	Job    PluginJob `gorm:"foreignKey:JobID;constraint:OnDelete:CASCADE"`
	Plugin Plugin    `gorm:"foreignKey:PluginID;constraint:OnDelete:CASCADE"`
}

func (PluginJobRun) TableName() string {
	return "plugin_job_runs"
}
