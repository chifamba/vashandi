package services

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestReconcileOnStartupAdoptsPersistedLocalProcess(t *testing.T) {
	db := openWorkspaceRuntimeTestDB(t)
	tempHome := t.TempDir()
	t.Setenv("PAPERCLIP_HOME", tempHome)
	t.Setenv("PAPERCLIP_INSTANCE_ID", "test")

	cwd := t.TempDir()
	cmd := exec.Command("sh", "-lc", "sleep 5")
	cmd.Dir = cwd
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sleep process: %v", err)
	}
	t.Cleanup(func() {
		TerminateLocalService(cmd.Process.Pid, cmd.Process.Pid, 2000)
	})

	workspaceID := "ew-1"
	serviceID := "svc-1"
	reuseKey := "reuse-1"
	if err := db.Create(&models.WorkspaceRuntimeService{
		ID:                   serviceID,
		CompanyID:            "comp-1",
		ProjectID:            stringOrNil("proj-1"),
		ExecutionWorkspaceID: &workspaceID,
		ServiceName:          "sleepy",
		Status:               RuntimeServiceStatusRunning,
		Lifecycle:            "shared",
		ScopeType:            "execution_workspace",
		ScopeID:              &workspaceID,
		ReuseKey:             &reuseKey,
		Command:              stringOrNil("sleep 5"),
		Cwd:                  &cwd,
		Provider:             "local_process",
		StartedAt:            time.Now(),
		LastUsedAt:           time.Now(),
		HealthStatus:         "healthy",
	}).Error; err != nil {
		t.Fatalf("insert runtime service row: %v", err)
	}

	serviceKey := CreateLocalServiceKey(LocalServiceIdentityInput{
		ProfileKind:    "workspace-runtime",
		ServiceName:    "sleepy",
		Cwd:            cwd,
		Command:        "sleep 5",
		EnvFingerprint: computeEnvFingerprint(nil),
		Scope: map[string]interface{}{
			"projectWorkspaceId":   nil,
			"executionWorkspaceId": workspaceID,
		},
	})
	if err := WriteLocalServiceRegistryRecord(&LocalServiceRegistryRecord{
		Version:          1,
		ServiceKey:       serviceKey,
		ProfileKind:      "workspace-runtime",
		ServiceName:      "sleepy",
		Command:          "sleep 5",
		Cwd:              cwd,
		EnvFingerprint:   computeEnvFingerprint(nil),
		PID:              cmd.Process.Pid,
		ProcessGroupID:   intPtr(cmd.Process.Pid),
		Provider:         "local_process",
		RuntimeServiceID: &serviceID,
		ReuseKey:         &reuseKey,
		StartedAt:        time.Now().UTC().Format(time.RFC3339),
		LastSeenAt:       time.Now().UTC().Format(time.RFC3339),
		Metadata: map[string]interface{}{
			"executionWorkspaceId": workspaceID,
		},
	}); err != nil {
		t.Fatalf("write registry record: %v", err)
	}
	t.Cleanup(func() {
		_ = RemoveLocalServiceRegistryRecord(serviceKey)
	})

	manager := NewWorkspaceRuntimeManager(db)
	result, err := ReconcileOnStartup(context.Background(), db, manager)
	if err != nil {
		t.Fatalf("reconcile on startup: %v", err)
	}
	if result.Adopted != 1 || result.Stopped != 0 {
		t.Fatalf("unexpected reconcile result: %#v", result)
	}
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	if manager.byID[serviceID] == nil {
		t.Fatal("expected adopted runtime service to be loaded into manager state")
	}
}

