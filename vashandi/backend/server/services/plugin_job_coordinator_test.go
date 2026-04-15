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

type MockJobStore struct {
	SyncedChan  chan string
	DeletedChan chan string
}

func NewMockJobStore() *MockJobStore {
	return &MockJobStore{
		SyncedChan:  make(chan string, 10),
		DeletedChan: make(chan string, 10),
	}
}

func (m *MockJobStore) SyncJobDeclarations(ctx context.Context, pluginID string, jobs []map[string]interface{}) error {
	m.SyncedChan <- pluginID
	return nil
}

func (m *MockJobStore) DeleteAllJobs(ctx context.Context, pluginID string) error {
	m.DeletedChan <- pluginID
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
		status text NOT NULL,
		last_error text,
		created_at datetime DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)
	return db
}

func TestPluginJobCoordinator_Loaded(t *testing.T) {
	db := setupJobCoordinatorTestDB(t)
	lc := NewPluginLifecycleService(db)
	scheduler := NewMockScheduler()
	store := NewMockJobStore()
	coordinator := NewPluginJobCoordinator(db, lc, scheduler, store)

	db.Exec("INSERT INTO plugins (id, plugin_key, status) VALUES ('plugin-1', 'com.test.1', 'ready')")

	coordinator.Start()

	// Simulate the lifecycle emitting an event manually
	lc.emit("plugin.loaded", map[string]interface{}{
		"pluginId":  "plugin-1",
		"pluginKey": "com.test.1",
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
	lc := NewPluginLifecycleService(db)
	scheduler := NewMockScheduler()
	store := NewMockJobStore()
	coordinator := NewPluginJobCoordinator(db, lc, scheduler, store)

	coordinator.Start()

	lc.emit("plugin.disabled", map[string]interface{}{
		"pluginId":  "plugin-2",
		"pluginKey": "com.test.2",
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

func TestPluginJobCoordinator_Unloaded(t *testing.T) {
	db := setupJobCoordinatorTestDB(t)
	lc := NewPluginLifecycleService(db)
	scheduler := NewMockScheduler()
	store := NewMockJobStore()
	coordinator := NewPluginJobCoordinator(db, lc, scheduler, store)

	coordinator.Start()

	// 1. Without removeData
	lc.emit("plugin.unloaded", map[string]interface{}{
		"pluginId":   "plugin-3",
		"pluginKey":  "com.test.3",
		"removeData": false,
	})

	select {
	case unregistered := <-scheduler.UnregisteredChan:
		if unregistered != "plugin-3" {
			t.Errorf("expected plugin-3 to be unregistered, got %s", unregistered)
		}
	case <-time.After(1 * time.Second):
		t.Errorf("timeout waiting for plugin-3 unregistration")
	}

	// Make sure DeleteAllJobs isn't called
	select {
	case deleted := <-store.DeletedChan:
		t.Errorf("expected jobs not to be deleted without removeData, but got call for %s", deleted)
	default:
		// success
	}

	// 2. With removeData
	lc.emit("plugin.unloaded", map[string]interface{}{
		"pluginId":   "plugin-4",
		"pluginKey":  "com.test.4",
		"removeData": true,
	})

	select {
	case unregistered := <-scheduler.UnregisteredChan:
		if unregistered != "plugin-4" {
			t.Errorf("expected plugin-4 to be unregistered, got %s", unregistered)
		}
	case <-time.After(1 * time.Second):
		t.Errorf("timeout waiting for plugin-4 unregistration")
	}

	select {
	case deleted := <-store.DeletedChan:
		if deleted != "plugin-4" {
			t.Errorf("expected plugin-4 to be deleted, got %s", deleted)
		}
	case <-time.After(1 * time.Second):
		t.Errorf("timeout waiting for plugin-4 deletion")
	}
}
