package opencodelocal

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type PreparedRuntimeConfig struct {
	Env     map[string]string
	Cleanup func()
}

func GetXDGConfigHome(env map[string]string) string {
	if val := strings.TrimSpace(env["XDG_CONFIG_HOME"]); val != "" {
		return val
	}
	if val := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); val != "" {
		return val
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config")
}

// copyDir recursively copies a directory tree, attempting to preserve permissions.
// Source directory must exist.
func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		sourcePath := filepath.Join(src, entry.Name())
		destPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := os.MkdirAll(destPath, 0755); err != nil {
				return err
			}
			if err := copyDir(sourcePath, destPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(sourcePath, destPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func PrepareOpenCodeRuntimeConfig(env map[string]string) (*PreparedRuntimeConfig, error) {
	xdgConfigHome := GetXDGConfigHome(env)
	sourceConfigDir := filepath.Join(xdgConfigHome, "opencode")

	// Create temp override directory
	tmpDir, err := os.MkdirTemp(os.TempDir(), "paperclip-opencode-config-")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp config dir: %w", err)
	}

	targetConfigDir := filepath.Join(tmpDir, "opencode")
	targetConfigPath := filepath.Join(targetConfigDir, "opencode.json")

	if err := os.MkdirAll(targetConfigDir, 0755); err != nil {
		os.RemoveAll(tmpDir)
		return nil, err
	}

	// Best-effort copy of the config DBs
	if _, err := os.Stat(sourceConfigDir); err == nil {
		copyDir(sourceConfigDir, targetConfigDir)
	}

	// Read existing opencode.json if it exists
	var config map[string]interface{}
	data, err := os.ReadFile(targetConfigPath)
	if err == nil {
		json.Unmarshal(data, &config)
	}
	if config == nil {
		config = make(map[string]interface{})
	}

	// Inject auto-affirm permission override
	perm, ok := config["permission"].(map[string]interface{})
	if !ok {
		perm = make(map[string]interface{})
	}
	perm["external_directory"] = "allow"
	config["permission"] = perm

	outData, _ := json.MarshalIndent(config, "", "  ")
	os.WriteFile(targetConfigPath, []byte(string(outData)+"\n"), 0644)

	newEnv := make(map[string]string)
	for k, v := range env {
		newEnv[k] = v
	}
	newEnv["XDG_CONFIG_HOME"] = tmpDir

	return &PreparedRuntimeConfig{
		Env: newEnv,
		Cleanup: func() {
			os.RemoveAll(tmpDir)
		},
	}, nil
}

func BuildPaperclipEnv(agentId, companyId, runId string) map[string]string {
	return map[string]string{
		"PAPERCLIP_AGENT_ID":   agentId,
		"PAPERCLIP_COMPANY_ID": companyId,
		"PAPERCLIP_RUN_ID":     runId,
	}
}

func EnsurePathInEnv(env map[string]string) map[string]string {
	path := env["PATH"]
	if path == "" {
		path = os.Getenv("PATH")
	}
	
	commonDirs := []string{
		"/usr/local/bin",
		"/opt/homebrew/bin",
		"/usr/bin",
		"/bin",
	}
	
	existing := strings.Split(path, string(os.PathListSeparator))
	existingMap := make(map[string]bool)
	for _, d := range existing {
		existingMap[d] = true
	}
	
	for _, d := range commonDirs {
		if !existingMap[d] {
			path = d + string(os.PathListSeparator) + path
		}
	}
	
	env["PATH"] = path
	return env
}

func MapToSlice(m map[string]string) []string {
	s := make([]string, 0, len(m))
	for k, v := range m {
		s = append(s, k+"="+v)
	}
	return s
}
