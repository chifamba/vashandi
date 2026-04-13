package services

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"github.com/chifamba/vashandi/vashandi/backend/shared"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// AgentRunner defines the interface for executing an agent run.
// This abstraction allows for in-process execution (LocalRunner) or
// remote execution via microservices/sidecars in the future.
type AgentRunner interface {
	Execute(ctx context.Context, run *models.HeartbeatRun, env map[string]string) error
}

type HeartbeatService struct {
	DB         *gorm.DB
	Secrets    *SecretService
	Runner     AgentRunner
	Logs       *RunLogStore
	Costs      *CostService
	Workspaces *WorkspaceService
	Activity   *ActivityService
}

func NewHeartbeatService(db *gorm.DB, secrets *SecretService, activity *ActivityService, runner AgentRunner) *HeartbeatService {
	logStore := NewRunLogStore("")
	costSvc := NewCostService(db)
	workspaceSvc := NewWorkspaceService()
	if runner == nil {
		runner = &LocalRunner{Logs: logStore}
	}
	return &HeartbeatService{
		DB:         db,
		Secrets:    secrets,
		Runner:     runner,
		Logs:       logStore,
		Costs:      costSvc,
		Workspaces: workspaceSvc,
		Activity:   activity,
	}
}

// Wakeup triggers an agent run.
func (s *HeartbeatService) Wakeup(ctx context.Context, companyID, agentID string, opts WakeupOptions) (*models.HeartbeatRun, error) {
	var agent models.Agent
	if err := s.DB.WithContext(ctx).Where("id = ? AND company_id = ?", agentID, companyID).First(&agent).Error; err != nil {
		return nil, fmt.Errorf("agent not found: %w", err)
	}

	// Create the run record
	contextJSON, _ := json.Marshal(opts.Context)
	run := &models.HeartbeatRun{
		CompanyID:        companyID,
		AgentID:          agentID,
		InvocationSource: opts.Source,
		TriggerDetail:    &opts.TriggerDetail,
		Status:           "queued",
		ContextSnapshot:  datatypes.JSON(contextJSON),
	}

	if err := s.DB.WithContext(ctx).Create(run).Error; err != nil {
		return nil, fmt.Errorf("failed to create heartbeat run: %w", err)
	}

	// Log activity
	if s.Activity != nil {
		_, _ = s.Activity.Log(ctx, LogEntry{
			CompanyID:  companyID,
			ActorType:  "system",
			ActorID:    "system",
			Action:     "heartbeat.wakeup",
			EntityType: "agent",
			EntityID:   agentID,
			AgentID:    &agentID,
			RunID:      &run.ID,
			Details: map[string]interface{}{
				"source": opts.Source,
			},
		})
	}

	// Initialize log handle
	handle, err := s.Logs.Begin(companyID, agentID, run.ID)
	if err == nil {
		run.LogStore = &handle.Store
		run.LogRef = &handle.LogRef
		s.DB.WithContext(ctx).Save(run)
	}

	// Trigger execution (asynchronously for Phase 1)
	go func() {
		err := s.StartRun(context.Background(), run.ID)
		if err != nil {
			fmt.Printf("Run %s failed: %v\n", run.ID, err)
		}
	}()

	return run, nil
}

// StartRun handles the setup and execution of a specific run.
func (s *HeartbeatService) StartRun(ctx context.Context, runID string) error {
	var run models.HeartbeatRun
	if err := s.DB.WithContext(ctx).Preload("Agent").Where("id = ?", runID).First(&run).Error; err != nil {
		return err
	}

	// Update status to starting
	now := time.Now()
	run.Status = "starting"
	run.StartedAt = &now
	s.DB.WithContext(ctx).Save(&run)

	// Resolve environment from agent config and secrets
	runtimeConfig := make(map[string]interface{})
	_ = json.Unmarshal(run.Agent.RuntimeConfig, &runtimeConfig)
	
	env := make(map[string]string)
	if envInput, ok := runtimeConfig["env"].(map[string]interface{}); ok {
		resolved, err := s.Secrets.ResolveEnvBindings(ctx, run.CompanyID, envInput)
		if err == nil {
			env = resolved
		}
	}

	// 1. Realize Workspace
	var project models.Project
	repoURL := ""
	
	// Extract projectId from contextSnapshot since it's not a column
	contextData := make(map[string]interface{})
	_ = json.Unmarshal(run.ContextSnapshot, &contextData)
	contextProjectID, _ := contextData["projectId"].(string)

	if contextProjectID != "" {
		if err := s.DB.WithContext(ctx).Where("id = ?", contextProjectID).First(&project).Error; err == nil {
			policy := make(map[string]interface{})
			_ = json.Unmarshal(project.ExecutionWorkspacePolicy, &policy)
			if url, ok := policy["repoUrl"].(string); ok {
				repoURL = url
			}
		}
	}

	cwd, workspaceErr := s.Workspaces.RealizeWorkspace(ctx, run.CompanyID, contextProjectID, repoURL)
	if workspaceErr != nil {
		// Log error but attempt to continue if possible, or fail
		return workspaceErr
	}

	// 2. Execute via runner
	run.Status = "running"
	s.DB.WithContext(ctx).Save(&run)
	
	// Pass the resolved CWD to the runner env or directly
	env["PAPERCLIP_CWD"] = cwd
	
	err := s.Runner.Execute(ctx, &run, env)
	
	finishedAt := time.Now()
	run.FinishedAt = &finishedAt
	if err != nil {
		run.Status = "failed"
		errMsg := err.Error()
		run.Error = &errMsg
	} else {
		run.Status = "completed"

		// Record a placeholder cost event for parity (in production this uses real usage)
		_, _ = s.Costs.CreateEvent(ctx, run.CompanyID, &models.CostEvent{
			AgentID:        run.AgentID,
			HeartbeatRunID: &run.ID,
			Provider:       run.Agent.AdapterType,
			Model:          "default",
			CostCents:      0, // Placeholder
			OccurredAt:     finishedAt,
		})
	}
	
	return s.DB.WithContext(ctx).Save(&run).Error
}

// --- Local Runner Implementation ---

type LocalRunner struct {
	Logs *RunLogStore
}

func (r *LocalRunner) Execute(ctx context.Context, run *models.HeartbeatRun, env map[string]string) error {
	cwd := env["PAPERCLIP_CWD"]
	if cwd == "" {
		cwd = "."
	}
	cmd := exec.CommandContext(ctx, "paperclipai", "run") 
	cmd.Dir = cwd
	
	// Add env vars
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	
	// Set execution fields
	pid := 0
	run.ProcessPid = &pid
	startedAt := time.Now()
	run.ProcessStartedAt = &startedAt
	
	// Capture output through log store if available
	if run.LogStore != nil && r.Logs != nil {
		handle := &RunLogHandle{Store: *run.LogStore, LogRef: *run.LogRef}
		
		stdout, _ := cmd.StdoutPipe()
		stderr, _ := cmd.StderrPipe()
		
		go func() {
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				_ = r.Logs.Append(handle, "stdout", scanner.Text())
			}
		}()
		
		go func() {
			scanner := bufio.NewScanner(stderr)
			for scanner.Scan() {
				_ = r.Logs.Append(handle, "stderr", scanner.Text())
			}
		}()
	}

	if err := cmd.Start(); err != nil {
		return err
	}
	
	pid = cmd.Process.Pid
	run.ProcessPid = &pid
	
	err := cmd.Wait()
	
	exitCode := cmd.ProcessState.ExitCode()
	run.ExitCode = &exitCode
	
	return err
}
