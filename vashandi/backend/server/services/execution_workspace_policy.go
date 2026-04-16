package services

type ExecutionWorkspaceStrategy struct {
	Type              string `json:"type"`
	BaseRef           string `json:"baseRef,omitempty"`
	BranchTemplate    string `json:"branchTemplate,omitempty"`
	WorktreeParentDir string `json:"worktreeParentDir,omitempty"`
	ProvisionCommand  string `json:"provisionCommand,omitempty"`
	TeardownCommand   string `json:"teardownCommand,omitempty"`
}

type ProjectExecutionWorkspacePolicy struct {
	Enabled                   bool                        `json:"enabled"`
	DefaultMode               string                      `json:"defaultMode,omitempty"`
	DefaultProjectWorkspaceID string                      `json:"defaultProjectWorkspaceId,omitempty"`
	AllowIssueOverride        *bool                       `json:"allowIssueOverride,omitempty"`
	WorkspaceStrategy         *ExecutionWorkspaceStrategy `json:"workspaceStrategy,omitempty"`
	WorkspaceRuntime          map[string]interface{}      `json:"workspaceRuntime,omitempty"`
	BranchPolicy              map[string]interface{}      `json:"branchPolicy,omitempty"`
	PullRequestPolicy         map[string]interface{}      `json:"pullRequestPolicy,omitempty"`
	RuntimePolicy             map[string]interface{}      `json:"runtimePolicy,omitempty"`
	CleanupPolicy             map[string]interface{}      `json:"cleanupPolicy,omitempty"`
}

type IssueExecutionWorkspaceSettings struct {
	Mode              string                      `json:"mode,omitempty"`
	WorkspaceStrategy *ExecutionWorkspaceStrategy `json:"workspaceStrategy,omitempty"`
	WorkspaceRuntime  map[string]interface{}      `json:"workspaceRuntime,omitempty"`
}

func parseExecutionWorkspaceStrategy(raw interface{}) *ExecutionWorkspaceStrategy {
	parsed := parseJSONObject(raw)
	typ := readNonEmptyString(parsed["type"])
	switch typ {
	case "project_primary", "git_worktree", "adapter_managed", "cloud_sandbox":
	default:
		return nil
	}
	return &ExecutionWorkspaceStrategy{
		Type:              typ,
		BaseRef:           readNonEmptyString(parsed["baseRef"]),
		BranchTemplate:    readNonEmptyString(parsed["branchTemplate"]),
		WorktreeParentDir: readNonEmptyString(parsed["worktreeParentDir"]),
		ProvisionCommand:  readNonEmptyString(parsed["provisionCommand"]),
		TeardownCommand:   readNonEmptyString(parsed["teardownCommand"]),
	}
}

func normalizeProjectExecutionWorkspaceDefaultMode(value string) string {
	switch value {
	case "shared_workspace", "isolated_workspace", "operator_branch", "adapter_default":
		return value
	case "project_primary":
		return "shared_workspace"
	case "isolated":
		return "isolated_workspace"
	default:
		return ""
	}
}

func normalizeIssueExecutionWorkspaceMode(value string) string {
	switch value {
	case "inherit", "shared_workspace", "isolated_workspace", "operator_branch", "reuse_existing", "agent_default":
		return value
	case "project_primary":
		return "shared_workspace"
	case "isolated":
		return "isolated_workspace"
	default:
		return ""
	}
}

func ParseProjectExecutionWorkspacePolicy(raw interface{}) *ProjectExecutionWorkspacePolicy {
	parsed := parseJSONObject(raw)
	if len(parsed) == 0 {
		return nil
	}

	policy := &ProjectExecutionWorkspacePolicy{
		Enabled:                   false,
		DefaultMode:               normalizeProjectExecutionWorkspaceDefaultMode(readNonEmptyString(parsed["defaultMode"])),
		DefaultProjectWorkspaceID: readNonEmptyString(parsed["defaultProjectWorkspaceId"]),
		WorkspaceStrategy:         parseExecutionWorkspaceStrategy(parsed["workspaceStrategy"]),
		WorkspaceRuntime:          cloneMapRecord(parsed["workspaceRuntime"]),
		BranchPolicy:              cloneMapRecord(parsed["branchPolicy"]),
		PullRequestPolicy:         cloneMapRecord(parsed["pullRequestPolicy"]),
		RuntimePolicy:             cloneMapRecord(parsed["runtimePolicy"]),
		CleanupPolicy:             cloneMapRecord(parsed["cleanupPolicy"]),
	}
	if enabled, ok := readBool(parsed["enabled"]); ok {
		policy.Enabled = enabled
	}
	if allowIssueOverride, ok := readBool(parsed["allowIssueOverride"]); ok {
		policy.AllowIssueOverride = &allowIssueOverride
	}

	if !policy.Enabled &&
		policy.DefaultMode == "" &&
		policy.DefaultProjectWorkspaceID == "" &&
		policy.AllowIssueOverride == nil &&
		policy.WorkspaceStrategy == nil &&
		policy.WorkspaceRuntime == nil &&
		policy.BranchPolicy == nil &&
		policy.PullRequestPolicy == nil &&
		policy.RuntimePolicy == nil &&
		policy.CleanupPolicy == nil {
		return nil
	}

	return policy
}

