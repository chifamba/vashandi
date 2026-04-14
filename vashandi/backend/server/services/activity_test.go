package services

import (
	"context"
	"encoding/json"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupActivityServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&activity_svc_test=1"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.Exec("DROP TABLE IF EXISTS activity_log")
	db.Exec("DROP TABLE IF EXISTS agents")
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
	db.Exec(`CREATE TABLE activity_log (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		actor_type text NOT NULL DEFAULT 'system',
		actor_id text NOT NULL,
		action text NOT NULL,
		entity_type text NOT NULL,
		entity_id text NOT NULL,
		agent_id text,
		run_id text,
		details text,
		created_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)
	return db
}

func TestActivityService_Log(t *testing.T) {
	db := setupActivityServiceTestDB(t)
	svc := NewActivityService(db)

	entry := LogEntry{
		CompanyID:  "comp-1",
		ActorType:  "user",
		ActorID:    "user-1",
		Action:     "issue.created",
		EntityType: "issue",
		EntityID:   "iss-1",
	}

	log, err := svc.Log(context.Background(), entry)
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	if log.CompanyID != "comp-1" {
		t.Errorf("expected CompanyID 'comp-1', got %q", log.CompanyID)
	}
	if log.Action != "issue.created" {
		t.Errorf("expected Action 'issue.created', got %q", log.Action)
	}
	if log.EntityType != "issue" {
		t.Errorf("expected EntityType 'issue', got %q", log.EntityType)
	}
}

func TestActivityService_Log_WithDetails(t *testing.T) {
	db := setupActivityServiceTestDB(t)
	svc := NewActivityService(db)

	entry := LogEntry{
		CompanyID:  "comp-1",
		ActorType:  "agent",
		ActorID:    "agent-1",
		Action:     "run.completed",
		EntityType: "run",
		EntityID:   "run-1",
		Details:    map[string]interface{}{"exitCode": 0, "duration": 42.5},
	}

	log, err := svc.Log(context.Background(), entry)
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	// Verify details were stored
	if log.Details == nil {
		t.Fatal("expected Details to be set")
	}

	var details map[string]interface{}
	if err := json.Unmarshal(log.Details, &details); err != nil {
		t.Fatalf("unmarshal details: %v", err)
	}
	if details["exitCode"] != float64(0) {
		t.Errorf("expected exitCode 0, got %v", details["exitCode"])
	}
}

func TestActivityService_Log_WithAgentAndRunID(t *testing.T) {
	db := setupActivityServiceTestDB(t)
	svc := NewActivityService(db)

	agentID := "agent-abc"
	runID := "run-xyz"
	entry := LogEntry{
		CompanyID:  "comp-1",
		ActorType:  "agent",
		ActorID:    agentID,
		Action:     "issue.checkout",
		EntityType: "issue",
		EntityID:   "iss-1",
		AgentID:    &agentID,
		RunID:      &runID,
	}

	log, err := svc.Log(context.Background(), entry)
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	if log.AgentID == nil || *log.AgentID != agentID {
		t.Errorf("expected AgentID %q, got %v", agentID, log.AgentID)
	}
	if log.RunID == nil || *log.RunID != runID {
		t.Errorf("expected RunID %q, got %v", runID, log.RunID)
	}
}

func TestActivityService_List_CompanyScoping(t *testing.T) {
	db := setupActivityServiceTestDB(t)
	svc := NewActivityService(db)

	// Insert activities for two companies
	db.Exec("INSERT INTO activity_log (id, company_id, actor_type, actor_id, action, entity_type, entity_id) VALUES ('al1', 'comp-a', 'system', 's', 'issue.created', 'issue', 'i1')")
	db.Exec("INSERT INTO activity_log (id, company_id, actor_type, actor_id, action, entity_type, entity_id) VALUES ('al2', 'comp-b', 'system', 's', 'issue.created', 'issue', 'i2')")
	db.Exec("INSERT INTO activity_log (id, company_id, actor_type, actor_id, action, entity_type, entity_id) VALUES ('al3', 'comp-a', 'system', 's', 'run.started', 'run', 'r1')")

	activities, err := svc.List(context.Background(), ActivityFilters{CompanyID: "comp-a"})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(activities) != 2 {
		t.Errorf("expected 2 activities for comp-a, got %d", len(activities))
	}
}

func TestActivityService_List_EntityTypeFilter(t *testing.T) {
	db := setupActivityServiceTestDB(t)
	svc := NewActivityService(db)

	db.Exec("INSERT INTO activity_log (id, company_id, actor_type, actor_id, action, entity_type, entity_id) VALUES ('al1', 'comp-a', 'system', 's', 'issue.created', 'issue', 'i1')")
	db.Exec("INSERT INTO activity_log (id, company_id, actor_type, actor_id, action, entity_type, entity_id) VALUES ('al2', 'comp-a', 'system', 's', 'run.started', 'run', 'r1')")

	activities, err := svc.List(context.Background(), ActivityFilters{CompanyID: "comp-a", EntityType: "issue"})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(activities) != 1 {
		t.Errorf("expected 1 issue activity, got %d", len(activities))
	}
}

func TestActivityService_List_DefaultLimit(t *testing.T) {
	db := setupActivityServiceTestDB(t)
	svc := NewActivityService(db)

	// The default limit is 50 — just verify it doesn't error with no results
	activities, err := svc.List(context.Background(), ActivityFilters{CompanyID: "comp-empty"})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(activities) != 0 {
		t.Errorf("expected 0 activities, got %d", len(activities))
	}
}

func TestActivityService_List_CustomLimit(t *testing.T) {
	db := setupActivityServiceTestDB(t)
	svc := NewActivityService(db)

	for i := 0; i < 5; i++ {
		db.Exec("INSERT INTO activity_log (id, company_id, actor_type, actor_id, action, entity_type, entity_id) VALUES (?, 'comp-a', 'system', 's', 'test', 'test', ?)",
			"al-"+string(rune('a'+i)), "e"+string(rune('0'+i)))
	}

	activities, err := svc.List(context.Background(), ActivityFilters{CompanyID: "comp-a", Limit: 2})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(activities) != 2 {
		t.Errorf("expected 2 activities with Limit=2, got %d", len(activities))
	}
}
