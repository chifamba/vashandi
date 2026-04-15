package services

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"gorm.io/gorm"
)

type PluginJobScheduler interface {
	RegisterPlugin(ctx context.Context, pluginID string) error
	UnregisterPlugin(ctx context.Context, pluginID string) error
}

type PluginJobStore interface {
	SyncJobDeclarations(ctx context.Context, pluginID string, jobs []map[string]interface{}) error
	DeleteAllJobs(ctx context.Context, pluginID string) error
}

type PluginJobCoordinator struct {
	DB        *gorm.DB
	Lifecycle *PluginLifecycleService
	Scheduler PluginJobScheduler
	JobStore  PluginJobStore
	attached  bool
}

func NewPluginJobCoordinator(db *gorm.DB, lifecycle *PluginLifecycleService, scheduler PluginJobScheduler, jobStore PluginJobStore) *PluginJobCoordinator {
	return &PluginJobCoordinator{
		DB:        db,
		Lifecycle: lifecycle,
		Scheduler: scheduler,
		JobStore:  jobStore,
	}
}

func (c *PluginJobCoordinator) onPluginLoaded(payload interface{}) {
	data, ok := payload.(map[string]interface{})
	if !ok {
		return
	}

	pluginID, _ := data["pluginId"].(string)
	pluginKey, _ := data["pluginKey"].(string)

	ctx := context.Background()
	slog.Info("plugin loaded — syncing jobs and registering with scheduler", "pluginId", pluginID, "pluginKey", pluginKey)

	var plugin models.Plugin
	err := c.DB.WithContext(ctx).First(&plugin, "id = ?", pluginID).Error
	if err != nil {
		slog.Warn("plugin loaded but no manifest found — skipping job sync", "pluginId", pluginID)
		return
	}

	// Assuming ManifestJson is a map or can be unmarshaled
	var manifest map[string]interface{}
	if plugin.ManifestJSON != nil {
		// Mock logic since we haven't fully ported the manifest JSON parsing
		// into the DB model yet.
		// manifest = *plugin.ManifestJSON
	}

	var jobDeclarations []map[string]interface{}
	if jobs, ok := manifest["jobs"].([]interface{}); ok {
		for _, j := range jobs {
			if jobMap, ok := j.(map[string]interface{}); ok {
				jobDeclarations = append(jobDeclarations, jobMap)
			}
		}
	}

	if len(jobDeclarations) > 0 {
		slog.Info("syncing job declarations from manifest", "pluginId", pluginID, "jobCount", len(jobDeclarations))
		err := c.JobStore.SyncJobDeclarations(ctx, pluginID, jobDeclarations)
		if err != nil {
			slog.Error("failed to sync jobs", "err", err, "pluginId", pluginID)
		}
	}

	err = c.Scheduler.RegisterPlugin(ctx, pluginID)
	if err != nil {
		slog.Error("failed to register plugin with scheduler", "err", err, "pluginId", pluginID)
	}
}

func (c *PluginJobCoordinator) onPluginDisabled(payload interface{}) {
	data, ok := payload.(map[string]interface{})
	if !ok {
		return
	}

	pluginID, _ := data["pluginId"].(string)
	pluginKey, _ := data["pluginKey"].(string)
	reason, _ := data["reason"].(string)

	ctx := context.Background()
	slog.Info("plugin disabled — unregistering from scheduler", "pluginId", pluginID, "pluginKey", pluginKey, "reason", reason)

	err := c.Scheduler.UnregisterPlugin(ctx, pluginID)
	if err != nil {
		slog.Error("failed to unregister plugin from scheduler", "err", err, "pluginId", pluginID)
	}
}

func (c *PluginJobCoordinator) onPluginUnloaded(payload interface{}) {
	data, ok := payload.(map[string]interface{})
	if !ok {
		return
	}

	pluginID, _ := data["pluginId"].(string)
	pluginKey, _ := data["pluginKey"].(string)
	removeData, _ := data["removeData"].(bool)

	ctx := context.Background()
	slog.Info("plugin unloaded — unregistering from scheduler", "pluginId", pluginID, "pluginKey", pluginKey, "removeData", removeData)

	err := c.Scheduler.UnregisterPlugin(ctx, pluginID)
	if err != nil {
		slog.Error("failed to unregister plugin from scheduler during unload", "err", err, "pluginId", pluginID)
	}

	if removeData {
		slog.Info("purging job data for uninstalled plugin", "pluginId", pluginID)
		err := c.JobStore.DeleteAllJobs(ctx, pluginID)
		if err != nil {
			slog.Error("failed to purge job data", "err", err, "pluginId", pluginID)
		}
	}
}

func (c *PluginJobCoordinator) Start() {
	if c.attached {
		return
	}
	c.attached = true

	c.Lifecycle.On("plugin.loaded", c.onPluginLoaded)
	c.Lifecycle.On("plugin.disabled", c.onPluginDisabled)
	c.Lifecycle.On("plugin.unloaded", c.onPluginUnloaded)

	fmt.Println("plugin job coordinator started — listening to lifecycle events")
}

func (c *PluginJobCoordinator) Stop() {
	if !c.attached {
		return
	}
	c.attached = false
	// Normally we would have Off methods on Lifecycle to remove these specific listeners
	fmt.Println("plugin job coordinator stopped")
}
