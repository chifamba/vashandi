package services

import (
	"context"
	"encoding/json"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

/**
 * Plugin State Store — scoped key-value persistence for plugin workers.
 *
 * Provides Get, Set, Delete, and List operations over the plugin_state table.
 * Each plugin's data is strictly namespaced by pluginID so plugins cannot
 * read or write each other's state.
 *
 * The five-part composite key is: (pluginId, scopeKind, scopeId, namespace, stateKey)
 */
type PluginStateStore struct {
	DB *gorm.DB
}

func NewPluginStateStore(db *gorm.DB) *PluginStateStore {
	return &PluginStateStore{DB: db}
}

type PluginStateParams struct {
	ScopeKind string  `json:"scopeKind"`
	ScopeID   *string `json:"scopeId"`
	Namespace string  `json:"namespace"`
	StateKey  string  `json:"stateKey"`
}

func (s *PluginStateStore) Get(ctx context.Context, pluginID string, params PluginStateParams) (interface{}, error) {
	var state models.PluginState
	query := s.DB.WithContext(ctx).Where("plugin_id = ? AND scope_kind = ? AND namespace = ? AND state_key = ?", 
		pluginID, params.ScopeKind, params.Namespace, params.StateKey)
	
	if params.ScopeID == nil {
		query = query.Where("scope_id IS NULL")
	} else {
		query = query.Where("scope_id = ?", *params.ScopeID)
	}

	err := query.First(&state).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}

	var val interface{}
	if err := json.Unmarshal(state.ValueJSON, &val); err != nil {
		return nil, err
	}
	return val, nil
}

func (s *PluginStateStore) Set(ctx context.Context, pluginID string, params PluginStateParams, value interface{}) error {
	val, err := json.Marshal(value)
	if err != nil {
		return err
	}

	state := models.PluginState{
		PluginID:  pluginID,
		ScopeKind: params.ScopeKind,
		ScopeID:   params.ScopeID,
		Namespace: params.Namespace,
		StateKey:  params.StateKey,
		ValueJSON: datatypes.JSON(val),
		UpdatedAt: time.Now(),
	}

	// Use OnConflict to handle UPSERT across the five-part unique key.
	// This relies on the database having a unique index on these 5 columns.
	return s.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "plugin_id"},
			{Name: "scope_kind"},
			{Name: "scope_id"},
			{Name: "namespace"},
			{Name: "state_key"},
		},
		DoUpdates: clause.AssignmentColumns([]string{"value_json", "updated_at"}),
	}).Create(&state).Error
}

func (s *PluginStateStore) Delete(ctx context.Context, pluginID string, params PluginStateParams) error {
	query := s.DB.WithContext(ctx).Where("plugin_id = ? AND scope_kind = ? AND namespace = ? AND state_key = ?", 
		pluginID, params.ScopeKind, params.Namespace, params.StateKey)
	
	if params.ScopeID == nil {
		query = query.Where("scope_id IS NULL")
	} else {
		query = query.Where("scope_id = ?", *params.ScopeID)
	}

	return query.Delete(&models.PluginState{}).Error
}

type PluginStateFilter struct {
	ScopeKind *string `json:"scopeKind"`
	ScopeID   *string `json:"scopeId"`
	Namespace *string `json:"namespace"`
}

func (s *PluginStateStore) List(ctx context.Context, pluginID string, filter PluginStateFilter) ([]models.PluginState, error) {
	var states []models.PluginState
	query := s.DB.WithContext(ctx).Where("plugin_id = ?", pluginID)

	if filter.ScopeKind != nil {
		query = query.Where("scope_kind = ?", *filter.ScopeKind)
	}
	if filter.ScopeID != nil {
		query = query.Where("scope_id = ?", *filter.ScopeID)
	}
	if filter.Namespace != nil {
		query = query.Where("namespace = ?", *filter.Namespace)
	}

	err := query.Find(&states).Error
	return states, err
}

func (s *PluginStateStore) DeleteAll(ctx context.Context, pluginID string) error {
	return s.DB.WithContext(ctx).Where("plugin_id = ?", pluginID).Delete(&models.PluginState{}).Error
}
