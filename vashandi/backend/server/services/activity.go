package services

import (
	"context"
	"encoding/json"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// ActivityService handles auditing and event logging
type ActivityService struct {
	db *gorm.DB
}

// NewActivityService creates a new ActivityService
func NewActivityService(db *gorm.DB) *ActivityService {
	return &ActivityService{db: db}
}

// LogEntry represents the data for a new activity log
type LogEntry struct {
	CompanyID  string
	ActorType  string // "user", "agent", "system"
	ActorID    string
	Action     string
	EntityType string
	EntityID   string
	AgentID    *string
	RunID      *string
	Details    map[string]interface{}
}

// Log writes a new entry to the activity log
func (s *ActivityService) Log(ctx context.Context, entry LogEntry) (*models.ActivityLog, error) {
	log := &models.ActivityLog{
		CompanyID:  entry.CompanyID,
		ActorType:  entry.ActorType,
		ActorID:    entry.ActorID,
		Action:     entry.Action,
		EntityType: entry.EntityType,
		EntityID:   entry.EntityID,
		AgentID:    entry.AgentID,
		RunID:      entry.RunID,
	}

	if entry.Details != nil {
		log.Details = datatypes.JSON(datatypes.JSON(mustMarshal(entry.Details)))
	}

	if err := s.db.WithContext(ctx).Create(log).Error; err != nil {
		return nil, err
	}

	return log, nil
}

// ActivityFilters represents filters for listing activity
type ActivityFilters struct {
	CompanyID  string
	AgentID    string
	EntityType string
	EntityID   string
	Limit      int
	Offset     int
}

// List retrieves activity log entries matching the filters
func (s *ActivityService) List(ctx context.Context, filters ActivityFilters) ([]models.ActivityLog, error) {
	var activities []models.ActivityLog
	query := s.db.WithContext(ctx).Where("company_id = ?", filters.CompanyID)

	if filters.AgentID != "" {
		query = query.Where("agent_id = ?", filters.AgentID)
	}
	if filters.EntityType != "" {
		query = query.Where("entity_type = ?", filters.EntityType)
	}
	if filters.EntityID != "" {
		query = query.Where("entity_id = ?", filters.EntityID)
	}

	limit := filters.Limit
	if limit <= 0 {
		limit = 50
	}

	err := query.Order("created_at DESC").
		Limit(limit).
		Offset(filters.Offset).
		Preload("Agent").
		Find(&activities).Error

	return activities, err
}

func mustMarshal(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}
