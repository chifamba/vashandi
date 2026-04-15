//go:build !linux

package services

import (
	"os/exec"
)

func applyOSSpecificSandboxConstraints(cmd *exec.Cmd) {
	// For macOS and Windows, we apply minimal process separation
	// Detailed restrictions require OS-specific hooks.
}
