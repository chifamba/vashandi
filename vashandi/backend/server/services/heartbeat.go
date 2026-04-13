package services

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"github.com/chifamba/vashandi/vashandi/backend/shared"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var (
	ErrProcessLost = errors.New("process lost")
)

type ProcessHandle struct {
	RunID string
	Cmd   *exec.Cmd
}

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
	Ops        *WorkspaceOperationService
	Memory     MemoryAdapter

	// In-memory process tracking
	runningProcesses   map[string]*ProcessHandle
	runningProcessesMu sync.RWMutex
}

func NewHeartbeatService(db *gorm.DB, secrets *SecretService, activity *ActivityService, ops *WorkspaceOperationService, memory MemoryAdapter, runner AgentRunner) *HeartbeatService {
	logStore := NewRunLogStore("")
	costSvc := NewCostService(db)
	workspaceSvc := NewWorkspaceService(db)
	if runner == nil {
		runner = &LocalRunner{Logs: logStore}
	}
	if memory == nil {
		memory = NewOpenBrainAdapter()
	}
	return &HeartbeatService{
		DB:               db,
		Secrets:          secrets,
		Runner:           runner,
		Logs:             logStore,
		Costs:            costSvc,
		Workspaces:       workspaceSvc,
		Activity:         activity,
		Ops:              ops,
		Memory:           memory,
		runningProcesses: make(map[string]*ProcessHandle),
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

	// Trigger execution (asynchronously)
	// In production, we would check concurrency limits here before starting.
	go func() {
		// For now, we resume queued runs for this agent
		s.ResumeQueuedRuns(context.Background(), agentID)
	}()

	return run, nil
}

// ResumeQueuedRuns attempts to start queued runs for an agent, respecting concurrency limits.
func (s *HeartbeatService) ResumeQueuedRuns(ctx context.Context, agentID string) {
	var agent models.Agent
	if err := s.DB.Where("id = ?", agentID).First(&agent).Error; err != nil {
		return
	}

	// Simple concurrency check: default 1
	var runningCount int64
	s.DB.Model(&models.HeartbeatRun{}).Where("agent_id = ? AND status = ?", agentID, "running").Count(&runningCount)

	if runningCount >= 1 { // In parity, we often start with 1
		return
	}

	var nextRun models.HeartbeatRun
	if err := s.DB.Where("agent_id = ? AND status = ?", agentID, "queued").Order("created_at asc").First(&nextRun).Error; err == nil {
		_ = s.StartRun(ctx, nextRun.ID)
	}
}

// ReapOrphanedRuns cleans up runs that are marked as running but the process is gone.
func (s *HeartbeatService) ReapOrphanedRuns(ctx context.Context) error {
	var activeRuns []models.HeartbeatRun
	if err := s.DB.Where("status = ?", "running").Find(&activeRuns).Error; err != nil {
		return err
	}

	for _, run := range activeRuns {
		s.runningProcessesMu.RLock()
		_, tracked := s.runningProcesses[run.ID]
		s.runningProcessesMu.RUnlock()

		if tracked {
			continue
		}

		// If not tracked in memory, check if PID is alive (if we have one)
		isAlive := false
		if run.ProcessPid != nil && *run.ProcessPid > 0 {
			proc, err := os.FindProcess(*run.ProcessPid)
			if err == nil {
				// On Unix, p.Signal(0) checks if process exists
				err = proc.Signal(syscall.Signal(0))
				if err == nil {
					isAlive = true
				}
			}
		}

		if !isAlive {
			finishedAt := time.Now()
			run.Status = "completed"
			run.FinishedAt = &finishedAt
			s.DB.Save(&run)

			// notify OpenBrain
			go func() {
				summary := fmt.Sprintf("Run %s completed for agent %s on task %s", run.ID, run.AgentID, run.TaskID)
				_, _ = s.Memory.HandleTrigger(context.Background(), run.CompanyID, "run_complete", TriggerRequest{
					AgentID:   run.AgentID,
					TaskQuery: run.TaskID,
					Summary:   summary,
					Metadata: map[string]any{
						"runId":      run.ID,
						"exitCode":   0,
						"finishedAt": finishedAt,
					},
				})
			}()

			s.resumeNextRun(run.AgentID)
		}
	}
	return nil
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
	
	// 1. Resolve secrets in the adapter config itself
	resolvedConfig, err := s.Secrets.ResolveAdapterConfigForRuntime(ctx, run.CompanyID, runtimeConfig)
	if err == nil {
		runtimeConfig = resolvedConfig
	}

	env := make(map[string]string)
	if envInput, ok := runtimeConfig["env"].(map[string]interface{}); ok {
		resolved, err := s.Secrets.ResolveEnvBindings(ctx, run.CompanyID, envInput)
		if err == nil {
			env = resolved
		}
	}

	// 1. Realize Workspace
	recorder := s.Ops.CreateRecorder(run.CompanyID, &run.ID, nil)
	phase := "realize_workspace"
	op, _ := recorder.Begin(ctx, phase, nil)

	var project models.Project
	repoURL := ""
	
	// Extract projectId from contextSnapshot
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

	// 1. Budget Check
	if contextProjectID != "" {
		blocked, err := CheckProjectBudget(s.DB, contextProjectID)
		if err != nil {
			return err
		}
		if blocked {
			run.Status = "failed"
			errMsg := "Budget exceeded for project"
			run.Error = &errMsg
			s.DB.WithContext(ctx).Save(&run)
			return fmt.Errorf("budget exceeded")
		}
	}

	strategy := StrategyPrimary
	if strategyStr, ok := contextData["workspaceStrategy"].(string); ok && strategyStr == "git_worktree" {
		strategy = StrategyWorktree
	}

	cwd, workspaceErr := s.Workspaces.RealizeWorkspace(ctx, run.CompanyID, contextProjectID, repoURL, RealizeOptions{
		Strategy:   strategy,
		RunID:      run.ID,
		BranchName: "",
	})
	
	if workspaceErr != nil {
		recorder.Finish(ctx, op.ID, 1, workspaceErr)
		return workspaceErr
	}
	recorder.Finish(ctx, op.ID, 0, nil)
	
	// --- Fat Context Injection ---
	semanticContext, err := s.Memory.CompileContext(ctx, ContextRequest{
		NamespaceID: run.CompanyID,
		AgentID:     run.AgentID,
		Intent:      "heartbeat_invocation",
		Query:       run.TaskID,
	})
	if err == nil && semanticContext != nil {
		// Merge into context data
		contextData["openBrainMemories"] = semanticContext
		updatedContextJSON, _ := json.Marshal(contextData)
		run.ContextSnapshot = datatypes.JSON(updatedContextJSON)
	}
	// -----------------------------

	// 2. Execute via runner
	run.Status = "running"
	s.DB.WithContext(ctx).Save(&run)
	
	env["PAPERCLIP_CWD"] = cwd
	
	return s.executeAndTrack(ctx, &run, env)
}

func (s *HeartbeatService) executeAndTrack(ctx context.Context, run *models.HeartbeatRun, env map[string]string) error {
	err := s.Runner.Execute(ctx, run, env)
	
	s.runningProcessesMu.Lock()
	delete(s.runningProcesses, run.ID)
	s.runningProcessesMu.Unlock()

	finishedAt := time.Now()
	run.FinishedAt = &finishedAt
	if err != nil {
		run.Status = "failed"
		msg := err.Error()
		run.Error = &msg
	} else {
		run.Status = "completed"
		// Record cost event
		_, _ = s.Costs.CreateEvent(ctx, run.CompanyID, &models.CostEvent{
			AgentID:        run.AgentID,
			HeartbeatRunID: &run.ID,
			Provider:       run.Agent.AdapterType,
			Model:          "default",
			CostCents:      0, // Placeholder
			OccurredAt:     finishedAt,
		})
	}
	s.DB.WithContext(ctx).Save(run)

	// notify OpenBrain
	go func() {
		summary := fmt.Sprintf("Run %s completed for agent %s on task %s", run.ID, run.AgentID, run.TaskID)
		if err == nil {
			// standard experience persistence
			_ = s.Memory.CreateMemory(context.Background(), run.CompanyID, MemoryPayload{
				EntityType: "run_summary",
				Text:       summary,
				Title:      "Heartbeat Run",
				Metadata: map[string]interface{}{
					"runId":    run.ID,
					"agentId":  run.AgentID,
					"outcome":  "succeeded",
					"finished": finishedAt,
				},
			})
		}
	}()

	s.resumeNextRun(run.AgentID)
	return err
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
	
	// In-process runners in Go don't easily have access to the parent service
	// so we expect the service to handle the tracking.
	// However, if we move to a microservice model, this wouldn't matter.
	
	err := cmd.Wait()
	
	exitCode := cmd.ProcessState.ExitCode()
	run.ExitCode = &exitCode
	
	return err
}
