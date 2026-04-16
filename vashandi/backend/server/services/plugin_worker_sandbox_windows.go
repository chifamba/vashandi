//go:build windows

package services

import (
	"os/exec"
)

// applySandboxConstraints is a no-op on Windows for now.
func applySandboxConstraints(cmd *exec.Cmd, cfg *PluginSandboxConfig) {
	// Resource limits via Job Objects are complex to implement via os/exec 
	// without external packages. Minimal sandboxing is enforced via environment
	// sanitization in the platform-independent worker manager.
}
