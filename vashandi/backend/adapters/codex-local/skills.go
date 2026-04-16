package codexlocal

import (
	"fmt"
	"os"
	"path/filepath"
)

// EnsureCodexSkillsInjected provisions a skills directory within CODEX_HOME
// and symlinks referenced skills from the repository Root into it.
func EnsureCodexSkillsInjected(codexHome string, skillsRoot string, desiredSkills []string) error {
	if codexHome == "" {
		return fmt.Errorf("codexHome is empty")
	}
	
	skillsHome := filepath.Join(codexHome, "skills")
	if err := os.MkdirAll(skillsHome, 0755); err != nil {
		return fmt.Errorf("failed to create skills home: %w", err)
	}

	entries, err := os.ReadDir(skillsRoot)
	if err != nil {
		return nil // No skills root, silent continue
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
		// Filter by desired skills (if list provided)
		if len(desiredSkills) > 0 && !desiredSet[name] && !desiredSet["paperclipai/paperclip/"+name] {
			continue
		}

		sourcePath := filepath.Join(skillsRoot, name)
		targetPath := filepath.Join(skillsHome, name)
		
		// Ensure fresh symlink
		os.Remove(targetPath)
		if err := os.Symlink(sourcePath, targetPath); err != nil {
			fmt.Fprintf(os.Stderr, "failed to symlink skill %s: %v\n", name, err)
		}
	}

	// Clean up stale or undesired symlinks
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

// ResolveRepoSkillsDir tries to find the global skills directory.
func ResolveRepoSkillsDir() (string, error) {
	// Heuristic: check if we are in a monorepo structure.
	candidates := []string{"skills", "../skills", "../../skills", "../../../skills"}
	for _, c := range candidates {
		abs, _ := filepath.Abs(c)
		if info, err := os.Stat(abs); err == nil && info.IsDir() {
			return abs, nil
		}
	}
	return "", fmt.Errorf("could not find skills directory")
}
