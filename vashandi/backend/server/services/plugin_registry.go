package services

import (
	"context"
	"fmt"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"gorm.io/gorm"
)

type PluginRegistryService struct {
	DB *gorm.DB
}

func NewPluginRegistryService(db *gorm.DB) *PluginRegistryService {
	return &PluginRegistryService{DB: db}
}

// GetByID finds a plugin by its UUID.
func (s *PluginRegistryService) GetByID(ctx context.Context, id string) (*models.Plugin, error) {
	var plugin models.Plugin
	err := s.DB.WithContext(ctx).First(&plugin, "id = ?", id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &plugin, nil
}

// GetByKey finds a plugin by its pluginKey.
func (s *PluginRegistryService) GetByKey(ctx context.Context, key string) (*models.Plugin, error) {
	var plugin models.Plugin
	err := s.DB.WithContext(ctx).First(&plugin, "plugin_key = ?", key).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &plugin, nil
}

// List returns a list of installed plugins, optionally including uninstalled ones.
func (s *PluginRegistryService) List(ctx context.Context, includeUninstalled bool) ([]models.Plugin, error) {
	var plugins []models.Plugin
	query := s.DB.WithContext(ctx)
	if !includeUninstalled {
		query = query.Where("status != ?", "uninstalled")
	}
	err := query.Order("install_order ASC, installed_at DESC").Find(&plugins).Error
	return plugins, err
}

type InstallPluginInput struct {
	PluginKey    string                 `json:"pluginKey"`
	Version      string                 `json:"version"`
	ManifestJSON map[string]interface{} `json:"manifestJson"`
	ConfigJSON   map[string]interface{} `json:"configJson,omitempty"`
}

// Install registers a new plugin.
func (s *PluginRegistryService) Install(ctx context.Context, input InstallPluginInput) (*models.Plugin, error) {
	existing, err := s.GetByKey(ctx, input.PluginKey)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, fmt.Errorf("plugin with key '%s' is already installed", input.PluginKey)
	}

	plugin := models.Plugin{
		PluginKey: input.PluginKey,
		Version:   input.Version,
		Status:    "pending",
		// Assuming JSON marshaling works directly here if ManifestJSON is proper type in model
		// ManifestJSON: input.ManifestJSON,
	}

	err = s.DB.WithContext(ctx).Create(&plugin).Error
	if err != nil {
		return nil, err
	}

	if input.ConfigJSON != nil {
		config := models.PluginConfig{
			// PluginID is currently missing from models.PluginConfig
		}
		s.DB.WithContext(ctx).Create(&config)
	}

	return &plugin, nil
}

// Uninstall soft-deletes a plugin by changing its status to 'uninstalled' or hard-deletes it.
func (s *PluginRegistryService) Uninstall(ctx context.Context, id string, removeData bool) (*models.Plugin, error) {
	var plugin models.Plugin
	err := s.DB.WithContext(ctx).First(&plugin, "id = ?", id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}

	if removeData {
		err = s.DB.WithContext(ctx).Delete(&plugin).Error
		if err != nil {
			return nil, err
		}
		return &plugin, nil
	}

	err = s.DB.WithContext(ctx).Model(&plugin).Updates(map[string]interface{}{
		"status":     "uninstalled",
		"updated_at": time.Now(),
	}).Error

	if err != nil {
		return nil, err
	}

	plugin.Status = "uninstalled"
	return &plugin, nil
}

// Map the DB access via generic interfaces or Raw SQL if the GORM model isn't complete yet
type PluginConfigMock struct {
	ID        string
	PluginID  string
}

func (s *PluginRegistryService) GetConfig(ctx context.Context, pluginID string) (*PluginConfigMock, error) {
	var config PluginConfigMock
	err := s.DB.WithContext(ctx).Table("plugin_configs").First(&config, "plugin_id = ?", pluginID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &config, nil
}

func (s *PluginRegistryService) SetConfig(ctx context.Context, pluginID string, configJSON map[string]interface{}) (*PluginConfigMock, error) {
	config, err := s.GetConfig(ctx, pluginID)
	if err != nil {
		return nil, err
	}

	if config != nil {
		err = s.DB.WithContext(ctx).Table("plugin_configs").Where("plugin_id = ?", pluginID).Updates(map[string]interface{}{
			"updated_at": time.Now(),
		}).Error
		return config, err
	}

	newConfig := PluginConfigMock{
		PluginID: pluginID,
	}
	err = s.DB.WithContext(ctx).Table("plugin_configs").Create(&newConfig).Error
	return &newConfig, err
}

func (s *PluginRegistryService) DeleteConfig(ctx context.Context, pluginID string) (*PluginConfigMock, error) {
	config, err := s.GetConfig(ctx, pluginID)
	if err != nil || config == nil {
		return nil, err
	}

	err = s.DB.WithContext(ctx).Table("plugin_configs").Where("plugin_id = ?", pluginID).Delete(config).Error
	return config, err
}
