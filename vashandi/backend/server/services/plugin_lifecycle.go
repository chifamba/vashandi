package services

import (
	"context"
	"fmt"
	"sync"
	"time"
	"github.com/google/uuid"
	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"gorm.io/gorm"
)

type PluginStatus string

const (
	PluginStatusPending        PluginStatus = "pending"
	PluginStatusInstalling     PluginStatus = "installing"
	PluginStatusReady          PluginStatus = "ready"
	PluginStatusDisabled       PluginStatus = "disabled"
	PluginStatusError          PluginStatus = "error"
	PluginStatusUninstalled    PluginStatus = "uninstalled"
	PluginStatusUpgradePending PluginStatus = "upgrade_pending"
)

var validTransitions = map[PluginStatus][]PluginStatus{
	PluginStatusPending:        {PluginStatusInstalling, PluginStatusError, PluginStatusUninstalled},
	PluginStatusInstalling:     {PluginStatusReady, PluginStatusError, PluginStatusUninstalled},
	PluginStatusReady:          {PluginStatusDisabled, PluginStatusError, PluginStatusUpgradePending, PluginStatusUninstalled},
	PluginStatusDisabled:       {PluginStatusReady, PluginStatusError, PluginStatusUninstalled},
	PluginStatusError:          {PluginStatusInstalling, PluginStatusReady, PluginStatusDisabled, PluginStatusUninstalled},
	PluginStatusUpgradePending: {PluginStatusInstalling, PluginStatusReady, PluginStatusDisabled, PluginStatusError, PluginStatusUninstalled},
	PluginStatusUninstalled:    {PluginStatusPending}, // Can reinstall
}

func isValidTransition(from, to PluginStatus) bool {
	if from == to {
		return true // Self-transitions are no-ops
	}
	allowed, ok := validTransitions[from]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}

type PluginLifecycleService struct {
	DB            *gorm.DB
	WorkerManager *PluginWorkerManager
	EventBus      *PluginEventBus
	ToolRegistry  *PluginToolRegistry
	listeners     map[string][]func(interface{})
	mu            sync.RWMutex
}

func NewPluginLifecycleService(db *gorm.DB, wm *PluginWorkerManager, eb *PluginEventBus, tr *PluginToolRegistry) *PluginLifecycleService {
	return &PluginLifecycleService{
		DB:            db,
		WorkerManager: wm,
		EventBus:      eb,
		ToolRegistry:  tr,
		listeners:     make(map[string][]func(interface{})),
	}
}

func (s *PluginLifecycleService) emit(event string, payload interface{}) {
	s.mu.RLock()
	funcs := s.listeners[event]
	s.mu.RUnlock()

	for _, f := range funcs {
		go f(payload)
	}
}

func (s *PluginLifecycleService) transition(ctx context.Context, pluginID string, to PluginStatus, errorMsg *string) (*models.Plugin, error) {
	var plugin models.Plugin
	err := s.DB.WithContext(ctx).First(&plugin, "id = ?", pluginID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("plugin not found: %s", pluginID)
		}
		return nil, err
	}

	from := PluginStatus(plugin.Status)

	if !isValidTransition(from, to) {
		return nil, fmt.Errorf("invalid transition from %s to %s", from, to)
	}

	if from == to && (errorMsg == nil || (plugin.LastError != nil && *errorMsg == *plugin.LastError)) {
		return &plugin, nil // no-op
	}

	updates := map[string]interface{}{
		"status":     string(to),
		"updated_at": time.Now(),
	}

	if errorMsg != nil {
		updates["last_error"] = *errorMsg
	} else if to == PluginStatusReady || to == PluginStatusDisabled {
		updates["last_error"] = gorm.Expr("NULL")
	}

	err = s.DB.WithContext(ctx).Model(&plugin).Updates(updates).Error
	if err != nil {
		return nil, err
	}

	plugin.Status = string(to)
	if errorMsg != nil {
		plugin.LastError = errorMsg
	} else {
		plugin.LastError = nil
	}

	s.emit("plugin.status_changed", map[string]interface{}{
		"pluginId":       pluginID,
		"pluginKey":      plugin.PluginKey,
		"previousStatus": from,
		"newStatus":      to,
	})

	if s.EventBus != nil {
		s.EventBus.Publish(ctx, PluginEvent{
			EventID:    uuid.New().String(),
			EventType:  fmt.Sprintf("plugin.%s.status_changed", pluginID),
			OccurredAt: time.Now().UTC().Format(time.RFC3339),
			ActorType:  "system",
			ActorID:    "host",
			Payload: map[string]interface{}{
				"pluginId":       pluginID,
				"pluginKey":      plugin.PluginKey,
				"previousStatus": from,
				"newStatus":      to,
			},
		})
	}

	return &plugin, nil
}

