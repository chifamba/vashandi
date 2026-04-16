package services

import (
	"context"
	"os"
	"strings"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"github.com/chifamba/vashandi/vashandi/backend/shared"
)

type ResolvedWorkspaceForRun struct {
	CWD         string
	Source      string
	ProjectID   string
	WorkspaceID string
	RepoURL     string
	RepoRef     string
	Warnings    []string
}

func (s *HeartbeatService) resolveWorkspaceForRun(ctx context.Context, run *models.HeartbeatRun, contextSnapshot map[string]interface{}, previousSessionParams map[string]interface{}) (*ResolvedWorkspaceForRun, error) {
	issueID := readNonEmptyString(contextSnapshot["issueId"])
	contextProjectID := readNonEmptyString(contextSnapshot["projectId"])
	contextWorkspaceID := readNonEmptyString(contextSnapshot["projectWorkspaceId"])
	resolvedProjectID := contextProjectID
	preferredWorkspaceID := contextWorkspaceID
	if issueID != "" {
		var issue models.Issue
		if err := s.DB.WithContext(ctx).Where("id = ? AND company_id = ?", issueID, run.CompanyID).First(&issue).Error; err == nil {
			resolvedProjectID = firstNonEmpty(derefString(issue.ProjectID), resolvedProjectID)
			preferredWorkspaceID = firstNonEmpty(derefString(issue.ProjectWorkspaceID), preferredWorkspaceID)
		}
	}

	if resolvedProjectID != "" {
		var workspaces []models.ProjectWorkspace
		if err := s.DB.WithContext(ctx).
			Where("company_id = ? AND project_id = ?", run.CompanyID, resolvedProjectID).
			Order("created_at asc, id asc").
			Find(&workspaces).Error; err == nil {
			workspaces = prioritizeProjectWorkspaces(workspaces, preferredWorkspaceID)
			for _, workspace := range workspaces {
				if workspace.Cwd == nil || strings.TrimSpace(*workspace.Cwd) == "" {
					continue
				}
				if info, err := os.Stat(*workspace.Cwd); err == nil && info.IsDir() {
					return &ResolvedWorkspaceForRun{
						CWD:         *workspace.Cwd,
						Source:      "project_primary",
						ProjectID:   resolvedProjectID,
						WorkspaceID: workspace.ID,
						RepoURL:     derefString(workspace.RepoURL),
						RepoRef:     derefString(workspace.RepoRef),
					}, nil
				}
			}
			repoURL := ""
			if len(workspaces) > 0 {
				repoURL = derefString(workspaces[0].RepoURL)
			}
			if repoURL == "" {
				var project models.Project
				if err := s.DB.WithContext(ctx).Where("id = ?", resolvedProjectID).First(&project).Error; err == nil {
					projectPolicy := parseJSONObject(project.ExecutionWorkspacePolicy)
					repoURL = readNonEmptyString(projectPolicy["repoUrl"])
				}
			}
			strategy := StrategyPrimary
			if readNonEmptyString(contextSnapshot["workspaceStrategy"]) == "git_worktree" {
				strategy = StrategyWorktree
			}
			cwd, err := s.Workspaces.RealizeWorkspace(ctx, run.CompanyID, resolvedProjectID, repoURL, RealizeOptions{
				Strategy: strategy,
				RunID:    run.ID,
			})
			if err == nil {
				return &ResolvedWorkspaceForRun{
					CWD:         cwd,
					Source:      "project_primary",
					ProjectID:   resolvedProjectID,
					WorkspaceID: preferredWorkspaceID,
					RepoURL:     repoURL,
				}, nil
			}
		}
	}

	sessionCWD := readNonEmptyString(previousSessionParams["cwd"])
	if sessionCWD != "" {
		if info, err := os.Stat(sessionCWD); err == nil && info.IsDir() {
			return &ResolvedWorkspaceForRun{
				CWD:         sessionCWD,
				Source:      "task_session",
				ProjectID:   resolvedProjectID,
				WorkspaceID: readNonEmptyString(previousSessionParams["workspaceId"]),
				RepoURL:     readNonEmptyString(previousSessionParams["repoUrl"]),
				RepoRef:     readNonEmptyString(previousSessionParams["repoRef"]),
			}, nil
		}
	}

	cwd := shared.ResolveDefaultAgentWorkspaceDir(run.AgentID)
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		return nil, err
	}
	warnings := []string{}
	if sessionCWD != "" {
		warnings = append(warnings, "Saved session workspace is not available; using agent home fallback.")
	} else if resolvedProjectID != "" {
		warnings = append(warnings, "No project workspace directory is available; using agent home fallback.")
	}
	return &ResolvedWorkspaceForRun{
		CWD:       cwd,
		Source:    "agent_home",
		ProjectID: resolvedProjectID,
		Warnings:  warnings,
	}, nil
}

func prioritizeProjectWorkspaces(workspaces []models.ProjectWorkspace, preferredWorkspaceID string) []models.ProjectWorkspace {
	if preferredWorkspaceID == "" {
		return workspaces
	}
	out := make([]models.ProjectWorkspace, 0, len(workspaces))
	for _, workspace := range workspaces {
		if workspace.ID == preferredWorkspaceID {
			out = append(out, workspace)
		}
	}
	for _, workspace := range workspaces {
		if workspace.ID != preferredWorkspaceID {
			out = append(out, workspace)
		}
	}
	return out
}
