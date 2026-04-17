package geminilocal

import (
	"fmt"
	"os"
	"path/filepath"
)

func EnsureGeminiSkillsInjected(skillsRoot string, desiredSkills []string) error {
	// Gemini doesn't have a specific skills concept like Cursor or Claude yet.
	// But keeping this for structural consistency.
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
