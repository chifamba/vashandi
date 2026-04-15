package services

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/chifamba/vashandi/vashandi/backend/shared"
)

func TestDeriveRepoNameFromURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"https URL with .git", "https://github.com/user/repo.git", "repo"},
		{"https URL without .git", "https://github.com/user/repo", "repo"},
		{"ssh URL with .git", "git@github.com:user/repo.git", "repo"},
		{"bare name", "my-repo", "my-repo"},
		{"bare name with .git", "my-repo.git", "my-repo"},
		{"empty string", "", ""},
		{"nested path", "https://github.com/org/sub/repo.git", "repo"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := deriveRepoNameFromURL(tc.input)
			if got != tc.expected {
				t.Errorf("deriveRepoNameFromURL(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestRealizeWorkspace_WithoutRepoURLCreatesPrimaryWorkspace(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("PAPERCLIP_HOME", tempHome)
	t.Setenv("PAPERCLIP_INSTANCE_ID", "test")

	svc := NewWorkspaceService(nil)
	cwd, err := svc.RealizeWorkspace(context.Background(), "comp-1", "proj-1", "", RealizeOptions{})
	if err != nil {
		t.Fatalf("RealizeWorkspace returned error: %v", err)
	}

	expected := shared.ResolveManagedProjectWorkspaceDir("comp-1", "proj-1", "")
	if cwd != expected {
		t.Fatalf("expected workspace %q, got %q", expected, cwd)
	}
	if info, err := os.Stat(cwd); err != nil {
		t.Fatalf("expected workspace dir to exist: %v", err)
	} else if !info.IsDir() {
		t.Fatalf("expected %q to be a directory", cwd)
	}
}

func TestRealizeWorkspace_WorktreeCreatesSeparateCheckout(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required for worktree integration coverage")
	}

	tempHome := t.TempDir()
	t.Setenv("PAPERCLIP_HOME", tempHome)
	t.Setenv("PAPERCLIP_INSTANCE_ID", "test")

	primaryCwd := shared.ResolveManagedProjectWorkspaceDir("comp-1", "proj-1", "repo")
	if err := os.MkdirAll(primaryCwd, 0o755); err != nil {
		t.Fatalf("mkdir primary workspace: %v", err)
	}

	runGit := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, string(output))
		}
	}

	runGit(primaryCwd, "init", "-b", "main")
	runGit(primaryCwd, "config", "user.email", "tests@example.com")
	runGit(primaryCwd, "config", "user.name", "Tests")
	readmePath := filepath.Join(primaryCwd, "README.md")
	if err := os.WriteFile(readmePath, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	runGit(primaryCwd, "add", "README.md")
	runGit(primaryCwd, "commit", "-m", "init")

	svc := NewWorkspaceService(nil)
	worktreeCwd, err := svc.RealizeWorkspace(context.Background(), "comp-1", "proj-1", "https://example.com/repo.git", RealizeOptions{
		Strategy:   StrategyWorktree,
		RunID:      "run-123",
		BranchName: "main",
	})
	if err != nil {
		t.Fatalf("RealizeWorkspace returned error: %v", err)
	}

	expected := primaryCwd + "-worktree-run-123"
	if worktreeCwd != expected {
		t.Fatalf("expected worktree %q, got %q", expected, worktreeCwd)
	}
	if worktreeCwd == primaryCwd {
		t.Fatal("expected worktree path to differ from primary workspace")
	}
	if _, err := os.Stat(filepath.Join(worktreeCwd, "README.md")); err != nil {
		t.Fatalf("expected worktree checkout to contain repository files: %v", err)
	}
}
