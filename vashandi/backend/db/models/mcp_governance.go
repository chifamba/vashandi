package models

import "time"

type MCPToolDefinition struct {
	ID          string    `gorm:"primaryKey" json:"id"`
	CompanyID   string    `gorm:"index;not null" json:"company_id"`
	Name        string    `gorm:"type:text;not null" json:"name"`
	Description string    `gorm:"type:text" json:"description"`
	SchemaJSON  string    `gorm:"type:jsonb;not null;default:'{}'" json:"schema_json"`
	Source      string    `gorm:"type:varchar(50);not null" json:"source"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (MCPToolDefinition) TableName() string { return "mcp_tool_definitions" }

type MCPEntitlementProfile struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	CompanyID string    `gorm:"index;not null" json:"company_id"`
	Name      string    `gorm:"type:text;not null" json:"name"`
	ToolIDs   string    `gorm:"type:text[];not null" json:"tool_ids"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (MCPEntitlementProfile) TableName() string { return "mcp_entitlement_profiles" }

type AgentMCPEntitlement struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	CompanyID string    `gorm:"index;not null" json:"company_id"`
	AgentID   string    `gorm:"index;not null" json:"agent_id"`
	ProfileID string    `gorm:"index;not null" json:"profile_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (AgentMCPEntitlement) TableName() string { return "agent_mcp_entitlements" }
