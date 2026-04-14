package shared

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const DefaultInstanceID = "default"

var (
	PathSegmentRegex         = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	FriendlyPathSegmentRegex = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)
)

func expandHomePrefix(value string) string {
	if value == "~" {
		home, _ := os.UserHomeDir()
		return home
	}
	if strings.HasPrefix(value, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, value[2:])
	}
	return value
}

func ResolvePaperclipHomeDir() string {
	envHome := strings.TrimSpace(os.Getenv("PAPERCLIP_HOME"))
	if envHome != "" {
		abs, _ := filepath.Abs(expandHomePrefix(envHome))
		return abs
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".paperclip")
}

func ResolvePaperclipInstanceID() string {
	raw := strings.TrimSpace(os.Getenv("PAPERCLIP_INSTANCE_ID"))
	if raw == "" {
		raw = DefaultInstanceID
	}
	return raw
}

func ResolvePaperclipInstanceRoot() string {
	return filepath.Join(ResolvePaperclipHomeDir(), "instances", ResolvePaperclipInstanceID())
}

func SanitizeFriendlyPathSegment(value string, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	sanitized := FriendlyPathSegmentRegex.ReplaceAllString(trimmed, "-")
	sanitized = strings.Trim(sanitized, "-")
	if sanitized == "" {
		return fallback
	}
	return sanitized
}

func ResolveManagedProjectWorkspaceDir(companyID, projectID, repoName string) string {
	return filepath.Join(
		ResolvePaperclipInstanceRoot(),
		"projects",
		SanitizeFriendlyPathSegment(companyID, "company"),
		SanitizeFriendlyPathSegment(projectID, "project"),
		SanitizeFriendlyPathSegment(repoName, "_default"),
	)
}

func ResolveDefaultAgentWorkspaceDir(agentID string) string {
	return filepath.Join(ResolvePaperclipInstanceRoot(), "workspaces", strings.TrimSpace(agentID))
}
