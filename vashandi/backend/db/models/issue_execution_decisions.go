package models

import (
	"time"
)

type IssueExecutionDecision struct {
	ID             string    `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID      string    `gorm:"column:company_id;type:uuid;not null;index:issue_execution_decisions_company_issue_idx;index:issue_execution_decisions_company_stage_idx"`
	IssueID        string    `gorm:"column:issue_id;type:uuid;not null;index:issue_execution_decisions_company_issue_idx"`
	StageID        string    `gorm:"column:stage_id;type:uuid;not null;index:issue_execution_decisions_company_stage_idx"`
	StageType      string    `gorm:"column:stage_type;not null"`
	ActorAgentID   *string   `gorm:"column:actor_agent_id;type:uuid"`
	ActorUserID    *string   `gorm:"column:actor_user_id"`
	Outcome        string    `gorm:"column:outcome;not null"`
	Body           string    `gorm:"column:body;not null"`
	CreatedByRunID *string   `gorm:"column:created_by_run_id;type:uuid"`
	CreatedAt      time.Time `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt      time.Time `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	Company      Company       `gorm:"foreignKey:CompanyID"`
	Issue        Issue         `gorm:"foreignKey:IssueID;constraint:OnDelete:CASCADE"`
	ActorAgent   *Agent        `gorm:"foreignKey:ActorAgentID"`
	CreatedByRun *HeartbeatRun `gorm:"foreignKey:CreatedByRunID;constraint:OnDelete:SET NULL"`
}

func (IssueExecutionDecision) TableName() string {
	return "issue_execution_decisions"
}
