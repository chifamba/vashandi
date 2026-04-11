package models

import (
	"time"

	"gorm.io/datatypes"
)

type FeedbackVote struct {
	ID               string         `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID        string         `gorm:"column:company_id;type:uuid;not null;index:feedback_votes_company_issue_idx;uniqueIndex:feedback_votes_company_target_author_idx"`
	IssueID          string         `gorm:"column:issue_id;type:uuid;not null;index:feedback_votes_company_issue_idx;index:feedback_votes_issue_target_idx"`
	TargetType       string         `gorm:"column:target_type;not null;index:feedback_votes_issue_target_idx;uniqueIndex:feedback_votes_company_target_author_idx"`
	TargetID         string         `gorm:"column:target_id;not null;index:feedback_votes_issue_target_idx;uniqueIndex:feedback_votes_company_target_author_idx"`
	AuthorUserID     string         `gorm:"column:author_user_id;not null;index:feedback_votes_author_idx;uniqueIndex:feedback_votes_company_target_author_idx"`
	Vote             string         `gorm:"column:vote;not null"`
	Reason           *string        `gorm:"column:reason"`
	SharedWithLabs   bool           `gorm:"column:shared_with_labs;not null;default:false"`
	SharedAt         *time.Time     `gorm:"column:shared_at;type:timestamptz"`
	ConsentVersion   *string        `gorm:"column:consent_version"`
	RedactionSummary datatypes.JSON `gorm:"column:redaction_summary;type:jsonb"`
	CreatedAt        time.Time      `gorm:"column:created_at;type:timestamptz;not null;default:now();index:feedback_votes_author_idx"`
	UpdatedAt        time.Time      `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	Company Company `gorm:"foreignKey:CompanyID"`
	Issue   Issue   `gorm:"foreignKey:IssueID"`
	Author  User    `gorm:"foreignKey:AuthorUserID"`
}

func (FeedbackVote) TableName() string {
	return "feedback_votes"
}

type FeedbackExport struct {
	ID               string         `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID        string         `gorm:"column:company_id;type:uuid;not null;index:feedback_exports_company_created_idx;index:feedback_exports_company_status_idx;index:feedback_exports_company_issue_idx;index:feedback_exports_company_project_idx;index:feedback_exports_company_author_idx"`
	FeedbackVoteID   string         `gorm:"column:feedback_vote_id;type:uuid;not null;uniqueIndex:feedback_exports_feedback_vote_idx"`
	IssueID          string         `gorm:"column:issue_id;type:uuid;not null;index:feedback_exports_company_issue_idx"`
	ProjectID        *string        `gorm:"column:project_id;type:uuid;index:feedback_exports_company_project_idx"`
	AuthorUserID     string         `gorm:"column:author_user_id;not null;index:feedback_exports_company_author_idx"`
	TargetType       string         `gorm:"column:target_type;not null"`
	TargetID         string         `gorm:"column:target_id;not null"`
	Vote             string         `gorm:"column:vote;not null"`
	Status           string         `gorm:"column:status;not null;default:local_only;index:feedback_exports_company_status_idx"`
	Destination      *string        `gorm:"column:destination"`
	ExportID         *string        `gorm:"column:export_id"`
	ConsentVersion   *string        `gorm:"column:consent_version"`
	SchemaVersion    string         `gorm:"column:schema_version;not null;default:paperclip-feedback-envelope-v2"`
	BundleVersion    string         `gorm:"column:bundle_version;not null;default:paperclip-feedback-bundle-v2"`
	PayloadVersion   string         `gorm:"column:payload_version;not null;default:paperclip-feedback-v1"`
	PayloadDigest    *string        `gorm:"column:payload_digest"`
	PayloadSnapshot  datatypes.JSON `gorm:"column:payload_snapshot;type:jsonb"`
	TargetSummary    datatypes.JSON `gorm:"column:target_summary;type:jsonb;not null"`
	RedactionSummary datatypes.JSON `gorm:"column:redaction_summary;type:jsonb"`
	AttemptCount     int            `gorm:"column:attempt_count;not null;default:0"`
	LastAttemptedAt  *time.Time     `gorm:"column:last_attempted_at;type:timestamptz"`
	ExportedAt       *time.Time     `gorm:"column:exported_at;type:timestamptz"`
	FailureReason    *string        `gorm:"column:failure_reason"`
	CreatedAt        time.Time      `gorm:"column:created_at;type:timestamptz;not null;default:now();index:feedback_exports_company_created_idx;index:feedback_exports_company_status_idx;index:feedback_exports_company_issue_idx;index:feedback_exports_company_project_idx;index:feedback_exports_company_author_idx"`
	UpdatedAt        time.Time      `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	Company      Company      `gorm:"foreignKey:CompanyID"`
	FeedbackVote FeedbackVote `gorm:"foreignKey:FeedbackVoteID;constraint:OnDelete:CASCADE"`
	Issue        Issue        `gorm:"foreignKey:IssueID;constraint:OnDelete:CASCADE"`
	Project      *Project     `gorm:"foreignKey:ProjectID;constraint:OnDelete:SET NULL"`
	Author       User         `gorm:"foreignKey:AuthorUserID"`
}

func (FeedbackExport) TableName() string {
	return "feedback_exports"
}
