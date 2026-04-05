package models

import (
	"time"

	"gorm.io/datatypes"
)

type JoinRequest struct {
	ID                    string         `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	InviteID              string         `gorm:"column:invite_id;type:uuid;not null;uniqueIndex:join_requests_invite_unique_idx"`
	CompanyID             string         `gorm:"column:company_id;type:uuid;not null;index:join_requests_company_status_type_created_idx"`
	RequestType           string         `gorm:"column:request_type;not null;index:join_requests_company_status_type_created_idx"`
	Status                string         `gorm:"column:status;not null;default:pending_approval;index:join_requests_company_status_type_created_idx"`
	RequestIP             string         `gorm:"column:request_ip;not null"`
	RequestingUserID      *string        `gorm:"column:requesting_user_id"`
	RequestEmailSnapshot  *string        `gorm:"column:request_email_snapshot"`
	AgentName             *string        `gorm:"column:agent_name"`
	AdapterType           *string        `gorm:"column:adapter_type"`
	Capabilities          *string        `gorm:"column:capabilities"`
	AgentDefaultsPayload  datatypes.JSON `gorm:"column:agent_defaults_payload;type:jsonb"`
	ClaimSecretHash       *string        `gorm:"column:claim_secret_hash"`
	ClaimSecretExpiresAt  *time.Time     `gorm:"column:claim_secret_expires_at;type:timestamptz"`
	ClaimSecretConsumedAt *time.Time     `gorm:"column:claim_secret_consumed_at;type:timestamptz"`
	CreatedAgentID        *string        `gorm:"column:created_agent_id;type:uuid"`
	ApprovedByUserID      *string        `gorm:"column:approved_by_user_id"`
	ApprovedAt            *time.Time     `gorm:"column:approved_at;type:timestamptz"`
	RejectedByUserID      *string        `gorm:"column:rejected_by_user_id"`
	RejectedAt            *time.Time     `gorm:"column:rejected_at;type:timestamptz"`
	CreatedAt             time.Time      `gorm:"column:created_at;type:timestamptz;not null;default:now();index:join_requests_company_status_type_created_idx"`
	UpdatedAt             time.Time      `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	Invite         Invite   `gorm:"foreignKey:InviteID"`
	Company        Company  `gorm:"foreignKey:CompanyID"`
	RequestingUser *User    `gorm:"foreignKey:RequestingUserID"`
	CreatedAgent   *Agent   `gorm:"foreignKey:CreatedAgentID"`
	ApprovedByUser *User    `gorm:"foreignKey:ApprovedByUserID"`
	RejectedByUser *User    `gorm:"foreignKey:RejectedByUserID"`
}

func (JoinRequest) TableName() string {
	return "join_requests"
}
