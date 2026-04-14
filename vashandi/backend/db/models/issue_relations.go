package models

import (
	"time"
)

type IssueRelation struct {
	ID                string    `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID         string    `gorm:"column:company_id;type:uuid;not null;index:issue_relations_company_issue_idx;index:issue_relations_company_related_issue_idx"`
	IssueID           string    `gorm:"column:issue_id;type:uuid;not null;index:issue_relations_company_issue_idx;uniqueIndex:issue_relations_issue_related_type_uq"`
	RelatedIssueID    string    `gorm:"column:related_issue_id;type:uuid;not null;index:issue_relations_company_related_issue_idx;uniqueIndex:issue_relations_issue_related_type_uq"`
	Type              string    `gorm:"column:type;not null;uniqueIndex:issue_relations_issue_related_type_uq"`
	CreatedByAgentID  *string   `gorm:"column:created_by_agent_id;type:uuid"`
	CreatedByUserID   *string   `gorm:"column:created_by_user_id"`
	CreatedAt         time.Time `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt         time.Time `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	Company          Company `gorm:"foreignKey:CompanyID"`
	Issue            Issue   `gorm:"foreignKey:IssueID;constraint:OnDelete:CASCADE"`
	RelatedIssue     Issue   `gorm:"foreignKey:RelatedIssueID;constraint:OnDelete:CASCADE"`
	CreatedByAgent   *Agent  `gorm:"foreignKey:CreatedByAgentID;constraint:OnDelete:SET NULL"`
}

func (IssueRelation) TableName() string {
	return "issue_relations"
}
