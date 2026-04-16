package cursorlocal

import (
	"fmt"
	"os"
	"path/filepath"
)

// EnsureCursorSkillsInjected creates isolated skills links inside ~/.cursor/skills
func EnsureCursorSkillsInjected(skillsRoot string, desiredSkills []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to resolve home directory: %w", err)
	}

	skillsHome := filepath.Join(home, ".cursor", "skills")
	if err := os.MkdirAll(skillsHome, 0755); err != nil {
		return fmt.Errorf("failed to prepare cursor skills directory: %w", err)
	}

	entries, err := os.ReadDir(skillsRoot)
	if err != nil {
		return nil // No skills root, continue without skills
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
		// Filter by desired skills if the list is provided
		if len(desiredSkills) > 0 && !desiredSet[name] && !desiredSet["paperclipai/paperclip/"+name] {
			continue
		}

		sourcePath := filepath.Join(skillsRoot, name)
		targetPath := filepath.Join(skillsHome, name)
		
		// Create a fresh symlink
		os.Remove(targetPath)
		if err := os.Symlink(sourcePath, targetPath); err != nil {
			fmt.Fprintf(os.Stderr, "[paperclip] failed to symlink cursor skill %s: %v\n", name, err)
		}
	}

	// Prune stale cursor skills
	currentEntries, _ := os.ReadDir(skillsHome)
	for _, entry := range currentEntries {
		name := entry.Name()
		if len(desiredSkills) > 0 && !desiredSet[name] && !desiredSet["paperclipai/paperclip/"+name] {
			targetPath := filepath.Join(skillsHome, name)
			if stat, err := os.Lstat(targetPath); err == nil && stat.Mode()&os.ModeSymlink != 0 {
				os.Remove(targetPath)
			}
		}
	}

	return nil
}

func ResolveRepoSkillsDir() (string, error) {
	candidates := []string{"skills", "../skills", "../../skills", "../../../skills"}
	for _, c := range candidates {
		abs, _ := filepath.Abs(c)
		if info, err := os.Stat(abs); err == nil && info.IsDir() {
			return abs, nil
		}
	}
	return "", fmt.Errorf("could not find skills directory")
}
