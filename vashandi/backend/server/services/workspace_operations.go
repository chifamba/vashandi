package services

import (
	"context"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"gorm.io/gorm"
)

type WorkspaceOperationService struct {
	DB *gorm.DB
}

func NewWorkspaceOperationService(db *gorm.DB) *WorkspaceOperationService {
	return &WorkspaceOperationService{DB: db}
}

type WorkspaceOperationRecorder struct {
	service              *WorkspaceOperationService
	companyID            string
	heartbeatRunID       *string
	executionWorkspaceID *string
}

func (s *WorkspaceOperationService) CreateRecorder(companyID string, heartbeatRunID *string, executionWorkspaceID *string) *WorkspaceOperationRecorder {
	return &WorkspaceOperationRecorder{
		service:              s,
		companyID:            companyID,
		heartbeatRunID:       heartbeatRunID,
		executionWorkspaceID: executionWorkspaceID,
	}
}

func (r *WorkspaceOperationRecorder) Begin(ctx context.Context, phase string, command *string) (*models.WorkspaceOperation, error) {
	op := &models.WorkspaceOperation{
		CompanyID:            r.companyID,
		HeartbeatRunID:       r.heartbeatRunID,
		ExecutionWorkspaceID: r.executionWorkspaceID,
		Phase:                phase,
		Command:              command,
		Status:               "running",
		StartedAt:            time.Now(),
	}

	if err := r.service.DB.WithContext(ctx).Create(op).Error; err != nil {
		return nil, err
	}
	return op, nil
}

func (r *WorkspaceOperationRecorder) Finish(ctx context.Context, opID string, exitCode int, err error) error {
	status := "succeeded"
	if err != nil {
		status = "failed"
	}

	finishedAt := time.Now()
	cols := map[string]interface{}{
		"status":      status,
		"exit_code":   exitCode,
		"finished_at": &finishedAt,
		"updated_at":  finishedAt,
	}

	if err != nil {
		msg := err.Error()
		cols["error"] = &msg
	}

	return r.service.DB.WithContext(ctx).Model(&models.WorkspaceOperation{}).Where("id = ?", opID).Updates(cols).Error
}
