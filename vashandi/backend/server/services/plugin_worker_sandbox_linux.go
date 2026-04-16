//go:build linux

package services

import (
	"os/exec"
	"syscall"
)

// applySandboxConstraints sets OS-level resource limits for the worker process.
// On Unix this uses syscall.Setrlimit via SysProcAttr.
func applySandboxConstraints(cmd *exec.Cmd, cfg *PluginSandboxConfig) {
	if cfg == nil {
		return
	}

	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}

	// Ensure the child is in its own process group to prevent it from
	// catching signals intended for the host or vice versa.
	cmd.SysProcAttr.Setpgid = true

	// Memory limit (Address Space)
	if cfg.MemoryLimitMb > 0 {
		limit := uint64(cfg.MemoryLimitMb) * 1024 * 1024
		// RLIMIT_AS (address space) is generally more effective at capping
		// Node.js than RLIMIT_RSS on most modern Linux/macOS.
		cmd.SysProcAttr.Rlimit = append(cmd.SysProcAttr.Rlimit, syscall.Rlimit{
			Cur: limit,
			Max: limit,
		})
	}
}
