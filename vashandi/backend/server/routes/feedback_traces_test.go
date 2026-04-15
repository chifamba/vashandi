package routes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/go-chi/chi/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/chifamba/vashandi/vashandi/backend/server/services"
)

func setupFeedbackTracesTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dbName := fmt.Sprintf("file:feedback_traces_%s?mode=memory&cache=shared", url.QueryEscape(t.Name()))
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	for _, tbl := range []string{"feedback_exports", "issues", "companies"} {
		db.Exec("DROP TABLE IF EXISTS " + tbl)
	}
	db.Exec(`CREATE TABLE companies (
		id text PRIMARY KEY,
		name text NOT NULL,
		status text NOT NULL DEFAULT 'active',
		issue_prefix text NOT NULL DEFAULT 'PAP',
		issue_counter integer NOT NULL DEFAULT 0,
		budget_monthly_cents integer NOT NULL DEFAULT 0,
		spent_monthly_cents integer NOT NULL DEFAULT 0,
		require_board_approval_for_new_agents boolean NOT NULL DEFAULT 1,
		feedback_data_sharing_enabled boolean NOT NULL DEFAULT 0,
		brand_color text,
		is_archived boolean NOT NULL DEFAULT 0,
		created_at datetime,
		updated_at datetime
	)`)
	db.Exec(`CREATE TABLE issues (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		project_id text,
		project_workspace_id text,
		goal_id text,
		parent_id text,
		title text NOT NULL,
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
		assignee_adapter_overrides text DEFAULT '{}',
		execution_workspace_id text,
		execution_workspace_preference text,
		execution_workspace_settings text DEFAULT '{}',
		started_at datetime,
		completed_at datetime,
		cancelled_at datetime,
		hidden_at datetime,
		created_at datetime,
		updated_at datetime
	)`)
	db.Exec(`CREATE TABLE feedback_exports (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		feedback_vote_id text NOT NULL,
		issue_id text NOT NULL,
		project_id text,
		author_user_id text NOT NULL,
		target_type text NOT NULL,
		target_id text NOT NULL,
		vote text NOT NULL,
		status text NOT NULL DEFAULT 'local_only',
		destination text,
		export_id text,
		consent_version text,
		schema_version text NOT NULL DEFAULT 'paperclip-feedback-envelope-v2',
		bundle_version text NOT NULL DEFAULT 'paperclip-feedback-bundle-v2',
		payload_version text NOT NULL DEFAULT 'paperclip-feedback-v1',
		payload_digest text,
		payload_snapshot text,
		target_summary text NOT NULL DEFAULT '{}',
		redaction_summary text,
		attempt_count integer NOT NULL DEFAULT 0,
		last_attempted_at datetime,
		exported_at datetime,
		failure_reason text,
		created_at datetime DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)

	db.Exec("INSERT INTO companies (id, name, issue_prefix, issue_counter) VALUES ('comp-a', 'Alpha', 'ALP', 5)")
	db.Exec("INSERT INTO issues (id, company_id, title, identifier, status, priority, origin_kind) VALUES ('issue-1', 'comp-a', 'My Issue', 'ALP-1', 'backlog', 'medium', 'manual')")
	db.Exec(`INSERT INTO feedback_exports (id, company_id, feedback_vote_id, issue_id, author_user_id, target_type, target_id, vote, status, target_summary)
		VALUES ('trace-1', 'comp-a', 'vote-1', 'issue-1', 'user-1', 'issue_comment', 'comment-1', 'up', 'local_only', '{}')`)
	db.Exec(`INSERT INTO feedback_exports (id, company_id, feedback_vote_id, issue_id, author_user_id, target_type, target_id, vote, status, target_summary)
		VALUES ('trace-2', 'comp-a', 'vote-2', 'issue-1', 'user-1', 'issue_comment', 'comment-2', 'down', 'pending', '{}')`)
	return db
}

func newIssueRoutesForFeedback(t *testing.T, db *gorm.DB) *IssueRoutes {
	t.Helper()
	actSvc := services.NewActivityService(db)
	return NewIssueRoutes(db, actSvc)
}

// ---------- ListIssueFeedbackTracesHandler ----------

func TestListIssueFeedbackTracesHandler_BoardOnly(t *testing.T) {
	db := setupFeedbackTracesTestDB(t)
	ir := newIssueRoutesForFeedback(t, db)
	router := chi.NewRouter()
	router.Get("/issues/{id}/feedback-traces", ir.ListIssueFeedbackTracesHandler)

	req := httptest.NewRequest(http.MethodGet, "/issues/issue-1/feedback-traces", nil)
	// No actor = anonymous, not board
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestListIssueFeedbackTracesHandler_ReturnsTraces(t *testing.T) {
	db := setupFeedbackTracesTestDB(t)
	ir := newIssueRoutesForFeedback(t, db)
	router := chi.NewRouter()
	router.Get("/issues/{id}/feedback-traces", ir.ListIssueFeedbackTracesHandler)

	req := httptest.NewRequest(http.MethodGet, "/issues/issue-1/feedback-traces", nil)
	req = req.WithContext(WithActor(req.Context(), ActorInfo{UserID: "user-1", ActorType: "board"}))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	var traces []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&traces)
	if len(traces) != 2 {
		t.Errorf("expected 2 traces, got %d", len(traces))
	}
}

func TestListIssueFeedbackTracesHandler_IssueNotFound(t *testing.T) {
	db := setupFeedbackTracesTestDB(t)
	ir := newIssueRoutesForFeedback(t, db)
	router := chi.NewRouter()
	router.Get("/issues/{id}/feedback-traces", ir.ListIssueFeedbackTracesHandler)

	req := httptest.NewRequest(http.MethodGet, "/issues/no-such-issue/feedback-traces", nil)
	req = req.WithContext(WithActor(req.Context(), ActorInfo{UserID: "user-1", ActorType: "board"}))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestListIssueFeedbackTracesHandler_StatusFilter(t *testing.T) {
	db := setupFeedbackTracesTestDB(t)
	ir := newIssueRoutesForFeedback(t, db)
	router := chi.NewRouter()
	router.Get("/issues/{id}/feedback-traces", ir.ListIssueFeedbackTracesHandler)

	req := httptest.NewRequest(http.MethodGet, "/issues/issue-1/feedback-traces?status=pending", nil)
	req = req.WithContext(WithActor(req.Context(), ActorInfo{UserID: "user-1", ActorType: "board"}))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	var traces []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&traces)
	if len(traces) != 1 {
		t.Errorf("expected 1 pending trace, got %d", len(traces))
	}
}

func TestListIssueFeedbackTracesHandler_SharedOnly(t *testing.T) {
	db := setupFeedbackTracesTestDB(t)
	ir := newIssueRoutesForFeedback(t, db)
	router := chi.NewRouter()
	router.Get("/issues/{id}/feedback-traces", ir.ListIssueFeedbackTracesHandler)

	req := httptest.NewRequest(http.MethodGet, "/issues/issue-1/feedback-traces?sharedOnly=true", nil)
	req = req.WithContext(WithActor(req.Context(), ActorInfo{UserID: "user-1", ActorType: "board"}))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	var traces []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&traces)
	// trace-2 is "pending" (not local_only), trace-1 is "local_only"
	if len(traces) != 1 {
		t.Errorf("expected 1 shared trace, got %d", len(traces))
	}
}

func TestListIssueFeedbackTracesHandler_PayloadExcludedByDefault(t *testing.T) {
	db := setupFeedbackTracesTestDB(t)
	db.Exec(`UPDATE feedback_exports SET payload_snapshot = '{"secret":"data"}' WHERE id = 'trace-1'`)
	ir := newIssueRoutesForFeedback(t, db)
	router := chi.NewRouter()
	router.Get("/issues/{id}/feedback-traces", ir.ListIssueFeedbackTracesHandler)

	req := httptest.NewRequest(http.MethodGet, "/issues/issue-1/feedback-traces", nil)
	req = req.WithContext(WithActor(req.Context(), ActorInfo{UserID: "user-1", ActorType: "board"}))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var traces []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&traces)
	if len(traces) == 0 {
		t.Fatal("expected at least one trace")
	}
	if traces[0]["payloadSnapshot"] != nil {
		t.Errorf("expected payloadSnapshot to be nil when includePayload not set, got %v", traces[0]["payloadSnapshot"])
	}
}

// ---------- GetFeedbackTraceByIDHandler ----------

func TestGetFeedbackTraceByIDHandler_BoardOnly(t *testing.T) {
	db := setupFeedbackTracesTestDB(t)
	ir := newIssueRoutesForFeedback(t, db)
	router := chi.NewRouter()
	router.Get("/feedback-traces/{traceId}", ir.GetFeedbackTraceByIDHandler)

	req := httptest.NewRequest(http.MethodGet, "/feedback-traces/trace-1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestGetFeedbackTraceByIDHandler_Found(t *testing.T) {
	db := setupFeedbackTracesTestDB(t)
	ir := newIssueRoutesForFeedback(t, db)
	router := chi.NewRouter()
	router.Get("/feedback-traces/{traceId}", ir.GetFeedbackTraceByIDHandler)

	req := httptest.NewRequest(http.MethodGet, "/feedback-traces/trace-1", nil)
	req = req.WithContext(WithActor(req.Context(), ActorInfo{UserID: "user-1", ActorType: "board"}))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	var trace map[string]interface{}
	json.NewDecoder(w.Body).Decode(&trace)
	if trace["id"] != "trace-1" {
		t.Errorf("expected trace id 'trace-1', got %v", trace["id"])
	}
	if trace["issueTitle"] != "My Issue" {
		t.Errorf("expected issueTitle 'My Issue', got %v", trace["issueTitle"])
	}
}

func TestGetFeedbackTraceByIDHandler_NotFound(t *testing.T) {
	db := setupFeedbackTracesTestDB(t)
	ir := newIssueRoutesForFeedback(t, db)
	router := chi.NewRouter()
	router.Get("/feedback-traces/{traceId}", ir.GetFeedbackTraceByIDHandler)

	req := httptest.NewRequest(http.MethodGet, "/feedback-traces/no-such-trace", nil)
	req = req.WithContext(WithActor(req.Context(), ActorInfo{UserID: "user-1", ActorType: "board"}))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestGetFeedbackTraceByIDHandler_IncludesPayloadByDefault(t *testing.T) {
	db := setupFeedbackTracesTestDB(t)
	db.Exec(`UPDATE feedback_exports SET payload_snapshot = '{"key":"value"}' WHERE id = 'trace-1'`)
	ir := newIssueRoutesForFeedback(t, db)
	router := chi.NewRouter()
	router.Get("/feedback-traces/{traceId}", ir.GetFeedbackTraceByIDHandler)

	req := httptest.NewRequest(http.MethodGet, "/feedback-traces/trace-1", nil)
	req = req.WithContext(WithActor(req.Context(), ActorInfo{UserID: "user-1", ActorType: "board"}))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var trace map[string]interface{}
	json.NewDecoder(w.Body).Decode(&trace)
	if trace["payloadSnapshot"] == nil {
		t.Errorf("expected payloadSnapshot to be present by default")
	}
}

// ---------- GetFeedbackTraceBundleHandler ----------

func TestGetFeedbackTraceBundleHandler_BoardOnly(t *testing.T) {
	db := setupFeedbackTracesTestDB(t)
	ir := newIssueRoutesForFeedback(t, db)
	router := chi.NewRouter()
	router.Get("/feedback-traces/{traceId}/bundle", ir.GetFeedbackTraceBundleHandler)

	req := httptest.NewRequest(http.MethodGet, "/feedback-traces/trace-1/bundle", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestGetFeedbackTraceBundleHandler_Found(t *testing.T) {
	db := setupFeedbackTracesTestDB(t)
	ir := newIssueRoutesForFeedback(t, db)
	router := chi.NewRouter()
	router.Get("/feedback-traces/{traceId}/bundle", ir.GetFeedbackTraceBundleHandler)

	req := httptest.NewRequest(http.MethodGet, "/feedback-traces/trace-1/bundle", nil)
	req = req.WithContext(WithActor(req.Context(), ActorInfo{UserID: "user-1", ActorType: "board"}))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	var bundle map[string]interface{}
	json.NewDecoder(w.Body).Decode(&bundle)
	if bundle["traceId"] != "trace-1" {
		t.Errorf("expected traceId 'trace-1', got %v", bundle["traceId"])
	}
	if bundle["companyId"] != "comp-a" {
		t.Errorf("expected companyId 'comp-a', got %v", bundle["companyId"])
	}
	if bundle["captureStatus"] != "unavailable" {
		t.Errorf("expected captureStatus 'unavailable', got %v", bundle["captureStatus"])
	}
	files, ok := bundle["files"].([]interface{})
	if !ok {
		t.Errorf("expected files to be array, got %T", bundle["files"])
	} else if len(files) != 0 {
		t.Errorf("expected 0 files in simplified bundle, got %d", len(files))
	}
}

func TestGetFeedbackTraceBundleHandler_NotFound(t *testing.T) {
	db := setupFeedbackTracesTestDB(t)
	ir := newIssueRoutesForFeedback(t, db)
	router := chi.NewRouter()
	router.Get("/feedback-traces/{traceId}/bundle", ir.GetFeedbackTraceBundleHandler)

	req := httptest.NewRequest(http.MethodGet, "/feedback-traces/no-such-trace/bundle", nil)
	req = req.WithContext(WithActor(req.Context(), ActorInfo{UserID: "user-1", ActorType: "board"}))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ---------- ListCompanyFeedbackTracesHandler ----------

func TestListCompanyFeedbackTracesHandler_BoardOnly(t *testing.T) {
	db := setupFeedbackTracesTestDB(t)
	router := chi.NewRouter()
	router.Get("/companies/{companyId}/feedback-traces", ListCompanyFeedbackTracesHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-a/feedback-traces", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestListCompanyFeedbackTracesHandler_ReturnsTraces(t *testing.T) {
	db := setupFeedbackTracesTestDB(t)
	router := chi.NewRouter()
	router.Get("/companies/{companyId}/feedback-traces", ListCompanyFeedbackTracesHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-a/feedback-traces", nil)
	req = req.WithContext(WithActor(req.Context(), ActorInfo{UserID: "user-1", ActorType: "board"}))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	var traces []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&traces)
	if len(traces) != 2 {
		t.Errorf("expected 2 traces, got %d", len(traces))
	}
}

func TestListCompanyFeedbackTracesHandler_VoteFilter(t *testing.T) {
	db := setupFeedbackTracesTestDB(t)
	router := chi.NewRouter()
	router.Get("/companies/{companyId}/feedback-traces", ListCompanyFeedbackTracesHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-a/feedback-traces?vote=up", nil)
	req = req.WithContext(WithActor(req.Context(), ActorInfo{UserID: "user-1", ActorType: "board"}))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	var traces []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&traces)
	if len(traces) != 1 {
		t.Errorf("expected 1 'up' trace, got %d", len(traces))
	}
}
