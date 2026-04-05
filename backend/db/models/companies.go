package models

import (
	"time"
)

type Company struct {
	ID                                 string     `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	Name                               string     `gorm:"column:name;not null"`
	Description                        *string    `gorm:"column:description"`
	Status                             string     `gorm:"column:status;not null;default:active"`
	PauseReason                        *string    `gorm:"column:pause_reason"`
	PausedAt                           *time.Time `gorm:"column:paused_at;type:timestamptz"`
	IssuePrefix                        string     `gorm:"column:issue_prefix;not null;default:PAP;uniqueIndex:companies_issue_prefix_idx"`
	IssueCounter                       int        `gorm:"column:issue_counter;not null;default:0"`
	BudgetMonthlyCents                 int        `gorm:"column:budget_monthly_cents;not null;default:0"`
	SpentMonthlyCents                  int        `gorm:"column:spent_monthly_cents;not null;default:0"`
	RequireBoardApprovalForNewAgents   bool       `gorm:"column:require_board_approval_for_new_agents;not null;default:true"`
	FeedbackDataSharingEnabled         bool       `gorm:"column:feedback_data_sharing_enabled;not null;default:false"`
	FeedbackDataSharingConsentAt       *time.Time `gorm:"column:feedback_data_sharing_consent_at;type:timestamptz"`
	FeedbackDataSharingConsentByUserID *string    `gorm:"column:feedback_data_sharing_consent_by_user_id"`
	FeedbackDataSharingTermsVersion    *string    `gorm:"column:feedback_data_sharing_terms_version"`
	BrandColor                         *string    `gorm:"column:brand_color"`
	CreatedAt                          time.Time  `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt                          time.Time  `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`
}

func (Company) TableName() string {
	return "companies"
}
