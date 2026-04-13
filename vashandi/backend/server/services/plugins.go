package services

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"gorm.io/gorm"
)

type PluginService struct {
	DB       *gorm.DB
	Activity *ActivityService
}

func NewPluginService(db *gorm.DB, activity *ActivityService) *PluginService {
	return &PluginService{
		DB:       db,
		Activity: activity,
	}
}

// ListPlugins returns all installed plugins.
func (s *PluginService) ListPlugins(ctx context.Context) ([]models.Plugin, error) {
	var plugins []models.Plugin
	if err := s.DB.WithContext(ctx).Where("status = ?", "installed").Order("install_order ASC, installed_at DESC").Find(&plugins).Error; err != nil {
		return nil, fmt.Errorf("failed to list plugins: %w", err)
	}
	return plugins, nil
}

// GetPluginManifest retrieves and parses the manifest for a specific plugin.
func (s *PluginService) GetPluginManifest(ctx context.Context, pluginKey string) (map[string]interface{}, error) {
	var plugin models.Plugin
	if err := s.DB.WithContext(ctx).Where("plugin_key = ?", pluginKey).First(&plugin).Error; err != nil {
		return nil, fmt.Errorf("plugin not found: %w", err)
	}

	var manifest map[string]interface{}
	if err := json.Unmarshal(plugin.ManifestJSON, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest for %s: %w", pluginKey, err)
	}

	return manifest, nil
}

// UpdatePluginStatus updates the operational status of a plugin and logs the change.
func (s *PluginService) UpdatePluginStatus(ctx context.Context, pluginKey string, status string, lastError *string) error {
	err := s.DB.WithContext(ctx).Model(&models.Plugin{}).
		Where("plugin_key = ?", pluginKey).
		Updates(map[string]interface{}{
			"status":     status,
			"last_error": lastError,
		}).Error

	if err == nil && s.Activity != nil {
		_, _ = s.Activity.Log(ctx, LogEntry{
			CompanyID:  "system", // Plugins are often system-wide or belong to a specific context
			ActorType:  "system",
			ActorID:    "system",
			Action:     "plugin.status_updated",
			EntityType: "plugin",
			EntityID:   pluginKey,
			Details: map[string]interface{}{
				"status": status,
				"error":  lastError,
			},
		})
	}
	return err
}
