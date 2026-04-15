package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"gorm.io/datatypes"
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

// Resolve finds a plugin by UUID or plugin key. It tries UUID first, then
// falls back to key lookup. This avoids depending on UUID format validation
// so that both production UUIDs and test fixtures work.
func (s *PluginRegistryService) Resolve(ctx context.Context, pluginID string) (*models.Plugin, error) {
	p, err := s.GetByID(ctx, pluginID)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return p, nil
	}
	return s.GetByKey(ctx, pluginID)
}

// ListInstalled returns all plugins that have not been uninstalled.
func (s *PluginRegistryService) ListInstalled(ctx context.Context) ([]models.Plugin, error) {
	var plugins []models.Plugin
	err := s.DB.WithContext(ctx).
		Where("status != ?", "uninstalled").
		Order("install_order ASC, installed_at DESC").
		Find(&plugins).Error
	return plugins, err
}

// ListByStatus returns plugins matching the given status string.
func (s *PluginRegistryService) ListByStatus(ctx context.Context, status string) ([]models.Plugin, error) {
	var plugins []models.Plugin
	err := s.DB.WithContext(ctx).
		Where("status = ?", status).
		Order("install_order ASC, installed_at DESC").
		Find(&plugins).Error
	return plugins, err
}

type RegisterPluginInput struct {
	PluginKey   string
	PackageName string
	PackagePath *string
	Version     string
	ManifestRaw json.RawMessage
}

// Register creates a new plugin record in the database with status "pending".
// If a plugin with the same key already exists (even if uninstalled), it
// updates the record and transitions the status to "pending".
func (s *PluginRegistryService) Register(ctx context.Context, input RegisterPluginInput) (*models.Plugin, error) {
	existing, err := s.GetByKey(ctx, input.PluginKey)
	if err != nil {
		return nil, err
	}

	if existing != nil {
		updates := map[string]interface{}{
			"version":      input.Version,
			"manifest_json": input.ManifestRaw,
			"status":       "pending",
			"updated_at":   time.Now(),
		}
		if input.PackageName != "" {
			updates["package_name"] = input.PackageName
		}
		if input.PackagePath != nil {
			updates["package_path"] = *input.PackagePath
		}
		err = s.DB.WithContext(ctx).Model(existing).Updates(updates).Error
		if err != nil {
			return nil, err
		}
		return s.GetByID(ctx, existing.ID)
	}

	plugin := models.Plugin{
		PluginKey:    input.PluginKey,
		PackageName:  input.PackageName,
		PackagePath:  input.PackagePath,
		Version:      input.Version,
		ManifestJSON: datatypes.JSON(input.ManifestRaw),
		Status:       "pending",
	}

	err = s.DB.WithContext(ctx).Create(&plugin).Error
	if err != nil {
		return nil, err
	}
	return &plugin, nil
}

