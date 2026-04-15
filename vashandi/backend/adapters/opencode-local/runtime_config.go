package opencodelocal

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// PreparedRuntimeConfig holds the environment overlay and cleanup function
// produced by PrepareOpenCodeRuntimeConfig.
type PreparedRuntimeConfig struct {
	// Env is the environment key-value overlay to merge into the process env.
	Env map[string]string
	// Notes are human-readable messages describing what was configured.
	Notes []string
	// Cleanup releases temporary resources (temp directories, etc.).
	Cleanup func() error
}

func resolveXDGConfigHome(env map[string]string) string {
	if v := strings.TrimSpace(env["XDG_CONFIG_HOME"]); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config")
}

func readJSONObject(path string) map[string]interface{} {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

func copyDirRecursive(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		// Copy file, dereferencing symlinks by reading through them.
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func asBool(v interface{}, def bool) bool {
	switch b := v.(type) {
	case bool:
		return b
	}
	return def
}

// PrepareOpenCodeRuntimeConfig creates a temporary XDG config directory with
// permission.external_directory=allow injected (unless dangerouslySkipPermissions
// is explicitly false in config). The caller must invoke Cleanup() when done.
func PrepareOpenCodeRuntimeConfig(env map[string]string, config map[string]interface{}) (*PreparedRuntimeConfig, error) {
	skipPermissions := asBool(config["dangerouslySkipPermissions"], true)
	if !skipPermissions {
		// No-op: return the env unchanged with a no-op cleanup.
		return &PreparedRuntimeConfig{
			Env:     env,
			Notes:   nil,
			Cleanup: func() error { return nil },
		}, nil
	}

	sourceConfigDir := filepath.Join(resolveXDGConfigHome(env), "opencode")
	runtimeConfigHome, err := os.MkdirTemp("", "paperclip-opencode-config-*")
	if err != nil {
		return nil, fmt.Errorf("create runtime config temp dir: %w", err)
	}
	runtimeConfigDir := filepath.Join(runtimeConfigHome, "opencode")
	runtimeConfigPath := filepath.Join(runtimeConfigDir, "opencode.json")

	if err := os.MkdirAll(runtimeConfigDir, 0o755); err != nil {
		_ = os.RemoveAll(runtimeConfigHome)
		return nil, fmt.Errorf("create runtime config dir: %w", err)
	}

	// Copy the existing opencode config directory into the temp directory, if
	// it exists.  ENOENT is acceptable; any other error is propagated.
	if err := copyDirRecursive(sourceConfigDir, runtimeConfigDir); err != nil && !os.IsNotExist(err) {
		_ = os.RemoveAll(runtimeConfigHome)
		return nil, fmt.Errorf("copy opencode config: %w", err)
	}

	// Read existing config, merge permission.external_directory=allow, write back.
	existingConfig := readJSONObject(runtimeConfigPath)
	if existingConfig == nil {
		existingConfig = make(map[string]interface{})
	}
	existingPermission, _ := existingConfig["permission"].(map[string]interface{})
	if existingPermission == nil {
		existingPermission = make(map[string]interface{})
	}
	existingPermission["external_directory"] = "allow"
	existingConfig["permission"] = existingPermission

	data, err := json.MarshalIndent(existingConfig, "", "  ")
	if err != nil {
		_ = os.RemoveAll(runtimeConfigHome)
		return nil, fmt.Errorf("marshal runtime config: %w", err)
	}
	if err := os.WriteFile(runtimeConfigPath, append(data, '\n'), 0o644); err != nil {
		_ = os.RemoveAll(runtimeConfigHome)
		return nil, fmt.Errorf("write runtime config: %w", err)
	}

	outEnv := make(map[string]string, len(env)+1)
	for k, v := range env {
		outEnv[k] = v
	}
	outEnv["XDG_CONFIG_HOME"] = runtimeConfigHome

	cleanup := func() error {
		return os.RemoveAll(runtimeConfigHome)
	}

	return &PreparedRuntimeConfig{
		Env: outEnv,
		Notes: []string{
			"Injected runtime OpenCode config with permission.external_directory=allow to avoid headless approval prompts.",
		},
		Cleanup: cleanup,
	}, nil
}
