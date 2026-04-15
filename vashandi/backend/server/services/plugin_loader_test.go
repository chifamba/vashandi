package services

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
)

func ptr(s string) *string {
	return &s
}

func TestPluginLoaderService_ResolveWorkerEntrypoint(t *testing.T) {
	svc := NewPluginLoaderService()

	manifestJSON := map[string]interface{}{
		"entrypoints": map[string]interface{}{
			"worker": "dist/worker.js",
		},
	}
	manifestBytes, _ := json.Marshal(manifestJSON)

	plugin := &models.Plugin{
		PluginKey:    "test-plugin",
		PackageName:  "@test/plugin",
		ManifestJSON: manifestBytes,
	}

	localPluginDir := "/opt/vashandi/plugins"

	// Mock file system
	fs := map[string]bool{
		filepath.Join("/opt/vashandi/plugins/node_modules/@test/plugin/dist/worker.js"): true,
	}

	existsSync := func(path string) bool {
		return fs[path]
	}

	// 1. Success - node_modules
	entrypoint, err := svc.ResolveWorkerEntrypoint(plugin, localPluginDir, existsSync)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := filepath.Join("/opt/vashandi/plugins/node_modules/@test/plugin/dist/worker.js")
	if entrypoint != expected {
		t.Errorf("expected %s, got %s", expected, entrypoint)
	}

	// 2. Local Path Install override
	plugin.PackagePath = ptr("/Users/dev/my-plugin")
	fs["/Users/dev/my-plugin"] = true
	fs["/Users/dev/my-plugin/dist/worker.js"] = true

	entrypoint, err = svc.ResolveWorkerEntrypoint(plugin, localPluginDir, existsSync)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected = filepath.Join("/Users/dev/my-plugin/dist/worker.js")
	if entrypoint != expected {
		t.Errorf("expected local override %s, got %s", expected, entrypoint)
	}

	// 3. Fallback direct install (not in node_modules)
	plugin.PackagePath = nil // clear override
	plugin.PackageName = "direct-plugin"

	// Clear all files
	fs = map[string]bool{
		filepath.Join("/opt/vashandi/plugins/direct-plugin/dist/worker.js"): true,
	}

	entrypoint, err = svc.ResolveWorkerEntrypoint(plugin, localPluginDir, existsSync)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected = filepath.Join("/opt/vashandi/plugins/direct-plugin/dist/worker.js")
	if entrypoint != expected {
		t.Errorf("expected direct path %s, got %s", expected, entrypoint)
	}

	// 4. Not found
	fs = map[string]bool{} // Empty
	_, err = svc.ResolveWorkerEntrypoint(plugin, localPluginDir, existsSync)
	if err == nil {
		t.Fatalf("expected error for not found")
	}
}

func TestPluginLoaderService_IsPathInsideDir(t *testing.T) {
	svc := NewPluginLoaderService()

	cases := []struct {
		candidate string
		parent    string
		expected  bool
	}{
		{"/opt/plugins/node_modules/my-plugin/dist/worker.js", "/opt/plugins/node_modules/my-plugin", true},
		{"/opt/plugins/node_modules/my-plugin", "/opt/plugins/node_modules/my-plugin", true},
		{"/opt/plugins/node_modules/my-plugin/../other/worker.js", "/opt/plugins/node_modules/my-plugin", false},
		{"/etc/passwd", "/opt/plugins", false},
	}

	for _, c := range cases {
		if svc.isPathInsideDir(c.candidate, c.parent) != c.expected {
			t.Errorf("expected %t for candidate=%s parent=%s", c.expected, c.candidate, c.parent)
		}
	}
}
