package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type heartbeatTestRunner struct {
	called bool
	env    map[string]string
	result *AgentRunResult
}

func (r *heartbeatTestRunner) Execute(_ context.Context, _ *models.HeartbeatRun, env map[string]string) (*AgentRunResult, error) {
	r.called = true
	r.env = map[string]string{}
	for k, v := range env {
		r.env[k] = v
	}
	if r.result != nil {
		return r.result, nil
	}
	return &AgentRunResult{}, nil
}

type heartbeatTestMemory struct {
	createCalls []MemoryPayload
	createdCh   chan struct{}
}

func (m *heartbeatTestMemory) CreateNamespace(context.Context, string, string) error {
	return nil
}
func (m *heartbeatTestMemory) IngestMemory(context.Context, string, string, map[string]string) error {
	return nil
}
func (m *heartbeatTestMemory) CreateMemory(_ context.Context, _ string, payload MemoryPayload) error {
	m.createCalls = append(m.createCalls, payload)
	if m.createdCh != nil {
		select {
		case m.createdCh <- struct{}{}:
		default:
		}
	}
	return nil
}
func (m *heartbeatTestMemory) QueryMemory(context.Context, string, string, int) ([]MemoryResult, error) {
	return nil, nil
}
func (m *heartbeatTestMemory) CompileContext(context.Context, ContextRequest) (map[string]interface{}, error) {
	return nil, nil
}
func (m *heartbeatTestMemory) RegisterAgent(context.Context, string, string, string) error {
	return nil
}
func (m *heartbeatTestMemory) DeregisterAgent(context.Context, string, string) error {
	return nil
}
func (m *heartbeatTestMemory) HandleTrigger(context.Context, string, string, TriggerRequest) (*TriggerResponse, error) {
	return &TriggerResponse{Status: "ok"}, nil
}
func (m *heartbeatTestMemory) ExportAudit(context.Context, string, string) ([]byte, string, error) {
	return nil, "", nil
}
func (m *heartbeatTestMemory) ArchiveNamespace(context.Context, string) error {
	return nil
}
func (m *heartbeatTestMemory) DeleteNamespace(context.Context, string) error {
	return nil
}
func (m *heartbeatTestMemory) ListProposals(context.Context, string) ([]map[string]interface{}, error) {
	return nil, nil
}
func (m *heartbeatTestMemory) ResolveProposal(context.Context, string, string, string) error {
	return nil
}

func setupHeartbeatServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dbName := fmt.Sprintf("file:heartbeat_svc_%s?mode=memory&cache=shared", url.QueryEscape(t.Name()))
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	db.Exec(`CREATE TABLE companies (
		id text PRIMARY KEY,
		name text NOT NULL,
		spent_monthly_cents integer NOT NULL DEFAULT 0,
		created_at datetime DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)
	db.Exec(`CREATE TABLE agents (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		name text NOT NULL,
		role text NOT NULL DEFAULT 'general',
		title text,
		icon text,
		status text NOT NULL DEFAULT 'idle',
		reports_to text,
		capabilities text,
		adapter_type text NOT NULL DEFAULT 'process',
		adapter_config text NOT NULL DEFAULT '{}',
		runtime_config text NOT NULL DEFAULT '{}',
		budget_monthly_cents integer NOT NULL DEFAULT 0,
		spent_monthly_cents integer NOT NULL DEFAULT 0,
		pause_reason text,
		paused_at datetime,
		permissions text NOT NULL DEFAULT '{}',
		last_heartbeat_at datetime,
		metadata text,
		created_at datetime DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime DEFAULT CURRENT_TIMESTAMP,
		deleted_at datetime
	)`)
	db.Exec(`CREATE TABLE heartbeat_runs (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		agent_id text NOT NULL,
		invocation_source text NOT NULL DEFAULT 'on_demand',
		trigger_detail text,
		status text NOT NULL DEFAULT 'queued',
		started_at datetime,
		finished_at datetime,
		error text,
		wakeup_request_id text,
		exit_code integer,
		signal text,
		usage_json text DEFAULT '{}',
		result_json text DEFAULT '{}',
		session_id_before text,
		session_id_after text,
		log_store text,
		log_ref text,
		log_bytes integer,
		log_sha256 text,
		log_compressed boolean NOT NULL DEFAULT 0,
		stdout_excerpt text,
		stderr_excerpt text,
		error_code text,
		external_run_id text,
		process_pid integer,
		process_started_at datetime,
		retry_of_run_id text,
		process_loss_retry_count integer NOT NULL DEFAULT 0,
		context_snapshot text DEFAULT '{}',
		handoff_markdown text,
		task_id text NOT NULL DEFAULT '',
		created_at datetime DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)
	db.Exec(`CREATE TABLE agent_task_sessions (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		agent_id text NOT NULL,
		adapter_type text NOT NULL,
		task_key text NOT NULL,
		session_params_json text,
		session_display_id text,
		last_run_id text,
		last_error text,
		created_at datetime DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)
	db.Exec(`CREATE TABLE agent_wakeup_requests (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		agent_id text NOT NULL,
		source text NOT NULL,
		trigger_detail text,
		reason text,
		payload text,
		status text NOT NULL DEFAULT 'queued',
		coalesced_count integer NOT NULL DEFAULT 0,
		requested_by_actor_type text,
		requested_by_actor_id text,
		idempotency_key text,
		run_id text,
		requested_at datetime DEFAULT CURRENT_TIMESTAMP,
		claimed_at datetime,
		finished_at datetime,
		error text,
		created_at datetime DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)
	db.Exec(`CREATE TABLE agent_runtime_state (
		agent_id text PRIMARY KEY,
		company_id text NOT NULL,
		adapter_type text NOT NULL,
		session_id text,
		state_json text DEFAULT '{}',
		last_run_id text,
		last_run_status text,
		total_input_tokens integer NOT NULL DEFAULT 0,
		total_output_tokens integer NOT NULL DEFAULT 0,
		total_cached_input_tokens integer NOT NULL DEFAULT 0,
		total_cost_cents integer NOT NULL DEFAULT 0,
		last_error text,
		created_at datetime DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)
	db.Exec(`CREATE TABLE projects (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		name text NOT NULL,
		execution_workspace_policy text DEFAULT '{}',
		created_at datetime DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)
	db.Exec(`CREATE TABLE project_workspaces (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		project_id text NOT NULL,
		name text NOT NULL,
		status text NOT NULL DEFAULT 'active',
		mode text NOT NULL DEFAULT 'shared_workspace',
		source_type text NOT NULL DEFAULT 'local_path',
		cwd text,
		repo_url text,
		repo_ref text,
		default_ref text,
		visibility text NOT NULL DEFAULT 'default',
		setup_command text,
		cleanup_command text,
		remote_provider text,
		remote_workspace_ref text,
		shared_workspace_key text,
		metadata text DEFAULT '{}',
		is_primary boolean NOT NULL DEFAULT 0,
		created_at datetime DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)
	db.Exec(`CREATE TABLE issues (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		project_id text,
		project_workspace_id text,
		title text NOT NULL DEFAULT '',
		status text NOT NULL DEFAULT 'todo',
		priority text NOT NULL DEFAULT 'medium',
		created_at datetime DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)
	db.Exec(`CREATE TABLE workspace_operations (
		id text PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
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
	db.Exec(`CREATE TABLE cost_events (
		id text PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
		company_id text NOT NULL,
		agent_id text NOT NULL,
		issue_id text,
		project_id text,
		goal_id text,
		heartbeat_run_id text,
		billing_code text,
		provider text NOT NULL,
		biller text NOT NULL DEFAULT 'unknown',
		billing_type text NOT NULL DEFAULT 'unknown',
		model text NOT NULL,
		input_tokens integer NOT NULL DEFAULT 0,
		cached_input_tokens integer NOT NULL DEFAULT 0,
		output_tokens integer NOT NULL DEFAULT 0,
		cost_cents integer NOT NULL DEFAULT 0,
		occurred_at datetime NOT NULL,
		created_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)

	return db
}

func TestHeartbeatService_StartRun_CompletesRunAndRecordsWorkspaceOperation(t *testing.T) {
	t.Setenv("PAPERCLIP_HOME", t.TempDir())
	t.Setenv("PAPERCLIP_INSTANCE_ID", "test")

	db := setupHeartbeatServiceTestDB(t)
	db.Exec("INSERT INTO companies (id, name) VALUES ('comp-1', 'Acme')")
	db.Exec(`INSERT INTO agents (id, company_id, name, adapter_type, runtime_config, permissions)
		VALUES ('agent-1', 'comp-1', 'Runner', 'openai', '{"env":{"FOO":{"type":"plain","value":"bar"}}}', '{}')`)
	db.Exec(`INSERT INTO heartbeat_runs (id, company_id, agent_id, invocation_source, status, context_snapshot, task_id)
		VALUES ('run-1', 'comp-1', 'agent-1', 'api', 'starting', '{}', 'task-1')`)

	sessionParamsJSON, err := json.Marshal(map[string]interface{}{
		"sessionId": "sess-1",
		"cwd":       t.TempDir(),
	})
	if err != nil {
		t.Fatalf("marshal session params: %v", err)
	}
	runner := &heartbeatTestRunner{
		result: &AgentRunResult{
			ExitCode:          0,
			CostUsd:           1.25,
			UsageJSON:         json.RawMessage(`{"inputTokens":123,"cachedInputTokens":4,"outputTokens":56}`),
			ResultJSON:        json.RawMessage(`{"summary":"done","model":"gpt-test"}`),
			SessionParamsJSON: sessionParamsJSON,
		},
	}
	memory := &heartbeatTestMemory{createdCh: make(chan struct{}, 1)}
	svc := &HeartbeatService{
		DB:               db,
		Secrets:          NewSecretService(db, nil),
		Runner:           runner,
		Logs:             NewRunLogStore(""),
		Costs:            NewCostService(db),
		Workspaces:       NewWorkspaceService(db),
		Ops:              NewWorkspaceOperationService(db),
		Memory:           memory,
		runningProcesses: map[string]*ProcessHandle{},
	}

	if err := svc.StartRun(context.Background(), "run-1"); err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}

	if !runner.called {
		t.Fatal("expected runner to be invoked")
	}
	if got := runner.env["FOO"]; got != "bar" {
		t.Fatalf("expected resolved env FOO=bar, got %q", got)
	}
	cwd := runner.env["PAPERCLIP_CWD"]
	if cwd == "" {
		t.Fatal("expected PAPERCLIP_CWD to be set")
	}
	if info, err := os.Stat(cwd); err != nil {
		t.Fatalf("expected workspace to exist: %v", err)
	} else if !info.IsDir() {
		t.Fatalf("expected workspace path %q to be a directory", cwd)
	}

	var run models.HeartbeatRun
	if err := db.First(&run, "id = ?", "run-1").Error; err != nil {
		t.Fatalf("load run: %v", err)
	}
	if run.Status != "completed" {
		t.Fatalf("expected run status completed, got %q", run.Status)
	}
	if run.FinishedAt == nil {
		t.Fatal("expected finished_at to be set")
	}
	if run.SessionIDAfter == nil || *run.SessionIDAfter != "sess-1" {
		t.Fatalf("expected session_id_after sess-1, got %#v", run.SessionIDAfter)
	}

	var op models.WorkspaceOperation
	if err := db.First(&op, "heartbeat_run_id = ?", "run-1").Error; err != nil {
		t.Fatalf("load workspace operation: %v", err)
	}
	if op.Phase != "realize_workspace" {
		t.Fatalf("expected workspace phase realize_workspace, got %q", op.Phase)
	}
	if op.Status != "succeeded" {
		t.Fatalf("expected workspace operation status succeeded, got %q", op.Status)
	}

	var costCount int64
	if err := db.Model(&models.CostEvent{}).Where("heartbeat_run_id = ?", "run-1").Count(&costCount).Error; err != nil {
		t.Fatalf("count cost events: %v", err)
	}
	if costCount != 1 {
		t.Fatalf("expected 1 cost event, got %d", costCount)
	}
	var costEvent models.CostEvent
	if err := db.First(&costEvent, "heartbeat_run_id = ?", "run-1").Error; err != nil {
		t.Fatalf("load cost event: %v", err)
	}
	if costEvent.CostCents != 125 {
		t.Fatalf("expected cost cents 125, got %d", costEvent.CostCents)
	}
	if costEvent.InputTokens != 123 || costEvent.CachedInputTokens != 4 || costEvent.OutputTokens != 56 {
		t.Fatalf("unexpected cost usage totals: %#v", costEvent)
	}
	var session models.AgentTaskSession
	if err := db.First(&session, "agent_id = ? AND task_key = ?", "agent-1", "task-1").Error; err != nil {
		t.Fatalf("load task session: %v", err)
	}
	if session.SessionDisplayID == nil || *session.SessionDisplayID != "sess-1" {
		t.Fatalf("expected task session display id sess-1, got %#v", session.SessionDisplayID)
	}

	select {
	case <-memory.createdCh:
	case <-time.After(2 * time.Second):
		t.Fatal("expected run completion to create a memory payload")
	}
	if memory.createCalls[0].EntityType != "run_summary" {
		t.Fatalf("expected run_summary memory payload, got %q", memory.createCalls[0].EntityType)
	}
}

func TestHeartbeatService_Wakeup_CoalescesSameTaskKey(t *testing.T) {
	db := setupHeartbeatServiceTestDB(t)
	db.Exec("INSERT INTO companies (id, name) VALUES ('comp-1', 'Acme')")
	db.Exec(`INSERT INTO agents (id, company_id, name, adapter_type, runtime_config, permissions)
		VALUES ('agent-1', 'comp-1', 'Runner', 'cursor', '{}', '{}')`)
	svc := &HeartbeatService{
		DB:               db,
		Secrets:          NewSecretService(db, nil),
		Runner:           &heartbeatTestRunner{},
		Logs:             NewRunLogStore(""),
		Costs:            NewCostService(db),
		Workspaces:       NewWorkspaceService(db),
		Ops:              NewWorkspaceOperationService(db),
		Memory:           &heartbeatTestMemory{},
		runningProcesses: map[string]*ProcessHandle{},
	}

	db.Exec(`INSERT INTO heartbeat_runs (id, company_id, agent_id, invocation_source, status, context_snapshot, task_id)
		VALUES ('run-1', 'comp-1', 'agent-1', 'automation', 'queued', '{"taskId":"task-1"}', 'task-1')`)

	secondRun, err := svc.Wakeup(context.Background(), "comp-1", "agent-1", WakeupOptions{
		Source:  "automation",
		Context: map[string]interface{}{"taskId": "task-1", "commentId": "c-2"},
	})
	if err != nil {
		t.Fatalf("second wakeup: %v", err)
	}
	if secondRun == nil || secondRun.ID != "run-1" {
		t.Fatalf("expected coalesced run run-1, got %#v", secondRun)
	}
	var runCount int64
	if err := db.Model(&models.HeartbeatRun{}).Count(&runCount).Error; err != nil {
		t.Fatalf("count runs: %v", err)
	}
	if runCount != 1 {
		t.Fatalf("expected 1 heartbeat run, got %d", runCount)
	}
	var wakeups []models.AgentWakeupRequest
	if err := db.Order("created_at asc").Find(&wakeups).Error; err != nil {
		t.Fatalf("load wakeups: %v", err)
	}
	if len(wakeups) != 1 || wakeups[0].Status != "coalesced" {
		t.Fatalf("expected second wakeup to be coalesced, got %#v", wakeups)
	}
	var storedRun models.HeartbeatRun
	if err := db.First(&storedRun, "id = ?", "run-1").Error; err != nil {
		t.Fatalf("load run: %v", err)
	}
	contextSnapshot := parseJSONObject(storedRun.ContextSnapshot)
	if readNonEmptyString(contextSnapshot["commentId"]) != "c-2" {
		t.Fatalf("expected merged comment id c-2, got %#v", contextSnapshot["commentId"])
	}
}

func TestHeartbeatService_TickTimers_EnqueuesDueAgents(t *testing.T) {
	db := setupHeartbeatServiceTestDB(t)
	db.Exec("INSERT INTO companies (id, name) VALUES ('comp-1', 'Acme')")
	db.Exec(`INSERT INTO agents (id, company_id, name, adapter_type, runtime_config, permissions, created_at, last_heartbeat_at)
		VALUES ('agent-1', 'comp-1', 'Runner', 'cursor', '{"heartbeat":{"enabled":true,"intervalSec":60}}', '{}', datetime('now','-2 hour'), datetime('now','-2 hour'))`)
	svc := &HeartbeatService{
		DB:               db,
		Secrets:          NewSecretService(db, nil),
		Runner:           &heartbeatTestRunner{},
		Logs:             NewRunLogStore(""),
		Costs:            NewCostService(db),
		Workspaces:       NewWorkspaceService(db),
		Ops:              NewWorkspaceOperationService(db),
		Memory:           &heartbeatTestMemory{},
		runningProcesses: map[string]*ProcessHandle{},
	}

	result, err := svc.TickTimers(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("tick timers: %v", err)
	}
	if result.Checked != 1 || result.Enqueued != 1 {
		t.Fatalf("unexpected tick result: %#v", result)
	}
	var run models.HeartbeatRun
	if err := db.First(&run).Error; err != nil {
		t.Fatalf("load run: %v", err)
	}
	contextSnapshot := parseJSONObject(run.ContextSnapshot)
	if readNonEmptyString(contextSnapshot["taskKey"]) != heartbeatTaskKey {
		t.Fatalf("expected heartbeat task key, got %#v", contextSnapshot["taskKey"])
	}
}

func TestHeartbeatService_EvaluateSessionCompaction_RotatesAfterThreshold(t *testing.T) {
	db := setupHeartbeatServiceTestDB(t)
	db.Exec("INSERT INTO companies (id, name) VALUES ('comp-1', 'Acme')")
	db.Exec(`INSERT INTO agents (id, company_id, name, adapter_type, runtime_config, permissions)
		VALUES ('agent-1', 'comp-1', 'Runner', 'cursor', '{"heartbeat":{"sessionCompaction":{"maxSessionRuns":1}}}', '{}')`)
	db.Exec(`INSERT INTO agent_task_sessions (id, company_id, agent_id, adapter_type, task_key, session_display_id)
		VALUES ('sess-row', 'comp-1', 'agent-1', 'cursor', 'task-1', 'sess-1')`)
	db.Exec(`INSERT INTO heartbeat_runs (id, company_id, agent_id, invocation_source, status, session_id_after, result_json, created_at)
		VALUES 
		('run-1', 'comp-1', 'agent-1', 'api', 'completed', 'sess-1', '{"summary":"first"}', datetime('now','-2 hour')),
		('run-2', 'comp-1', 'agent-1', 'api', 'completed', 'sess-1', '{"summary":"second"}', datetime('now','-1 hour'))`)
	svc := &HeartbeatService{DB: db}

	decision, err := svc.evaluateSessionCompaction(context.Background(), "agent-1", "sess-1")
	if err != nil {
		t.Fatalf("evaluate session compaction: %v", err)
	}
	if decision == nil || !decision.Rotate {
		t.Fatalf("expected rotation decision, got %#v", decision)
	}
	if decision.Reason == "" || decision.HandoffMarkdown == "" {
		t.Fatalf("expected compaction reason and handoff markdown, got %#v", decision)
	}
}
