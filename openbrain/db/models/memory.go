package models

import (
	"time"

	"gorm.io/gorm"
)

// Namespace represents a company scope for memories
type Namespace struct {
	ID        string         `gorm:"primaryKey"`
	CompanyID string         `gorm:"index;not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

// Memory represents a single context node
type Memory struct {
	ID          string         `gorm:"primaryKey"`
	NamespaceID string         `gorm:"index;not null"`
	Text        string         `gorm:"type:text"`
	Metadata    string         `gorm:"type:jsonb"` // Store as JSONB in postgres
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}
