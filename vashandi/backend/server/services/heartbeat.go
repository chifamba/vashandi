package services

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
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
	Execute(ctx context.Context, run *models.HeartbeatRun, env map[string]string) (*AgentRunResult, error)
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
	EventBus   *PluginEventBus
	BudgetEnforcementHook func(context.Context, BudgetScope) error

	// Notify, when non-nil, is called after a run's status changes so that
	// the live-events hub can broadcast the update to connected clients.
	// The companyID and a JSON-encoded event payload are passed as arguments.
	Notify func(companyID string, data []byte)

	// In-memory process tracking
	runningProcesses   map[string]*ProcessHandle
	runningProcessesMu sync.RWMutex
	budgetRunCancels   map[string]context.CancelFunc
	budgetRunCancelsMu sync.RWMutex
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
	svc := &HeartbeatService{
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
		budgetRunCancels: make(map[string]context.CancelFunc),
	}
	svc.BudgetEnforcementHook = svc.CancelBudgetScopeWork
	svc.Costs.BudgetEnforcementHook = svc.BudgetEnforcementHook
	return svc
}

// WakeupOptions configures an agent wakeup invocation.
type WakeupOptions struct {
	Source               string
	TriggerDetail        string
	Context              map[string]interface{}
	Reason               string
	Payload              map[string]interface{}
	IdempotencyKey       string
	RequestedByActorType string
	RequestedByActorID   string
}

// Wakeup triggers an agent run.
func (s *HeartbeatService) Wakeup(ctx context.Context, companyID, agentID string, opts WakeupOptions) (*models.HeartbeatRun, error) {
	run, err := s.enqueueWakeup(ctx, companyID, agentID, opts)
	if err != nil {
		return nil, err
	}
	if run != nil && s.Activity != nil {
		_, _ = s.Activity.Log(ctx, LogEntry{
			CompanyID:  companyID,
			ActorType:  firstNonEmpty(opts.RequestedByActorType, "system"),
			ActorID:    firstNonEmpty(opts.RequestedByActorID, "system"),
			Action:     "heartbeat.wakeup",
			EntityType: "agent",
			EntityID:   agentID,
			AgentID:    &agentID,
			RunID:      &run.ID,
			Details: map[string]interface{}{
				"source": opts.Source,
				"reason": opts.Reason,
			},
		})
	}
	return run, nil
}

// ResumeQueuedRuns attempts to start queued runs for an agent, respecting concurrency limits.
func (s *HeartbeatService) ResumeQueuedRuns(ctx context.Context, agentID string) {
	var agent models.Agent
	if err := s.DB.Where("id = ?", agentID).First(&agent).Error; err != nil {
		return
	}

	policy := parseHeartbeatPolicy(&agent)
	var runningCount int64
	s.DB.Model(&models.HeartbeatRun{}).Where("agent_id = ? AND status = ?", agentID, "running").Count(&runningCount)

	if runningCount >= int64(policy.MaxConcurrentRuns) {
		return
	}

	var nextRun models.HeartbeatRun
	if err := s.DB.Where("agent_id = ? AND status = ?", agentID, "queued").Order("created_at asc").First(&nextRun).Error; err == nil {
		// Atomic claim
		if claimed := s.claimQueuedRun(ctx, nextRun.ID); claimed != nil {
			_ = s.StartRun(ctx, claimed.ID)
		}
	}
}

