package services

import (
	"context"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"github.com/chifamba/vashandi/vashandi/backend/db/models"
)

func setupPluginLifecycleTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&svc_plugin_lifecycle=1"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	db.Exec("DROP TABLE IF EXISTS plugins")

	db.Exec(`CREATE TABLE plugins (
		id text PRIMARY KEY,
		plugin_key text NOT NULL UNIQUE,
		status text NOT NULL DEFAULT 'pending',
		version text,
		hash text,
		last_error text,
		manifest_json json,
		created_at datetime DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)

	return db
}

func TestPluginLifecycleService_LoadDisable(t *testing.T) {
	db := setupPluginLifecycleTestDB(t)
	svc := NewPluginLifecycleService(db)
	ctx := context.Background()

	now := time.Now()
	db.Exec("INSERT INTO plugins (id, plugin_key, status, created_at, updated_at) VALUES ('plugin-1', 'com.example.plugin', 'installing', ?, ?)", now, now)

	// 1. Load (installing -> ready)
	loaded, err := svc.Load(ctx, "plugin-1")
	if err != nil || loaded == nil {
		t.Fatalf("unexpected error loading: %v", err)
	}
	if loaded.Status != "ready" {
		t.Errorf("expected status 'ready', got %s", loaded.Status)
	}

	// 2. Disable (ready -> disabled)
	disabled, err := svc.Disable(ctx, "plugin-1")
	if err != nil || disabled == nil {
		t.Fatalf("unexpected error disabling: %v", err)
	}
	if disabled.Status != "disabled" {
		t.Errorf("expected status 'disabled', got %s", disabled.Status)
	}
}

func TestPluginLifecycleService_ErrorsAndUpgrades(t *testing.T) {
	db := setupPluginLifecycleTestDB(t)
	svc := NewPluginLifecycleService(db)
	ctx := context.Background()

	now := time.Now()
	db.Exec("INSERT INTO plugins (id, plugin_key, status, created_at, updated_at) VALUES ('plugin-2', 'com.example.plugin2', 'ready', ?, ?)", now, now)

	// 1. Mark Error (ready -> error)
	errPlg, err := svc.MarkError(ctx, "plugin-2", "crash detected")
	if err != nil || errPlg == nil {
		t.Fatalf("unexpected error marking error: %v", err)
	}
	if errPlg.Status != "error" || errPlg.LastError == nil || *errPlg.LastError != "crash detected" {
		t.Errorf("invalid state: %s, %v", errPlg.Status, errPlg.LastError)
	}

	// 2. Upgrade Pending (error -> ready -> upgrade_pending)
	_, err = svc.Load(ctx, "plugin-2") // error -> ready
	if err != nil {
		t.Fatalf("unexpected error loading: %v", err)
	}

	upgPlg, err := svc.MarkUpgradePending(ctx, "plugin-2") // ready -> upgrade_pending
	if err != nil || upgPlg == nil {
		t.Fatalf("unexpected error marking upgrade pending: %v", err)
	}
	if upgPlg.Status != "upgrade_pending" {
		t.Errorf("expected status 'upgrade_pending', got %s", upgPlg.Status)
	}
}

func TestPluginLifecycleService_Unload(t *testing.T) {
	db := setupPluginLifecycleTestDB(t)
	svc := NewPluginLifecycleService(db)
	ctx := context.Background()

	now := time.Now()
	db.Exec("INSERT INTO plugins (id, plugin_key, status, created_at, updated_at) VALUES ('plugin-3', 'com.example.plugin3', 'ready', ?, ?)", now, now)

	// 1. Unload without remove data
	unloaded, err := svc.Unload(ctx, "plugin-3", false)
	if err != nil || unloaded == nil {
		t.Fatalf("unexpected error unloading: %v", err)
	}
	if unloaded.Status != "uninstalled" {
		t.Errorf("expected status 'uninstalled', got %s", unloaded.Status)
	}

	// Verify not deleted
	var count int64
	db.Model(&models.Plugin{}).Count(&count)
	if count != 1 {
		t.Errorf("expected plugin to still exist")
	}

	// 2. Unload again without remove data (should error)
	_, err = svc.Unload(ctx, "plugin-3", false)
	if err == nil {
		t.Errorf("expected error unloading already uninstalled plugin")
	}

	// 3. Unload again WITH remove data (should delete)
	_, err = svc.Unload(ctx, "plugin-3", true)
	if err != nil {
		t.Fatalf("unexpected error removing data: %v", err)
	}

	db.Model(&models.Plugin{}).Count(&count)
	if count != 0 {
		t.Errorf("expected plugin to be deleted")
	}
}

func TestPluginLifecycleService_Events(t *testing.T) {
	db := setupPluginLifecycleTestDB(t)
	svc := NewPluginLifecycleService(db)
	ctx := context.Background()

	now := time.Now()
	db.Exec("INSERT INTO plugins (id, plugin_key, status, created_at, updated_at) VALUES ('plugin-4', 'com.example.plugin4', 'installing', ?, ?)", now, now)

	var wg sync.WaitGroup
	var receivedEvent map[string]interface{}
	wg.Add(1)

	svc.On("plugin.status_changed", func(payload interface{}) {
		receivedEvent = payload.(map[string]interface{})
		wg.Done()
	})

	_, err := svc.Load(ctx, "plugin-4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wg.Wait()
	if receivedEvent == nil {
		t.Fatalf("expected event to be received")
	}
	if receivedEvent["pluginId"] != "plugin-4" || receivedEvent["newStatus"] != PluginStatusReady {
		t.Errorf("invalid event payload: %v", receivedEvent)
	}
}
