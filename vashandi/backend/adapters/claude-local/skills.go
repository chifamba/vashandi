package claudelocal

import (
	"fmt"
	"os"
	"path/filepath"
)

// SkillEntry represents a skill available in the repository.
type SkillEntry struct {
	Key         string
	RuntimeName string
	Source      string
}

// BuildEphemeralSkillsDir creates a temporary directory and symlinks skills into it.
func BuildEphemeralSkillsDir(skillsRoot string, desiredSkills []string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "vashandiskills-")
	if err != nil {
		return "", fmt.Errorf("failed to create tmp dir: %w", err)
	}

	target := filepath.Join(tmpDir, ".claude", "skills")
	if err := os.MkdirAll(target, 0755); err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("failed to create skills path: %w", err)
	}

	// For now, we list all directories in skillsRoot as available skills.
	entries, err := os.ReadDir(skillsRoot)
	if err != nil {
		return tmpDir, nil // Return empty if root not found
	}

	desiredSet := make(map[string]bool)
	for _, s := range desiredSkills {
		desiredSet[s] = true
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		
		name := entry.Name()
		// If desired (or if desired list is empty, treat as all for now like Node.js fallback)
		if len(desiredSkills) > 0 && !desiredSet[name] && !desiredSet["paperclipai/paperclip/"+name] {
			continue
		}

		sourcePath := filepath.Join(skillsRoot, name)
		targetPath := filepath.Join(target, name)
		
		if err := os.Symlink(sourcePath, targetPath); err != nil {
			// Log and continue
			fmt.Fprintf(os.Stderr, "failed to symlink skill %s: %v\n", name, err)
		}
	}

	return tmpDir, nil
}

// ResolveRepoSkillsDir tries to find the skills directory relative to the binary.
func ResolveRepoSkillsDir() (string, error) {
	// Heuristic: check if we are in a monorepo structure.
	// We check for 'skills' in CWD or up one level.
	candidates := []string{"skills", "../skills", "../../skills", "../../../skills"}
	for _, c := range candidates {
		abs, _ := filepath.Abs(c)
		if info, err := os.Stat(abs); err == nil && info.IsDir() {
			return abs, nil
		}
	}
	return "", fmt.Errorf("could not find skills directory")
}
