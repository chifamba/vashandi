//go:build darwin

package services

import (
	"os/exec"
	"syscall"
)

// applySandboxConstraints sets OS-level resource limits for the worker process.
// On Darwin, SysProcAttr does not support Rlimit directly.
func applySandboxConstraints(cmd *exec.Cmd, cfg *PluginSandboxConfig) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}

	// Ensure the child is in its own process group to prevent it from
	// catching signals intended for the host or vice versa.
	cmd.SysProcAttr.Setpgid = true
}
