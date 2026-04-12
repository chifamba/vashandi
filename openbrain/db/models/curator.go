package models

import "time"

type Proposal struct {
	ID            string     `gorm:"primaryKey" json:"id"`
	NamespaceID   string     `gorm:"index;not null" json:"namespaceId"`
	ProposalType  string     `gorm:"not null" json:"proposalType"`
	MemoryIDs     string     `gorm:"type:jsonb;not null;default:'[]'" json:"memoryIds"`
	Summary       string     `gorm:"type:text;not null" json:"summary"`
	SuggestedText string     `gorm:"type:text;not null" json:"suggestedText"`
	SuggestedTier *int       `json:"suggestedTier,omitempty"`
	Status        string     `gorm:"not null;default:'pending'" json:"status"`
	Details       string     `gorm:"type:jsonb;not null;default:'{}'" json:"details"`
	ReviewedBy    string     `json:"reviewedBy,omitempty"`
	ReviewedAt    *time.Time `json:"reviewedAt,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

func (Proposal) TableName() string { return "curator_proposals" }

type AuditLog struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	NamespaceID string    `gorm:"index;not null" json:"namespaceId"`
	AgentID     string    `gorm:"index" json:"agentId,omitempty"`
	ActorKind   string    `gorm:"not null" json:"actorKind"`
	Action      string    `gorm:"not null" json:"action"`
	EntityID    string    `gorm:"index" json:"entityId,omitempty"`
	EntityType  string    `json:"entityType,omitempty"`
	BeforeHash  string    `json:"beforeHash,omitempty"`
	AfterHash   string    `json:"afterHash,omitempty"`
	ChainHash   string    `json:"chainHash,omitempty"`
	RequestMeta string    `gorm:"type:jsonb;not null;default:'{}'" json:"requestMeta"`
	CreatedAt   time.Time `json:"createdAt"`
}

func (AuditLog) TableName() string { return "memory_audit_log" }