func ParseIssueExecutionWorkspaceSettings(raw interface{}) *IssueExecutionWorkspaceSettings {
	parsed := parseJSONObject(raw)
	if len(parsed) == 0 {
		return nil
	}

	settings := &IssueExecutionWorkspaceSettings{
		Mode:              normalizeIssueExecutionWorkspaceMode(readNonEmptyString(parsed["mode"])),
		WorkspaceStrategy: parseExecutionWorkspaceStrategy(parsed["workspaceStrategy"]),
		WorkspaceRuntime:  cloneMapRecord(parsed["workspaceRuntime"]),
	}
	if settings.Mode == "" && settings.WorkspaceStrategy == nil && settings.WorkspaceRuntime == nil {
		return nil
	}
	return settings
}

func ResolveExecutionWorkspaceMode(projectPolicy *ProjectExecutionWorkspacePolicy, issueSettings *IssueExecutionWorkspaceSettings, legacyUseProjectWorkspace *bool) string {
	issueMode := ""
	if issueSettings != nil {
		issueMode = issueSettings.Mode
	}
	if issueMode != "" && issueMode != "inherit" && issueMode != "reuse_existing" {
		return issueMode
	}
	if projectPolicy != nil && projectPolicy.Enabled {
		switch projectPolicy.DefaultMode {
		case "isolated_workspace":
			return "isolated_workspace"
		case "operator_branch":
			return "operator_branch"
		case "adapter_default":
			return "agent_default"
		default:
			return "shared_workspace"
		}
	}
	if legacyUseProjectWorkspace != nil && !*legacyUseProjectWorkspace {
		return "agent_default"
	}
	return "shared_workspace"
}

func BuildExecutionWorkspaceAdapterConfig(
	agentConfig map[string]interface{},
	projectPolicy *ProjectExecutionWorkspacePolicy,
	issueSettings *IssueExecutionWorkspaceSettings,
	mode string,
	legacyUseProjectWorkspace *bool,
) map[string]interface{} {
	nextConfig := map[string]interface{}{}
	for k, v := range agentConfig {
		nextConfig[k] = v
	}

	projectHasPolicy := projectPolicy != nil && projectPolicy.Enabled
	issueHasWorkspaceOverrides := issueSettings != nil &&
		(issueSettings.Mode != "" || issueSettings.WorkspaceStrategy != nil || issueSettings.WorkspaceRuntime != nil)
	hasWorkspaceControl := projectHasPolicy || issueHasWorkspaceOverrides || (legacyUseProjectWorkspace != nil && !*legacyUseProjectWorkspace)

	if !hasWorkspaceControl {
		return nextConfig
	}

	if mode == "isolated_workspace" {
		strategy := issueSettingsStrategy(issueSettings)
		if strategy == nil && projectPolicy != nil {
			strategy = projectPolicy.WorkspaceStrategy
		}
		if strategy == nil {
			strategy = parseExecutionWorkspaceStrategy(nextConfig["workspaceStrategy"])
		}
		if strategy == nil {
			strategy = &ExecutionWorkspaceStrategy{Type: "git_worktree"}
		}
		nextConfig["workspaceStrategy"] = map[string]interface{}{
			"type": strategy.Type,
		}
		if strategy.BaseRef != "" {
			nextConfig["workspaceStrategy"].(map[string]interface{})["baseRef"] = strategy.BaseRef
		}
		if strategy.BranchTemplate != "" {
			nextConfig["workspaceStrategy"].(map[string]interface{})["branchTemplate"] = strategy.BranchTemplate
		}
		if strategy.WorktreeParentDir != "" {
			nextConfig["workspaceStrategy"].(map[string]interface{})["worktreeParentDir"] = strategy.WorktreeParentDir
		}
		if strategy.ProvisionCommand != "" {
			nextConfig["workspaceStrategy"].(map[string]interface{})["provisionCommand"] = strategy.ProvisionCommand
		}
		if strategy.TeardownCommand != "" {
			nextConfig["workspaceStrategy"].(map[string]interface{})["teardownCommand"] = strategy.TeardownCommand
		}
	} else {
		delete(nextConfig, "workspaceStrategy")
	}

	if mode == "agent_default" {
		delete(nextConfig, "workspaceRuntime")
	} else if issueSettings != nil && issueSettings.WorkspaceRuntime != nil {
		nextConfig["workspaceRuntime"] = cloneMapRecord(issueSettings.WorkspaceRuntime)
	} else if projectPolicy != nil && projectPolicy.WorkspaceRuntime != nil {
		nextConfig["workspaceRuntime"] = cloneMapRecord(projectPolicy.WorkspaceRuntime)
	}

	return nextConfig
}

func issueSettingsStrategy(issueSettings *IssueExecutionWorkspaceSettings) *ExecutionWorkspaceStrategy {
	if issueSettings == nil {
		return nil
	}
	return issueSettings.WorkspaceStrategy
}
