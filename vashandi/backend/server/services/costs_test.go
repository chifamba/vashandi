package services

import (
	"context"
	"testing"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupCostServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&cost_svc_test=1"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.Exec("DROP TABLE IF EXISTS cost_events")
	db.Exec("DROP TABLE IF EXISTS agents")
	db.Exec("DROP TABLE IF EXISTS companies")
	db.Exec(`CREATE TABLE companies (
		id text PRIMARY KEY,
		name text NOT NULL,
		issue_prefix text NOT NULL DEFAULT 'PAP',
		issue_counter integer NOT NULL DEFAULT 0,
		budget_monthly_cents integer NOT NULL DEFAULT 0,
		spent_monthly_cents integer NOT NULL DEFAULT 0,
		status text NOT NULL DEFAULT 'active',
		pause_reason text,
		paused_at datetime,
		require_board_approval_for_new_agents boolean NOT NULL DEFAULT 1,
		feedback_data_sharing_enabled boolean NOT NULL DEFAULT 0,
		created_at datetime,
		updated_at datetime
	)`)
	db.Exec(`CREATE TABLE agents (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		name text NOT NULL,
		role text NOT NULL DEFAULT 'general',
		status text NOT NULL DEFAULT 'idle',
		adapter_type text NOT NULL DEFAULT 'process',
		adapter_config text NOT NULL DEFAULT '{}',
		runtime_config text NOT NULL DEFAULT '{}',
		budget_monthly_cents integer NOT NULL DEFAULT 0,
		spent_monthly_cents integer NOT NULL DEFAULT 0,
		pause_reason text,
		paused_at datetime,
		permissions text NOT NULL DEFAULT '{}',
		created_at datetime,
		updated_at datetime
	)`)
	db.Exec(`CREATE TABLE projects (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		name text NOT NULL,
		pause_reason text,
		paused_at datetime,
		created_at datetime,
		updated_at datetime
	)`)
	db.Exec(`CREATE TABLE cost_events (
		id text PRIMARY KEY,
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
		cost_cents integer NOT NULL,
		occurred_at datetime NOT NULL,
		created_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)
	db.Exec(`CREATE TABLE budget_policies (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		scope_type text NOT NULL,
		scope_id text NOT NULL,
		metric text NOT NULL DEFAULT 'billed_cents',
		window_kind text NOT NULL DEFAULT 'calendar_month_utc',
		amount integer NOT NULL DEFAULT 0,
		warn_percent integer NOT NULL DEFAULT 80,
		hard_stop_enabled boolean NOT NULL DEFAULT 1,
		notify_enabled boolean NOT NULL DEFAULT 1,
		is_active boolean NOT NULL DEFAULT 1,
		created_at datetime,
		updated_at datetime
	)`)
	db.Exec(`CREATE TABLE budget_incidents (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		policy_id text NOT NULL,
		scope_type text NOT NULL,
		scope_id text NOT NULL,
		metric text NOT NULL DEFAULT 'billed_cents',
		window_kind text NOT NULL DEFAULT 'calendar_month_utc',
		window_start datetime NOT NULL,
		window_end datetime NOT NULL,
		threshold_type text NOT NULL,
		amount_limit integer NOT NULL DEFAULT 0,
		amount_observed integer NOT NULL DEFAULT 0,
		status text NOT NULL DEFAULT 'open',
		approval_id text,
		resolved_at datetime,
		created_at datetime,
		updated_at datetime
	)`)
	return db
}

func TestCostService_CreateEvent_Basic(t *testing.T) {
	db := setupCostServiceTestDB(t)
	svc := NewCostService(db)

	db.Exec("INSERT INTO companies (id, name) VALUES ('comp-1', 'Test Co')")
	db.Exec("INSERT INTO agents (id, company_id, name) VALUES ('agent-1', 'comp-1', 'Agent A')")

	event := &models.CostEvent{
		AgentID:   "agent-1",
		Provider:  "claude",
		Model:     "claude-3",
		CostCents: 150,
	}

	created, err := svc.CreateEvent(nil, "comp-1", event)
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}
	if created.CompanyID != "comp-1" {
		t.Errorf("expected CompanyID 'comp-1', got %q", created.CompanyID)
	}
	if created.CostCents != 150 {
		t.Errorf("expected CostCents 150, got %d", created.CostCents)
	}
	if created.OccurredAt.IsZero() {
		t.Error("expected OccurredAt to be set")
	}
}

func TestCostService_CreateEvent_UpdatesAgentSpend(t *testing.T) {
	db := setupCostServiceTestDB(t)
	svc := NewCostService(db)

	db.Exec("INSERT INTO companies (id, name) VALUES ('comp-1', 'Test Co')")
	db.Exec("INSERT INTO agents (id, company_id, name) VALUES ('agent-1', 'comp-1', 'Agent A')")

	now := time.Now()
	event := &models.CostEvent{
		AgentID:    "agent-1",
		Provider:   "claude",
		Model:      "claude-3",
		CostCents:  200,
		OccurredAt: now,
	}

	_, err := svc.CreateEvent(nil, "comp-1", event)
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}

	var agentSpend int
	db.Raw("SELECT spent_monthly_cents FROM agents WHERE id = 'agent-1'").Scan(&agentSpend)
	if agentSpend != 200 {
		t.Errorf("expected agent spent_monthly_cents 200, got %d", agentSpend)
	}
}

func TestCostService_CreateEvent_UpdatesCompanySpend(t *testing.T) {
	db := setupCostServiceTestDB(t)
	svc := NewCostService(db)

	db.Exec("INSERT INTO companies (id, name) VALUES ('comp-1', 'Test Co')")
	db.Exec("INSERT INTO agents (id, company_id, name) VALUES ('agent-1', 'comp-1', 'Agent A')")
	db.Exec("INSERT INTO agents (id, company_id, name) VALUES ('agent-2', 'comp-1', 'Agent B')")

	now := time.Now()
	_, err := svc.CreateEvent(nil, "comp-1", &models.CostEvent{
		AgentID:    "agent-1",
		Provider:   "claude",
		Model:      "claude-3",
		CostCents:  100,
		OccurredAt: now,
	})
	if err != nil {
		t.Fatalf("CreateEvent 1 failed: %v", err)
	}

	_, err = svc.CreateEvent(nil, "comp-1", &models.CostEvent{
		AgentID:    "agent-2",
		Provider:   "codex",
		Model:      "gpt-4o",
		CostCents:  300,
		OccurredAt: now,
	})
	if err != nil {
		t.Fatalf("CreateEvent 2 failed: %v", err)
	}

	var companySpend int
	db.Raw("SELECT spent_monthly_cents FROM companies WHERE id = 'comp-1'").Scan(&companySpend)
	if companySpend != 400 {
		t.Errorf("expected company spent_monthly_cents 400, got %d", companySpend)
	}
}

func TestCostService_CreateEvent_DefaultsOccurredAt(t *testing.T) {
	db := setupCostServiceTestDB(t)
	svc := NewCostService(db)

	db.Exec("INSERT INTO companies (id, name) VALUES ('comp-1', 'Test Co')")
	db.Exec("INSERT INTO agents (id, company_id, name) VALUES ('agent-1', 'comp-1', 'Agent A')")

	event := &models.CostEvent{
		AgentID:   "agent-1",
		Provider:  "claude",
		Model:     "claude-3",
		CostCents: 50,
	}

	before := time.Now()
	created, err := svc.CreateEvent(nil, "comp-1", event)
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}

	if created.OccurredAt.Before(before) {
		t.Error("expected OccurredAt to be at or after test start time")
	}
}

func TestCostService_CreateEvent_TriggersBudgetEnforcementHook(t *testing.T) {
	db := setupCostServiceTestDB(t)
	svc := NewCostService(db)

	db.Exec("INSERT INTO companies (id, name) VALUES ('comp-1', 'Test Co')")
	db.Exec("INSERT INTO agents (id, company_id, name) VALUES ('agent-1', 'comp-1', 'Agent A')")
	db.Exec(`INSERT INTO budget_policies (id, company_id, scope_type, scope_id, amount, hard_stop_enabled, is_active)
		VALUES ('bp-1', 'comp-1', 'agent', 'agent-1', 100, 1, 1)`)

	var triggered []BudgetScope
	svc.BudgetEnforcementHook = func(_ context.Context, scope BudgetScope) error {
		triggered = append(triggered, scope)
		return nil
	}

	_, err := svc.CreateEvent(context.Background(), "comp-1", &models.CostEvent{
		AgentID:    "agent-1",
		Provider:   "claude",
		Model:      "claude-3",
		CostCents:  150,
		OccurredAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}
	if len(triggered) != 1 {
		t.Fatalf("expected one triggered budget scope, got %#v", triggered)
	}
	if triggered[0].ScopeType != "agent" || triggered[0].ScopeID != "agent-1" {
		t.Fatalf("unexpected triggered scope: %#v", triggered[0])
	}

	var incident models.BudgetIncident
	if err := db.First(&incident, "policy_id = ?", "bp-1").Error; err != nil {
		t.Fatalf("load budget incident: %v", err)
	}
	if incident.ThresholdType != "hard" || incident.Status != "open" {
		t.Fatalf("unexpected incident state: %#v", incident)
	}

	var agent models.Agent
	if err := db.First(&agent, "id = ?", "agent-1").Error; err != nil {
		t.Fatalf("load agent: %v", err)
	}
	if agent.Status != "paused" {
		t.Fatalf("expected agent to be paused, got %q", agent.Status)
	}
	if agent.PauseReason == nil || *agent.PauseReason != "budget" {
		t.Fatalf("expected agent pause reason budget, got %#v", agent.PauseReason)
	}
}
