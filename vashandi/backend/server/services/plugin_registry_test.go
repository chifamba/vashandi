package services

import (
	"context"
	"fmt"
	"net/url"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupPluginRegistryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file::memory:?cache=shared&%s=1", url.QueryEscape(t.Name()))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	db.Exec("DROP TABLE IF EXISTS plugins")
	db.Exec("DROP TABLE IF EXISTS plugin_configs")

	db.Exec(`CREATE TABLE plugins (
		id text PRIMARY KEY,
		plugin_key text NOT NULL UNIQUE,
		package_name text NOT NULL DEFAULT '',
		status text NOT NULL DEFAULT 'pending',
		version text,
		api_version integer NOT NULL DEFAULT 1,
		categories text NOT NULL DEFAULT '[]',
		hash text,
		install_order integer,
		package_path text,
		last_error text,
		manifest_json text,
		installed_at datetime DEFAULT CURRENT_TIMESTAMP,
		created_at datetime DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)

	db.Exec(`CREATE TABLE plugin_configs (
		id text PRIMARY KEY,
		plugin_id text NOT NULL UNIQUE,
		config_json json,
		last_error text,
		created_at datetime DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(plugin_id) REFERENCES plugins(id) ON DELETE CASCADE
	)`)

	return db
}

func TestPluginRegistryService_ListGetInstallUninstall(t *testing.T) {
	db := setupPluginRegistryTestDB(t)
	svc := NewPluginRegistryService(db)
	ctx := context.Background()

	// Install plugin directly using SQL since our mock service models are incomplete
	db.Exec("INSERT INTO plugins (id, plugin_key, status, version) VALUES ('plugin-1', 'com.test.1', 'pending', '1.0')")

	// Try to install duplicate key
	_, err := svc.Install(ctx, InstallPluginInput{
		PluginKey: "com.test.1", // Same key
		Version:   "2.0",
	})
	if err == nil {
		t.Errorf("expected error for duplicate plugin key")
	}

	// Add another plugin directly
	db.Exec("INSERT INTO plugins (id, plugin_key, status) VALUES ('plugin-2', 'com.test.2', 'uninstalled')")

	// List without uninstalled
	plugins, err := svc.List(ctx, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plugins) != 1 {
		t.Errorf("expected 1 active plugin, got %d", len(plugins))
	}
	if plugins[0].PluginKey != "com.test.1" {
		t.Errorf("expected plugin 1, got %s", plugins[0].PluginKey)
	}

	// List with uninstalled
	plugins, err = svc.List(ctx, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plugins) != 2 {
		t.Errorf("expected 2 plugins total, got %d", len(plugins))
	}

	// GetByID
	p, err := svc.GetByID(ctx, "plugin-1")
	if err != nil || p == nil {
		t.Fatalf("expected to get plugin 1 by ID")
	}

	// GetByKey
	p, err = svc.GetByKey(ctx, "com.test.1")
	if err != nil || p == nil {
		t.Fatalf("expected to get plugin 1 by Key")
	}

	// Uninstall (soft)
	uninstalled, err := svc.Uninstall(ctx, "plugin-1", false)
	if err != nil || uninstalled == nil {
		t.Fatalf("unexpected error uninstalling (soft): %v", err)
	}
	if uninstalled.Status != "uninstalled" {
		t.Errorf("expected status 'uninstalled', got %s", uninstalled.Status)
	}

	var count int64
	db.Table("plugins").Count(&count)
	if count != 2 {
		t.Errorf("expected plugin to still exist after soft uninstall")
	}

	// Uninstall (hard)
	_, err = svc.Uninstall(ctx, "plugin-1", true)
	if err != nil {
		t.Fatalf("unexpected error uninstalling (hard): %v", err)
	}

	db.Table("plugins").Count(&count)
	if count != 1 {
		t.Errorf("expected plugin to be deleted after hard uninstall, count=%d", count)
	}
}

func TestPluginRegistryService_Config(t *testing.T) {
	db := setupPluginRegistryTestDB(t)
	svc := NewPluginRegistryService(db)
	ctx := context.Background()

	now := time.Now()
	pluginID := "plugin-config-1"
	db.Exec("INSERT INTO plugins (id, plugin_key, status, updated_at) VALUES (?, 'com.test.config', 'ready', ?)", pluginID, now)

	// Set config
	configMap := map[string]interface{}{"key": "val"}
	cfg, err := svc.SetConfig(ctx, pluginID, configMap)
	if err != nil || cfg == nil {
		t.Fatalf("expected to set config: %v", err)
	}

	// Get config
	cfg, err = svc.GetConfig(ctx, pluginID)
	if err != nil || cfg == nil {
		t.Fatalf("expected to get config: %v", err)
	}
	if cfg.PluginID != pluginID {
		t.Errorf("expected config pluginID %s, got %s", pluginID, cfg.PluginID)
	}

	// Delete config
	cfg, err = svc.DeleteConfig(ctx, pluginID)
	if err != nil {
		t.Fatalf("expected to delete config: %v", err)
	}

	// Verify delete
	cfg, _ = svc.GetConfig(ctx, pluginID)
	if cfg != nil {
		t.Errorf("expected config to be nil after deletion")
	}
}
