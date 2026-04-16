package codexlocal

import (
	"os"
	"strings"
)

// BuildPaperclipEnv returns the standard environment variables.
func BuildPaperclipEnv(agentId, companyId, runId string) map[string]string {
	return map[string]string{
		"PAPERCLIP_AGENT_ID":   agentId,
		"PAPERCLIP_COMPANY_ID": companyId,
		"PAPERCLIP_RUN_ID":     runId,
	}
}

// EnsurePathInEnv ensures common binary dirs are in PATH if missing.
func EnsurePathInEnv(env map[string]string) map[string]string {
	path := env["PATH"]
	if path == "" {
		path = os.Getenv("PATH")
	}
	
	commonDirs := []string{
		"/usr/local/bin",
		"/opt/homebrew/bin",
		"/usr/bin",
		"/bin",
	}
	
	existing := strings.Split(path, string(os.PathListSeparator))
	existingMap := make(map[string]bool)
	for _, d := range existing {
		existingMap[d] = true
	}
	
	for _, d := range commonDirs {
		if !existingMap[d] {
			path = d + string(os.PathListSeparator) + path
		}
	}
	
	env["PATH"] = path
	return env
}

func MapToSlice(m map[string]string) []string {
	s := make([]string, 0, len(m))
	for k, v := range m {
		s = append(s, k+"="+v)
	}
	return s
}
