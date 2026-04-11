package models

import (
	"time"

	"gorm.io/datatypes"
)

type CompanySecret struct {
	ID               string    `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID        string    `gorm:"column:company_id;type:uuid;not null;index:company_secrets_company_idx;index:company_secrets_company_provider_idx;uniqueIndex:company_secrets_company_name_uq"`
	Name             string    `gorm:"column:name;not null;uniqueIndex:company_secrets_company_name_uq"`
	Provider         string    `gorm:"column:provider;not null;default:local_encrypted;index:company_secrets_company_provider_idx"`
	ExternalRef      *string   `gorm:"column:external_ref"`
	LatestVersion    int       `gorm:"column:latest_version;not null;default:1"`
	Description      *string   `gorm:"column:description"`
	CreatedByAgentID *string   `gorm:"column:created_by_agent_id;type:uuid"`
	CreatedByUserID  *string   `gorm:"column:created_by_user_id"`
	CreatedAt        time.Time `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt        time.Time `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	Company        Company `gorm:"foreignKey:CompanyID"`
	CreatedByAgent *Agent  `gorm:"foreignKey:CreatedByAgentID;constraint:OnDelete:SET NULL"`
}

func (CompanySecret) TableName() string {
	return "company_secrets"
}

type CompanySecretVersion struct {
	ID               string         `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	SecretID         string         `gorm:"column:secret_id;type:uuid;not null;index:company_secret_versions_secret_idx;uniqueIndex:company_secret_versions_secret_version_uq"`
	Version          int            `gorm:"column:version;not null;uniqueIndex:company_secret_versions_secret_version_uq"`
	Material         datatypes.JSON `gorm:"column:material;type:jsonb;not null"`
	ValueSha256      string         `gorm:"column:value_sha256;not null;index:company_secret_versions_value_sha256_idx"`
	CreatedByAgentID *string        `gorm:"column:created_by_agent_id;type:uuid"`
	CreatedByUserID  *string        `gorm:"column:created_by_user_id"`
	CreatedAt        time.Time      `gorm:"column:created_at;type:timestamptz;not null;default:now();index:company_secret_versions_secret_idx"`
	RevokedAt        *time.Time     `gorm:"column:revoked_at;type:timestamptz"`

	Secret         CompanySecret `gorm:"foreignKey:SecretID;constraint:OnDelete:CASCADE"`
	CreatedByAgent *Agent        `gorm:"foreignKey:CreatedByAgentID;constraint:OnDelete:SET NULL"`
}

func (CompanySecretVersion) TableName() string {
	return "company_secret_versions"
}

type CompanySkill struct {
	ID            string         `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID     string         `gorm:"column:company_id;type:uuid;not null;uniqueIndex:company_skills_company_key_idx;index:company_skills_company_name_idx"`
	Key           string         `gorm:"column:key;not null;uniqueIndex:company_skills_company_key_idx"`
	Slug          string         `gorm:"column:slug;not null"`
	Name          string         `gorm:"column:name;not null;index:company_skills_company_name_idx"`
	Description   *string        `gorm:"column:description"`
	Markdown      string         `gorm:"column:markdown;not null"`
	SourceType    string         `gorm:"column:source_type;not null;default:local_path"`
	SourceLocator *string        `gorm:"column:source_locator"`
	SourceRef     *string        `gorm:"column:source_ref"`
	TrustLevel    string         `gorm:"column:trust_level;not null;default:markdown_only"`
	Compatibility string         `gorm:"column:compatibility;not null;default:compatible"`
	FileInventory datatypes.JSON `gorm:"column:file_inventory;type:jsonb;not null;default:'[]'"`
	Metadata      datatypes.JSON `gorm:"column:metadata;type:jsonb"`
	CreatedAt     time.Time      `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt     time.Time      `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	Company Company `gorm:"foreignKey:CompanyID"`
}

func (CompanySkill) TableName() string {
	return "company_skills"
}
