package services

import (
	"context"
	"encoding/json"
	"log/slog"
)

type PluginJobCoordinator struct {
	Store     *PluginJobStore
	Scheduler *PluginJobScheduler
	Registry  *PluginRegistryService
	Lifecycle *PluginLifecycleService
}

func NewPluginJobCoordinator(store *PluginJobStore, scheduler *PluginJobScheduler, registry *PluginRegistryService, lifecycle *PluginLifecycleService) *PluginJobCoordinator {
	return &PluginJobCoordinator{
		Store:     store,
		Scheduler: scheduler,
		Registry:  registry,
		Lifecycle: lifecycle,
	}
}

func (c *PluginJobCoordinator) Start() {
	c.Lifecycle.On("plugin.status_changed", c.handleStatusChanged)
	slog.Info("Plugin job coordinator started — listening to lifecycle events")
}

func (c *PluginJobCoordinator) handleStatusChanged(payload interface{}) {
	data, ok := payload.(map[string]interface{})
	if !ok {
		return
	}

	pluginID, _ := data["pluginId"].(string)
	pluginKey, _ := data["pluginKey"].(string)
	newStatus, _ := data["newStatus"].(PluginStatus)

	if pluginID == "" {
		return
	}

	ctx := context.Background()

	switch newStatus {
	case PluginStatusReady:
		slog.Info("Plugin ready — syncing jobs and registering with scheduler", "pluginId", pluginID, "pluginKey", pluginKey)
		c.syncAndRegister(ctx, pluginID)
	case PluginStatusDisabled, PluginStatusError:
		slog.Info("Plugin disabled/error — unregistering from scheduler", "pluginId", pluginID, "pluginKey", pluginKey)
		c.Scheduler.UnregisterPlugin(ctx, pluginID)
	case PluginStatusUninstalled:
		slog.Info("Plugin uninstalled — unregistering from scheduler", "pluginId", pluginID, "pluginKey", pluginKey)
		c.Scheduler.UnregisterPlugin(ctx, pluginID)
		// Note: Data purging is handled by the Unload method in LifecycleService if removeData is true.
		// However, the coordinator could also trigger store.DeleteAllJobs if we had the removeData flag here.
		// Since the payload doesn't have removeData, we'll assume it's handled or we can check the DB.
	}
}

func (c *PluginJobCoordinator) syncAndRegister(ctx context.Context, pluginID string) {
	// 1. Get the manifest from the registry.
	plugin, err := c.Registry.GetByID(ctx, pluginID)
	if err != nil || plugin == nil {
		slog.Error("Coordinator: failed to get plugin for job sync", "pluginId", pluginID, "error", err)
		return
	}

	// 2. Parse jobs from manifest.
	var manifest struct {
		Jobs []PluginJobDeclaration `json:"jobs"`
	}
	if err := json.Unmarshal(plugin.ManifestJSON, &manifest); err != nil {
		slog.Error("Coordinator: failed to parse manifest for jobs", "pluginId", pluginID, "error", err)
		return
	}

	// 3. Sync job declarations.
	if len(manifest.Jobs) > 0 {
		if err := c.Store.SyncJobDeclarations(ctx, pluginID, manifest.Jobs); err != nil {
			slog.Error("Coordinator: failed to sync job declarations", "pluginId", pluginID, "error", err)
		}
	}

	// 4. Register with scheduler.
	if err := c.Scheduler.RegisterPlugin(ctx, pluginID); err != nil {
		slog.Error("Coordinator: failed to register plugin with scheduler", "pluginId", pluginID, "error", err)
	}
}
