package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
)

type CleanupExecutionWorkspaceInput struct {
	Workspace        *models.ExecutionWorkspace
	ProjectWorkspace *models.ProjectWorkspace
	CleanupCommand   string
	TeardownCommand  string
	Recorder         *WorkspaceOperationRecorder
}

type CleanupExecutionWorkspaceResult struct {
	Warnings []string
}

func CleanupExecutionWorkspaceArtifacts(ctx context.Context, input CleanupExecutionWorkspaceInput) (*CleanupExecutionWorkspaceResult, error) {
	if input.Workspace == nil {
		return nil, fmt.Errorf("workspace is required")
	}

	var warnings []string
	workspacePath := firstNonEmpty(derefString(input.Workspace.ProviderRef), derefString(input.Workspace.Cwd))
	repoRoot := ""
	if input.Workspace.ProviderType == "git_worktree" && workspacePath != "" {
		repoRoot = resolveGitRepoRootForWorkspaceCleanup(ctx, workspacePath, derefString(input.getProjectWorkspaceCwd()))
	}

	cleanupEnv := buildCleanupWorkspaceEnv(input.Workspace, input.ProjectWorkspace)
	for _, command := range []string{
		strings.TrimSpace(input.CleanupCommand),
		strings.TrimSpace(derefString(input.getProjectWorkspaceCleanupCommand())),
		strings.TrimSpace(input.TeardownCommand),
	} {
		if command == "" {
			continue
		}
		resolved := command
		if repoRoot != "" {
			resolved = resolveRepoManagedWorkspaceCommand(command, repoRoot)
		}
		if err := recordShellCommand(ctx, input.Recorder, "workspace_teardown", firstNonEmpty(workspacePath, derefString(input.getProjectWorkspaceCwd()), "."), command, resolved, cleanupEnv); err != nil {
			warnings = append(warnings, err.Error())
		}
	}

	if input.Workspace.ProviderType == "git_worktree" && workspacePath != "" {
		if _, err := os.Stat(workspacePath); err == nil {
			if repoRoot == "" {
				warnings = append(warnings, fmt.Sprintf("could not resolve git repo root for %q", workspacePath))
			} else if err := recordGitCommand(ctx, input.Recorder, "worktree_cleanup", repoRoot, []string{"worktree", "remove", "--force", workspacePath}); err != nil {
				warnings = append(warnings, err.Error())
			}
		}
	}

	return &CleanupExecutionWorkspaceResult{Warnings: warnings}, nil
}

func buildCleanupWorkspaceEnv(workspace *models.ExecutionWorkspace, projectWorkspace *models.ProjectWorkspace) []string {
	envMap := map[string]string{}
	for _, item := range SanitizeRuntimeServiceBaseEnv(os.Environ()) {
		if idx := strings.IndexByte(item, '='); idx > 0 {
			envMap[item[:idx]] = item[idx+1:]
		}
	}
	envMap["PAPERCLIP_WORKSPACE_CWD"] = derefString(workspace.Cwd)
	envMap["PAPERCLIP_WORKSPACE_PATH"] = derefString(workspace.Cwd)
	envMap["PAPERCLIP_WORKSPACE_WORKTREE_PATH"] = firstNonEmpty(derefString(workspace.ProviderRef), derefString(workspace.Cwd))
	envMap["PAPERCLIP_WORKSPACE_BRANCH"] = derefString(workspace.BranchName)
	envMap["PAPERCLIP_WORKSPACE_BASE_CWD"] = derefString(extractProjectWorkspaceCwd(projectWorkspace))
	envMap["PAPERCLIP_WORKSPACE_REPO_ROOT"] = derefString(extractProjectWorkspaceCwd(projectWorkspace))
	envMap["PAPERCLIP_WORKSPACE_REPO_URL"] = derefString(workspace.RepoURL)
	envMap["PAPERCLIP_WORKSPACE_REPO_REF"] = derefString(workspace.BaseRef)
	envMap["PAPERCLIP_PROJECT_ID"] = workspace.ProjectID
	envMap["PAPERCLIP_PROJECT_WORKSPACE_ID"] = derefString(workspace.ProjectWorkspaceID)
	envMap["PAPERCLIP_ISSUE_ID"] = derefString(workspace.SourceIssueID)

	env := make([]string, 0, len(envMap))
	for key, value := range envMap {
		env = append(env, key+"="+value)
	}
	return env
}

func resolveGitRepoRootForWorkspaceCleanup(ctx context.Context, worktreePath string, projectWorkspaceCwd string) string {
	if projectWorkspaceCwd != "" {
		if gitCommonDir, err := runGitCommand(ctx, projectWorkspaceCwd, "rev-parse", "--git-common-dir"); err == nil && gitCommonDir != "" {
			return filepath.Dir(filepath.Clean(filepath.Join(projectWorkspaceCwd, gitCommonDir)))
		}
	}
	if gitCommonDir, err := runGitCommand(ctx, worktreePath, "rev-parse", "--git-common-dir"); err == nil && gitCommonDir != "" {
		return filepath.Dir(filepath.Clean(filepath.Join(worktreePath, gitCommonDir)))
	}
	return ""
}

func (input CleanupExecutionWorkspaceInput) getProjectWorkspaceCwd() *string {
	if input.ProjectWorkspace == nil {
		return nil
	}
	return input.ProjectWorkspace.Cwd
}

func (input CleanupExecutionWorkspaceInput) getProjectWorkspaceCleanupCommand() *string {
	if input.ProjectWorkspace == nil {
		return nil
	}
	return input.ProjectWorkspace.CleanupCommand
}

func extractProjectWorkspaceCwd(projectWorkspace *models.ProjectWorkspace) *string {
	if projectWorkspace == nil {
		return nil
	}
	return projectWorkspace.Cwd
}
