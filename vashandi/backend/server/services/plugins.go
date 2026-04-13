package services

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"gorm.io/gorm"
)

type PluginService struct {
	DB *gorm.DB
}

func NewPluginService(db *gorm.DB) *PluginService {
	return &PluginService{DB: db}
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

// UpdatePluginStatus updates the operational status of a plugin.
func (s *PluginService) UpdatePluginStatus(ctx context.Context, pluginKey string, status string, lastError *string) error {
	return s.DB.WithContext(ctx).Model(&models.Plugin{}).
		Where("plugin_key = ?", pluginKey).
		Updates(map[string]interface{}{
			"status":     status,
			"last_error": lastError,
		}).Error
}