func (s *PluginLifecycleService) Load(ctx context.Context, pluginID string) (*models.Plugin, error) {
	plugin, err := s.transition(ctx, pluginID, PluginStatusReady, nil)
	if err != nil {
		return nil, err
	}

	if s.WorkerManager != nil {
		_ = s.WorkerManager.StartWorker(ctx, pluginID)
	}

	if s.ToolRegistry != nil {
		s.ToolRegistry.RegisterPlugin(plugin.PluginKey, plugin.ManifestJSON, plugin.ID)
	}

	return plugin, nil
}

func (s *PluginLifecycleService) Disable(ctx context.Context, pluginID string) (*models.Plugin, error) {
	plugin, err := s.transition(ctx, pluginID, PluginStatusDisabled, nil)
	if err != nil {
		return nil, err
	}

	if s.WorkerManager != nil {
		_ = s.WorkerManager.StopWorker(ctx, pluginID)
	}

	if s.ToolRegistry != nil {
		s.ToolRegistry.UnregisterPlugin(plugin.PluginKey)
	}

	return plugin, nil
}

func (s *PluginLifecycleService) Unload(ctx context.Context, pluginID string, removeData bool) (*models.Plugin, error) {
	var plugin models.Plugin
	if err := s.DB.WithContext(ctx).First(&plugin, "id = ?", pluginID).Error; err != nil {
		return nil, err
	}

	if s.WorkerManager != nil {
		_ = s.WorkerManager.StopWorker(ctx, pluginID)
	}

	if s.ToolRegistry != nil {
		s.ToolRegistry.UnregisterPlugin(plugin.PluginKey)
	}

	if PluginStatus(plugin.Status) == PluginStatusUninstalled {
		if removeData {
			err := s.DB.WithContext(ctx).Delete(&plugin).Error
			if err != nil {
				return nil, err
			}
			return &plugin, nil
		}
		return nil, fmt.Errorf("plugin %s is already uninstalled", plugin.PluginKey)
	}

	result, err := s.transition(ctx, pluginID, PluginStatusUninstalled, nil)
	if err != nil {
		return nil, err
	}

	if removeData {
		err := s.DB.WithContext(ctx).Delete(&plugin).Error
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

func (s *PluginLifecycleService) MarkError(ctx context.Context, pluginID, errMsg string) (*models.Plugin, error) {
	if s.WorkerManager != nil {
		_ = s.WorkerManager.StopWorker(ctx, pluginID)
	}
	return s.transition(ctx, pluginID, PluginStatusError, &errMsg)
}

func (s *PluginLifecycleService) MarkUpgradePending(ctx context.Context, pluginID string) (*models.Plugin, error) {
	return s.transition(ctx, pluginID, PluginStatusUpgradePending, nil)
}

func (s *PluginLifecycleService) GetStatus(ctx context.Context, pluginID string) (*PluginStatus, error) {
	var plugin models.Plugin
	err := s.DB.WithContext(ctx).Select("status").First(&plugin, "id = ?", pluginID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	status := PluginStatus(plugin.Status)
	return &status, nil
}

func (s *PluginLifecycleService) CanTransition(ctx context.Context, pluginID string, to PluginStatus) (bool, error) {
	status, err := s.GetStatus(ctx, pluginID)
	if err != nil || status == nil {
		return false, err
	}
	return isValidTransition(*status, to), nil
}

func (s *PluginLifecycleService) On(event string, listener func(interface{})) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listeners[event] = append(s.listeners[event], listener)
}
