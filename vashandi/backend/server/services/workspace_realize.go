package services

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
)

// Keep worktree branch names comfortably below Git/path-length limits while still
// allowing descriptive issue-derived names.
const maxWorkspaceBranchNameLength = 120

type RealizeExecutionWorkspaceInput struct {
	BaseCwd            string
	ProjectID          string
	ProjectWorkspaceID string
	RepoURL            string
	RepoRef            string
	Strategy           *ExecutionWorkspaceStrategy
	Issue              *models.Issue
	Agent              *models.Agent
	Recorder           *WorkspaceOperationRecorder
}

type RealizedExecutionWorkspace struct {
	Cwd          string
	StrategyType string
	BaseRef      string
	BranchName   string
	WorktreePath string
	Created      bool
	Warnings     []string
}

func RealizeExecutionWorkspace(ctx context.Context, input RealizeExecutionWorkspaceInput) (*RealizedExecutionWorkspace, error) {
	baseCwd := strings.TrimSpace(input.BaseCwd)
	if baseCwd == "" {
		return nil, fmt.Errorf("base workspace directory is required")
	}
	if err := os.MkdirAll(baseCwd, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create base workspace directory: %w", err)
	}

	strategy := input.Strategy
	if strategy == nil || strategy.Type == "" || strategy.Type == "project_primary" {
		return &RealizedExecutionWorkspace{
			Cwd:          baseCwd,
			StrategyType: "project_primary",
			BaseRef:      strings.TrimSpace(input.RepoRef),
		}, nil
	}
	if strategy.Type != "git_worktree" {
		return &RealizedExecutionWorkspace{
			Cwd:          baseCwd,
			StrategyType: strategy.Type,
			BaseRef:      strings.TrimSpace(input.RepoRef),
		}, nil
	}

	repoRoot, err := runGitCommand(ctx, baseCwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}

	branchTemplate := firstNonEmpty(strategy.BranchTemplate, "{{issue.identifier}}-{{slug}}")
	branchName := sanitizeBranchName(renderWorkspaceTemplate(branchTemplate, input))
	worktreeParentDir := filepath.Join(repoRoot, ".paperclip", "worktrees")
	if strings.TrimSpace(strategy.WorktreeParentDir) != "" {
		worktreeParentDir = resolveConfiguredWorkspacePath(strategy.WorktreeParentDir, repoRoot)
	}
	if err := os.MkdirAll(worktreeParentDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create worktree parent directory: %w", err)
	}

	worktreePath := filepath.Join(worktreeParentDir, branchName)
	baseRef := firstNonEmpty(strings.TrimSpace(strategy.BaseRef), strings.TrimSpace(input.RepoRef))
	if baseRef == "" {
		baseRef = firstNonEmpty(detectDefaultBranch(ctx, repoRoot), "HEAD")
	}

	if isGitWorktree(ctx, worktreePath) {
		if err := runWorkspaceStrategyCommand(ctx, input.Recorder, "workspace_provision", strategy.ProvisionCommand, repoRoot, worktreePath, buildWorkspaceCommandEnv(input, repoRoot, worktreePath, branchName, false)); err != nil {
			return nil, err
		}
		return &RealizedExecutionWorkspace{
			Cwd:          worktreePath,
			StrategyType: "git_worktree",
			BaseRef:      baseRef,
			BranchName:   branchName,
			WorktreePath: worktreePath,
			Created:      false,
		}, nil
	}

	if info, err := os.Stat(worktreePath); err == nil && info.IsDir() {
		return nil, fmt.Errorf("configured worktree path %q already exists and is not a git worktree", worktreePath)
	}

	if err := recordGitCommand(ctx, input.Recorder, "worktree_prepare", repoRoot, []string{"worktree", "add", "-b", branchName, worktreePath, baseRef}); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "already exists") {
			return nil, err
		}
		if err := recordGitCommand(ctx, input.Recorder, "worktree_prepare", repoRoot, []string{"worktree", "add", worktreePath, branchName}); err != nil {
			return nil, err
		}
	}

	if err := runWorkspaceStrategyCommand(ctx, input.Recorder, "workspace_provision", strategy.ProvisionCommand, repoRoot, worktreePath, buildWorkspaceCommandEnv(input, repoRoot, worktreePath, branchName, true)); err != nil {
		return nil, err
	}

	return &RealizedExecutionWorkspace{
		Cwd:          worktreePath,
		StrategyType: "git_worktree",
		BaseRef:      baseRef,
		BranchName:   branchName,
		WorktreePath: worktreePath,
		Created:      true,
	}, nil
}

