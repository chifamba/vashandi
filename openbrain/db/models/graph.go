package models

import (
	"time"

	"gorm.io/gorm"
)

// Edge represents an adjacency/relationship between two memories
type Edge struct {
	ID             string         `gorm:"primaryKey"`
	NamespaceID    string         `gorm:"index;not null"`
	SourceMemoryID string         `gorm:"index;not null"`
	TargetMemoryID string         `gorm:"index;not null"`
	Weight         float32        `gorm:"not null;default:1.0"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      gorm.DeletedAt `gorm:"index"`

	// Optional relations
	SourceMemory Memory `gorm:"foreignKey:SourceMemoryID;constraint:OnDelete:CASCADE"`
	TargetMemory Memory `gorm:"foreignKey:TargetMemoryID;constraint:OnDelete:CASCADE"`
	Namespace    Namespace `gorm:"foreignKey:NamespaceID;constraint:OnDelete:CASCADE"`
}
