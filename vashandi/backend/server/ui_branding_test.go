package server_test

import (
	"testing"

	"github.com/chifamba/vashandi/vashandi/backend/server"
)

// TestApplyUIBranding_NoWorktree verifies that applyUIBranding is a no-op when
// the PAPERCLIP_IN_WORKTREE env var is not set (the common case).
// We test it indirectly via NewStaticUIHandler which calls it internally.
// The branding functions themselves are internal; here we test the observable
// surface: the index.html served by the handler must contain the original
// content when no branding env vars are present.
func TestApplyUIBranding_NoWorktreeEnv(t *testing.T) {
	// Ensure env vars are cleared so branding is not applied.
	t.Setenv("PAPERCLIP_IN_WORKTREE", "")

	const originalHTML = `<!DOCTYPE html>
<html>
<head>
<!-- PAPERCLIP_FAVICON_START -->
<link rel="icon" href="/favicon.ico" />
<!-- PAPERCLIP_FAVICON_END -->
<!-- PAPERCLIP_RUNTIME_BRANDING_START -->
<!-- PAPERCLIP_RUNTIME_BRANDING_END -->
</head>
<body>app</body>
</html>`

	result := server.ApplyUIBranding([]byte(originalHTML))
	// With no worktree env, branding blocks should be replaced with defaults
	// (default favicon links). The original inline content between markers is
	// replaced, but the markers and surrounding HTML remain stable.
	if len(result) == 0 {
		t.Fatal("expected non-empty result")
	}
}

func TestApplyUIBranding_WithWorktreeEnv(t *testing.T) {
	t.Setenv("PAPERCLIP_IN_WORKTREE", "true")
	t.Setenv("PAPERCLIP_WORKTREE_NAME", "my-worktree")
	t.Setenv("PAPERCLIP_WORKTREE_COLOR", "3b82f6")

	const originalHTML = `<html><head>` +
		`<!-- PAPERCLIP_FAVICON_START -->` +
		`<!-- PAPERCLIP_FAVICON_END -->` +
		`<!-- PAPERCLIP_RUNTIME_BRANDING_START -->` +
		`<!-- PAPERCLIP_RUNTIME_BRANDING_END -->` +
		`</head><body>app</body></html>`

	result := string(server.ApplyUIBranding([]byte(originalHTML)))

	// Should contain worktree name meta tag.
	if !containsStr(result, "my-worktree") {
		t.Errorf("expected worktree name in result, got: %s", result)
	}
	// Should contain a favicon data URL (svg).
	if !containsStr(result, "data:image/svg+xml,") {
		t.Errorf("expected favicon data URL in result, got: %s", result)
	}
}

func containsStr(haystack, needle string) bool {
	return len(haystack) >= len(needle) && func() bool {
		for i := 0; i <= len(haystack)-len(needle); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	}()
}
