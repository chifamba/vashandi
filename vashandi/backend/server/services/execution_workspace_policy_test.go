package services

import "testing"

func TestParseProjectExecutionWorkspacePolicyAndResolveMode(t *testing.T) {
	policy := ParseProjectExecutionWorkspacePolicy(map[string]interface{}{
		"enabled":     true,
		"defaultMode": "isolated_workspace",
		"workspaceStrategy": map[string]interface{}{
			"type":             "git_worktree",
			"branchTemplate":   "{{issue.identifier}}-{{slug}}",
			"provisionCommand": "./scripts/provision.sh",
		},
		"workspaceRuntime": map[string]interface{}{
			"services": []interface{}{
				map[string]interface{}{"name": "api"},
			},
		},
	})
	if policy == nil {
		t.Fatal("expected policy to be parsed")
	}
	if !policy.Enabled {
		t.Fatal("expected policy to be enabled")
	}
	if policy.WorkspaceStrategy == nil || policy.WorkspaceStrategy.Type != "git_worktree" {
		t.Fatalf("expected git worktree strategy, got %#v", policy.WorkspaceStrategy)
	}
	if got := ResolveExecutionWorkspaceMode(policy, nil, nil); got != "isolated_workspace" {
		t.Fatalf("expected isolated mode, got %q", got)
	}
}

func TestBuildExecutionWorkspaceAdapterConfigPrefersIssueOverrides(t *testing.T) {
	config := BuildExecutionWorkspaceAdapterConfig(
		map[string]interface{}{
			"workspaceStrategy": map[string]interface{}{"type": "project_primary"},
			"workspaceRuntime":  map[string]interface{}{"services": []interface{}{}},
		},
		&ProjectExecutionWorkspacePolicy{
			Enabled: true,
			WorkspaceStrategy: &ExecutionWorkspaceStrategy{
				Type:           "git_worktree",
				BranchTemplate: "{{issue.identifier}}-{{slug}}",
			},
		},
		&IssueExecutionWorkspaceSettings{
			Mode: "isolated_workspace",
			WorkspaceStrategy: &ExecutionWorkspaceStrategy{
				Type:              "git_worktree",
				WorktreeParentDir: ".paperclip/custom",
			},
			WorkspaceRuntime: map[string]interface{}{
				"services": []interface{}{map[string]interface{}{"name": "worker"}},
			},
		},
		"isolated_workspace",
		nil,
	)

	strategy, ok := config["workspaceStrategy"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected workspace strategy map, got %#v", config["workspaceStrategy"])
	}
	if got := strategy["type"]; got != "git_worktree" {
		t.Fatalf("expected git_worktree strategy type, got %#v", got)
	}
	if got := strategy["worktreeParentDir"]; got != ".paperclip/custom" {
		t.Fatalf("expected issue override worktree parent, got %#v", got)
	}
	runtimeConfig, ok := config["workspaceRuntime"].(map[string]interface{})
	if !ok || len(runtimeConfig) == 0 {
		t.Fatalf("expected workspace runtime override, got %#v", config["workspaceRuntime"])
	}
}
