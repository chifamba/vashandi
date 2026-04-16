//go:build linux

package services

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/unix"
)

// applySandboxConstraints sets OS-level resource limits for the worker process.
// On Linux this places the child in its own process group; the memory cap is
// applied via prlimit(2) after the process starts (see applyPrlimitAfterStart).
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
}

// applyPrlimitAfterStart applies RLIMIT_AS to the already-started child process
// using prlimit(2). It is a no-op when limitMb is zero or the process is nil.
func applyPrlimitAfterStart(cmd *exec.Cmd, limitMb int) {
	if limitMb <= 0 || cmd == nil || cmd.Process == nil {
		return
	}
	limit := uint64(limitMb) * 1024 * 1024
	rl := unix.Rlimit{Cur: limit, Max: limit}
	// Best-effort: ignore errors (e.g., insufficient privilege).
	_ = unix.Prlimit(cmd.Process.Pid, unix.RLIMIT_AS, &rl, nil)
}
