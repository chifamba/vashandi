package models

import "time"

type MemoryBinding struct {
	ID               string    `gorm:"primaryKey" json:"id"`
	CompanyID        string    `gorm:"index;not null" json:"company_id"`
	Key              string    `gorm:"type:text;not null" json:"key"`
	ProviderPluginID string    `gorm:"type:text;not null" json:"provider_plugin_id"`
	Config           string    `gorm:"type:jsonb;not null;default:'{}'" json:"config"`
	Enabled          bool      `gorm:"not null;default:true" json:"enabled"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (MemoryBinding) TableName() string { return "memory_bindings" }

type MemoryBindingTarget struct {
	ID         string    `gorm:"primaryKey" json:"id"`
	CompanyID  string    `gorm:"index;not null" json:"company_id"`
	BindingID  string    `gorm:"index;not null" json:"binding_id"`
	TargetType string    `gorm:"type:varchar(50);not null" json:"target_type"`
	TargetID   string    `gorm:"index;not null" json:"target_id"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (MemoryBindingTarget) TableName() string { return "memory_binding_targets" }

type MemoryOperation struct {
	ID            string    `gorm:"primaryKey" json:"id"`
	CompanyID     string    `gorm:"index;not null" json:"company_id"`
	BindingID     string    `gorm:"index;not null" json:"binding_id"`
	OperationType string    `gorm:"type:varchar(50);not null" json:"operation_type"`
	Scope         string    `gorm:"type:jsonb;not null;default:'{}'" json:"scope"`
	SourceRef     string    `gorm:"type:jsonb;not null;default:'{}'" json:"source_ref"`
	Usage         string    `gorm:"type:jsonb;not null;default:'{}'" json:"usage"`
	Success       bool      `gorm:"not null" json:"success"`
	Error         *string   `gorm:"type:text" json:"error"`
	CreatedAt     time.Time `json:"created_at"`
}

func (MemoryOperation) TableName() string { return "memory_operations" }
