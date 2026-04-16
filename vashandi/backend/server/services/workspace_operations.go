package services

import (
	"context"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"gorm.io/gorm"
)

type WorkspaceOperationService struct {
	DB       *gorm.DB
	logStore WorkspaceOperationLogStore
}

func NewWorkspaceOperationService(db *gorm.DB) *WorkspaceOperationService {
	return &WorkspaceOperationService{
		DB:       db,
		logStore: GetWorkspaceOperationLogStore(),
	}
}

// NewWorkspaceOperationServiceWithLogStore creates a WorkspaceOperationService with a custom log store
func NewWorkspaceOperationServiceWithLogStore(db *gorm.DB, logStore WorkspaceOperationLogStore) *WorkspaceOperationService {
	return &WorkspaceOperationService{
		DB:       db,
		logStore: logStore,
	}
}

type WorkspaceOperationRecorder struct {
	service              *WorkspaceOperationService
	companyID            string
	heartbeatRunID       *string
	executionWorkspaceID *string
	createdIDs           []string
	logHandle            *WorkspaceOperationLogHandle
	stdoutExcerpt        string
	stderrExcerpt        string
}

func (s *WorkspaceOperationService) CreateRecorder(companyID string, heartbeatRunID *string, executionWorkspaceID *string) *WorkspaceOperationRecorder {
	return &WorkspaceOperationRecorder{
		service:              s,
		companyID:            companyID,
		heartbeatRunID:       heartbeatRunID,
		executionWorkspaceID: executionWorkspaceID,
		createdIDs:           make([]string, 0),
	}
}

// AttachExecutionWorkspaceID updates the execution workspace ID for all created operations
func (r *WorkspaceOperationRecorder) AttachExecutionWorkspaceID(ctx context.Context, executionWorkspaceID *string) error {
	r.executionWorkspaceID = executionWorkspaceID
	if executionWorkspaceID == nil || len(r.createdIDs) == 0 {
		return nil
	}
	return r.service.DB.WithContext(ctx).
		Model(&models.WorkspaceOperation{}).
		Where("id IN ?", r.createdIDs).
		Updates(map[string]interface{}{
			"execution_workspace_id": executionWorkspaceID,
			"updated_at":             time.Now(),
		}).Error
}

func (r *WorkspaceOperationRecorder) Begin(ctx context.Context, phase string, command *string) (*models.WorkspaceOperation, error) {
	now := time.Now()

	op := &models.WorkspaceOperation{
		CompanyID:            r.companyID,
		HeartbeatRunID:       r.heartbeatRunID,
		ExecutionWorkspaceID: r.executionWorkspaceID,
		Phase:                phase,
		Command:              command,
		Status:               "running",
		StartedAt:            now,
	}

	if err := r.service.DB.WithContext(ctx).Create(op).Error; err != nil {
		return nil, err
	}

	// Create the log handle with the operation ID
	handle, err := r.service.logStore.Begin(WorkspaceOperationLogBeginInput{
		CompanyID:   r.companyID,
		OperationID: op.ID,
	})
	if err == nil && handle != nil {
		r.logHandle = handle
		storeType := string(handle.Store)
		op.LogStore = &storeType
		op.LogRef = &handle.LogRef

		// Update the operation with log store info (best-effort, non-blocking)
		if updateErr := r.service.DB.WithContext(ctx).Model(&models.WorkspaceOperation{}).Where("id = ?", op.ID).Updates(map[string]interface{}{
			"log_store": storeType,
			"log_ref":   handle.LogRef,
		}).Error; updateErr != nil {
			// Log store metadata update failed but operation can continue
			// The log will still be written, just not tracked in the DB
			r.logHandle = nil
		}
	}

	r.createdIDs = append(r.createdIDs, op.ID)
	return op, nil
}

