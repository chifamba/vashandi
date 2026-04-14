package services

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/chifamba/vashandi/vashandi/backend/shared"
	"gorm.io/gorm"
)

type WorkspaceStrategy string

const (
	StrategyPrimary  WorkspaceStrategy = "primary"
	StrategyWorktree WorkspaceStrategy = "worktree"
)

type RealizeOptions struct {
	Strategy   WorkspaceStrategy
	BranchName string
	RunID      string
}

type WorkspaceService struct {
	DB *gorm.DB
}

func NewWorkspaceService(db *gorm.DB) *WorkspaceService {
	return &WorkspaceService{DB: db}
}


// RealizeWorkspace prepares the directory for an agent run.
func (s *WorkspaceService) RealizeWorkspace(ctx context.Context, companyID, projectID, repoURL string, opts RealizeOptions) (string, error) {
	// 1. Resolve Primary Path
	repoName := deriveRepoNameFromURL(repoURL)
	primaryCwd := shared.ResolveManagedProjectWorkspaceDir(companyID, projectID, repoName)

	// 2. Ensure parent dir
	if err := os.MkdirAll(filepath.Dir(primaryCwd), 0755); err != nil {
		return "", fmt.Errorf("failed to create projects directory: %w", err)
	}

	// 3. Simple logic: If no repoURL, just create dir and return primary
	if repoURL == "" {
		if err := os.MkdirAll(primaryCwd, 0755); err != nil {
			return "", err
		}
		return primaryCwd, nil
	}

	// 4. Ensure Primary Repo exists
	gitDir := filepath.Join(primaryCwd, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		// Clone if missing
		cmd := exec.CommandContext(ctx, "git", "clone", repoURL, primaryCwd)
		if output, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("git clone failed: %w (output: %s)", err, string(output))
		}
	}

	// 5. If Strategy is Primary, we are done
	if opts.Strategy == StrategyPrimary || opts.Strategy == "" {
		return primaryCwd, nil
	}

	// 6. Git Worktree Logic
	if opts.Strategy == StrategyWorktree {
		worktreeDir := fmt.Sprintf("%s-worktree-%s", primaryCwd, opts.RunID)
		if opts.RunID == "" {
			worktreeDir = fmt.Sprintf("%s-worktree-temp", primaryCwd)
		}

		// Ensure worktree doesn't already exist from a stale run
		os.RemoveAll(worktreeDir)

		branch := opts.BranchName
		if branch == "" {
			branch = "main" // Default to main if not specified
		}

		// git worktree add <path> <branch>
		// Note: This assumes the branch exists or creates a new one if needed?
		// Usually we want to create a unique branch for the work or just checkout an existing one.
		cmd := exec.CommandContext(ctx, "git", "worktree", "add", worktreeDir, branch)
		cmd.Dir = primaryCwd
		if _, err := cmd.CombinedOutput(); err != nil {
			// If it fails, maybe the branch doesn't exist? Try -b
			cmdRetry := exec.CommandContext(ctx, "git", "worktree", "add", "-b", fmt.Sprintf("run-%s", opts.RunID), worktreeDir)
			cmdRetry.Dir = primaryCwd
			if output2, err2 := cmdRetry.CombinedOutput(); err2 != nil {
				return "", fmt.Errorf("git worktree add failed: %w (output: %s)", err2, string(output2))
			}
		}

		return worktreeDir, nil
	}

	return primaryCwd, nil
}

func deriveRepoNameFromURL(repoURL string) string {
	if repoURL == "" {
		return ""
	}
	base := filepath.Base(repoURL)
	return strings.TrimSuffix(base, ".git")
}
