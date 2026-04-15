package pilocal

import (
	"os"
	"path/filepath"
	"strings"
)

// SkillEntry describes a skill available for injection into Pi's skills directory.
type SkillEntry struct {
	Key         string // logical key used to select/deselect the skill
	RuntimeName string // filename used inside Pi's agent/skills dir
	Source      string // absolute path to the skill source directory/file
	Required    bool   // if true, always injected regardless of desiredSkills
}

// InstalledSkill records a skill symlink found inside Pi's skills directory.
type InstalledSkill struct {
	Name       string
	TargetPath string // resolved symlink target
}

// SkillStatus represents the installation status of a single skill.
type SkillStatus struct {
	Key         string `json:"key"`
	RuntimeName string `json:"runtimeName"`
	Status      string `json:"status"` // "installed" | "missing" | "external" | "conflict"
	Detail      string `json:"detail,omitempty"`
}

// SkillSnapshot is the output of ListPiSkills / SyncPiSkills.
type SkillSnapshot struct {
	AdapterType string        `json:"adapterType"`
	SkillsHome  string        `json:"skillsHome"`
	Skills      []SkillStatus `json:"skills"`
}

// resolvePiSkillsHome derives the Pi skills home from the adapter config.
// It reads HOME from config.env (if set) otherwise falls back to os.UserHomeDir().
func resolvePiSkillsHome(config map[string]interface{}) string {
	home := ""
	if envRaw, ok := config["env"]; ok {
		if envMap, ok := envRaw.(map[string]interface{}); ok {
			if h, ok := envMap["HOME"].(string); ok {
				home = strings.TrimSpace(h)
			}
		}
	}
	if home == "" {
		home = homeDir()
	}
	return filepath.Join(home, ".pi", "agent", "skills")
}

// readInstalledSkillTargets reads the skills symlinks present in dir and
// returns a map of filename → InstalledSkill.
func readInstalledSkillTargets(dir string) (map[string]InstalledSkill, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]InstalledSkill{}, nil
		}
		return nil, err
	}
	result := make(map[string]InstalledSkill, len(entries))
	for _, e := range entries {
		target, err := os.Readlink(filepath.Join(dir, e.Name()))
		if err != nil {
			// Not a symlink or unreadable – record without target.
			result[e.Name()] = InstalledSkill{Name: e.Name()}
			continue
		}
		result[e.Name()] = InstalledSkill{Name: e.Name(), TargetPath: target}
	}
	return result, nil
}

// ensurePaperclipSkillSymlink creates or repairs a symlink at target pointing
// to source.  Returns "created", "repaired", or "skipped".
func ensurePaperclipSkillSymlink(source, target string) (string, error) {
	existing, err := os.Readlink(target)
	if err == nil {
		// Symlink exists – check if it already points to the right place.
		if existing == source {
			return "skipped", nil
		}
		// Wrong target – remove and re-create.
		if err := os.Remove(target); err != nil {
			return "", err
		}
		if err := os.Symlink(source, target); err != nil {
			return "", err
		}
		return "repaired", nil
	}
	// No symlink; check for a regular file/dir in the way.
	if _, err2 := os.Lstat(target); err2 == nil {
		// Something else exists at target – remove it.
		if err := os.RemoveAll(target); err != nil {
			return "", err
		}
	}
	if err := os.Symlink(source, target); err != nil {
		return "", err
	}
	return "created", nil
}

// removeMaintainerOnlySkillSymlinks removes symlinks from skillsHome whose
// name is NOT in keepNames and whose target is a Paperclip-managed path (best-
// effort heuristic: name contains "paperclip" or "pi-skill").
// Returns the names of removed symlinks.
func removeMaintainerOnlySkillSymlinks(skillsHome string, keepNames []string) ([]string, error) {
	keepSet := make(map[string]bool, len(keepNames))
	for _, n := range keepNames {
		keepSet[n] = true
	}

	entries, err := os.ReadDir(skillsHome)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var removed []string
	for _, e := range entries {
		name := e.Name()
		if keepSet[name] {
			continue
		}
		// Only remove symlinks that look like Paperclip-managed skills.
		target, err := os.Readlink(filepath.Join(skillsHome, name))
		if err != nil {
			continue // not a symlink
		}
		if strings.Contains(strings.ToLower(target), "paperclip") ||
			strings.Contains(strings.ToLower(name), "paperclip") ||
			strings.Contains(strings.ToLower(name), "pi-skill") {
			if err := os.Remove(filepath.Join(skillsHome, name)); err == nil {
				removed = append(removed, name)
			}
		}
	}
	return removed, nil
}