// AppendLog appends content to the operation log
func (r *WorkspaceOperationRecorder) AppendLog(stream, chunk string) error {
	if r.logHandle == nil || chunk == "" {
		return nil
	}

	// Append to excerpt
	if stream == "stdout" {
		r.stdoutExcerpt = appendExcerpt(r.stdoutExcerpt, chunk)
	} else if stream == "stderr" {
		r.stderrExcerpt = appendExcerpt(r.stderrExcerpt, chunk)
	}

	return r.service.logStore.Append(r.logHandle, WorkspaceOperationLogEvent{
		Stream: stream,
		Chunk:  chunk,
	})
}

// maxExcerptBytes is the maximum size of stdout/stderr excerpts stored in the database
const maxExcerptBytes = 4096

// appendExcerpt keeps the last maxExcerptBytes of text
func appendExcerpt(current, chunk string) string {
	result := current + chunk
	if len(result) > maxExcerptBytes {
		result = result[len(result)-maxExcerptBytes:]
	}
	return result
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

	// Finalize the log and get summary
	if r.logHandle != nil {
		summary, logErr := r.service.logStore.Finalize(r.logHandle)
		if logErr == nil && summary != nil {
			cols["log_bytes"] = summary.Bytes
			cols["log_sha256"] = summary.SHA256
			cols["log_compressed"] = summary.Compressed
		}
	}

	// Add excerpts
	if r.stdoutExcerpt != "" {
		cols["stdout_excerpt"] = r.stdoutExcerpt
	}
	if r.stderrExcerpt != "" {
		cols["stderr_excerpt"] = r.stderrExcerpt
	}

	return r.service.DB.WithContext(ctx).Model(&models.WorkspaceOperation{}).Where("id = ?", opID).Updates(cols).Error
}

// GetByID retrieves a workspace operation by ID
func (s *WorkspaceOperationService) GetByID(ctx context.Context, id string) (*models.WorkspaceOperation, error) {
	var op models.WorkspaceOperation
	if err := s.DB.WithContext(ctx).First(&op, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &op, nil
}

// ListForRun retrieves workspace operations for a run
func (s *WorkspaceOperationService) ListForRun(ctx context.Context, runID string, executionWorkspaceID *string) ([]models.WorkspaceOperation, error) {
	var operations []models.WorkspaceOperation
	query := s.DB.WithContext(ctx).Where("heartbeat_run_id = ?", runID)

	if executionWorkspaceID != nil {
		// Also include operations that don't have a run ID but are for the same workspace
		query = s.DB.WithContext(ctx).Where(
			"heartbeat_run_id = ? OR (execution_workspace_id = ? AND heartbeat_run_id IS NULL)",
			runID, *executionWorkspaceID,
		)
	}

	if err := query.Order("started_at ASC, created_at ASC, id ASC").Find(&operations).Error; err != nil {
		return nil, err
	}
	return operations, nil
}

// ListForExecutionWorkspace retrieves workspace operations for an execution workspace
func (s *WorkspaceOperationService) ListForExecutionWorkspace(ctx context.Context, executionWorkspaceID string) ([]models.WorkspaceOperation, error) {
	var operations []models.WorkspaceOperation
	if err := s.DB.WithContext(ctx).
		Where("execution_workspace_id = ?", executionWorkspaceID).
		Order("started_at DESC, created_at DESC").
		Find(&operations).Error; err != nil {
		return nil, err
	}
	return operations, nil
}

// ReadLog reads the log content for a workspace operation
func (s *WorkspaceOperationService) ReadLog(ctx context.Context, operationID string, opts *WorkspaceOperationLogReadOptions) (*WorkspaceOperationLogReadResult, error) {
	op, err := s.GetByID(ctx, operationID)
	if err != nil {
		return nil, err
	}
	if op == nil || op.LogStore == nil || op.LogRef == nil {
		return nil, ErrLogNotFound
	}

	return s.logStore.Read(&WorkspaceOperationLogHandle{
		Store:  WorkspaceOperationLogStoreType(*op.LogStore),
		LogRef: *op.LogRef,
	}, opts)
}
