package models

import "time"

type Namespace struct {
	ID         string     `gorm:"primaryKey" json:"id"`
	CompanyID  string     `gorm:"index;not null" json:"companyId"`
	TeamID     string     `gorm:"index" json:"teamId,omitempty"`
	Settings   string     `gorm:"type:jsonb;not null;default:'{}'" json:"settings"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
	ArchivedAt *time.Time `json:"archivedAt,omitempty"`
}

type Memory struct {
	ID             string     `gorm:"primaryKey" json:"id"`
	NamespaceID    string     `gorm:"index:idx_memory_namespace_type_tier,priority:1;index:idx_memory_namespace_updated,priority:1;not null" json:"namespaceId"`
	TeamID         string     `gorm:"index" json:"teamId,omitempty"`
	EntityType     string     `gorm:"index:idx_memory_namespace_type_tier,priority:2;not null" json:"entityType"`
	Title          string     `json:"title,omitempty"`
	Text           string     `gorm:"type:text;not null" json:"text"`
	Embedding      string     `gorm:"type:jsonb;not null;default:'[]'" json:"embedding"`
	Provenance     string     `gorm:"type:jsonb;not null;default:'{}'" json:"provenance"`
	Identity       string     `gorm:"type:jsonb;not null;default:'{}'" json:"identity"`
	Metadata       string     `gorm:"type:jsonb;not null;default:'{}'" json:"metadata"`
	Tier           int        `gorm:"index:idx_memory_namespace_type_tier,priority:3;not null;default:0" json:"tier"`
	Version        int        `gorm:"not null;default:1" json:"version"`
	IsDeleted      bool       `gorm:"index:idx_memory_namespace_updated,priority:2;not null;default:false" json:"isDeleted"`
	AccessCount    int        `gorm:"not null;default:0" json:"accessCount"`
	ManualPromote  bool       `gorm:"not null;default:false" json:"manualPromote"`
	LastAccessedAt *time.Time `json:"lastAccessedAt,omitempty"`
	DecayAt        *time.Time `json:"decayAt,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `gorm:"index:idx_memory_namespace_updated,priority:3" json:"updatedAt"`
}

func (Memory) TableName() string { return "memory_entities" }

type MemoryVersion struct {
	ID           string    `gorm:"primaryKey" json:"id"`
	NamespaceID  string    `gorm:"index:idx_memory_versions_namespace_entity_version,priority:1;not null" json:"namespaceId"`
	EntityID     string    `gorm:"index:idx_memory_versions_namespace_entity_version,priority:2;not null" json:"entityId"`
	Version      int       `gorm:"index:idx_memory_versions_namespace_entity_version,priority:3;not null" json:"version"`
	Title        string    `json:"title,omitempty"`
	Text         string    `gorm:"type:text;not null" json:"text"`
	Embedding    string    `gorm:"type:jsonb;not null;default:'[]'" json:"embedding"`
	Metadata     string    `gorm:"type:jsonb;not null;default:'{}'" json:"metadata"`
	Provenance   string    `gorm:"type:jsonb;not null;default:'{}'" json:"provenance"`
	Identity     string    `gorm:"type:jsonb;not null;default:'{}'" json:"identity"`
	Tier         int       `gorm:"not null;default:0" json:"tier"`
	ChangedBy    string    `gorm:"type:jsonb;not null;default:'{}'" json:"changedBy"`
	ChangeReason string    `json:"changeReason,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
}

func (MemoryVersion) TableName() string { return "memory_entity_versions" }

type RegisteredAgent struct {
	ID              string     `gorm:"primaryKey" json:"id"`
	NamespaceID     string     `gorm:"index:idx_registered_agents_namespace_agent,priority:1;not null" json:"namespaceId"`
	VashandiAgentID string     `gorm:"index:idx_registered_agents_namespace_agent,priority:2;not null" json:"vashandiAgentId"`
	Name            string     `gorm:"not null" json:"name"`
	TrustTier       int        `gorm:"not null;default:1" json:"trustTier"`
	RecallProfile   string     `gorm:"type:jsonb;not null;default:'{}'" json:"recallProfile"`
	IsActive        bool       `gorm:"not null;default:true" json:"isActive"`
	RegisteredAt    time.Time  `json:"registeredAt"`
	DeregisteredAt  *time.Time `json:"deregisteredAt,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

func (RegisteredAgent) TableName() string { return "registered_agents" }

type ContextPacket struct {
	ID          string     `gorm:"primaryKey" json:"id"`
	NamespaceID string     `gorm:"index:idx_context_packets_namespace_agent,priority:1;not null" json:"namespaceId"`
	AgentID     string     `gorm:"index:idx_context_packets_namespace_agent,priority:2;not null" json:"agentId"`
	TriggerType string     `gorm:"not null" json:"triggerType"`
	Payload     string     `gorm:"type:jsonb;not null;default:'{}'" json:"payload"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
	DeliveredAt *time.Time `json:"deliveredAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
}

func (ContextPacket) TableName() string { return "context_packets" }
