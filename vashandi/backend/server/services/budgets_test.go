package services

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupBudgetTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&budget_svc_test=1"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.Exec("DROP TABLE IF EXISTS budget_policies")
	db.Exec("DROP TABLE IF EXISTS cost_events")
	db.Exec(`CREATE TABLE budget_policies (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		scope_type text NOT NULL,
		scope_id text NOT NULL,
		metric text NOT NULL DEFAULT 'billed_cents',
		window_kind text NOT NULL DEFAULT 'monthly',
		amount integer NOT NULL DEFAULT 0,
		warn_percent integer NOT NULL DEFAULT 80,
		hard_stop_enabled boolean NOT NULL DEFAULT 1,
		notify_enabled boolean NOT NULL DEFAULT 1,
		is_active boolean NOT NULL DEFAULT 1,
		created_at datetime,
		updated_at datetime
	)`)
	db.Exec(`CREATE TABLE cost_events (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		agent_id text NOT NULL,
		project_id text,
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

func TestCheckProjectBudget_NoPolicyExists(t *testing.T) {
	db := setupBudgetTestDB(t)

	blocked, err := CheckProjectBudget(db, "proj-no-policy")
	if err != nil {
		t.Fatalf("CheckProjectBudget failed: %v", err)
	}
	if blocked {
		t.Error("expected false (no budget policy = unlimited), got true")
	}
}

func TestCheckProjectBudget_WithinBudget(t *testing.T) {
	db := setupBudgetTestDB(t)

	db.Exec("INSERT INTO budget_policies (id, company_id, scope_type, scope_id, amount, is_active, window_kind) VALUES ('bp-1', 'comp-1', 'project', 'proj-1', 1000, 1, 'monthly')")
	db.Exec("INSERT INTO cost_events (id, company_id, agent_id, project_id, provider, model, cost_cents, occurred_at) VALUES ('ce-1', 'comp-1', 'agent-1', 'proj-1', 'claude', 'claude-3', 500, '2026-04-01')")

	blocked, err := CheckProjectBudget(db, "proj-1")
	if err != nil {
		t.Fatalf("CheckProjectBudget failed: %v", err)
	}
	if blocked {
		t.Error("expected false (within budget 500/1000), got true")
	}
}

func TestCheckProjectBudget_ExceedsBudget(t *testing.T) {
	db := setupBudgetTestDB(t)

	db.Exec("INSERT INTO budget_policies (id, company_id, scope_type, scope_id, amount, is_active, window_kind) VALUES ('bp-2', 'comp-1', 'project', 'proj-2', 1000, 1, 'monthly')")
	db.Exec("INSERT INTO cost_events (id, company_id, agent_id, project_id, provider, model, cost_cents, occurred_at) VALUES ('ce-2', 'comp-1', 'agent-1', 'proj-2', 'claude', 'claude-3', 600, '2026-04-01')")
	db.Exec("INSERT INTO cost_events (id, company_id, agent_id, project_id, provider, model, cost_cents, occurred_at) VALUES ('ce-3', 'comp-1', 'agent-1', 'proj-2', 'claude', 'claude-3', 500, '2026-04-02')")

	blocked, err := CheckProjectBudget(db, "proj-2")
	if err != nil {
		t.Fatalf("CheckProjectBudget failed: %v", err)
	}
	if !blocked {
		t.Error("expected true (exceeds budget 1100/1000), got false")
	}
}

func TestCheckProjectBudget_ExactlyAtBudget(t *testing.T) {
	db := setupBudgetTestDB(t)

	db.Exec("INSERT INTO budget_policies (id, company_id, scope_type, scope_id, amount, is_active, window_kind) VALUES ('bp-3', 'comp-1', 'project', 'proj-3', 500, 1, 'monthly')")
	db.Exec("INSERT INTO cost_events (id, company_id, agent_id, project_id, provider, model, cost_cents, occurred_at) VALUES ('ce-4', 'comp-1', 'agent-1', 'proj-3', 'claude', 'claude-3', 500, '2026-04-01')")

	blocked, err := CheckProjectBudget(db, "proj-3")
	if err != nil {
		t.Fatalf("CheckProjectBudget failed: %v", err)
	}
	if !blocked {
		t.Error("expected true (exactly at budget 500/500 should be blocked), got false")
	}
}

func TestCheckProjectBudget_InactivePolicy_Ignored(t *testing.T) {
	db := setupBudgetTestDB(t)

	db.Exec("INSERT INTO budget_policies (id, company_id, scope_type, scope_id, amount, is_active, window_kind) VALUES ('bp-4', 'comp-1', 'project', 'proj-4', 100, 0, 'monthly')")
	db.Exec("INSERT INTO cost_events (id, company_id, agent_id, project_id, provider, model, cost_cents, occurred_at) VALUES ('ce-5', 'comp-1', 'agent-1', 'proj-4', 'claude', 'claude-3', 9999, '2026-04-01')")

	blocked, err := CheckProjectBudget(db, "proj-4")
	if err != nil {
		t.Fatalf("CheckProjectBudget failed: %v", err)
	}
	if blocked {
		t.Error("expected false (inactive policy should be ignored), got true")
	}
}
