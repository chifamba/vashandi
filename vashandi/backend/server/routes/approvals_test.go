package routes

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
)

func setupApprovalsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&approvals_test=1"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.Exec("DROP TABLE IF EXISTS approval_comments")
	db.Exec("DROP TABLE IF EXISTS issue_approvals")
	db.Exec("DROP TABLE IF EXISTS approvals")
	db.Exec(`CREATE TABLE approvals (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		type text NOT NULL DEFAULT 'generic',
		requested_by_agent_id text,
		requested_by_user_id text,
		status text NOT NULL DEFAULT 'pending',
		payload text NOT NULL DEFAULT '{}',
		decision_note text,
		decided_by_user_id text,
		decided_at datetime,
		created_at datetime,
		updated_at datetime
	)`)
	db.Exec(`CREATE TABLE approval_comments (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		approval_id text NOT NULL,
		author_agent_id text,
		author_user_id text,
		body text NOT NULL,
		created_at datetime,
		updated_at datetime
	)`)
	db.Exec(`CREATE TABLE issue_approvals (
		id text PRIMARY KEY,
		approval_id text NOT NULL,
		issue_id text NOT NULL,
		created_at datetime
	)`)
	return db
}

func TestListApprovalsHandler_CompanyScoping(t *testing.T) {
	db := setupApprovalsTestDB(t)
	db.Exec("INSERT INTO approvals (id, company_id, type, status, payload) VALUES ('a1', 'comp-a', 'run', 'pending', '{}')")
	db.Exec("INSERT INTO approvals (id, company_id, type, status, payload) VALUES ('a2', 'comp-b', 'run', 'pending', '{}')")
	db.Exec("INSERT INTO approvals (id, company_id, type, status, payload) VALUES ('a3', 'comp-a', 'run', 'approved', '{}')")

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/approvals", ListApprovalsHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-a/approvals", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var approvals []models.Approval
	json.NewDecoder(w.Body).Decode(&approvals)
	if len(approvals) != 2 {
		t.Errorf("expected 2 approvals for comp-a, got %d", len(approvals))
	}
}

func TestListApprovalsHandler_StatusFilter(t *testing.T) {
	db := setupApprovalsTestDB(t)
	db.Exec("INSERT INTO approvals (id, company_id, type, status, payload) VALUES ('a1', 'comp-a', 'run', 'pending', '{}')")
	db.Exec("INSERT INTO approvals (id, company_id, type, status, payload) VALUES ('a2', 'comp-a', 'run', 'approved', '{}')")

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/approvals", ListApprovalsHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-a/approvals?status=pending", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var approvals []models.Approval
	json.NewDecoder(w.Body).Decode(&approvals)
	if len(approvals) != 1 {
		t.Errorf("expected 1 pending approval, got %d", len(approvals))
	}
	if len(approvals) > 0 && approvals[0].Status != "pending" {
		t.Errorf("expected status 'pending', got %q", approvals[0].Status)
	}
}

func TestCreateApprovalHandler_CompanyScoping(t *testing.T) {
	db := setupApprovalsTestDB(t)

	router := chi.NewRouter()
	router.Post("/companies/{companyId}/approvals", CreateApprovalHandler(db))

	body, _ := json.Marshal(map[string]interface{}{
		"type":    "run",
		"payload": map[string]string{"action": "deploy"},
	})
	req := httptest.NewRequest(http.MethodPost, "/companies/comp-xyz/approvals", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", w.Code, w.Body.String())
	}

	var approval models.Approval
	json.NewDecoder(w.Body).Decode(&approval)
	if approval.CompanyID != "comp-xyz" {
		t.Errorf("expected CompanyID 'comp-xyz', got %q", approval.CompanyID)
	}
	if approval.Status != "pending" {
		t.Errorf("expected status 'pending', got %q", approval.Status)
	}
}

