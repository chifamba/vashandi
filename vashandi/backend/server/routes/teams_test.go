package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
)

func setupTeamsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&teams_test=1"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.Exec("DROP TABLE IF EXISTS teams")
	db.Exec(`CREATE TABLE teams (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		name text NOT NULL,
		description text,
		lead_agent_id text,
		status text NOT NULL DEFAULT 'active',
		created_at datetime,
		updated_at datetime
	)`)
	return db
}

func TestTeamsHandler_CompanyScoping(t *testing.T) {
	db := setupTeamsTestDB(t)
	db.Exec("INSERT INTO teams (id, company_id, name) VALUES ('t1', 'comp-a', 'Alpha Team')")
	db.Exec("INSERT INTO teams (id, company_id, name) VALUES ('t2', 'comp-b', 'Beta Team')")

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/teams", TeamsHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-a/teams", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var teams []models.Team
	json.NewDecoder(w.Body).Decode(&teams)
	if len(teams) != 1 {
		t.Errorf("expected 1 team for comp-a, got %d", len(teams))
	}
	if len(teams) > 0 && teams[0].Name != "Alpha Team" {
		t.Errorf("expected name 'Alpha Team', got %q", teams[0].Name)
	}
}

func TestTeamsHandler_MissingCompanyID(t *testing.T) {
	db := setupTeamsTestDB(t)

	req := httptest.NewRequest(http.MethodGet, "/teams", nil)
	w := httptest.NewRecorder()

	TeamsHandler(db)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 when companyId is missing, got %d", w.Code)
	}
}

func TestTeamHandler_Found(t *testing.T) {
	db := setupTeamsTestDB(t)
	db.Exec("INSERT INTO teams (id, company_id, name) VALUES ('team-abc', 'comp-1', 'My Team')")

	router := chi.NewRouter()
	router.Get("/teams/{teamId}", TeamHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/teams/team-abc", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var team models.Team
	json.NewDecoder(w.Body).Decode(&team)
	if team.ID != "team-abc" {
		t.Errorf("expected ID 'team-abc', got %q", team.ID)
	}
}

func TestTeamHandler_NotFound(t *testing.T) {
	db := setupTeamsTestDB(t)

	router := chi.NewRouter()
	router.Get("/teams/{teamId}", TeamHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/teams/missing", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestTeamHandler_MissingTeamID(t *testing.T) {
	db := setupTeamsTestDB(t)

	req := httptest.NewRequest(http.MethodGet, "/teams/", nil)
	w := httptest.NewRecorder()

	TeamHandler(db)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 when teamId is missing, got %d", w.Code)
	}
}
