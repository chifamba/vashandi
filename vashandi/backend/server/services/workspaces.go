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

type WorkspaceService struct{}

func NewWorkspaceService() *WorkspaceService {
	return &WorkspaceService{}
}


// RealizeWorkspace prepares the directory for an agent run.
func (s *WorkspaceService) RealizeWorkspace(ctx context.Context, companyID, projectID, repoURL string) (string, error) {
	// 1. Resolve Path
	repoName := deriveRepoNameFromURL(repoURL)
	cwd := shared.ResolveManagedProjectWorkspaceDir(companyID, projectID, repoName)

	// 2. Ensure parent dir
	if err := os.MkdirAll(filepath.Dir(cwd), 0755); err != nil {
		return "", fmt.Errorf("failed to create projects directory: %w", err)
	}

	// 3. Simple logic: If no repoURL, just create dir
	if repoURL == "" {
		if err := os.MkdirAll(cwd, 0755); err != nil {
			return "", err
		}
		return cwd, nil
	}

	// 4. Git Logic: If .git exists, assume ready
	if _, err := os.Stat(filepath.Join(cwd, ".git")); err == nil {
		return cwd, nil
	}

	// 5. Clone
	cmd := exec.CommandContext(ctx, "git", "clone", repoURL, cwd)
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git clone failed: %w (output: %s)", err, string(output))
	}

	return cwd, nil
}

func deriveRepoNameFromURL(repoURL string) string {
	if repoURL == "" {
		return ""
	}
	base := filepath.Base(repoURL)
	return strings.TrimSuffix(base, ".git")
}
