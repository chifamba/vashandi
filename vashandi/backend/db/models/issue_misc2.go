package models

import (
	"time"
)

type IssueDocument struct {
	ID         string    `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID  string    `gorm:"column:company_id;type:uuid;not null;uniqueIndex:issue_documents_company_issue_key_uq;index:issue_documents_company_issue_updated_idx"`
	IssueID    string    `gorm:"column:issue_id;type:uuid;not null;uniqueIndex:issue_documents_company_issue_key_uq;index:issue_documents_company_issue_updated_idx"`
	DocumentID string    `gorm:"column:document_id;type:uuid;not null;uniqueIndex:issue_documents_document_uq"`
	Key        string    `gorm:"column:key;not null;uniqueIndex:issue_documents_company_issue_key_uq"`
	CreatedAt  time.Time `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt  time.Time `gorm:"column:updated_at;type:timestamptz;not null;default:now();index:issue_documents_company_issue_updated_idx"`

	Company Company `gorm:"foreignKey:CompanyID"`
	Issue   Issue   `gorm:"foreignKey:IssueID;constraint:OnDelete:CASCADE"`
	// Document  Document `gorm:"foreignKey:DocumentID;constraint:OnDelete:CASCADE"` // Will port documents.go shortly
}

func (IssueDocument) TableName() string {
	return "issue_documents"
}

type IssueInboxArchive struct {
	ID         string    `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID  string    `gorm:"column:company_id;type:uuid;not null;index:issue_inbox_archives_company_issue_idx;index:issue_inbox_archives_company_user_idx;uniqueIndex:issue_inbox_archives_company_issue_user_idx"`
	IssueID    string    `gorm:"column:issue_id;type:uuid;not null;index:issue_inbox_archives_company_issue_idx;uniqueIndex:issue_inbox_archives_company_issue_user_idx"`
	UserID     string    `gorm:"column:user_id;not null;index:issue_inbox_archives_company_user_idx;uniqueIndex:issue_inbox_archives_company_issue_user_idx"`
	ArchivedAt time.Time `gorm:"column:archived_at;type:timestamptz;not null;default:now()"`
	CreatedAt  time.Time `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt  time.Time `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	Company Company `gorm:"foreignKey:CompanyID"`
	Issue   Issue   `gorm:"foreignKey:IssueID"`
	User    User    `gorm:"foreignKey:UserID"`
}

func (IssueInboxArchive) TableName() string {
	return "issue_inbox_archives"
}

type IssueLabel struct {
	IssueID   string    `gorm:"column:issue_id;type:uuid;not null;primaryKey;index:issue_labels_issue_idx"`
	LabelID   string    `gorm:"column:label_id;type:uuid;not null;primaryKey;index:issue_labels_label_idx"`
	CompanyID string    `gorm:"column:company_id;type:uuid;not null;index:issue_labels_company_idx"`
	CreatedAt time.Time `gorm:"column:created_at;type:timestamptz;not null;default:now()"`

	Issue Issue `gorm:"foreignKey:IssueID;constraint:OnDelete:CASCADE"`
	// Label   Label   `gorm:"foreignKey:LabelID;constraint:OnDelete:CASCADE"` // Will port labels.go shortly
	Company Company `gorm:"foreignKey:CompanyID;constraint:OnDelete:CASCADE"`
}

func (IssueLabel) TableName() string {
	return "issue_labels"
}
