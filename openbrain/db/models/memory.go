package models

import (
	"time"

	"github.com/pgvector/pgvector-go"
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
	ID          string          `gorm:"primaryKey"`
	NamespaceID string          `gorm:"index;not null"`
	Text        string          `gorm:"type:text"`
	Metadata    string          `gorm:"type:jsonb"` // Store as JSONB in postgres
	Embedding   pgvector.Vector `gorm:"type:vector(1536)"` // OpenBrain default is text-embedding-3-small (1536d)
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt  `gorm:"index"`
}
