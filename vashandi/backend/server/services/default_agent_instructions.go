package services

import (
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
)

type DefaultAgentBundleRole string

const (
	DefaultAgentBundleRoleDefault DefaultAgentBundleRole = "default"
	DefaultAgentBundleRoleCEO     DefaultAgentBundleRole = "ceo"
)

var defaultAgentBundleFiles = map[DefaultAgentBundleRole][]string{
	DefaultAgentBundleRoleDefault: {"AGENTS.md"},
	DefaultAgentBundleRoleCEO:     {"AGENTS.md", "HEARTBEAT.md", "SOUL.md", "TOOLS.md"},
}

// In Go, since we can't easily use import.meta.url, we pass a VFS or base path
// We'll accept an fs.FS to allow mocking or reading from an embedded filesystem
func LoadDefaultAgentInstructionsBundle(vfs fs.FS, basePath string, role string) (map[string]string, error) {
	resolvedRole := ResolveDefaultAgentInstructionsBundleRole(role)
	fileNames := defaultAgentBundleFiles[resolvedRole]

	bundle := make(map[string]string)
	for _, fileName := range fileNames {
		// e.g. basePath = "onboarding-assets"
		fullPath := filepath.Join(basePath, string(resolvedRole), fileName)

		file, err := vfs.Open(fullPath)
		if err != nil {
			return nil, fmt.Errorf("failed to open bundle file %s: %w", fileName, err)
		}
		defer file.Close()

		content, err := io.ReadAll(file)
		if err != nil {
			return nil, fmt.Errorf("failed to read bundle file %s: %w", fileName, err)
		}

		bundle[fileName] = string(content)
	}

	return bundle, nil
}

func ResolveDefaultAgentInstructionsBundleRole(role string) DefaultAgentBundleRole {
	if role == "ceo" {
		return DefaultAgentBundleRoleCEO
	}
	return DefaultAgentBundleRoleDefault
}
