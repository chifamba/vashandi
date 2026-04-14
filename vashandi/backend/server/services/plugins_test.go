package services

import (
	"context"
	"encoding/json"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupPluginServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&plugin_svc_test=1"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.Exec("DROP TABLE IF EXISTS plugins")
	db.Exec("DROP TABLE IF EXISTS activity_log")
	db.Exec(`CREATE TABLE plugins (
		id text PRIMARY KEY,
		plugin_key text NOT NULL UNIQUE,
		package_name text NOT NULL,
		version text NOT NULL,
		api_version integer NOT NULL DEFAULT 1,
		categories text NOT NULL DEFAULT '[]',
		manifest_json text NOT NULL DEFAULT '{}',
		status text NOT NULL DEFAULT 'installed',
		install_order integer,
		package_path text,
		last_error text,
		installed_at datetime DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)
	db.Exec(`CREATE TABLE activity_log (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		actor_type text NOT NULL DEFAULT 'system',
		actor_id text NOT NULL,
		action text NOT NULL,
		entity_type text NOT NULL,
		entity_id text NOT NULL,
		agent_id text,
		run_id text,
		details text,
		created_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)
	return db
}

func TestPluginService_ListPlugins_Empty(t *testing.T) {
	db := setupPluginServiceTestDB(t)
	svc := NewPluginService(db, nil)

	plugins, err := svc.ListPlugins(context.Background())
	if err != nil {
		t.Fatalf("ListPlugins failed: %v", err)
	}
	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins, got %d", len(plugins))
	}
}

func TestPluginService_ListPlugins_OnlyInstalled(t *testing.T) {
	db := setupPluginServiceTestDB(t)
	svc := NewPluginService(db, nil)

	db.Exec("INSERT INTO plugins (id, plugin_key, package_name, version, manifest_json, status) VALUES ('p1', 'slack', 'slack-plugin', '1.0.0', '{}', 'installed')")
	db.Exec("INSERT INTO plugins (id, plugin_key, package_name, version, manifest_json, status) VALUES ('p2', 'github', 'github-plugin', '2.0.0', '{}', 'uninstalled')")
	db.Exec("INSERT INTO plugins (id, plugin_key, package_name, version, manifest_json, status) VALUES ('p3', 'jira', 'jira-plugin', '1.0.0', '{}', 'installed')")

	plugins, err := svc.ListPlugins(context.Background())
	if err != nil {
		t.Fatalf("ListPlugins failed: %v", err)
	}
	if len(plugins) != 2 {
		t.Errorf("expected 2 installed plugins, got %d", len(plugins))
	}
}

func TestPluginService_GetPluginManifest_Found(t *testing.T) {
	db := setupPluginServiceTestDB(t)
	svc := NewPluginService(db, nil)

	manifest := `{"name":"slack","capabilities":["notify"]}`
	db.Exec("INSERT INTO plugins (id, plugin_key, package_name, version, manifest_json, status) VALUES ('p1', 'slack', 'slack-plugin', '1.0.0', ?, 'installed')", manifest)

	result, err := svc.GetPluginManifest(context.Background(), "slack")
	if err != nil {
		t.Fatalf("GetPluginManifest failed: %v", err)
	}
	if result["name"] != "slack" {
		t.Errorf("expected name 'slack', got %v", result["name"])
	}
	caps, ok := result["capabilities"].([]interface{})
	if !ok {
		t.Fatal("expected capabilities array")
	}
	if len(caps) != 1 || caps[0] != "notify" {
		t.Errorf("expected capabilities ['notify'], got %v", caps)
	}
}

func TestPluginService_GetPluginManifest_NotFound(t *testing.T) {
	db := setupPluginServiceTestDB(t)
	svc := NewPluginService(db, nil)

	_, err := svc.GetPluginManifest(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for missing plugin")
	}
}

func TestPluginService_GetPluginManifest_InvalidJSON(t *testing.T) {
	db := setupPluginServiceTestDB(t)
	svc := NewPluginService(db, nil)

	db.Exec("INSERT INTO plugins (id, plugin_key, package_name, version, manifest_json, status) VALUES ('p1', 'bad', 'bad-plugin', '1.0.0', 'not-json', 'installed')")

	_, err := svc.GetPluginManifest(context.Background(), "bad")
	if err == nil {
		t.Error("expected error for invalid manifest JSON")
	}
}

func TestPluginService_UpdatePluginStatus(t *testing.T) {
	db := setupPluginServiceTestDB(t)
	activitySvc := NewActivityService(db)
	svc := NewPluginService(db, activitySvc)

	db.Exec("INSERT INTO plugins (id, plugin_key, package_name, version, manifest_json, status) VALUES ('p1', 'slack', 'slack-plugin', '1.0.0', '{}', 'installed')")

	err := svc.UpdatePluginStatus(context.Background(), "slack", "error", strPtr("connection failed"))
	if err != nil {
		t.Fatalf("UpdatePluginStatus failed: %v", err)
	}

	var status string
	db.Raw("SELECT status FROM plugins WHERE plugin_key = 'slack'").Scan(&status)
	if status != "error" {
		t.Errorf("expected status 'error', got %q", status)
	}
}

func TestPluginService_UpdatePluginStatus_LogsActivity(t *testing.T) {
	db := setupPluginServiceTestDB(t)
	activitySvc := NewActivityService(db)
	svc := NewPluginService(db, activitySvc)

	db.Exec("INSERT INTO plugins (id, plugin_key, package_name, version, manifest_json, status) VALUES ('p1', 'slack', 'slack-plugin', '1.0.0', '{}', 'installed')")

	_ = svc.UpdatePluginStatus(context.Background(), "slack", "disabled", nil)

	var count int64
	db.Raw("SELECT COUNT(*) FROM activity_log WHERE action = 'plugin.status_updated' AND entity_id = 'slack'").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 activity log entry, got %d", count)
	}

	// Verify the details include status
	var details string
	db.Raw("SELECT details FROM activity_log WHERE entity_id = 'slack'").Scan(&details)
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(details), &parsed); err == nil {
		if parsed["status"] != "disabled" {
			t.Errorf("expected status 'disabled' in activity details, got %v", parsed["status"])
		}
	}
}

func strPtr(s string) *string {
	return &s
}
