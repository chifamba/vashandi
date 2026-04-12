package models

import "time"

type Edge struct {
	ID           string    `gorm:"primaryKey" json:"id"`
	NamespaceID  string    `gorm:"index:idx_memory_edges_namespace_from_type,priority:1;index:idx_memory_edges_namespace_to_type,priority:1;not null" json:"namespaceId"`
	FromEntityID string    `gorm:"index:idx_memory_edges_namespace_from_type,priority:2;not null" json:"fromEntityId"`
	ToEntityID   string    `gorm:"index:idx_memory_edges_namespace_to_type,priority:2;not null" json:"toEntityId"`
	EdgeType     string    `gorm:"index:idx_memory_edges_namespace_from_type,priority:3;index:idx_memory_edges_namespace_to_type,priority:3;not null" json:"edgeType"`
	Weight       float64   `gorm:"not null;default:1" json:"weight"`
	Metadata     string    `gorm:"type:jsonb;not null;default:'{}'" json:"metadata"`
	CreatedAt    time.Time `json:"createdAt"`
}

func (Edge) TableName() string { return "memory_edges" }
