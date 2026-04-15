package services

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func setupSchedulerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&scheduler_test=1"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	db.Exec("DROP TABLE IF EXISTS routine_runs")
	db.Exec("DROP TABLE IF EXISTS routine_triggers")
	db.Exec("DROP TABLE IF EXISTS routines")
	db.Exec("DROP TABLE IF EXISTS issues")
	db.Exec("DROP TABLE IF EXISTS heartbeat_runs")
	db.Exec("DROP TABLE IF EXISTS activity_log")

	db.Exec(`CREATE TABLE routines (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		project_id text NOT NULL,
		goal_id text,
		parent_issue_id text,
		title text NOT NULL DEFAULT '',
		description text,
		assignee_agent_id text NOT NULL DEFAULT '',
		priority text NOT NULL DEFAULT 'medium',
		status text NOT NULL DEFAULT 'active',
		concurrency_policy text NOT NULL DEFAULT 'coalesce_if_active',
		catch_up_policy text NOT NULL DEFAULT 'skip_missed',
		variables text NOT NULL DEFAULT '[]',
		created_by_agent_id text,
		created_by_user_id text,
		updated_by_agent_id text,
		updated_by_user_id text,
		last_triggered_at datetime,
		last_enqueued_at datetime,
		created_at datetime DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)
	db.Exec(`CREATE TABLE routine_triggers (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		routine_id text NOT NULL,
		kind text NOT NULL,
		label text,
		enabled integer NOT NULL DEFAULT 1,
		cron_expression text,
		timezone text,
		next_run_at datetime,
		last_fired_at datetime,
		public_id text,
		secret_id text,
		signing_mode text,
		replay_window_sec integer,
		last_rotated_at datetime,
		last_result text,
		created_by_agent_id text,
		created_by_user_id text,
		updated_by_agent_id text,
		updated_by_user_id text,
		created_at datetime DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)
	db.Exec(`CREATE TABLE routine_runs (
		id text PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
		company_id text NOT NULL,
		routine_id text NOT NULL,
		trigger_id text,
		source text NOT NULL,
		status text NOT NULL DEFAULT 'received',
		triggered_at datetime DEFAULT CURRENT_TIMESTAMP,
		idempotency_key text,
		trigger_payload text,
		linked_issue_id text,
		coalesced_into_run_id text,
		failure_reason text,
		completed_at datetime,
		created_at datetime DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)
	db.Exec(`CREATE TABLE issues (
		id text PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
		company_id text NOT NULL,
		project_id text,
		goal_id text,
		parent_id text,
		title text NOT NULL DEFAULT '',
		description text,
		status text NOT NULL DEFAULT 'backlog',
		priority text NOT NULL DEFAULT 'medium',
		assignee_agent_id text,
		assignee_user_id text,
		checkout_run_id text,
		execution_run_id text,
		execution_agent_name_key text,
		execution_locked_at datetime,
		created_by_agent_id text,
		created_by_user_id text,
		issue_number integer,
		identifier text,
		origin_kind text NOT NULL DEFAULT 'manual',
		origin_id text,
		origin_run_id text,
		request_depth integer NOT NULL DEFAULT 0,
		billing_code text,
		assignee_adapter_overrides text,
		execution_workspace_id text,
		execution_workspace_preference text,
		execution_workspace_settings text,
		started_at datetime,
		completed_at datetime,
		cancelled_at datetime,
		hidden_at datetime,
		created_at datetime DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)
	db.Exec(`CREATE TABLE heartbeat_runs (
		id text PRIMARY KEY,
		company_id text NOT NULL DEFAULT '',
		agent_id text NOT NULL DEFAULT '',
		invocation_source text NOT NULL DEFAULT 'on_demand',
		trigger_detail text,
		status text NOT NULL DEFAULT 'queued',
		task_id text NOT NULL DEFAULT '',
		log_compressed integer NOT NULL DEFAULT 0,
		process_loss_retry_count integer NOT NULL DEFAULT 0,
		context_snapshot text,
		created_at datetime DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)
	db.Exec(`CREATE TABLE activity_log (
		id text PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
		company_id text NOT NULL,
		actor_type text NOT NULL DEFAULT 'system',
		actor_id text NOT NULL DEFAULT '',
		action text NOT NULL DEFAULT '',
		entity_type text NOT NULL DEFAULT '',
		entity_id text NOT NULL DEFAULT '',
		agent_id text,
		run_id text,
		details text,
		created_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)

	return db
}

func newTestScheduler(db *gorm.DB) *RoutineSchedulerService {
	activity := NewActivityService(db)
	return &RoutineSchedulerService{
		DB:       db,
		Activity: activity,
	}
}

// ---------------------------------------------------------------------------
// TickScheduledTriggers
// ---------------------------------------------------------------------------

func TestTick_NoDueTriggers(t *testing.T) {
	db := setupSchedulerTestDB(t)
	svc := newTestScheduler(db)

	now := time.Now().UTC()
	result, err := svc.TickScheduledTriggers(context.Background(), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Triggered != 0 {
		t.Errorf("expected 0 triggered, got %d", result.Triggered)
	}
}

func TestTick_SkipsFutureTrigger(t *testing.T) {
	db := setupSchedulerTestDB(t)
	svc := newTestScheduler(db)

	now := time.Now().UTC()
	future := now.Add(10 * time.Minute)

	db.Exec("INSERT INTO routines (id, company_id, project_id, title, assignee_agent_id) VALUES ('r1', 'c1', 'p1', 'T', 'a1')")
	db.Exec("INSERT INTO routine_triggers (id, company_id, routine_id, kind, enabled, cron_expression, timezone, next_run_at) VALUES ('trg1','c1','r1','schedule',1,'* * * * *','UTC',?)", future)

	result, err := svc.TickScheduledTriggers(context.Background(), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Triggered != 0 {
		t.Errorf("expected 0 triggered, got %d", result.Triggered)
	}
}

func TestTick_SkipsDisabledTrigger(t *testing.T) {
	db := setupSchedulerTestDB(t)
	svc := newTestScheduler(db)

	now := time.Now().UTC()
	past := now.Add(-5 * time.Minute)

	db.Exec("INSERT INTO routines (id, company_id, project_id, title, assignee_agent_id) VALUES ('r1','c1','p1','T','a1')")
	db.Exec("INSERT INTO routine_triggers (id, company_id, routine_id, kind, enabled, cron_expression, timezone, next_run_at) VALUES ('trg1','c1','r1','schedule',0,'* * * * *','UTC',?)", past)

	result, err := svc.TickScheduledTriggers(context.Background(), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Triggered != 0 {
		t.Errorf("expected 0 triggered, got %d", result.Triggered)
	}
}

func TestTick_SkipsInactiveRoutine(t *testing.T) {
	db := setupSchedulerTestDB(t)
	svc := newTestScheduler(db)

	now := time.Now().UTC()
	past := now.Add(-5 * time.Minute)

	db.Exec("INSERT INTO routines (id, company_id, project_id, title, assignee_agent_id, status) VALUES ('r1','c1','p1','T','a1','inactive')")
	db.Exec("INSERT INTO routine_triggers (id, company_id, routine_id, kind, enabled, cron_expression, timezone, next_run_at) VALUES ('trg1','c1','r1','schedule',1,'* * * * *','UTC',?)", past)

	result, err := svc.TickScheduledTriggers(context.Background(), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Triggered != 0 {
		t.Errorf("expected 0 triggered, got %d", result.Triggered)
	}
}

func TestTick_DueTrigger_CreatesRoutineRun(t *testing.T) {
	db := setupSchedulerTestDB(t)
	svc := newTestScheduler(db)

	now := time.Now().UTC()
	past := now.Add(-2 * time.Minute)

	db.Exec("INSERT INTO routines (id, company_id, project_id, title, assignee_agent_id) VALUES ('r1','c1','p1','Daily','a1')")
	db.Exec("INSERT INTO routine_triggers (id, company_id, routine_id, kind, enabled, cron_expression, timezone, next_run_at) VALUES ('trg1','c1','r1','schedule',1,'* * * * *','UTC',?)", past)

	result, err := svc.TickScheduledTriggers(context.Background(), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Triggered != 1 {
		t.Errorf("expected 1 triggered, got %d", result.Triggered)
	}

	var count int64
	db.Model(&models.RoutineRun{}).Where("routine_id = ?", "r1").Count(&count)
	if count != 1 {
		t.Errorf("expected 1 routine_run record, got %d", count)
	}
}

func TestTick_AtomicClaim_NoDoubleDispatch(t *testing.T) {
	db := setupSchedulerTestDB(t)
	svc := newTestScheduler(db)

	now := time.Now().UTC()
	past := now.Add(-2 * time.Minute)

	db.Exec("INSERT INTO routines (id, company_id, project_id, title, assignee_agent_id) VALUES ('r1','c1','p1','T','a1')")
	db.Exec("INSERT INTO routine_triggers (id, company_id, routine_id, kind, enabled, cron_expression, timezone, next_run_at) VALUES ('trg1','c1','r1','schedule',1,'* * * * *','UTC',?)", past)

	// Tick twice concurrently — only one should dispatch.
	_, _ = svc.TickScheduledTriggers(context.Background(), now)
	result2, err := svc.TickScheduledTriggers(context.Background(), now)
	if err != nil {
		t.Fatalf("second tick error: %v", err)
	}
	if result2.Triggered != 0 {
		t.Errorf("second tick should dispatch 0 (trigger already claimed), got %d", result2.Triggered)
	}
}

func TestTick_CatchUp_EnqueuesMissedRuns(t *testing.T) {
	db := setupSchedulerTestDB(t)
	svc := newTestScheduler(db)

	now := time.Now().UTC()
	// A trigger that was last set 3 minutes ago with every-minute schedule.
	past := now.Add(-3 * time.Minute)

	db.Exec("INSERT INTO routines (id, company_id, project_id, title, assignee_agent_id, catch_up_policy, concurrency_policy) VALUES ('r1','c1','p1','T','a1','enqueue_missed_with_cap','always_enqueue')")
	db.Exec("INSERT INTO routine_triggers (id, company_id, routine_id, kind, enabled, cron_expression, timezone, next_run_at) VALUES ('trg1','c1','r1','schedule',1,'* * * * *','UTC',?)", past)

	result, err := svc.TickScheduledTriggers(context.Background(), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should dispatch 3 catch-up runs (for minutes -3, -2, -1 relative to now).
	if result.Triggered < 2 {
		t.Errorf("expected at least 2 catch-up runs, got %d", result.Triggered)
	}
}

// ---------------------------------------------------------------------------
// isUniqueConstraintError
// ---------------------------------------------------------------------------

func TestIsUniqueConstraintError_DetectsCode(t *testing.T) {
	err := fmt.Errorf("ERROR: duplicate key value violates unique constraint (SQLSTATE 23505)")
	if !isUniqueConstraintError(err) {
		t.Error("expected true for 23505 error")
	}
}

func TestIsUniqueConstraintError_Nil(t *testing.T) {
	if isUniqueConstraintError(nil) {
		t.Error("expected false for nil error")
	}
}

func TestIsUniqueConstraintError_OtherError(t *testing.T) {
	err := fmt.Errorf("some other db error")
	if isUniqueConstraintError(err) {
		t.Error("expected false for non-unique error")
	}
}