// Uninstall transitions a plugin to "uninstalled" or hard-deletes it if removeData is true.
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
		if err := s.DB.WithContext(ctx).Delete(&plugin).Error; err != nil {
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

// GetConfig retrieves the instance config for a plugin, or nil if not set.
func (s *PluginRegistryService) GetConfig(ctx context.Context, pluginID string) (*models.PluginConfig, error) {
	var config models.PluginConfig
	err := s.DB.WithContext(ctx).First(&config, "plugin_id = ?", pluginID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &config, nil
}

type UpsertConfigInput struct {
	ConfigJSON map[string]interface{}
}

// UpsertConfig creates or replaces the instance config for a plugin.
func (s *PluginRegistryService) UpsertConfig(ctx context.Context, pluginID string, input UpsertConfigInput) (*models.PluginConfig, error) {
	raw, err := json.Marshal(input.ConfigJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	existing, err := s.GetConfig(ctx, pluginID)
	if err != nil {
		return nil, err
	}

	if existing != nil {
		err = s.DB.WithContext(ctx).
			Model(&models.PluginConfig{}).
			Where("plugin_id = ?", pluginID).
			Updates(map[string]interface{}{
				"config_json": string(raw),
				"updated_at":  time.Now(),
			}).Error
		if err != nil {
			return nil, err
		}
		existing.ConfigJSON = datatypes.JSON(raw)
		existing.UpdatedAt = time.Now()
		return existing, nil
	}

	config := models.PluginConfig{
		PluginID:   pluginID,
		ConfigJSON: datatypes.JSON(raw),
	}
	if err := s.DB.WithContext(ctx).Create(&config).Error; err != nil {
		return nil, err
	}
	return &config, nil
}

type ListLogsInput struct {
	Limit int
	Level string
	Since *time.Time
}

// ListLogs queries recent log entries for a plugin, newest first.
func (s *PluginRegistryService) ListLogs(ctx context.Context, pluginID string, input ListLogsInput) ([]models.PluginLog, error) {
	limit := input.Limit
	if limit <= 0 {
		limit = 25
	}
	if limit > 500 {
		limit = 500
	}

	q := s.DB.WithContext(ctx).Where("plugin_id = ?", pluginID)
	if input.Level != "" {
		q = q.Where("level = ?", input.Level)
	}
	if input.Since != nil {
		q = q.Where("created_at >= ?", *input.Since)
	}

	var logs []models.PluginLog
	err := q.Order("created_at DESC").Limit(limit).Find(&logs).Error
	return logs, err
}

// ListJobs returns scheduled jobs for a plugin, optionally filtered by status.
func (s *PluginRegistryService) ListJobs(ctx context.Context, pluginID string, status string) ([]models.PluginJob, error) {
	q := s.DB.WithContext(ctx).Where("plugin_id = ?", pluginID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var jobs []models.PluginJob
	err := q.Order("created_at ASC").Find(&jobs).Error
	return jobs, err
}

// GetJobByIDForPlugin retrieves a job by ID, verifying it belongs to the given plugin.
func (s *PluginRegistryService) GetJobByIDForPlugin(ctx context.Context, pluginID, jobID string) (*models.PluginJob, error) {
	var job models.PluginJob
	err := s.DB.WithContext(ctx).
		First(&job, "id = ? AND plugin_id = ?", jobID, pluginID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &job, nil
}

// ListJobRuns returns execution history for a job, newest first.
func (s *PluginRegistryService) ListJobRuns(ctx context.Context, jobID string, limit int) ([]models.PluginJobRun, error) {
	if limit <= 0 {
		limit = 25
	}
	if limit > 500 {
		limit = 500
	}
	var runs []models.PluginJobRun
	err := s.DB.WithContext(ctx).
		Where("job_id = ?", jobID).
		Order("created_at DESC").
		Limit(limit).
		Find(&runs).Error
	return runs, err
}

// ListJobRunsByPlugin returns recent job runs across all jobs for a plugin.
func (s *PluginRegistryService) ListJobRunsByPlugin(ctx context.Context, pluginID string, limit int) ([]models.PluginJobRun, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 500 {
		limit = 500
	}
	var runs []models.PluginJobRun
	err := s.DB.WithContext(ctx).
		Where("plugin_id = ?", pluginID).
		Order("created_at DESC").
		Limit(limit).
		Find(&runs).Error
	return runs, err
}

// ListWebhookDeliveries returns recent webhook deliveries for a plugin, newest first.
func (s *PluginRegistryService) ListWebhookDeliveries(ctx context.Context, pluginID string, limit int) ([]models.PluginWebhookDelivery, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 500 {
		limit = 500
	}
	var deliveries []models.PluginWebhookDelivery
	err := s.DB.WithContext(ctx).
		Where("plugin_id = ?", pluginID).
		Order("created_at DESC").
		Limit(limit).
		Find(&deliveries).Error
	return deliveries, err
}

// RecordWebhookDelivery inserts a new delivery record and returns its ID.
func (s *PluginRegistryService) RecordWebhookDelivery(ctx context.Context, pluginID, webhookKey string, payload, headers json.RawMessage) (string, error) {
	delivery := models.PluginWebhookDelivery{
		PluginID:   pluginID,
		WebhookKey: webhookKey,
		Status:     "pending",
		Payload:    datatypes.JSON(payload),
		Headers:    datatypes.JSON(headers),
		StartedAt:  ptrTime(time.Now()),
	}
	if err := s.DB.WithContext(ctx).Create(&delivery).Error; err != nil {
		return "", err
	}
	return delivery.ID, nil
}

// UpdateWebhookDelivery updates status and timing on an existing delivery record.
func (s *PluginRegistryService) UpdateWebhookDelivery(ctx context.Context, id, status string, durationMs int, errMsg *string) error {
	now := time.Now()
	updates := map[string]interface{}{
		"status":      status,
		"duration_ms": durationMs,
		"finished_at": now,
	}
	if errMsg != nil {
		updates["error"] = *errMsg
	}
	return s.DB.WithContext(ctx).Model(&models.PluginWebhookDelivery{}).
		Where("id = ?", id).
		Updates(updates).Error
}

func ptrTime(t time.Time) *time.Time { return &t }
