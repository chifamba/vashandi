package services

import (
	"context"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupGoalServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbName := fmt.Sprintf("file::memory:?cache=shared&goal_svc_%s=1", url.QueryEscape(t.Name()))
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	db.Exec("DROP TABLE IF EXISTS goals")
	db.Exec(`CREATE TABLE goals (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		title text NOT NULL,
		description text,
		level text NOT NULL DEFAULT 'task',
		status text NOT NULL DEFAULT 'planned',
		parent_id text,
		owner_agent_id text,
		created_at datetime DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)

	return db
}

func TestGoalService_ListGoals_CompanyScoping(t *testing.T) {
	db := setupGoalServiceTestDB(t)
	svc := NewGoalService(db)

	db.Exec("INSERT INTO goals (id, company_id, title, level, status) VALUES ('g1', 'comp-a', 'Goal A', 'task', 'planned')")
	db.Exec("INSERT INTO goals (id, company_id, title, level, status) VALUES ('g2', 'comp-b', 'Goal B', 'task', 'planned')")
	db.Exec("INSERT INTO goals (id, company_id, title, level, status) VALUES ('g3', 'comp-a', 'Goal C', 'company', 'active')")

	goals, err := svc.ListGoals(context.Background(), "comp-a")
	if err != nil {
		t.Fatalf("ListGoals failed: %v", err)
	}
	if len(goals) != 2 {
		t.Fatalf("expected 2 goals, got %d", len(goals))
	}
	for _, goal := range goals {
		if goal.CompanyID != "comp-a" {
			t.Fatalf("expected company scoping to comp-a, got %q", goal.CompanyID)
		}
	}
}

func TestGoalService_GetGoalByID(t *testing.T) {
	db := setupGoalServiceTestDB(t)
	svc := NewGoalService(db)

	db.Exec("INSERT INTO goals (id, company_id, title, level, status) VALUES ('goal-1', 'comp-a', 'My Goal', 'task', 'planned')")

	goal, err := svc.GetGoalByID(context.Background(), "goal-1")
	if err != nil {
		t.Fatalf("GetGoalByID failed: %v", err)
	}
	if goal == nil {
		t.Fatal("expected goal, got nil")
	}
	if goal.Title != "My Goal" {
		t.Fatalf("expected title My Goal, got %q", goal.Title)
	}

	missing, err := svc.GetGoalByID(context.Background(), "missing")
	if err != nil {
		t.Fatalf("GetGoalByID missing failed: %v", err)
	}
	if missing != nil {
		t.Fatalf("expected nil for missing goal, got %+v", missing)
	}
}

func TestGoalService_GetDefaultCompanyGoal_PrefersEarliestActiveRoot(t *testing.T) {
	db := setupGoalServiceTestDB(t)
	svc := NewGoalService(db)

	db.Exec("INSERT INTO goals (id, company_id, title, level, status, parent_id, created_at) VALUES ('other-company', 'comp-b', 'Other', 'company', 'active', NULL, '2025-01-01T00:00:00Z')")
	db.Exec("INSERT INTO goals (id, company_id, title, level, status, parent_id, created_at) VALUES ('later-root', 'comp-a', 'Later', 'company', 'active', NULL, '2025-02-01T00:00:00Z')")
	db.Exec("INSERT INTO goals (id, company_id, title, level, status, parent_id, created_at) VALUES ('earliest-root', 'comp-a', 'Earliest', 'company', 'active', NULL, '2025-01-01T00:00:00Z')")
	db.Exec("INSERT INTO goals (id, company_id, title, level, status, parent_id, created_at) VALUES ('child-root', 'comp-a', 'Child', 'company', 'active', 'parent-1', '2024-01-01T00:00:00Z')")

	goal, err := svc.GetDefaultCompanyGoal(context.Background(), "comp-a")
	if err != nil {
		t.Fatalf("GetDefaultCompanyGoal failed: %v", err)
	}
	if goal == nil {
		t.Fatal("expected goal, got nil")
	}
	if goal.ID != "earliest-root" {
		t.Fatalf("expected earliest active root, got %q", goal.ID)
	}
}

func TestGoalService_GetDefaultCompanyGoal_Fallbacks(t *testing.T) {
	db := setupGoalServiceTestDB(t)
	svc := NewGoalService(db)

	db.Exec("INSERT INTO goals (id, company_id, title, level, status, parent_id, created_at) VALUES ('planned-root', 'comp-a', 'Planned Root', 'company', 'planned', NULL, '2025-01-01T00:00:00Z')")
	db.Exec("INSERT INTO goals (id, company_id, title, level, status, parent_id, created_at) VALUES ('active-child', 'comp-a', 'Active Child', 'company', 'active', 'parent-1', '2024-01-01T00:00:00Z')")

	goal, err := svc.GetDefaultCompanyGoal(context.Background(), "comp-a")
	if err != nil {
		t.Fatalf("GetDefaultCompanyGoal fallback-to-root failed: %v", err)
	}
	if goal == nil || goal.ID != "planned-root" {
		t.Fatalf("expected planned-root fallback, got %+v", goal)
	}

	db.Exec("DELETE FROM goals")
	db.Exec("INSERT INTO goals (id, company_id, title, level, status, parent_id, created_at) VALUES ('company-non-root', 'comp-a', 'Company Goal', 'company', 'blocked', 'parent-1', '2025-01-01T00:00:00Z')")

	goal, err = svc.GetDefaultCompanyGoal(context.Background(), "comp-a")
	if err != nil {
		t.Fatalf("GetDefaultCompanyGoal fallback-to-company-level failed: %v", err)
	}
	if goal == nil || goal.ID != "company-non-root" {
		t.Fatalf("expected company-level fallback, got %+v", goal)
	}

	goal, err = svc.GetDefaultCompanyGoal(context.Background(), "missing-company")
	if err != nil {
		t.Fatalf("GetDefaultCompanyGoal empty failed: %v", err)
	}
	if goal != nil {
		t.Fatalf("expected nil for empty company, got %+v", goal)
	}
}

func TestGoalService_CreateGoal_ScopesAndDefaults(t *testing.T) {
	db := setupGoalServiceTestDB(t)
	svc := NewGoalService(db)

	description := "Grow revenue"
	ownerAgentID := "agent-1"
	created, err := svc.CreateGoal(context.Background(), "comp-a", &models.Goal{
		ID:           "goal-new",
		Title:        "Company Goal",
		Description:  &description,
		OwnerAgentID: &ownerAgentID,
	})
	if err != nil {
		t.Fatalf("CreateGoal failed: %v", err)
	}
	if created.CompanyID != "comp-a" {
		t.Fatalf("expected company comp-a, got %q", created.CompanyID)
	}
	if created.Level != "task" {
		t.Fatalf("expected default level task, got %q", created.Level)
	}
	if created.Status != "planned" {
		t.Fatalf("expected default status planned, got %q", created.Status)
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Fatal("expected timestamps to be populated")
	}

	var stored models.Goal
	if err := db.Where("id = ?", "goal-new").First(&stored).Error; err != nil {
		t.Fatalf("load stored goal: %v", err)
	}
	if stored.CompanyID != "comp-a" || stored.Title != "Company Goal" {
		t.Fatalf("unexpected stored goal: %+v", stored)
	}
}

func TestGoalService_UpdateGoal_PartialUpdateAndTimestamp(t *testing.T) {
	db := setupGoalServiceTestDB(t)
	svc := NewGoalService(db)

	oldTime := time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)
	db.Exec(
		"INSERT INTO goals (id, company_id, title, description, level, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		"goal-upd", "comp-a", "Old Title", "Old Description", "task", "planned", oldTime, oldTime,
	)

	updated, err := svc.UpdateGoal(context.Background(), "goal-upd", map[string]interface{}{
		"title":  "New Title",
		"status": "active",
	})
	if err != nil {
		t.Fatalf("UpdateGoal failed: %v", err)
	}
	if updated == nil {
		t.Fatal("expected updated goal, got nil")
	}
	if updated.Title != "New Title" {
		t.Fatalf("expected updated title, got %q", updated.Title)
	}
	if updated.Status != "active" {
		t.Fatalf("expected updated status active, got %q", updated.Status)
	}
	if updated.Description == nil || *updated.Description != "Old Description" {
		t.Fatalf("expected description to remain unchanged, got %+v", updated.Description)
	}
	if !updated.UpdatedAt.After(oldTime) {
		t.Fatalf("expected UpdatedAt after %v, got %v", oldTime, updated.UpdatedAt)
	}

	missing, err := svc.UpdateGoal(context.Background(), "missing", map[string]interface{}{"title": "x"})
	if err != nil {
		t.Fatalf("UpdateGoal missing failed: %v", err)
	}
	if missing != nil {
		t.Fatalf("expected nil for missing update, got %+v", missing)
	}
}

func TestGoalService_RemoveGoal_ReturnsDeletedRecord(t *testing.T) {
	db := setupGoalServiceTestDB(t)
	svc := NewGoalService(db)

	db.Exec("INSERT INTO goals (id, company_id, title, level, status) VALUES ('goal-del', 'comp-a', 'Delete Me', 'task', 'planned')")

	removed, err := svc.RemoveGoal(context.Background(), "goal-del")
	if err != nil {
		t.Fatalf("RemoveGoal failed: %v", err)
	}
	if removed == nil || removed.ID != "goal-del" {
		t.Fatalf("expected deleted goal-del, got %+v", removed)
	}

	var count int64
	if err := db.Model(&models.Goal{}).Where("id = ?", "goal-del").Count(&count).Error; err != nil {
		t.Fatalf("count remaining goals: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected goal to be deleted, count=%d", count)
	}

	missing, err := svc.RemoveGoal(context.Background(), "missing")
	if err != nil {
		t.Fatalf("RemoveGoal missing failed: %v", err)
	}
	if missing != nil {
		t.Fatalf("expected nil for missing delete, got %+v", missing)
	}
}
