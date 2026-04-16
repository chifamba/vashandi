package claudelocal

import (
	"os"
	"strings"
)

type RuntimeConfig struct {
	Command   string
	Cwd       string
	Env       []string
	TimeoutSec int
}

// BuildPaperclipEnv returns the standard environment variables for a run.
func BuildPaperclipEnv(agentId, companyId, runId string) map[string]string {
	return map[string]string{
		"PAPERCLIP_AGENT_ID":   agentId,
		"PAPERCLIP_COMPANY_ID": companyId,
		"PAPERCLIP_RUN_ID":     runId,
		// In a real port, we'd also resolve PAPERCLIP_API_URL here.
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

// CleanClaudeNestingVars removes environment variables that might prevent nested Claude calls.
func CleanClaudeNestingVars(env map[string]string) map[string]string {
	keys := []string{
		"CLAUDECODE",
		"CLAUDE_CODE_ENTRYPOINT",
		"CLAUDE_CODE_SESSION",
		"CLAUDE_CODE_PARENT_SESSION",
	}
	for _, k := range keys {
		delete(env, k)
	}
	return env
}

func MapToSlice(m map[string]string) []string {
	s := make([]string, 0, len(m))
	for k, v := range m {
		s = append(s, k+"="+v)
	}
	return s
}
