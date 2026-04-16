package services

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"gorm.io/gorm"
)

type WorkspaceRuntimeReconcileResult struct {
	Reconciled int
	Adopted    int
	Stopped    int
}

type WorkspaceRuntimeRehydrateResult struct {
	Restarted int
	Failed    int
}

func ReconcileOnStartup(ctx context.Context, db *gorm.DB, manager *WorkspaceRuntimeManager) (*WorkspaceRuntimeReconcileResult, error) {
	if db == nil || manager == nil {
		return &WorkspaceRuntimeReconcileResult{}, nil
	}

	var rows []models.WorkspaceRuntimeService
	if err := db.WithContext(ctx).
		Where("provider = ? AND status IN (?, ?)", "local_process", RuntimeServiceStatusStarting, RuntimeServiceStatusRunning).
		Find(&rows).Error; err != nil {
		return nil, err
	}

	result := &WorkspaceRuntimeReconcileResult{Reconciled: len(rows)}
	for _, row := range rows {
		adopted, err := FindLocalServiceRegistryRecordByRuntimeServiceID(struct {
			RuntimeServiceID string
			ProfileKind      string
		}{
			RuntimeServiceID: row.ID,
			ProfileKind:      "workspace-runtime",
		})
		if err != nil || adopted == nil {
			now := time.Now()
			_ = db.WithContext(ctx).Model(&models.WorkspaceRuntimeService{}).
				Where("id = ?", row.ID).
				Updates(map[string]interface{}{
					"status":        RuntimeServiceStatusStopped,
					"health_status": "unknown",
					"stopped_at":    now,
					"last_used_at":  now,
					"updated_at":    now,
				}).Error
			result.Stopped++
			continue
		}

		healthStatus := "healthy"
		url := derefString(adopted.URL)
		if url == "" {
			url = derefString(row.URL)
		}
		if url != "" && !isRuntimeServiceURLHealthy(ctx, url) {
			_ = RemoveLocalServiceRegistryRecord(adopted.ServiceKey)
			now := time.Now()
			_ = db.WithContext(ctx).Model(&models.WorkspaceRuntimeService{}).
				Where("id = ?", row.ID).
				Updates(map[string]interface{}{
					"status":        RuntimeServiceStatusStopped,
					"health_status": "unknown",
					"stopped_at":    now,
					"last_used_at":  now,
					"updated_at":    now,
				}).Error
			result.Stopped++
			continue
		}

		var processGroupID int
		if adopted.ProcessGroupID != nil {
			processGroupID = *adopted.ProcessGroupID
		}
		providerRef := row.ProviderRef
		if providerRef == nil {
			ref := derefString(adopted.RuntimeServiceID)
			if ref == "" {
				ref = adopted.ServiceKey
			}
			providerRef = &ref
		}
		if adopted.PID > 0 {
			pid := intToStringPtr(adopted.PID)
			providerRef = pid
		}

		rec := &RuntimeServiceRecord{
			ID:                   row.ID,
			CompanyID:            row.CompanyID,
			ProjectID:            row.ProjectID,
			ProjectWorkspaceID:   row.ProjectWorkspaceID,
			ExecutionWorkspaceID: row.ExecutionWorkspaceID,
			IssueID:              row.IssueID,
			ServiceName:          row.ServiceName,
			Status:               RuntimeServiceStatusRunning,
			Lifecycle:            row.Lifecycle,
			ScopeType:            row.ScopeType,
			ScopeID:              row.ScopeID,
			ReuseKey:             row.ReuseKey,
			Command:              derefString(row.Command),
			Cwd:                  derefString(row.Cwd),
			Port:                 firstIntPtr(adopted.Port, row.Port),
			URL:                  firstStringPtr(adopted.URL, row.URL),
			Provider:             "local_process",
			ProviderRef:          providerRef,
			OwnerAgentID:         row.OwnerAgentID,
			StartedByRunID:       row.StartedByRunID,
			LastUsedAt:           time.Now().UTC().Format(time.RFC3339),
			StartedAt:            row.StartedAt.UTC().Format(time.RFC3339),
			StopPolicy:           parseJSONObject(row.StopPolicy),
			HealthStatus:         healthStatus,
			Reused:               true,
			pid:                  adopted.PID,
			processGroupID:       processGroupID,
			serviceKey:           adopted.ServiceKey,
			profileKind:          "workspace-runtime",
		}
		manager.mu.Lock()
		manager.byID[rec.ID] = rec
		if rec.ReuseKey != nil {
			manager.byReuseKey[*rec.ReuseKey] = rec.ID
		}
		manager.mu.Unlock()
		_, _ = TouchLocalServiceRegistryRecord(adopted.ServiceKey, &LocalServiceRegistryRecord{
			RuntimeServiceID: &row.ID,
		})
		_ = manager.persistRecord(ctx, rec)
		result.Adopted++
	}

	return result, nil
}