func TestCreateApprovalHandler_BadBody(t *testing.T) {
	db := setupApprovalsTestDB(t)

	router := chi.NewRouter()
	router.Post("/companies/{companyId}/approvals", CreateApprovalHandler(db))

	req := httptest.NewRequest(http.MethodPost, "/companies/comp-1/approvals", bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestGetApprovalHandler_Found(t *testing.T) {
	db := setupApprovalsTestDB(t)
	db.Exec("INSERT INTO approvals (id, company_id, type, status, payload) VALUES ('appr-1', 'comp-1', 'run', 'pending', '{}')")

	router := chi.NewRouter()
	router.Get("/approvals/{id}", GetApprovalHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/approvals/appr-1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var approval models.Approval
	json.NewDecoder(w.Body).Decode(&approval)
	if approval.ID != "appr-1" {
		t.Errorf("expected ID 'appr-1', got %q", approval.ID)
	}
}

func TestGetApprovalHandler_NotFound(t *testing.T) {
	db := setupApprovalsTestDB(t)

	router := chi.NewRouter()
	router.Get("/approvals/{id}", GetApprovalHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/approvals/missing", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestApproveHandler_SetsApprovedStatus(t *testing.T) {
	db := setupApprovalsTestDB(t)
	db.Exec("INSERT INTO approvals (id, company_id, type, status, payload) VALUES ('appr-2', 'comp-1', 'run', 'pending', '{}')")

	router := chi.NewRouter()
	router.Put("/approvals/{id}/approve", ApproveHandler(db, nil))

	req := httptest.NewRequest(http.MethodPut, "/approvals/appr-2/approve", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	var approval models.Approval
	json.NewDecoder(w.Body).Decode(&approval)
	if approval.Status != "approved" {
		t.Errorf("expected status 'approved', got %q", approval.Status)
	}
	if approval.DecidedAt == nil {
		t.Error("expected DecidedAt to be set")
	}
}

func TestApproveHandler_NotFound(t *testing.T) {
	db := setupApprovalsTestDB(t)

	router := chi.NewRouter()
	router.Put("/approvals/{id}/approve", ApproveHandler(db, nil))

	req := httptest.NewRequest(http.MethodPut, "/approvals/missing/approve", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestRejectHandler_SetsRejectedStatus(t *testing.T) {
	db := setupApprovalsTestDB(t)
	db.Exec("INSERT INTO approvals (id, company_id, type, status, payload) VALUES ('appr-3', 'comp-1', 'run', 'pending', '{}')")

	router := chi.NewRouter()
	router.Put("/approvals/{id}/reject", RejectHandler(db))

	req := httptest.NewRequest(http.MethodPut, "/approvals/appr-3/reject", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	var approval models.Approval
	json.NewDecoder(w.Body).Decode(&approval)
	if approval.Status != "rejected" {
		t.Errorf("expected status 'rejected', got %q", approval.Status)
	}
	if approval.DecidedAt == nil {
		t.Error("expected DecidedAt to be set")
	}
}

func TestRejectHandler_NotFound(t *testing.T) {
	db := setupApprovalsTestDB(t)

	router := chi.NewRouter()
	router.Put("/approvals/{id}/reject", RejectHandler(db))

	req := httptest.NewRequest(http.MethodPut, "/approvals/missing/reject", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestResubmitApprovalHandler_ResetsToPending(t *testing.T) {
	db := setupApprovalsTestDB(t)
	db.Exec("INSERT INTO approvals (id, company_id, type, status, payload, decided_at) VALUES ('appr-4', 'comp-1', 'run', 'rejected', '{}', '2026-01-01')")

	router := chi.NewRouter()
	router.Put("/approvals/{id}/resubmit", ResubmitApprovalHandler(db))

	req := httptest.NewRequest(http.MethodPut, "/approvals/appr-4/resubmit", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	var approval models.Approval
	json.NewDecoder(w.Body).Decode(&approval)
	if approval.Status != "pending" {
		t.Errorf("expected status 'pending' after resubmit, got %q", approval.Status)
	}
	if approval.DecidedAt != nil {
		t.Error("expected DecidedAt to be nil after resubmit")
	}
}

func TestResubmitApprovalHandler_NotFound(t *testing.T) {
	db := setupApprovalsTestDB(t)

	router := chi.NewRouter()
	router.Put("/approvals/{id}/resubmit", ResubmitApprovalHandler(db))

	req := httptest.NewRequest(http.MethodPut, "/approvals/missing/resubmit", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestAddApprovalCommentHandler_CreatesComment(t *testing.T) {
	db := setupApprovalsTestDB(t)
	db.Exec("INSERT INTO approvals (id, company_id, type, status, payload) VALUES ('appr-5', 'comp-1', 'run', 'pending', '{}')")

	router := chi.NewRouter()
	router.Post("/approvals/{id}/comments", AddApprovalCommentHandler(db))

	body, _ := json.Marshal(map[string]string{"body": "Looks good"})
	req := httptest.NewRequest(http.MethodPost, "/approvals/appr-5/comments", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", w.Code, w.Body.String())
	}

	var comment models.ApprovalComment
	json.NewDecoder(w.Body).Decode(&comment)
	if comment.ApprovalID != "appr-5" {
		t.Errorf("expected ApprovalID 'appr-5', got %q", comment.ApprovalID)
	}
	if comment.CompanyID != "comp-1" {
		t.Errorf("expected CompanyID 'comp-1', got %q", comment.CompanyID)
	}
	if comment.Body != "Looks good" {
		t.Errorf("expected body 'Looks good', got %q", comment.Body)
	}
}

func TestAddApprovalCommentHandler_ApprovalNotFound(t *testing.T) {
	db := setupApprovalsTestDB(t)

	router := chi.NewRouter()
	router.Post("/approvals/{id}/comments", AddApprovalCommentHandler(db))

	body, _ := json.Marshal(map[string]string{"body": "Comment"})
	req := httptest.NewRequest(http.MethodPost, "/approvals/missing/comments", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestGetApprovalCommentsHandler(t *testing.T) {
	db := setupApprovalsTestDB(t)
	db.Exec("INSERT INTO approval_comments (id, company_id, approval_id, body) VALUES ('c1', 'comp-1', 'appr-1', 'First')")
	db.Exec("INSERT INTO approval_comments (id, company_id, approval_id, body) VALUES ('c2', 'comp-1', 'appr-1', 'Second')")
	db.Exec("INSERT INTO approval_comments (id, company_id, approval_id, body) VALUES ('c3', 'comp-1', 'appr-2', 'Other')")

	router := chi.NewRouter()
	router.Get("/approvals/{id}/comments", GetApprovalCommentsHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/approvals/appr-1/comments", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var comments []models.ApprovalComment
	json.NewDecoder(w.Body).Decode(&comments)
	if len(comments) != 2 {
		t.Errorf("expected 2 comments for appr-1, got %d", len(comments))
	}
}

func TestGetApprovalIssuesHandler(t *testing.T) {
	db := setupApprovalsTestDB(t)
	db.Exec("INSERT INTO issue_approvals (id, approval_id, issue_id) VALUES ('ia1', 'appr-1', 'issue-1')")
	db.Exec("INSERT INTO issue_approvals (id, approval_id, issue_id) VALUES ('ia2', 'appr-1', 'issue-2')")

	router := chi.NewRouter()
	router.Get("/approvals/{id}/issues", GetApprovalIssuesHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/approvals/appr-1/issues", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var issueApprovals []models.IssueApproval
	json.NewDecoder(w.Body).Decode(&issueApprovals)
	if len(issueApprovals) != 2 {
		t.Errorf("expected 2 issue-approval links, got %d", len(issueApprovals))
	}
}