func renderWorkspaceTemplate(template string, input RealizeExecutionWorkspaceInput) string {
	issueIdentifier := "issue"
	if input.Issue != nil {
		issueIdentifier = firstNonEmpty(derefString(input.Issue.Identifier), input.Issue.ID, issueIdentifier)
	}
	slug := sanitizeWorkspaceSlugPart("", sanitizeWorkspaceSlugPart(issueIdentifier, "issue"))
	if input.Issue != nil {
		slug = sanitizeWorkspaceSlugPart(input.Issue.Title, sanitizeWorkspaceSlugPart(issueIdentifier, "issue"))
	}

	replacer := strings.NewReplacer(
		"{{issue.id}}", valueOrEmpty(input.Issue, func(issue *models.Issue) string { return issue.ID }),
		"{{issue.identifier}}", valueOrEmpty(input.Issue, func(issue *models.Issue) string { return derefString(issue.Identifier) }),
		"{{issue.title}}", valueOrEmpty(input.Issue, func(issue *models.Issue) string { return issue.Title }),
		"{{agent.id}}", valueOrEmpty(input.Agent, func(agent *models.Agent) string { return agent.ID }),
		"{{agent.name}}", valueOrEmpty(input.Agent, func(agent *models.Agent) string { return agent.Name }),
		"{{project.id}}", input.ProjectID,
		"{{workspace.repoRef}}", input.RepoRef,
		"{{slug}}", slug,
	)
	return replacer.Replace(template)
}

func sanitizeWorkspaceSlugPart(value string, fallback string) string {
	raw := strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	for _, r := range raw {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			builder.WriteRune(r)
			continue
		}
		builder.WriteRune('-')
	}
	result := strings.Trim(builder.String(), "-_")
	for strings.Contains(result, "--") {
		result = strings.ReplaceAll(result, "--", "-")
	}
	if result == "" {
		return fallback
	}
	return result
}

func sanitizeBranchName(value string) string {
	trimmed := strings.TrimSpace(value)
	var builder strings.Builder
	for _, r := range trimmed {
		switch {
		case (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '/' || r == '-':
			builder.WriteRune(r)
		default:
			builder.WriteRune('-')
		}
	}
	result := builder.String()
	for strings.Contains(result, "--") {
		result = strings.ReplaceAll(result, "--", "-")
	}
	result = strings.Trim(result, "-/.")
	if len(result) > maxWorkspaceBranchNameLength {
		result = result[:maxWorkspaceBranchNameLength]
		result = strings.Trim(result, "-/.")
	}
	if result == "" {
		return "paperclip-work"
	}
	return result
}

func detectDefaultBranch(ctx context.Context, repoRoot string) string {
	remoteHead, err := runGitCommand(ctx, repoRoot, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD")
	if err == nil {
		if strings.HasPrefix(remoteHead, "origin/") {
			return strings.TrimPrefix(remoteHead, "origin/")
		}
		return remoteHead
	}
	for _, candidate := range []string{"main", "master"} {
		if _, err := runGitCommand(ctx, repoRoot, "rev-parse", "--verify", "refs/remotes/origin/"+candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func resolveConfiguredWorkspacePath(value string, baseDir string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return baseDir
	}
	if strings.HasPrefix(trimmed, "~"+string(os.PathSeparator)) || trimmed == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			if trimmed == "~" {
				return home
			}
			return filepath.Join(home, strings.TrimPrefix(trimmed, "~"+string(os.PathSeparator)))
		}
	}
	if filepath.IsAbs(trimmed) {
		return filepath.Clean(trimmed)
	}
	return filepath.Clean(filepath.Join(baseDir, trimmed))
}

func isGitWorktree(ctx context.Context, path string) bool {
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		return false
	}
	_, err := runGitCommand(ctx, path, "rev-parse", "--git-dir")
	return err == nil
}

func buildWorkspaceCommandEnv(input RealizeExecutionWorkspaceInput, repoRoot, worktreePath, branchName string, created bool) []string {
	envMap := map[string]string{}
	for _, item := range SanitizeRuntimeServiceBaseEnv(os.Environ()) {
		if idx := strings.IndexByte(item, '='); idx > 0 {
			envMap[item[:idx]] = item[idx+1:]
		}
	}
	envMap["PAPERCLIP_WORKSPACE_CWD"] = worktreePath
	envMap["PAPERCLIP_WORKSPACE_PATH"] = worktreePath
	envMap["PAPERCLIP_WORKSPACE_WORKTREE_PATH"] = worktreePath
	envMap["PAPERCLIP_WORKSPACE_BRANCH"] = branchName
	envMap["PAPERCLIP_WORKSPACE_BASE_CWD"] = input.BaseCwd
	envMap["PAPERCLIP_WORKSPACE_REPO_ROOT"] = repoRoot
	envMap["PAPERCLIP_WORKSPACE_REPO_REF"] = input.RepoRef
	envMap["PAPERCLIP_WORKSPACE_REPO_URL"] = input.RepoURL
	if created {
		envMap["PAPERCLIP_WORKSPACE_CREATED"] = "true"
	} else {
		envMap["PAPERCLIP_WORKSPACE_CREATED"] = "false"
	}
	envMap["PAPERCLIP_PROJECT_ID"] = input.ProjectID
	envMap["PAPERCLIP_PROJECT_WORKSPACE_ID"] = input.ProjectWorkspaceID
	if input.Agent != nil {
		envMap["PAPERCLIP_AGENT_ID"] = input.Agent.ID
		envMap["PAPERCLIP_AGENT_NAME"] = input.Agent.Name
		envMap["PAPERCLIP_COMPANY_ID"] = input.Agent.CompanyID
	}
	if input.Issue != nil {
		envMap["PAPERCLIP_ISSUE_ID"] = input.Issue.ID
		envMap["PAPERCLIP_ISSUE_IDENTIFIER"] = derefString(input.Issue.Identifier)
		envMap["PAPERCLIP_ISSUE_TITLE"] = input.Issue.Title
	}
	env := make([]string, 0, len(envMap))
	for key, value := range envMap {
		env = append(env, key+"="+value)
	}
	return env
}

func runWorkspaceStrategyCommand(ctx context.Context, recorder *WorkspaceOperationRecorder, phase string, command, repoRoot, cwd string, env []string) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil
	}
	resolved := resolveRepoManagedWorkspaceCommand(command, repoRoot)
	return recordShellCommand(ctx, recorder, phase, cwd, command, resolved, env)
}

