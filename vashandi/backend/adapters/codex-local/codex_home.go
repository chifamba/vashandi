package codexlocal

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	DefaultPaperclipInstanceID = "default"
)

var (
	CopiedSharedFiles   = []string{"config.json", "config.toml", "instructions.md"}
	SymlinkedSharedFiles = []string{"auth.json"}
)

func ResolveSharedCodexHomeDir(env map[string]string) string {
	if val := strings.TrimSpace(env["CODEX_HOME"]); val != "" {
		abs, _ := filepath.Abs(val)
		return abs
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex")
}

func ResolveManagedCodexHomeDir(env map[string]string, companyId string) string {
	paperclipHome := strings.TrimSpace(env["PAPERCLIP_HOME"])
	if paperclipHome == "" {
		home, _ := os.UserHomeDir()
		paperclipHome = filepath.Join(home, ".paperclip")
	}

	instanceId := strings.TrimSpace(env["PAPERCLIP_INSTANCE_ID"])
	if instanceId == "" {
		instanceId = DefaultPaperclipInstanceID
	}

	if companyId != "" {
		return filepath.Join(paperclipHome, "instances", instanceId, "companies", companyId, "codex-home")
	}
	return filepath.Join(paperclipHome, "instances", instanceId, "codex-home")
}

func isWorktreeMode(env map[string]string) bool {
	val := strings.ToLower(strings.TrimSpace(env["PAPERCLIP_IN_WORKTREE"]))
	return val == "1" || val == "true" || val == "yes" || val == "on"
}

func PrepareManagedCodexHome(env map[string]string, companyId string) (string, error) {
	targetHome := ResolveManagedCodexHomeDir(env, companyId)
	sourceHome := ResolveSharedCodexHomeDir(env)

	if targetHome == sourceHome {
		return targetHome, nil
	}

	if err := os.MkdirAll(targetHome, 0755); err != nil {
		return "", fmt.Errorf("failed to create managed codex home: %w", err)
	}

	for _, name := range SymlinkedSharedFiles {
		src := filepath.Join(sourceHome, name)
		tgt := filepath.Join(targetHome, name)
		if _, err := os.Stat(src); err == nil {
			ensureSymlink(tgt, src)
		}
	}

	for _, name := range CopiedSharedFiles {
		src := filepath.Join(sourceHome, name)
		tgt := filepath.Join(targetHome, name)
		if _, err := os.Stat(src); err == nil {
			ensureCopiedFile(tgt, src)
		}
	}

	return targetHome, nil
}

func ensureSymlink(target, source string) {
	_, err := os.Lstat(target)
	if err == nil {
		os.Remove(target) // Best effort replace
	}
	os.Symlink(source, target)
}

func ensureCopiedFile(target, source string) {
	if _, err := os.Stat(target); err == nil {
		return
	}
	src, err := os.Open(source)
	if err != nil {
		return
	}
	defer src.Close()

	dst, err := os.Create(target)
	if err != nil {
		return
	}
	defer dst.Close()

	io.Copy(dst, src)
}