func (s *HeartbeatService) claimQueuedRun(ctx context.Context, runID string) *models.HeartbeatRun {
	var run models.HeartbeatRun
	result := s.DB.WithContext(ctx).Model(&run).
		Where("id = ? AND status = ?", runID, "queued").
		Updates(map[string]interface{}{
			"status":     "starting",
			"started_at": time.Now(),
		})

	if result.Error != nil || result.RowsAffected == 0 {
		return nil
	}

	s.DB.First(&run, "id = ?", runID)
	return &run
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

	// Check if run is already starting or running (idempotency)
	if run.Status != "starting" {
		claimed := s.claimQueuedRun(ctx, runID)
		if claimed == nil {
			return nil // Already claimed or finished
		}
		run = *claimed
	}

	contextData := parseJSONObject(run.ContextSnapshot)
	taskKey := firstNonEmpty(deriveTaskKeyWithHeartbeatFallback(contextData, nil), run.TaskID)
	resetTaskSession := shouldResetTaskSessionForWake(contextData)
	var existingTaskSession *models.AgentTaskSession
	if taskKey != "" && !resetTaskSession {
		existingTaskSession, _ = s.getTaskSession(ctx, run.CompanyID, run.AgentID, run.Agent.AdapterType, taskKey)
	}
	previousSessionParams := map[string]interface{}{}
	if existingTaskSession != nil {
		previousSessionParams = parseJSONObject(existingTaskSession.SessionParamsJSON)
	}
	sessionIDBefore := ""
	if existingTaskSession != nil {
		sessionIDBefore = firstNonEmpty(derefString(existingTaskSession.SessionDisplayID), sessionIDFromParams(previousSessionParams))
	}
	if sessionIDBefore == "" && !resetTaskSession {
		if runtimeState, err := s.getRuntimeState(ctx, run.AgentID); err == nil && runtimeState != nil && runtimeState.SessionID != nil {
			sessionIDBefore = *runtimeState.SessionID
		}
	}

	decision, err := s.evaluateSessionCompaction(ctx, run.AgentID, sessionIDBefore)
	if err != nil {
		return err
	}
	if decision != nil && decision.Rotate {
		contextData["paperclipSessionHandoffMarkdown"] = decision.HandoffMarkdown
		contextData["paperclipSessionRotationReason"] = decision.Reason
		contextData["paperclipPreviousSessionId"] = sessionIDBefore
		sessionIDBefore = ""
	} else {
		delete(contextData, "paperclipSessionHandoffMarkdown")
		delete(contextData, "paperclipSessionRotationReason")
		delete(contextData, "paperclipPreviousSessionId")
	}

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

	var resolvedWorkspace *ResolvedWorkspaceForRun
	if s.Ops != nil {
		recorder := s.Ops.CreateRecorder(run.CompanyID, &run.ID, nil)
		op, _ := recorder.Begin(ctx, "realize_workspace", nil)
		resolvedWorkspace, err = s.resolveWorkspaceForRun(ctx, &run, contextData, previousSessionParams)
		if err != nil {
			recorder.Finish(ctx, op.ID, 1, err)
			return err
		}
		recorder.Finish(ctx, op.ID, 0, nil)
	} else {
		resolvedWorkspace, err = s.resolveWorkspaceForRun(ctx, &run, contextData, previousSessionParams)
		if err != nil {
			return err
		}
	}

	resolvedProjectID := ""
	if resolvedWorkspace != nil {
		resolvedProjectID = resolvedWorkspace.ProjectID
	}
	if resolvedProjectID == "" {
		resolvedProjectID = readNonEmptyString(contextData["projectId"])
	}
	if resolvedProjectID != "" && readNonEmptyString(contextData["projectId"]) == "" {
		contextData["projectId"] = resolvedProjectID
	}

	if budgetScopes, err := s.buildInvocationBudgetScopes(ctx, run.CompanyID, run.AgentID, contextData); err != nil {
		return err
	} else if budgetBlock, err := s.GetInvocationBlock(ctx, run.CompanyID, run.AgentID, budgetScopes); err != nil {
		return err
	} else if budgetBlock != nil {
		run.Status = "failed"
		errMsg := budgetBlock.Reason
		run.Error = &errMsg
		s.DB.WithContext(ctx).Save(&run)
		if run.WakeupRequestID != nil {
			_ = s.updateWakeupRequestStatus(ctx, *run.WakeupRequestID, "failed", errMsg, time.Now(), &run.ID)
		}
		return fmt.Errorf("budget blocked: %s", budgetBlock.Reason)
	}

	// --- Fat Context Injection ---
	if obAdapter, ok := s.Memory.(*OpenBrainAdapter); ok {
		memXML, xmlErr := obAdapter.CompileContextXML(ctx, run.CompanyID, run.AgentID, run.TaskID)
		if xmlErr == nil && memXML != "" {
			contextData["openBrainMemoryXML"] = memXML
			updatedContextJSON, _ := json.Marshal(contextData)
			run.ContextSnapshot = datatypes.JSON(updatedContextJSON)
		}
	} else {
		semanticContext, err := s.Memory.CompileContext(ctx, ContextRequest{
			NamespaceID: run.CompanyID,
			AgentID:     run.AgentID,
			Intent:      "heartbeat_invocation",
			Query:       run.TaskID,
		})
		if err == nil && semanticContext != nil {
			contextData["openBrainMemories"] = semanticContext
			updatedContextJSON, _ := json.Marshal(contextData)
			run.ContextSnapshot = datatypes.JSON(updatedContextJSON)
		}
	}
	// -----------------------------

	if resolvedWorkspace != nil {
		contextData["paperclipWorkspace"] = map[string]interface{}{
			"cwd":         resolvedWorkspace.CWD,
			"source":      resolvedWorkspace.Source,
			"projectId":   resolvedWorkspace.ProjectID,
			"workspaceId": resolvedWorkspace.WorkspaceID,
			"repoUrl":     resolvedWorkspace.RepoURL,
			"repoRef":     resolvedWorkspace.RepoRef,
		}
		if len(resolvedWorkspace.Warnings) > 0 {
			contextData["paperclipWorkspaceWarnings"] = resolvedWorkspace.Warnings
		}
	}
	if sessionIDBefore != "" {
		run.SessionIDBefore = &sessionIDBefore
	} else {
		run.SessionIDBefore = nil
	}
	updatedContextJSON, _ := json.Marshal(contextData)
	run.ContextSnapshot = datatypes.JSON(updatedContextJSON)

	// 2. Execute via runner
	run.Status = "running"
	if run.WakeupRequestID != nil {
		_ = s.updateWakeupRequestStatus(ctx, *run.WakeupRequestID, "running", "", time.Now(), &run.ID)
	}
	s.DB.WithContext(ctx).Save(&run)

	if resolvedWorkspace != nil {
		env["PAPERCLIP_CWD"] = resolvedWorkspace.CWD
	}

	return s.executeAndTrack(ctx, &run, env, taskKey, resetTaskSession)
}

