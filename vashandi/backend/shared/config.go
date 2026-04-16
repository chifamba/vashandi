package shared

import (
	"time"
)

type ConfigMeta struct {
	Version   int       `json:"version" validate:"required,eq=1"`
	UpdatedAt time.Time `json:"updatedAt" validate:"required"`
	Source    string    `json:"source" validate:"required,oneof=onboard configure doctor"`
}

type LlmConfig struct {
	Provider string `json:"provider" validate:"required,oneof=claude openai"`
	ApiKey   string `json:"apiKey,omitempty"`
}

type DatabaseBackupConfig struct {
	Enabled         bool   `json:"enabled"`
	IntervalMinutes int    `json:"intervalMinutes" validate:"min=1,max=10080"` // 7 * 24 * 60
	RetentionDays   int    `json:"retentionDays" validate:"min=1,max=3650"`
	Dir             string `json:"dir"`
}

type DatabaseConfig struct {
	Mode                    string               `json:"mode" validate:"oneof=embedded-postgres postgres"`
	ConnectionString        string               `json:"connectionString,omitempty"`
	EmbeddedPostgresDataDir string               `json:"embeddedPostgresDataDir"`
	EmbeddedPostgresPort    int                  `json:"embeddedPostgresPort" validate:"min=1,max=65535"`
	Backup                  DatabaseBackupConfig `json:"backup"`
}

type LoggingConfig struct {
	Mode   string `json:"mode" validate:"required,oneof=file cloud"`
	LogDir string `json:"logDir"`
}

type ServerConfig struct {
	DeploymentMode   string   `json:"deploymentMode" validate:"oneof=local_trusted authenticated"`
	Exposure         string   `json:"exposure" validate:"oneof=private public"`
	Host             string   `json:"host"`
	Port             int      `json:"port" validate:"min=1,max=65535"`
	AllowedHostnames []string `json:"allowedHostnames" validate:"omitempty,dive,min=1"`
	ServeUi          bool     `json:"serveUi"`
}

type AuthConfig struct {
	BaseUrlMode              string `json:"baseUrlMode" validate:"oneof=auto explicit"`
	PublicBaseUrl            string `json:"publicBaseUrl,omitempty" validate:"omitempty,url"`
	DisableSignUp            bool   `json:"disableSignUp"`
	RequireEmailVerification bool   `json:"requireEmailVerification"`
}

type StorageLocalDiskConfig struct {
	BaseDir string `json:"baseDir"`
}

type StorageS3Config struct {
	Bucket         string `json:"bucket" validate:"required,min=1"`
	Region         string `json:"region" validate:"required,min=1"`
	Endpoint       string `json:"endpoint,omitempty"`
	Prefix         string `json:"prefix"`
	ForcePathStyle bool   `json:"forcePathStyle"`
}

type StorageConfig struct {
	Provider  string                 `json:"provider" validate:"oneof=local_disk s3"`
	LocalDisk StorageLocalDiskConfig `json:"localDisk"`
	S3        StorageS3Config        `json:"s3"`
}

type SecretsLocalEncryptedConfig struct {
	KeyFilePath string `json:"keyFilePath"`
}

type SecretsConfig struct {
	Provider       string                      `json:"provider" validate:"oneof=local_encrypted"`
	StrictMode     bool                        `json:"strictMode"`
	LocalEncrypted SecretsLocalEncryptedConfig `json:"localEncrypted"`
}

type TelemetryConfig struct {
	Enabled bool `json:"enabled"`
}

type PaperclipConfig struct {
	Meta      ConfigMeta      `json:"$meta" validate:"required"`
	Llm       *LlmConfig      `json:"llm,omitempty"`
	Database  DatabaseConfig  `json:"database" validate:"required"`
	Logging   LoggingConfig   `json:"logging" validate:"required"`
	Server    ServerConfig    `json:"server" validate:"required"`
	Telemetry TelemetryConfig `json:"telemetry"`
	Auth      AuthConfig      `json:"auth" validate:"required"`
	Storage   StorageConfig   `json:"storage" validate:"required"`
	Secrets   SecretsConfig   `json:"secrets" validate:"required"`
}
