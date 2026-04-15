package services

import (
	"context"
	"encoding/json"
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
	db.Exec("DROP TABLE IF EXISTS plugin_config")

	db.Exec(`CREATE TABLE plugins (
		id text PRIMARY KEY,
		plugin_key text NOT NULL UNIQUE,
		package_name text NOT NULL DEFAULT '',
		status text NOT NULL DEFAULT 'pending',
		version text NOT NULL DEFAULT '',
		api_version integer NOT NULL DEFAULT 1,
		categories text NOT NULL DEFAULT '[]',
		install_order integer,
		package_path text,
		last_error text,
		manifest_json text NOT NULL DEFAULT '{}',
		installed_at datetime DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)

	db.Exec(`CREATE TABLE plugin_config (
		id text PRIMARY KEY,
		plugin_id text NOT NULL UNIQUE,
		config_json json NOT NULL DEFAULT '{}',
		last_error text,
		created_at datetime DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(plugin_id) REFERENCES plugins(id) ON DELETE CASCADE
	)`)

	return db
}

func TestPluginRegistryService_ListGetRegisterUninstall(t *testing.T) {
	db := setupPluginRegistryTestDB(t)
	svc := NewPluginRegistryService(db)
	ctx := context.Background()

	// Insert a plugin directly via SQL for initial state.
	db.Exec("INSERT INTO plugins (id, plugin_key, status, version, manifest_json) VALUES ('plugin-1', 'com.test.1', 'pending', '1.0', '{}')")

	// Registering with the same key should update (not error), transitioning to pending.
	_, err := svc.Register(ctx, RegisterPluginInput{
		PluginKey:   "com.test.1",
		PackageName: "test-pkg",
		Version:     "2.0",
		ManifestRaw: json.RawMessage(`{"id":"com.test.1"}`),
	})
	if err != nil {
		t.Fatalf("Register on existing key should update, got error: %v", err)
	}

	// Add another plugin (uninstalled).
	db.Exec("INSERT INTO plugins (id, plugin_key, status, version, manifest_json) VALUES ('plugin-2', 'com.test.2', 'uninstalled', '1.0', '{}')")

	// ListInstalled should exclude uninstalled.
	plugins, err := svc.ListInstalled(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plugins) != 1 {
		t.Errorf("expected 1 active plugin, got %d", len(plugins))
	}
	if plugins[0].PluginKey != "com.test.1" {
		t.Errorf("expected plugin 1, got %s", plugins[0].PluginKey)
	}

	// ListByStatus should return plugins matching the status.
	all, err := svc.ListByStatus(ctx, "uninstalled")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("expected 1 uninstalled plugin, got %d", len(all))
	}

	// GetByID.
	p, err := svc.GetByID(ctx, "plugin-1")
	if err != nil || p == nil {
		t.Fatalf("expected to get plugin 1 by ID")
	}

	// GetByKey.
	p, err = svc.GetByKey(ctx, "com.test.1")
	if err != nil || p == nil {
		t.Fatalf("expected to get plugin 1 by Key")
	}

	// Uninstall (soft).
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
		t.Errorf("expected plugin to still exist after soft uninstall, count=%d", count)
	}

	// Uninstall (hard).
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
	db.Exec("INSERT INTO plugins (id, plugin_key, status, updated_at, manifest_json) VALUES (?, 'com.test.config', 'ready', ?, '{}')", pluginID, now)

	// GetConfig returns nil when no config exists.
	cfg, err := svc.GetConfig(ctx, pluginID)
	if err != nil {
		t.Fatalf("expected nil error for missing config, got: %v", err)
	}
	if cfg != nil {
		t.Fatalf("expected nil config before any upsert")
	}

	// UpsertConfig (create).
	configMap := map[string]interface{}{"key": "val"}
	result, err := svc.UpsertConfig(ctx, pluginID, UpsertConfigInput{ConfigJSON: configMap})
	if err != nil || result == nil {
		t.Fatalf("expected to upsert config: %v", err)
	}
	if result.PluginID != pluginID {
		t.Errorf("expected config PluginID %s, got %s", pluginID, result.PluginID)
	}

	// GetConfig retrieves the record.
	cfg, err = svc.GetConfig(ctx, pluginID)
	if err != nil || cfg == nil {
		t.Fatalf("expected to get config: %v", err)
	}
	if cfg.PluginID != pluginID {
		t.Errorf("expected config pluginID %s, got %s", pluginID, cfg.PluginID)
	}

	// UpsertConfig (update).
	updated, err := svc.UpsertConfig(ctx, pluginID, UpsertConfigInput{ConfigJSON: map[string]interface{}{"key": "updated"}})
	if err != nil || updated == nil {
		t.Fatalf("expected update to succeed: %v", err)
	}
}

func TestPluginRegistryService_Resolve(t *testing.T) {
	db := setupPluginRegistryTestDB(t)
	svc := NewPluginRegistryService(db)
	ctx := context.Background()

	db.Exec("INSERT INTO plugins (id, plugin_key, status, version, manifest_json) VALUES ('abc-123', 'my.plugin', 'ready', '1.0', '{}')")

	// Resolve by ID.
	p, err := svc.Resolve(ctx, "abc-123")
	if err != nil || p == nil {
		t.Fatalf("expected to resolve by ID: %v", err)
	}

	// Resolve by key.
	p, err = svc.Resolve(ctx, "my.plugin")
	if err != nil || p == nil {
		t.Fatalf("expected to resolve by key: %v", err)
	}

	// Non-existent returns nil.
	p, err = svc.Resolve(ctx, "does-not-exist")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p != nil {
		t.Errorf("expected nil for non-existent plugin")
	}
}