func RehydratePersistentServices(ctx context.Context, db *gorm.DB, manager *WorkspaceRuntimeManager) (*WorkspaceRuntimeRehydrateResult, error) {
	if db == nil || manager == nil {
		return &WorkspaceRuntimeRehydrateResult{}, nil
	}

	result := &WorkspaceRuntimeRehydrateResult{}

	var projectWorkspaces []models.ProjectWorkspace
	if err := db.WithContext(ctx).Find(&projectWorkspaces).Error; err != nil {
		return nil, err
	}
	for _, workspace := range projectWorkspaces {
		runtimeConfig := ParseProjectWorkspaceRuntimeConfig(parseJSONObject(workspace.Metadata))
		if runtimeConfig == nil || runtimeConfig.DesiredState != "running" || runtimeConfig.WorkspaceRuntime == nil || workspace.Cwd == nil || strings.TrimSpace(*workspace.Cwd) == "" {
			continue
		}
		refs, err := manager.StartRuntimeServices(ctx, StartRuntimeServicesInput{
			CompanyID:          workspace.CompanyID,
			ProjectID:          &workspace.ProjectID,
			ProjectWorkspaceID: &workspace.ID,
			WorkspaceCwd:       *workspace.Cwd,
			RuntimeConfig:      runtimeConfig.WorkspaceRuntime,
		}, nil)
		if err != nil {
			result.Failed++
			continue
		}
		for _, ref := range refs {
			if !ref.Reused {
				result.Restarted++
			}
		}
	}

	var executionWorkspaces []models.ExecutionWorkspace
	if err := db.WithContext(ctx).
		Where("status IN (?, ?, ?, ?)", "active", "idle", "in_review", "cleanup_failed").
		Find(&executionWorkspaces).Error; err != nil {
		return nil, err
	}
	for _, workspace := range executionWorkspaces {
		config := parseExecutionWorkspaceRuntimeConfig(parseJSONObject(workspace.Metadata))
		if config == nil || config.DesiredState != "running" || config.WorkspaceRuntime == nil || workspace.Cwd == nil || strings.TrimSpace(*workspace.Cwd) == "" {
			continue
		}
		refs, err := manager.StartRuntimeServices(ctx, StartRuntimeServicesInput{
			CompanyID:            workspace.CompanyID,
			ProjectID:            &workspace.ProjectID,
			ProjectWorkspaceID:   workspace.ProjectWorkspaceID,
			ExecutionWorkspaceID: &workspace.ID,
			IssueID:              workspace.SourceIssueID,
			WorkspaceCwd:         *workspace.Cwd,
			RuntimeConfig:        config.WorkspaceRuntime,
		}, nil)
		if err != nil {
			result.Failed++
			continue
		}
		for _, ref := range refs {
			if !ref.Reused {
				result.Restarted++
			}
		}
	}

	return result, nil
}

type executionWorkspaceRuntimeConfig struct {
	WorkspaceRuntime map[string]interface{}
	DesiredState     string
}

func parseExecutionWorkspaceRuntimeConfig(metadata map[string]interface{}) *executionWorkspaceRuntimeConfig {
	config := nestedObject(metadata, "config")
	if len(config) == 0 {
		return nil
	}
	out := &executionWorkspaceRuntimeConfig{
		WorkspaceRuntime: cloneMapRecord(config["workspaceRuntime"]),
		DesiredState:     readNonEmptyString(config["desiredState"]),
	}
	if out.WorkspaceRuntime == nil && out.DesiredState == "" {
		return nil
	}
	return out
}

func isRuntimeServiceURLHealthy(ctx context.Context, url string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func firstIntPtr(values ...*int) *int {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func firstStringPtr(values ...*string) *string {
	for _, value := range values {
		if value != nil && strings.TrimSpace(*value) != "" {
			return value
		}
	}
	return nil
}

func intToStringPtr(value int) *string {
	if value <= 0 {
		return nil
	}
	text := fmt.Sprintf("%d", value)
	return &text
}
