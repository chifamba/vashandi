package services

import (
	"context"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"gorm.io/gorm"
)

// runtimeServiceIdentityKey returns a deduplication key for a workspace runtime service row.
// Mirrors the TypeScript runtimeServiceIdentityKey function.
func runtimeServiceIdentityKey(row *models.WorkspaceRuntimeService) string {
	if row.ReuseKey != nil && *row.ReuseKey != "" {
		return *row.ReuseKey
	}
	scopeID := ""
	if row.ScopeID != nil {
		scopeID = *row.ScopeID
	}
	projectWorkspaceID := ""
	if row.ProjectWorkspaceID != nil {
		projectWorkspaceID = *row.ProjectWorkspaceID
	}
	executionWorkspaceID := ""
	if row.ExecutionWorkspaceID != nil {
		executionWorkspaceID = *row.ExecutionWorkspaceID
	}
	command := ""
	if row.Command != nil {
		command = *row.Command
	}
	cwd := ""
	if row.Cwd != nil {
		cwd = *row.Cwd
	}
	return row.ScopeType + ":" + scopeID + ":" + projectWorkspaceID + ":" + executionWorkspaceID + ":" + row.ServiceName + ":" + command + ":" + cwd
}

// SelectCurrentRuntimeServiceRows deduplicates a slice of runtime service rows,
// keeping only the first (most-recently-updated) row per identity key.
// The input slice must already be ordered by (updated_at DESC, created_at DESC).
func SelectCurrentRuntimeServiceRows(rows []models.WorkspaceRuntimeService) []models.WorkspaceRuntimeService {
	current := make(map[string]*models.WorkspaceRuntimeService)
	var keys []string
	for i := range rows {
		key := runtimeServiceIdentityKey(&rows[i])
		if _, exists := current[key]; !exists {
			current[key] = &rows[i]
			keys = append(keys, key)
		}
	}
	result := make([]models.WorkspaceRuntimeService, 0, len(keys))
	for _, k := range keys {
		result = append(result, *current[k])
	}
	return result
}

// ListCurrentRuntimeServicesForProjectWorkspaces fetches and deduplicates runtime
// service rows grouped by project workspace ID.
// Mirrors the TypeScript listCurrentRuntimeServicesForProjectWorkspaces function.
func ListCurrentRuntimeServicesForProjectWorkspaces(
	ctx context.Context,
	db *gorm.DB,
	companyID string,
	projectWorkspaceIDs []string,
) (map[string][]models.WorkspaceRuntimeService, error) {
	if len(projectWorkspaceIDs) == 0 {
		return make(map[string][]models.WorkspaceRuntimeService), nil
	}

	var rows []models.WorkspaceRuntimeService
	err := db.WithContext(ctx).
		Where("company_id = ? AND project_workspace_id IN ? AND scope_type = ?",
			companyID, projectWorkspaceIDs, "project_workspace").
		Order("updated_at DESC, created_at DESC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}

	grouped := make(map[string][]models.WorkspaceRuntimeService)
	for _, row := range rows {
		if row.ProjectWorkspaceID == nil {
			continue
		}
		grouped[*row.ProjectWorkspaceID] = append(grouped[*row.ProjectWorkspaceID], row)
	}

	result := make(map[string][]models.WorkspaceRuntimeService, len(grouped))
	for workspaceID, workspaceRows := range grouped {
		result[workspaceID] = SelectCurrentRuntimeServiceRows(workspaceRows)
	}
	return result, nil
}

// ListCurrentRuntimeServicesForExecutionWorkspaces fetches and deduplicates runtime
// service rows grouped by execution workspace ID.
// Mirrors the TypeScript listCurrentRuntimeServicesForExecutionWorkspaces function.
func ListCurrentRuntimeServicesForExecutionWorkspaces(
	ctx context.Context,
	db *gorm.DB,
	companyID string,
	executionWorkspaceIDs []string,
) (map[string][]models.WorkspaceRuntimeService, error) {
	if len(executionWorkspaceIDs) == 0 {
		return make(map[string][]models.WorkspaceRuntimeService), nil
	}

	var rows []models.WorkspaceRuntimeService
	err := db.WithContext(ctx).
		Where("company_id = ? AND execution_workspace_id IN ?",
			companyID, executionWorkspaceIDs).
		Order("updated_at DESC, created_at DESC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}

	grouped := make(map[string][]models.WorkspaceRuntimeService)
	for _, row := range rows {
		if row.ExecutionWorkspaceID == nil {
			continue
		}
		grouped[*row.ExecutionWorkspaceID] = append(grouped[*row.ExecutionWorkspaceID], row)
	}

	result := make(map[string][]models.WorkspaceRuntimeService, len(grouped))
	for workspaceID, workspaceRows := range grouped {
		result[workspaceID] = SelectCurrentRuntimeServiceRows(workspaceRows)
	}
	return result, nil
}

// ensure sort is not imported unnecessarily