func TestRehydratePersistentServicesStartsDesiredProjectWorkspaceService(t *testing.T) {
	db := openWorkspaceRuntimeTestDB(t)
	tempHome := t.TempDir()
	t.Setenv("PAPERCLIP_HOME", tempHome)
	t.Setenv("PAPERCLIP_INSTANCE_ID", "test")

	cwd := t.TempDir()
	metadata, err := json.Marshal(map[string]interface{}{
		"runtimeConfig": map[string]interface{}{
			"desiredState": "running",
			"workspaceRuntime": map[string]interface{}{
				"services": []interface{}{
					map[string]interface{}{
						"name":      "sleepy",
						"command":   "sleep 5",
						"lifecycle": "shared",
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal workspace metadata: %v", err)
	}
	projectWorkspace := models.ProjectWorkspace{
		ID:         "pw-1",
		CompanyID:  "comp-1",
		ProjectID:  "proj-1",
		Name:       "Primary",
		Status:     "active",
		Mode:       "project_primary",
		SourceType: "local_path",
		Cwd:        &cwd,
		Metadata:   metadata,
	}
	if err := db.Create(&projectWorkspace).Error; err != nil {
		t.Fatalf("insert project workspace: %v", err)
	}

	manager := NewWorkspaceRuntimeManager(db)
	result, err := RehydratePersistentServices(context.Background(), db, manager)
	if err != nil {
		t.Fatalf("rehydrate persistent services: %v", err)
	}
	if result.Restarted != 1 || result.Failed != 0 {
		t.Fatalf("unexpected rehydrate result: %#v", result)
	}

	if _, err := manager.StopRuntimeServicesForProjectWorkspace(context.Background(), projectWorkspace.ID); err != nil {
		t.Fatalf("stop rehydrated services: %v", err)
	}
}

func openWorkspaceRuntimeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "runtime.sqlite")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	statements := []string{
		`CREATE TABLE workspace_runtime_services (
			id TEXT PRIMARY KEY,
			company_id TEXT NOT NULL,
			project_id TEXT,
			project_workspace_id TEXT,
			execution_workspace_id TEXT,
			issue_id TEXT,
			scope_type TEXT NOT NULL,
			scope_id TEXT,
			service_name TEXT NOT NULL,
			status TEXT NOT NULL,
			lifecycle TEXT NOT NULL,
			reuse_key TEXT,
			command TEXT,
			cwd TEXT,
			port INTEGER,
			url TEXT,
			provider TEXT NOT NULL,
			provider_ref TEXT,
			owner_agent_id TEXT,
			started_by_run_id TEXT,
			last_used_at DATETIME NOT NULL,
			started_at DATETIME NOT NULL,
			stopped_at DATETIME,
			stop_policy TEXT,
			health_status TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE project_workspaces (
			id TEXT PRIMARY KEY,
			company_id TEXT NOT NULL,
			project_id TEXT NOT NULL,
			name TEXT NOT NULL,
			status TEXT NOT NULL,
			mode TEXT NOT NULL,
			source_type TEXT NOT NULL,
			cwd TEXT,
			repo_url TEXT,
			repo_ref TEXT,
			default_ref TEXT,
			visibility TEXT,
			setup_command TEXT,
			cleanup_command TEXT,
			remote_provider TEXT,
			remote_workspace_ref TEXT,
			shared_workspace_key TEXT,
			metadata TEXT,
			is_primary BOOLEAN NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE execution_workspaces (
			id TEXT PRIMARY KEY,
			company_id TEXT NOT NULL,
			project_id TEXT NOT NULL,
			project_workspace_id TEXT,
			source_issue_id TEXT,
			mode TEXT NOT NULL,
			strategy_type TEXT NOT NULL,
			name TEXT NOT NULL,
			status TEXT NOT NULL,
			cwd TEXT,
			repo_url TEXT,
			base_ref TEXT,
			branch_name TEXT,
			provider_type TEXT NOT NULL,
			provider_ref TEXT,
			derived_from_execution_workspace_id TEXT,
			last_used_at DATETIME NOT NULL,
			opened_at DATETIME NOT NULL,
			closed_at DATETIME,
			cleanup_eligible_at DATETIME,
			cleanup_reason TEXT,
			metadata TEXT,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
	}
	for _, stmt := range statements {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("migrate schema: %v", err)
		}
	}
	return db
}

func intPtr(v int) *int {
	return &v
}
