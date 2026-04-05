package models

import (
	"time"
)

type Document struct {
	ID                   string    `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID            string    `gorm:"column:company_id;type:uuid;not null;index:documents_company_updated_idx;index:documents_company_created_idx"`
	Title                *string   `gorm:"column:title"`
	Format               string    `gorm:"column:format;not null;default:markdown"`
	LatestBody           string    `gorm:"column:latest_body;not null"`
	LatestRevisionID     *string   `gorm:"column:latest_revision_id;type:uuid"`
	LatestRevisionNumber int       `gorm:"column:latest_revision_number;not null;default:1"`
	CreatedByAgentID     *string   `gorm:"column:created_by_agent_id;type:uuid"`
	CreatedByUserID      *string   `gorm:"column:created_by_user_id"`
	UpdatedByAgentID     *string   `gorm:"column:updated_by_agent_id;type:uuid"`
	UpdatedByUserID      *string   `gorm:"column:updated_by_user_id"`
	CreatedAt            time.Time `gorm:"column:created_at;type:timestamptz;not null;default:now();index:documents_company_created_idx"`
	UpdatedAt            time.Time `gorm:"column:updated_at;type:timestamptz;not null;default:now();index:documents_company_updated_idx"`

	Company        Company `gorm:"foreignKey:CompanyID"`
	CreatedByAgent *Agent  `gorm:"foreignKey:CreatedByAgentID;constraint:OnDelete:SET NULL"`
	UpdatedByAgent *Agent  `gorm:"foreignKey:UpdatedByAgentID;constraint:OnDelete:SET NULL"`
}

func (Document) TableName() string {
	return "documents"
}

type DocumentRevision struct {
	ID               string    `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID        string    `gorm:"column:company_id;type:uuid;not null;index:document_revisions_company_document_created_idx"`
	DocumentID       string    `gorm:"column:document_id;type:uuid;not null;uniqueIndex:document_revisions_document_revision_uq;index:document_revisions_company_document_created_idx"`
	RevisionNumber   int       `gorm:"column:revision_number;not null;uniqueIndex:document_revisions_document_revision_uq"`
	Title            *string   `gorm:"column:title"`
	Format           string    `gorm:"column:format;not null;default:markdown"`
	Body             string    `gorm:"column:body;not null"`
	ChangeSummary    *string   `gorm:"column:change_summary"`
	CreatedByAgentID *string   `gorm:"column:created_by_agent_id;type:uuid"`
	CreatedByUserID  *string   `gorm:"column:created_by_user_id"`
	CreatedByRunID   *string   `gorm:"column:created_by_run_id;type:uuid"`
	CreatedAt        time.Time `gorm:"column:created_at;type:timestamptz;not null;default:now();index:document_revisions_company_document_created_idx"`

	Company        Company       `gorm:"foreignKey:CompanyID"`
	Document       Document      `gorm:"foreignKey:DocumentID;constraint:OnDelete:CASCADE"`
	CreatedByAgent *Agent        `gorm:"foreignKey:CreatedByAgentID;constraint:OnDelete:SET NULL"`
	CreatedByRun   *HeartbeatRun `gorm:"foreignKey:CreatedByRunID;constraint:OnDelete:SET NULL"`
}

func (DocumentRevision) TableName() string {
	return "document_revisions"
}
