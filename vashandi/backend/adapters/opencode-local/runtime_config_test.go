package opencodelocal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareOpenCodeRuntimeConfig_InjectsExternalDirectoryAllow(t *testing.T) {
	// Create a fake source config home.
	configHome := t.TempDir()
	configDir := filepath.Join(configHome, "opencode")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	initial := map[string]interface{}{
		"theme": "system",
		"permission": map[string]interface{}{
			"read": "allow",
		},
	}
	data, _ := json.MarshalIndent(initial, "", "  ")
	if err := os.WriteFile(filepath.Join(configDir, "opencode.json"), append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	env := map[string]string{"XDG_CONFIG_HOME": configHome}
	cfg, err := PrepareOpenCodeRuntimeConfig(env, map[string]interface{}{})
	if err != nil {
		t.Fatalf("PrepareOpenCodeRuntimeConfig: %v", err)
	}
	defer cfg.Cleanup() //nolint:errcheck

	// XDG_CONFIG_HOME must have changed to a temp dir.
	if cfg.Env["XDG_CONFIG_HOME"] == configHome {
		t.Error("expected XDG_CONFIG_HOME to be a temp dir, got original configHome")
	}

	runtimeConfigPath := filepath.Join(cfg.Env["XDG_CONFIG_HOME"], "opencode", "opencode.json")
	raw, err := os.ReadFile(runtimeConfigPath)
	if err != nil {
		t.Fatalf("read runtime config: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal runtime config: %v", err)
	}

	// Existing fields must be preserved.
	if got["theme"] != "system" {
		t.Errorf("expected theme=system, got %v", got["theme"])
	}
	perm, _ := got["permission"].(map[string]interface{})
	if perm == nil {
		t.Fatal("expected permission object in config")
	}
	if perm["read"] != "allow" {
		t.Errorf("expected permission.read=allow, got %v", perm["read"])
	}
	if perm["external_directory"] != "allow" {
		t.Errorf("expected permission.external_directory=allow, got %v", perm["external_directory"])
	}

	// Verify notes are set.
	if len(cfg.Notes) == 0 {
		t.Error("expected at least one note")
	}
}

func TestPrepareOpenCodeRuntimeConfig_CleanupRemovesTempDir(t *testing.T) {
	env := map[string]string{}
	cfg, err := PrepareOpenCodeRuntimeConfig(env, map[string]interface{}{})
	if err != nil {
		t.Fatalf("PrepareOpenCodeRuntimeConfig: %v", err)
	}
	tempDir := cfg.Env["XDG_CONFIG_HOME"]
	if _, err := os.Stat(tempDir); err != nil {
		t.Fatalf("expected temp dir to exist: %v", err)
	}
	if err := cfg.Cleanup(); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if _, err := os.Stat(tempDir); !os.IsNotExist(err) {
		t.Error("expected temp dir to be removed after Cleanup")
	}
}

func TestPrepareOpenCodeRuntimeConfig_ExplicitOptOut(t *testing.T) {
	configHome := t.TempDir()
	env := map[string]string{"XDG_CONFIG_HOME": configHome}
	cfg, err := PrepareOpenCodeRuntimeConfig(env, map[string]interface{}{
		"dangerouslySkipPermissions": false,
	})
	if err != nil {
		t.Fatalf("PrepareOpenCodeRuntimeConfig: %v", err)
	}
	// Env should be unchanged.
	if cfg.Env["XDG_CONFIG_HOME"] != configHome {
		t.Errorf("expected XDG_CONFIG_HOME=%s, got %s", configHome, cfg.Env["XDG_CONFIG_HOME"])
	}
	if len(cfg.Notes) != 0 {
		t.Errorf("expected no notes, got %v", cfg.Notes)
	}
	if err := cfg.Cleanup(); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
}

func TestPrepareOpenCodeRuntimeConfig_NoExistingConfig(t *testing.T) {
	// Source config dir does not exist — should not error.
	env := map[string]string{"XDG_CONFIG_HOME": t.TempDir()}
	// Remove the opencode sub-dir so it doesn't exist.
	cfg, err := PrepareOpenCodeRuntimeConfig(env, map[string]interface{}{})
	if err != nil {
		t.Fatalf("PrepareOpenCodeRuntimeConfig: %v", err)
	}
	defer cfg.Cleanup() //nolint:errcheck

	// Config file should still be created with external_directory=allow.
	runtimeConfigPath := filepath.Join(cfg.Env["XDG_CONFIG_HOME"], "opencode", "opencode.json")
	raw, err := os.ReadFile(runtimeConfigPath)
	if err != nil {
		t.Fatalf("read runtime config: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal runtime config: %v", err)
	}
	perm, _ := got["permission"].(map[string]interface{})
	if perm == nil || perm["external_directory"] != "allow" {
		t.Errorf("expected permission.external_directory=allow, got %v", got)
	}
}
