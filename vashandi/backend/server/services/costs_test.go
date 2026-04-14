package services

import (
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
		permissions text NOT NULL DEFAULT '{}',
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