// buildSkillSnapshot assembles a SkillSnapshot from the available entries,
// desired set, and what is currently installed in skillsHome.
func buildSkillSnapshot(
	adapterType string,
	available []SkillEntry,
	desiredSkills []string,
	installed map[string]InstalledSkill,
	skillsHome string,
) SkillSnapshot {
	desiredSet := make(map[string]bool, len(desiredSkills))
	for _, k := range desiredSkills {
		desiredSet[k] = true
	}
	// Required skills are always desired.
	for _, e := range available {
		if e.Required {
			desiredSet[e.Key] = true
		}
	}

	availableByRuntime := make(map[string]SkillEntry, len(available))
	for _, e := range available {
		availableByRuntime[e.RuntimeName] = e
	}

	skills := make([]SkillStatus, 0, len(available))
	for _, entry := range available {
		if !desiredSet[entry.Key] {
			continue
		}
		inst, ok := installed[entry.RuntimeName]
		if !ok {
			skills = append(skills, SkillStatus{
				Key:         entry.Key,
				RuntimeName: entry.RuntimeName,
				Status:      "missing",
				Detail:      "Configured but not currently linked into the Pi skills home.",
			})
			continue
		}
		if inst.TargetPath == "" {
			skills = append(skills, SkillStatus{
				Key:         entry.Key,
				RuntimeName: entry.RuntimeName,
				Status:      "conflict",
				Detail:      "Skill name is occupied by an external installation.",
			})
			continue
		}
		if inst.TargetPath != entry.Source {
			skills = append(skills, SkillStatus{
				Key:         entry.Key,
				RuntimeName: entry.RuntimeName,
				Status:      "external",
				Detail:      "Installed outside Paperclip management.",
			})
			continue
		}
		skills = append(skills, SkillStatus{
			Key:         entry.Key,
			RuntimeName: entry.RuntimeName,
			Status:      "installed",
		})
	}

	// Also surface installed skills not in available (external).
	for name := range installed {
		if _, managed := availableByRuntime[name]; managed {
			continue
		}
		skills = append(skills, SkillStatus{
			RuntimeName: name,
			Status:      "external",
			Detail:      "Installed outside Paperclip management.",
		})
	}

	return SkillSnapshot{
		AdapterType: adapterType,
		SkillsHome:  skillsHome,
		Skills:      skills,
	}
}

// SkillContext is the input for ListPiSkills / SyncPiSkills.
type SkillContext struct {
	Config          map[string]interface{}
	AvailableSkills []SkillEntry // provided by the caller (mirrors readPaperclipRuntimeSkillEntries)
}

// resolvePiDesiredSkillNames resolves the desired skill set from config.skills
// and always includes required entries.
func resolvePiDesiredSkillNames(config map[string]interface{}, available []SkillEntry) []string {
	var explicit []string
	if skills, ok := config["skills"]; ok {
		switch v := skills.(type) {
		case []interface{}:
			for _, item := range v {
				if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
					explicit = append(explicit, strings.TrimSpace(s))
				}
			}
		case []string:
			for _, s := range v {
				if t := strings.TrimSpace(s); t != "" {
					explicit = append(explicit, t)
				}
			}
		}
	}
	desired := make(map[string]bool)
	if len(explicit) == 0 {
		// Default: all available
		for _, e := range available {
			desired[e.Key] = true
		}
	} else {
		for _, k := range explicit {
			desired[k] = true
		}
	}
	// Always include required.
	for _, e := range available {
		if e.Required {
			desired[e.Key] = true
		}
	}
	result := make([]string, 0, len(desired))
	for k := range desired {
		result = append(result, k)
	}
	return result
}

// ListPiSkills returns the current skills snapshot without modifying the
// filesystem.
func ListPiSkills(ctx SkillContext) (SkillSnapshot, error) {
	skillsHome := resolvePiSkillsHome(ctx.Config)
	desired := resolvePiDesiredSkillNames(ctx.Config, ctx.AvailableSkills)
	installed, err := readInstalledSkillTargets(skillsHome)
	if err != nil {
		return SkillSnapshot{}, err
	}
	return buildSkillSnapshot("pi_local", ctx.AvailableSkills, desired, installed, skillsHome), nil
}

// SyncPiSkills installs/removes skill symlinks to match the desired set.
func SyncPiSkills(ctx SkillContext, desiredSkills []string) (SkillSnapshot, error) {
	skillsHome := resolvePiSkillsHome(ctx.Config)

	// Build the full desired set (includes required).
	desiredSet := make(map[string]bool, len(desiredSkills))
	for _, k := range desiredSkills {
		desiredSet[k] = true
	}
	for _, e := range ctx.AvailableSkills {
		if e.Required {
			desiredSet[e.Key] = true
		}
	}

	if err := os.MkdirAll(skillsHome, 0o755); err != nil {
		return SkillSnapshot{}, err
	}

	installed, err := readInstalledSkillTargets(skillsHome)
	if err != nil {
		return SkillSnapshot{}, err
	}

	availableByRuntime := make(map[string]SkillEntry, len(ctx.AvailableSkills))
	for _, e := range ctx.AvailableSkills {
		availableByRuntime[e.RuntimeName] = e
	}

	// Install desired skills.
	for _, entry := range ctx.AvailableSkills {
		if !desiredSet[entry.Key] {
			continue
		}
		target := filepath.Join(skillsHome, entry.RuntimeName)
		_, _ = ensurePaperclipSkillSymlink(entry.Source, target)
	}

	// Remove undesired Paperclip-managed symlinks.
	for name, inst := range installed {
		av, managed := availableByRuntime[name]
		if !managed {
			continue
		}
		if desiredSet[av.Key] {
			continue
		}
		if inst.TargetPath != av.Source {
			continue
		}
		_ = os.Remove(filepath.Join(skillsHome, name))
	}

	// Rebuild snapshot after mutations.
	installed2, err := readInstalledSkillTargets(skillsHome)
	if err != nil {
		return SkillSnapshot{}, err
	}
	desired2 := resolvePiDesiredSkillNames(ctx.Config, ctx.AvailableSkills)
	return buildSkillSnapshot("pi_local", ctx.AvailableSkills, desired2, installed2, skillsHome), nil
}
