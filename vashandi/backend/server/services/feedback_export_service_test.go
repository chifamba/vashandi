package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupFeedbackExportTestDB creates an in-memory SQLite DB with the
// feedback_exports table pre-seeded for testing.
func setupFeedbackExportTestDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:feedback_export_%s?mode=memory&cache=shared", name)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.Exec("DROP TABLE IF EXISTS feedback_exports")
	db.Exec(`CREATE TABLE feedback_exports (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		feedback_vote_id text NOT NULL DEFAULT '',
		issue_id text NOT NULL DEFAULT '',
		author_user_id text NOT NULL DEFAULT '',
		target_type text NOT NULL DEFAULT '',
		target_id text NOT NULL DEFAULT '',
		vote text NOT NULL DEFAULT 'up',
		status text NOT NULL DEFAULT 'local_only',
		export_id text,
		attempt_count integer NOT NULL DEFAULT 0,
		last_attempted_at datetime,
		exported_at datetime,
		failure_reason text,
		payload_snapshot text,
		target_summary text NOT NULL DEFAULT '{}',
		created_at datetime DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)
	return db
}

func insertFeedbackExport(db *gorm.DB, id, companyID, status string) {
	db.Exec(
		`INSERT INTO feedback_exports (id, company_id, status) VALUES (?, ?, ?)`,
		id, companyID, status,
	)
}

// ---------- FlushPendingFeedbackTraces — no share client ----------

func TestFeedbackExportService_NoClient_MarksPendingAsFailed(t *testing.T) {
	db := setupFeedbackExportTestDB(t, "no_client_pending")
	insertFeedbackExport(db, "trace-1", "comp-a", "pending")
	insertFeedbackExport(db, "trace-2", "comp-a", "pending")

	svc := NewFeedbackExportService(db, nil)
	result, err := svc.FlushPendingFeedbackTraces(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Attempted != 2 {
		t.Errorf("expected attempted=2, got %d", result.Attempted)
	}
	if result.Sent != 0 {
		t.Errorf("expected sent=0, got %d", result.Sent)
	}
	if result.Failed != 2 {
		t.Errorf("expected failed=2, got %d", result.Failed)
	}

	// Verify DB update.
	var count int64
	db.Table("feedback_exports").Where("status = ? AND failure_reason = ?", "failed", feedbackExportBackendNotConfigured).Count(&count)
	if count != 2 {
		t.Errorf("expected 2 rows with status=failed and correct reason, got %d", count)
	}
}

func TestFeedbackExportService_NoClient_IgnoresNonPending(t *testing.T) {
	db := setupFeedbackExportTestDB(t, "no_client_non_pending")
	insertFeedbackExport(db, "trace-1", "comp-a", "local_only")
	insertFeedbackExport(db, "trace-2", "comp-a", "failed")
	insertFeedbackExport(db, "trace-3", "comp-a", "sent")

	svc := NewFeedbackExportService(db, nil)
	result, err := svc.FlushPendingFeedbackTraces(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Attempted != 0 {
		t.Errorf("expected attempted=0, got %d", result.Attempted)
	}
}

// ---------- FlushPendingFeedbackTraces — with share client ----------

func TestFeedbackExportService_WithClient_SendsPendingAndFailed(t *testing.T) {
	// Start a mock HTTP server that accepts uploads.
	var received int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"objectKey": "feedback-traces/comp-a/key.json"})
	}))
	defer srv.Close()

	db := setupFeedbackExportTestDB(t, "with_client_sends")
	insertFeedbackExport(db, "trace-1", "comp-a", "pending")
	insertFeedbackExport(db, "trace-2", "comp-a", "failed")
	insertFeedbackExport(db, "trace-3", "comp-a", "local_only") // should be skipped

	client := NewFeedbackTraceShareClient(FeedbackShareClientConfig{BackendURL: srv.URL, Token: "tok"})
	svc := NewFeedbackExportService(db, client)
	result, err := svc.FlushPendingFeedbackTraces(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Attempted != 2 {
		t.Errorf("expected attempted=2, got %d", result.Attempted)
	}
	if result.Sent != 2 {
		t.Errorf("expected sent=2, got %d", result.Sent)
	}
	if result.Failed != 0 {
		t.Errorf("expected failed=0, got %d", result.Failed)
	}
	if received != 2 {
		t.Errorf("expected 2 HTTP requests to mock server, got %d", received)
	}

	// Verify DB status updated to "sent".
	var sentCount int64
	db.Table("feedback_exports").Where("status = ?", "sent").Count(&sentCount)
	if sentCount != 2 {
		t.Errorf("expected 2 rows with status=sent, got %d", sentCount)
	}
}

func TestFeedbackExportService_WithClient_MarksFailedOnUploadError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upload rejected", http.StatusBadRequest)
	}))
	defer srv.Close()

	db := setupFeedbackExportTestDB(t, "with_client_error")
	insertFeedbackExport(db, "trace-1", "comp-a", "pending")

	client := NewFeedbackTraceShareClient(FeedbackShareClientConfig{BackendURL: srv.URL, Token: "tok"})
	svc := NewFeedbackExportService(db, client)
	result, err := svc.FlushPendingFeedbackTraces(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Attempted != 1 {
		t.Errorf("expected attempted=1, got %d", result.Attempted)
	}
	if result.Sent != 0 {
		t.Errorf("expected sent=0, got %d", result.Sent)
	}
	if result.Failed != 1 {
		t.Errorf("expected failed=1, got %d", result.Failed)
	}

	var row struct {
		Status        string  `gorm:"column:status"`
		FailureReason *string `gorm:"column:failure_reason"`
		AttemptCount  int     `gorm:"column:attempt_count"`
	}
	db.Table("feedback_exports").Select("status, failure_reason, attempt_count").Where("id = ?", "trace-1").Scan(&row)
	if row.Status != "failed" {
		t.Errorf("expected status=failed, got %s", row.Status)
	}
	if row.FailureReason == nil || *row.FailureReason == "" {
		t.Errorf("expected non-empty failure_reason")
	}
	if row.AttemptCount != 1 {
		t.Errorf("expected attempt_count=1, got %d", row.AttemptCount)
	}
}

func TestFeedbackExportService_WithClient_CompanyIDFilter(t *testing.T) {
	var received int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"objectKey": "k"})
	}))
	defer srv.Close()

	db := setupFeedbackExportTestDB(t, "with_client_company_filter")
	insertFeedbackExport(db, "trace-1", "comp-a", "pending")
	insertFeedbackExport(db, "trace-2", "comp-b", "pending")

	client := NewFeedbackTraceShareClient(FeedbackShareClientConfig{BackendURL: srv.URL})
	svc := NewFeedbackExportService(db, client)

	companyID := "comp-a"
	result, err := svc.FlushPendingFeedbackTraces(context.Background(), &FeedbackExportFlushOptions{
		CompanyID: &companyID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Attempted != 1 {
		t.Errorf("expected attempted=1, got %d", result.Attempted)
	}
	if received != 1 {
		t.Errorf("expected 1 HTTP request, got %d", received)
	}
}

func TestFeedbackExportService_WithClient_TraceIDFilter(t *testing.T) {
	var received int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"objectKey": "k"})
	}))
	defer srv.Close()

	db := setupFeedbackExportTestDB(t, "with_client_trace_filter")
	insertFeedbackExport(db, "trace-1", "comp-a", "pending")
	insertFeedbackExport(db, "trace-2", "comp-a", "pending")

	client := NewFeedbackTraceShareClient(FeedbackShareClientConfig{BackendURL: srv.URL})
	svc := NewFeedbackExportService(db, client)

	traceID := "trace-1"
	result, err := svc.FlushPendingFeedbackTraces(context.Background(), &FeedbackExportFlushOptions{
		TraceID: &traceID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Attempted != 1 {
		t.Errorf("expected attempted=1, got %d", result.Attempted)
	}
	if received != 1 {
		t.Errorf("expected 1 HTTP request, got %d", received)
	}
}

func TestFeedbackExportService_LimitCappedAt200(t *testing.T) {
	db := setupFeedbackExportTestDB(t, "limit_capped")
	svc := NewFeedbackExportService(db, nil)

	now := time.Now()
	result, err := svc.FlushPendingFeedbackTraces(context.Background(), &FeedbackExportFlushOptions{
		Limit: 9999,
		Now:   &now,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// No rows → 0 attempted, just confirming no error from a capped limit.
	if result.Attempted != 0 {
		t.Errorf("expected 0 attempted (empty DB), got %d", result.Attempted)
	}
}

// ---------- truncateFeedbackFailureReason ----------

func TestTruncateFeedbackFailureReason_Long(t *testing.T) {
	long := fmt.Errorf("%s", string(make([]byte, 2000)))
	result := truncateFeedbackFailureReason(long)
	if len(result) > 1000 {
		t.Errorf("expected result to be <= 1000 chars, got %d", len(result))
	}
}

func TestTruncateFeedbackFailureReason_Empty(t *testing.T) {
	result := truncateFeedbackFailureReason(fmt.Errorf("   "))
	if result != "Feedback export failed" {
		t.Errorf("expected fallback message, got %q", result)
	}
}