func resolveRepoManagedWorkspaceCommand(command string, repoRoot string) string {
	trimmed := strings.TrimSpace(command)
	parts := strings.Fields(trimmed)
	if len(parts) == 0 {
		return trimmed
	}

	scriptIndex := 0
	if parts[0] == "bash" || parts[0] == "sh" || parts[0] == "zsh" {
		if len(parts) < 2 {
			return trimmed
		}
		scriptIndex = 1
	}
	if !strings.HasPrefix(parts[scriptIndex], "./") {
		return trimmed
	}
	repoManagedPath := filepath.Join(repoRoot, strings.TrimPrefix(parts[scriptIndex], "./"))
	if _, err := os.Stat(repoManagedPath); err != nil {
		return trimmed
	}
	parts[scriptIndex] = shellQuote(repoManagedPath)
	return strings.Join(parts, " ")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func recordGitCommand(ctx context.Context, recorder *WorkspaceOperationRecorder, phase string, cwd string, args []string) error {
	command := "git " + strings.Join(args, " ")
	op, _ := beginWorkspaceOperation(ctx, recorder, phase, command)
	_, err := runGitCommand(ctx, cwd, args...)
	finishWorkspaceOperation(ctx, recorder, op, err)
	return err
}

func recordShellCommand(ctx context.Context, recorder *WorkspaceOperationRecorder, phase, cwd, command, resolved string, env []string) error {
	op, _ := beginWorkspaceOperation(ctx, recorder, phase, command)
	cmd := exec.CommandContext(ctx, resolveShell(), "-lc", resolved)
	cmd.Dir = cwd
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("%s failed: %s", command, strings.TrimSpace(string(output)))
	}
	finishWorkspaceOperation(ctx, recorder, op, err)
	return err
}

func beginWorkspaceOperation(ctx context.Context, recorder *WorkspaceOperationRecorder, phase string, command string) (*models.WorkspaceOperation, error) {
	if recorder == nil {
		return nil, nil
	}
	return recorder.Begin(ctx, phase, stringOrNil(command))
}

func finishWorkspaceOperation(ctx context.Context, recorder *WorkspaceOperationRecorder, op *models.WorkspaceOperation, err error) {
	if recorder == nil || op == nil {
		return
	}
	exitCode := 0
	if err != nil {
		exitCode = 1
	}
	_ = recorder.Finish(ctx, op.ID, exitCode, err)
}

func runGitCommand(ctx context.Context, cwd string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = cwd
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("git %s failed: %s", strings.Join(args, " "), message)
	}
	return strings.TrimSpace(string(output)), nil
}

func valueOrEmpty[T any](value *T, getter func(*T) string) string {
	if value == nil {
		return ""
	}
	return getter(value)
}
