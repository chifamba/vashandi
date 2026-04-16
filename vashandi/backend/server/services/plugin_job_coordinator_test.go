package services

import (
	"context"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type MockScheduler struct {
	RegisteredChan   chan string
	UnregisteredChan chan string
}

func NewMockScheduler() *MockScheduler {
	return &MockScheduler{
		RegisteredChan:   make(chan string, 10),
		UnregisteredChan: make(chan string, 10),
	}
}

func (s *MockScheduler) RegisterPlugin(ctx context.Context, pluginID string) error {
	s.RegisteredChan <- pluginID
	return nil
}

func (s *MockScheduler) UnregisterPlugin(ctx context.Context, pluginID string) error {
	s.UnregisteredChan <- pluginID
	return nil
}

func setupJobCoordinatorTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&svc_pjc=1"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	db.Exec("DROP TABLE IF EXISTS plugins")

	db.Exec(`CREATE TABLE plugins (
		id text PRIMARY KEY,
		plugin_key text NOT NULL UNIQUE,
		package_name text NOT NULL DEFAULT '',
		version text NOT NULL DEFAULT '0.0.0',
		api_version integer NOT NULL DEFAULT 1,
		categories text NOT NULL DEFAULT '[]',
		manifest_json text NOT NULL DEFAULT '{}',
		status text NOT NULL,
		last_error text,
		created_at datetime DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)
	return db
}

// setupCoordinator creates a PluginJobCoordinator with real services wired to the
// provided DB and a mock scheduler for assertion.
func setupCoordinator(db *gorm.DB, scheduler PluginJobSchedulerIface) *PluginJobCoordinator {
	lc := NewPluginLifecycleService(db, nil, nil, nil)
	registry := NewPluginRegistryService(db)
	store := NewPluginJobStore(db)
	return NewPluginJobCoordinator(store, scheduler, registry, lc)
}

func TestPluginJobCoordinator_Ready(t *testing.T) {
	db := setupJobCoordinatorTestDB(t)
	scheduler := NewMockScheduler()
	coordinator := setupCoordinator(db, scheduler)

	db.Exec("INSERT INTO plugins (id, plugin_key, package_name, version, manifest_json, status) VALUES ('plugin-1', 'com.test.1', 'test', '1.0.0', '{}', 'ready')")

	coordinator.Start()

	// Emit plugin.status_changed with newStatus = PluginStatusReady.
	coordinator.Lifecycle.emit("plugin.status_changed", map[string]interface{}{
		"pluginId":  "plugin-1",
		"pluginKey": "com.test.1",
		"newStatus": PluginStatusReady,
	})

	select {
	case registered := <-scheduler.RegisteredChan:
		if registered != "plugin-1" {
			t.Errorf("expected plugin-1 to be registered, got %s", registered)
		}
	case <-time.After(1 * time.Second):
		t.Errorf("timeout waiting for plugin registration")
	}
}

func TestPluginJobCoordinator_Disabled(t *testing.T) {
	db := setupJobCoordinatorTestDB(t)
	scheduler := NewMockScheduler()
	coordinator := setupCoordinator(db, scheduler)

	coordinator.Start()

	coordinator.Lifecycle.emit("plugin.status_changed", map[string]interface{}{
		"pluginId":  "plugin-2",
		"pluginKey": "com.test.2",
		"newStatus": PluginStatusDisabled,
	})

	select {
	case unregistered := <-scheduler.UnregisteredChan:
		if unregistered != "plugin-2" {
			t.Errorf("expected plugin-2 to be unregistered, got %s", unregistered)
		}
	case <-time.After(1 * time.Second):
		t.Errorf("timeout waiting for plugin unregistration")
	}
}

func TestPluginJobCoordinator_Uninstalled(t *testing.T) {
	db := setupJobCoordinatorTestDB(t)
	scheduler := NewMockScheduler()
	coordinator := setupCoordinator(db, scheduler)

	coordinator.Start()

	coordinator.Lifecycle.emit("plugin.status_changed", map[string]interface{}{
		"pluginId":  "plugin-3",
		"pluginKey": "com.test.3",
		"newStatus": PluginStatusUninstalled,
	})

	select {
	case unregistered := <-scheduler.UnregisteredChan:
		if unregistered != "plugin-3" {
			t.Errorf("expected plugin-3 to be unregistered, got %s", unregistered)
		}
	case <-time.After(1 * time.Second):
		t.Errorf("timeout waiting for plugin-3 unregistration")
	}
}
