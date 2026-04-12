package models

import (
	"time"

	"gorm.io/gorm"
)

type Proposal struct {
	ID            string         `gorm:"primaryKey"`
	NamespaceID   string         `gorm:"index;not null"`
	MemoryIDs     string         `gorm:"type:text;not null"` // JSON array of memory IDs
	SuggestedText string         `gorm:"type:text;not null"`
	Status        string         `gorm:"not null;default:'pending'"` // pending, approved, rejected
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     gorm.DeletedAt `gorm:"index"`

	Namespace Namespace `gorm:"foreignKey:NamespaceID;constraint:OnDelete:CASCADE"`
}