func (s *HeartbeatService) executeAndTrack(ctx context.Context, run *models.HeartbeatRun, env map[string]string, taskKey string, resetTaskSession bool) error {
	runCtx, cancel := context.WithCancel(ctx)
	s.registerBudgetRunCancel(run.ID, cancel)
	defer func() {
		s.releaseBudgetRunCancel(run.ID)
		cancel()
	}()

	result, err := s.Runner.Execute(runCtx, run, env)

	s.runningProcessesMu.Lock()
	delete(s.runningProcesses, run.ID)
	s.runningProcessesMu.Unlock()

	finishedAt := time.Now()
	run.FinishedAt = &finishedAt
	usageJSON, usageTotals := normalizeUsageJSON(result)
	if result != nil {
		exitCode := result.ExitCode
		run.ExitCode = &exitCode
		if len(result.ResultJSON) > 0 {
			run.ResultJSON = datatypes.JSON(result.ResultJSON)
		}
		if len(usageJSON) > 0 {
			run.UsageJSON = usageJSON
		}
		if sessionAfter := sessionIDFromRaw(result.SessionParamsJSON); sessionAfter != "" {
			run.SessionIDAfter = &sessionAfter
		}
	}
	if err == nil && result != nil && result.ExitCode != 0 {
		err = fmt.Errorf("agent exited with code %d", result.ExitCode)
	}
	currentStatus, currentErr := s.loadRunStatus(ctx, run.ID)
	if currentErr != nil {
		return currentErr
	}
	if currentStatus == "cancelled" {
		run.Status = "cancelled"
		code := "cancelled"
		run.ErrorCode = &code
		if run.Error == nil {
			msg := "Cancelled due to budget pause"
			run.Error = &msg
		}
		err = nil
	} else if err != nil {
		run.Status = "failed"
		msg := err.Error()
		run.Error = &msg
	} else {
		run.Status = "completed"
		_, _ = s.Costs.CreateEvent(ctx, run.CompanyID, buildCostEvent(run, result, usageTotals, finishedAt))
	}
	s.DB.WithContext(ctx).Save(run)
	if taskKey != "" {
		if err == nil && result != nil && len(result.SessionParamsJSON) > 0 {
			_ = s.upsertTaskSession(ctx, taskSessionUpsertInput{
				CompanyID:         run.CompanyID,
				AgentID:           run.AgentID,
				AdapterType:       run.Agent.AdapterType,
				TaskKey:           taskKey,
				SessionParamsJSON: result.SessionParamsJSON,
				SessionDisplayID:  sessionIDFromRaw(result.SessionParamsJSON),
				LastRunID:         &run.ID,
				LastError:         nil,
			})
		} else if resetTaskSession {
			_, _ = s.clearTaskSessions(ctx, run.CompanyID, run.AgentID, &taskKey, &run.Agent.AdapterType)
		}
	}
	_ = s.upsertRuntimeState(ctx, &run.Agent, run, result, usageTotals, run.Status)
	if run.WakeupRequestID != nil {
		_ = s.updateWakeupRequestStatus(ctx, *run.WakeupRequestID, run.Status, derefString(run.Error), finishedAt, &run.ID)
	}
	_ = s.finalizeAgentStatus(ctx, run.AgentID, run.Status)

	// Broadcast the run status change to any live-events subscribers.
	s.publishRunStatus(run)

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

// publishRunStatus broadcasts a heartbeat.run.status event via Notify (if set).
func (s *HeartbeatService) publishRunStatus(run *models.HeartbeatRun) {
	evtPayload := map[string]interface{}{
		"runId":   run.ID,
		"agentId": run.AgentID,
		"status":  run.Status,
	}

	if s.EventBus != nil {
		s.EventBus.Publish(context.Background(), PluginEvent{
			EventID:    fmt.Sprintf("evt_%d", time.Now().UnixNano()),
			EventType:  "heartbeat.run.status",
			CompanyID:  run.CompanyID,
			OccurredAt: time.Now().UTC().Format(time.RFC3339),
			ActorType:  "system",
			ActorID:    "heartbeat",
			Payload:    evtPayload,
		})
	}

	if s.Notify == nil {
		return
	}
	payload, err := json.Marshal(map[string]interface{}{
		"type":      "heartbeat.run.status",
		"companyId": run.CompanyID,
		"payload":   evtPayload,
	})
	if err != nil {
		return
	}
	s.Notify(run.CompanyID, payload)
}

// resumeNextRun picks up the next queued run for the agent and starts it.
func (s *HeartbeatService) resumeNextRun(agentID string) {
	var next models.HeartbeatRun
	err := s.DB.
		Where("agent_id = ? AND status = ?", agentID, "queued").
		Order("created_at ASC").
		First(&next).Error
	if err != nil {
		return // no queued runs
	}
	go func() {
		_ = s.StartRun(context.Background(), next.ID)
	}()
}

// --- Local Runner Implementation ---

type LocalRunner struct {
	Logs *RunLogStore
}

func (r *LocalRunner) Execute(ctx context.Context, run *models.HeartbeatRun, env map[string]string) (*AgentRunResult, error) {
	cwd := env["PAPERCLIP_CWD"]
	if cwd == "" {
		cwd = "."
	}
	agentID := sanitizeCommandToken(run.AgentID, "")
	if agentID == "" {
		return nil, fmt.Errorf("invalid agent id %q", run.AgentID)
	}
	args := []string{"heartbeat", "run", "--agent-id", agentID, "--source", sanitizeCommandToken(run.InvocationSource, "on_demand")}
	if run.TriggerDetail != nil && *run.TriggerDetail != "" {
		args = append(args, "--trigger", sanitizeCommandToken(*run.TriggerDetail, "manual"))
	}
	cmd := exec.CommandContext(ctx, "paperclipai", args...)
	cmd.Dir = cwd

	// Add env vars
	cmd.Env = append(cmd.Env, os.Environ()...)
	for k, v := range env {
		if safeEnv, ok := sanitizeEnvVar(k, v); ok {
			cmd.Env = append(cmd.Env, safeEnv)
		}
	}

	// Set execution fields
	pid := 0
	run.ProcessPid = &pid
	startedAt := time.Now()
	run.ProcessStartedAt = &startedAt

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	result := &AgentRunResult{}
	var resultMu sync.Mutex
	appendLog := func(stream, chunk string) {
		if run.LogStore == nil || run.LogRef == nil || r.Logs == nil {
			return
		}
		handle := &RunLogHandle{Store: *run.LogStore, LogRef: *run.LogRef}
		_ = r.Logs.Append(handle, stream, chunk)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			appendLog("stdout", line)
			if parsed, ok := parseAgentRunResultLine(line); ok {
				resultMu.Lock()
				mergeAgentRunResult(result, parsed)
				resultMu.Unlock()
			}
		}
	}()
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderr)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			appendLog("stderr", scanner.Text())
		}
	}()

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	pid = cmd.Process.Pid
	run.ProcessPid = &pid

	// In-process runners in Go don't easily have access to the parent service
	// so we expect the service to handle the tracking.
	// However, if we move to a microservice model, this wouldn't matter.

	err := cmd.Wait()
	wg.Wait()

	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	run.ExitCode = &exitCode
	result.ExitCode = exitCode

	return result, err
}

func sanitizeCommandToken(value, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	for _, r := range trimmed {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' && r != '-' {
			return fallback
		}
	}
	return trimmed
}

func sanitizeEnvVar(key, value string) (string, bool) {
	key = strings.TrimSpace(key)
	if key == "" || strings.ContainsAny(key, "=\x00\r\n") || strings.ContainsAny(value, "\x00") {
		return "", false
	}
	return fmt.Sprintf("%s=%s", key, value), true
}
