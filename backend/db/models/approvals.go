package models

import (
	"time"

	"gorm.io/datatypes"
)

type Approval struct {
	ID                 string         `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID          string         `gorm:"column:company_id;type:uuid;not null;index:approvals_company_status_type_idx"`
	Type               string         `gorm:"column:type;not null;index:approvals_company_status_type_idx"`
	RequestedByAgentID *string        `gorm:"column:requested_by_agent_id;type:uuid"`
	RequestedByUserID  *string        `gorm:"column:requested_by_user_id"`
	Status             string         `gorm:"column:status;not null;default:pending;index:approvals_company_status_type_idx"`
	Payload            datatypes.JSON `gorm:"column:payload;type:jsonb;not null"`
	DecisionNote       *string        `gorm:"column:decision_note"`
	DecidedByUserID    *string        `gorm:"column:decided_by_user_id"`
	DecidedAt          *time.Time     `gorm:"column:decided_at;type:timestamptz"`
	CreatedAt          time.Time      `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt          time.Time      `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	Company          Company `gorm:"foreignKey:CompanyID"`
	RequestedByAgent *Agent  `gorm:"foreignKey:RequestedByAgentID"`
}

func (Approval) TableName() string {
	return "approvals"
}

type ApprovalComment struct {
	ID            string    `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID     string    `gorm:"column:company_id;type:uuid;not null;index:approval_comments_company_idx"`
	ApprovalID    string    `gorm:"column:approval_id;type:uuid;not null;index:approval_comments_approval_idx;index:approval_comments_approval_created_idx"`
	AuthorAgentID *string   `gorm:"column:author_agent_id;type:uuid"`
	AuthorUserID  *string   `gorm:"column:author_user_id"`
	Body          string    `gorm:"column:body;not null"`
	CreatedAt     time.Time `gorm:"column:created_at;type:timestamptz;not null;default:now();index:approval_comments_approval_created_idx"`
	UpdatedAt     time.Time `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	Company     Company  `gorm:"foreignKey:CompanyID"`
	Approval    Approval `gorm:"foreignKey:ApprovalID"`
	AuthorAgent *Agent   `gorm:"foreignKey:AuthorAgentID"`
}

func (ApprovalComment) TableName() string {
	return "approval_comments"
}
