package services

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
)

type PluginLoaderService struct{}

func NewPluginLoaderService() *PluginLoaderService {
	return &PluginLoaderService{}
}

// resolveWorkerEntrypoint mimics the TypeScript function to find the absolute path to a plugin's worker entrypoint
func (s *PluginLoaderService) ResolveWorkerEntrypoint(plugin *models.Plugin, localPluginDir string, existsSync func(string) bool) (string, error) {
	if plugin == nil || plugin.ManifestJSON == nil {
		return "", fmt.Errorf("plugin or manifest is nil")
	}

	var manifest map[string]interface{}
	if err := json.Unmarshal(plugin.ManifestJSON, &manifest); err != nil {
		return "", fmt.Errorf("invalid manifest format")
	}

	entrypoints, ok := manifest["entrypoints"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("manifest missing entrypoints")
	}

	workerRelPath, ok := entrypoints["worker"].(string)
	if !ok || workerRelPath == "" {
		return "", fmt.Errorf("manifest missing entrypoints.worker")
	}

	if plugin.PackagePath != nil && *plugin.PackagePath != "" {
		packagePath := *plugin.PackagePath
		if existsSync(packagePath) {
			entrypoint := filepath.Join(packagePath, workerRelPath)
			if s.isPathInsideDir(entrypoint, packagePath) && existsSync(entrypoint) {
				return entrypoint, nil
			}
		}
	}

	packageName := plugin.PackageName
	if packageName == "" {
		return "", fmt.Errorf("plugin missing package name")
	}

	var packageDir string
	if strings.HasPrefix(packageName, "@") {
		parts := strings.Split(packageName, "/")
		if len(parts) == 2 {
			packageDir = filepath.Join(localPluginDir, "node_modules", parts[0], parts[1])
		} else {
			packageDir = filepath.Join(localPluginDir, "node_modules", packageName)
		}
	} else {
		packageDir = filepath.Join(localPluginDir, "node_modules", packageName)
	}

	directDir := filepath.Join(localPluginDir, packageName)

	for _, dir := range []string{packageDir, directDir} {
		entrypoint := filepath.Join(dir, workerRelPath)
		if s.isPathInsideDir(entrypoint, dir) && existsSync(entrypoint) {
			return entrypoint, nil
		}
	}

	if filepath.IsAbs(workerRelPath) && existsSync(workerRelPath) {
		return workerRelPath, nil
	}

	return "", fmt.Errorf("worker entrypoint not found for plugin %q", plugin.PluginKey)
}

func (s *PluginLoaderService) isPathInsideDir(candidatePath, parentDir string) bool {
	rel, err := filepath.Rel(parentDir, candidatePath)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}
