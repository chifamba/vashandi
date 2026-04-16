package services

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"github.com/chifamba/vashandi/vashandi/backend/shared"
	"gorm.io/gorm"
)

type WorkspaceStrategy string

const (
	StrategyPrimary  WorkspaceStrategy = "primary"
	StrategyWorktree WorkspaceStrategy = "worktree"
)

type RealizeOptions struct {
	Strategy                   WorkspaceStrategy
	BranchName                 string
	RunID                      string
	RepoRef                    string
	BaseCwd                    string
	ProjectWorkspaceID         string
	Issue                      *models.Issue
	Agent                      *models.Agent
	ExecutionWorkspaceStrategy *ExecutionWorkspaceStrategy
	Recorder                   *WorkspaceOperationRecorder
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
	if strings.TrimSpace(opts.BaseCwd) != "" {
		primaryCwd = opts.BaseCwd
	}

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
		strategy := opts.ExecutionWorkspaceStrategy
		if strategy == nil {
			branchTemplate := "{{issue.identifier}}-{{slug}}"
			if strings.TrimSpace(opts.RunID) != "" {
				branchTemplate = "run-" + opts.RunID
			}
			strategy = &ExecutionWorkspaceStrategy{
				Type:           "git_worktree",
				BaseRef:        firstNonEmpty(opts.BranchName, opts.RepoRef),
				BranchTemplate: branchTemplate,
			}
		}
		realized, err := RealizeExecutionWorkspace(ctx, RealizeExecutionWorkspaceInput{
			BaseCwd:            primaryCwd,
			ProjectID:          projectID,
			ProjectWorkspaceID: opts.ProjectWorkspaceID,
			RepoURL:            repoURL,
			RepoRef:            opts.RepoRef,
			Strategy:           strategy,
			Issue:              opts.Issue,
			Agent:              opts.Agent,
			Recorder:           opts.Recorder,
		})
		if err != nil {
			return "", err
		}
		return realized.Cwd, nil
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
