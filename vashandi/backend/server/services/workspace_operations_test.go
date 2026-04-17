package services

import (
	"context"
	"errors"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
)

func setupWorkspaceOpsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&workspace_ops_test=1"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.Exec("DROP TABLE IF EXISTS workspace_operations")
	db.Exec(`CREATE TABLE workspace_operations (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		execution_workspace_id text,
		heartbeat_run_id text,
		phase text NOT NULL,
		command text,
		cwd text,
		status text NOT NULL DEFAULT 'running',
		exit_code integer,
		error text,
		log_store text,
		log_ref text,
		log_bytes integer,
		log_sha256 text,
		log_compressed boolean NOT NULL DEFAULT 0,
		stdout_excerpt text,
		stderr_excerpt text,
		metadata text,
		started_at datetime DEFAULT CURRENT_TIMESTAMP,
		finished_at datetime,
		created_at datetime DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)
	return db
}

func TestWorkspaceOperationService_CreateRecorder(t *testing.T) {
	db := setupWorkspaceOpsTestDB(t)
	svc := NewWorkspaceOperationService(db)

	runID := "run-1"
	wsID := "ws-1"
	recorder := svc.CreateRecorder("comp-1", &runID, &wsID)

	if recorder == nil {
		t.Fatal("expected non-nil recorder")
	}
	if recorder.companyID != "comp-1" {
		t.Errorf("expected companyID 'comp-1', got %q", recorder.companyID)
	}
	if *recorder.heartbeatRunID != "run-1" {
		t.Errorf("expected heartbeatRunID 'run-1', got %q", *recorder.heartbeatRunID)
	}
	if *recorder.executionWorkspaceID != "ws-1" {
		t.Errorf("expected executionWorkspaceID 'ws-1', got %q", *recorder.executionWorkspaceID)
	}
}

func TestWorkspaceOperationRecorder_Begin(t *testing.T) {
	db := setupWorkspaceOpsTestDB(t)
	svc := NewWorkspaceOperationService(db)

	runID := "run-1"
	wsID := "ws-1"
	recorder := svc.CreateRecorder("comp-1", &runID, &wsID)

	cmd := "git checkout main"
	op, err := recorder.Begin(context.Background(), "checkout", &cmd)
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}

	if op.Phase != "checkout" {
		t.Errorf("expected phase 'checkout', got %q", op.Phase)
	}
	if op.Status != "running" {
		t.Errorf("expected status 'running', got %q", op.Status)
	}
	if op.CompanyID != "comp-1" {
		t.Errorf("expected CompanyID 'comp-1', got %q", op.CompanyID)
	}
	if op.Command == nil || *op.Command != "git checkout main" {
		t.Errorf("expected command 'git checkout main', got %v", op.Command)
	}

	if op.ID != "" {
		var count int64
		db.Model(&models.WorkspaceOperation{}).Where("id = ?", op.ID).Count(&count)
		if count != 1 {
			t.Errorf("expected 1 operation in DB, found %d", count)
		}
	}
}

func TestWorkspaceOperationRecorder_BeginWithNilCommand(t *testing.T) {
	db := setupWorkspaceOpsTestDB(t)
	svc := NewWorkspaceOperationService(db)

	recorder := svc.CreateRecorder("comp-1", nil, nil)

	op, err := recorder.Begin(context.Background(), "setup", nil)
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}

	if op.Command != nil {
		t.Errorf("expected nil command, got %v", op.Command)
	}
	if op.HeartbeatRunID != nil {
		t.Errorf("expected nil heartbeatRunID, got %v", op.HeartbeatRunID)
	}
}

func TestWorkspaceOperationRecorder_FinishSuccess(t *testing.T) {
	db := setupWorkspaceOpsTestDB(t)
	svc := NewWorkspaceOperationService(db)

	runID := "run-1"
	recorder := svc.CreateRecorder("comp-1", &runID, nil)

	cmd := "npm install"
	op, err := recorder.Begin(context.Background(), "install", &cmd)
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}


	// Finish successfully
	if err := recorder.Finish(context.Background(), op.ID, 0, nil); err != nil {
		t.Fatalf("Finish failed: %v", err)
	}

	// Verify the operation was updated
	var updated models.WorkspaceOperation
	db.First(&updated, "id = ?", op.ID)

	if updated.Status != "succeeded" {
		t.Errorf("expected status 'succeeded', got %q", updated.Status)
	}
	if updated.ExitCode == nil || *updated.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %v", updated.ExitCode)
	}
	if updated.FinishedAt == nil {
		t.Error("expected FinishedAt to be set")
	}
}

func TestWorkspaceOperationRecorder_FinishWithError(t *testing.T) {
	db := setupWorkspaceOpsTestDB(t)
	svc := NewWorkspaceOperationService(db)

	recorder := svc.CreateRecorder("comp-1", nil, nil)

	cmd := "make build"
	op, err := recorder.Begin(context.Background(), "build", &cmd)
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}


	// Finish with error
	buildErr := errors.New("compilation failed")
	if err := recorder.Finish(context.Background(), op.ID, 1, buildErr); err != nil {
		t.Fatalf("Finish failed: %v", err)
	}

	var updated models.WorkspaceOperation
	db.First(&updated, "id = ?", op.ID)

	if updated.Status != "failed" {
		t.Errorf("expected status 'failed', got %q", updated.Status)
	}
	if updated.ExitCode == nil || *updated.ExitCode != 1 {
		t.Errorf("expected exit code 1, got %v", updated.ExitCode)
	}
}

func TestWorkspaceOperationRecorder_MultipleOperations(t *testing.T) {
	db := setupWorkspaceOpsTestDB(t)
	svc := NewWorkspaceOperationService(db)

	recorder := svc.CreateRecorder("comp-1", nil, nil)

	op1, _ := recorder.Begin(context.Background(), "checkout", nil)
	recorder.Finish(context.Background(), op1.ID, 0, nil)

	op2, _ := recorder.Begin(context.Background(), "build", nil)
	recorder.Finish(context.Background(), op2.ID, 0, nil)

	op3, _ := recorder.Begin(context.Background(), "test", nil)
	recorder.Finish(context.Background(), op3.ID, 1, errors.New("tests failed"))

	// Verify 3 operations stored
	var count int64
	db.Model(&models.WorkspaceOperation{}).Where("company_id = ?", "comp-1").Count(&count)
	if count != 3 {
		t.Errorf("expected 3 operations, found %d", count)
	}
}
