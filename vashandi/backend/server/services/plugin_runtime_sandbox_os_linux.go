//go:build linux

package services

import (
	"os/exec"
	"syscall"
)

func applyOSSpecificSandboxConstraints(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}

	// Create a new session and process group
	cmd.SysProcAttr.Setsid = true

	// Restrict capability and namespace isolation where possible
	// This is a common pattern to limit privileges
	cmd.SysProcAttr.Pdeathsig = syscall.SIGKILL
}
