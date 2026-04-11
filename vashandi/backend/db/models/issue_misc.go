package models

import (
	"time"
)

type IssueApproval struct {
	CompanyID       string    `gorm:"column:company_id;type:uuid;not null;index:issue_approvals_company_idx"`
	IssueID         string    `gorm:"column:issue_id;type:uuid;not null;primaryKey;index:issue_approvals_issue_idx"`
	ApprovalID      string    `gorm:"column:approval_id;type:uuid;not null;primaryKey;index:issue_approvals_approval_idx"`
	LinkedByAgentID *string   `gorm:"column:linked_by_agent_id;type:uuid"`
	LinkedByUserID  *string   `gorm:"column:linked_by_user_id"`
	CreatedAt       time.Time `gorm:"column:created_at;type:timestamptz;not null;default:now()"`

	Company       Company  `gorm:"foreignKey:CompanyID"`
	Issue         Issue    `gorm:"foreignKey:IssueID;constraint:OnDelete:CASCADE"`
	Approval      Approval `gorm:"foreignKey:ApprovalID;constraint:OnDelete:CASCADE"`
	LinkedByAgent *Agent   `gorm:"foreignKey:LinkedByAgentID;constraint:OnDelete:SET NULL"`
}

func (IssueApproval) TableName() string {
	return "issue_approvals"
}

type IssueComment struct {
	ID             string    `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID      string    `gorm:"column:company_id;type:uuid;not null;index:issue_comments_company_idx;index:issue_comments_company_issue_created_at_idx;index:issue_comments_company_author_issue_created_at_idx"`
	IssueID        string    `gorm:"column:issue_id;type:uuid;not null;index:issue_comments_issue_idx;index:issue_comments_company_issue_created_at_idx;index:issue_comments_company_author_issue_created_at_idx"`
	AuthorAgentID  *string   `gorm:"column:author_agent_id;type:uuid"`
	AuthorUserID   *string   `gorm:"column:author_user_id;index:issue_comments_company_author_issue_created_at_idx"`
	CreatedByRunID *string   `gorm:"column:created_by_run_id;type:uuid"`
	Body           string    `gorm:"column:body;not null"`
	CreatedAt      time.Time `gorm:"column:created_at;type:timestamptz;not null;default:now();index:issue_comments_company_issue_created_at_idx;index:issue_comments_company_author_issue_created_at_idx"`
	UpdatedAt      time.Time `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	Company      Company       `gorm:"foreignKey:CompanyID"`
	Issue        Issue         `gorm:"foreignKey:IssueID"`
	AuthorAgent  *Agent        `gorm:"foreignKey:AuthorAgentID"`
	CreatedByRun *HeartbeatRun `gorm:"foreignKey:CreatedByRunID;constraint:OnDelete:SET NULL"`
}

func (IssueComment) TableName() string {
	return "issue_comments"
}

type IssueAttachment struct {
	ID             string    `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID      string    `gorm:"column:company_id;type:uuid;not null;index:issue_attachments_company_issue_idx"`
	IssueID        string    `gorm:"column:issue_id;type:uuid;not null;index:issue_attachments_company_issue_idx"`
	AssetID        string    `gorm:"column:asset_id;type:uuid;not null;uniqueIndex:issue_attachments_asset_uq"`
	IssueCommentID *string   `gorm:"column:issue_comment_id;type:uuid;index:issue_attachments_issue_comment_idx"`
	CreatedAt      time.Time `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt      time.Time `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	Company      Company       `gorm:"foreignKey:CompanyID"`
	Issue        Issue         `gorm:"foreignKey:IssueID;constraint:OnDelete:CASCADE"`
	Asset        Asset         `gorm:"foreignKey:AssetID;constraint:OnDelete:CASCADE"`
	IssueComment *IssueComment `gorm:"foreignKey:IssueCommentID;constraint:OnDelete:SET NULL"`
}

func (IssueAttachment) TableName() string {
	return "issue_attachments"
}
