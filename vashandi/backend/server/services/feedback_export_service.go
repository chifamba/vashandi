package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	feedbackExportBackendNotConfigured = "Feedback export backend is not configured"
	feedbackExportDefaultLimit         = 25
	feedbackExportMaxLimit             = 200
)

// FeedbackExportFlushOptions parameterises a FlushPendingFeedbackTraces call.
type FeedbackExportFlushOptions struct {
	CompanyID *string
	TraceID   *string
	Limit     int
	Now       *time.Time
}

// FeedbackExportFlushResult reports the outcome of a flush cycle.
type FeedbackExportFlushResult struct {
	Attempted int
	Sent      int
	Failed    int
}

// FeedbackExportService flushes pending feedback export records to the
// remote telemetry endpoint using the supplied share client.
type FeedbackExportService struct {
	db          *gorm.DB
	shareClient *FeedbackTraceShareClient
}

// NewFeedbackExportService creates a FeedbackExportService.
// If shareClient is nil, pending records are marked as failed with a
// "backend not configured" reason rather than being uploaded.
func NewFeedbackExportService(db *gorm.DB, shareClient *FeedbackTraceShareClient) *FeedbackExportService {
	return &FeedbackExportService{db: db, shareClient: shareClient}
}

type feedbackExportRow struct {
	ID              string  `gorm:"column:id"`
	CompanyID       string  `gorm:"column:company_id"`
	ExportID        *string `gorm:"column:export_id"`
	AttemptCount    int     `gorm:"column:attempt_count"`
	PayloadSnapshot []byte  `gorm:"column:payload_snapshot"`
}

// FlushPendingFeedbackTraces processes feedback export records whose status
// is "pending" (or "failed" when a share client is configured) and either
// uploads them to the telemetry backend or marks them as failed.
func (s *FeedbackExportService) FlushPendingFeedbackTraces(ctx context.Context, opts *FeedbackExportFlushOptions) (FeedbackExportFlushResult, error) {
	limit := feedbackExportDefaultLimit
	if opts != nil && opts.Limit > 0 {
		if opts.Limit > feedbackExportMaxLimit {
			limit = feedbackExportMaxLimit
		} else {
			limit = opts.Limit
		}
	}

	now := time.Now()
	if opts != nil && opts.Now != nil {
		now = *opts.Now
	}

	if s.shareClient == nil {
		return s.flushWithoutClient(ctx, opts, limit, now)
	}
	return s.flushWithClient(ctx, opts, limit, now)
}

// flushWithoutClient handles the case when no share client is configured.
// It marks pending records as failed immediately.
func (s *FeedbackExportService) flushWithoutClient(ctx context.Context, opts *FeedbackExportFlushOptions, limit int, now time.Time) (FeedbackExportFlushResult, error) {
	var rows []feedbackExportRow
	q := s.db.WithContext(ctx).Table("feedback_exports").
		Select("id, attempt_count").
		Where("status = ?", "pending")
	q = applyFeedbackExportFilters(q, opts)
	q = q.Order("created_at ASC, id ASC").Limit(limit)
	if err := q.Scan(&rows).Error; err != nil {
		return FeedbackExportFlushResult{}, fmt.Errorf("query pending feedback exports: %w", err)
	}

	reason := feedbackExportBackendNotConfigured
	for _, row := range rows {
		s.db.WithContext(ctx).Table("feedback_exports").
			Where("id = ?", row.ID).
			Updates(map[string]interface{}{
				"status":            "failed",
				"attempt_count":     row.AttemptCount + 1,
				"last_attempted_at": now,
				"failure_reason":    reason,
				"updated_at":        now,
			})
	}

	return FeedbackExportFlushResult{
		Attempted: len(rows),
		Sent:      0,
		Failed:    len(rows),
	}, nil
}

// flushWithClient uploads pending and failed records to the telemetry endpoint.
func (s *FeedbackExportService) flushWithClient(ctx context.Context, opts *FeedbackExportFlushOptions, limit int, now time.Time) (FeedbackExportFlushResult, error) {
	var rows []feedbackExportRow
	q := s.db.WithContext(ctx).Table("feedback_exports").
		Select("id, company_id, export_id, attempt_count, payload_snapshot").
		Where("status IN ?", []string{"pending", "failed"})
	q = applyFeedbackExportFilters(q, opts)
	q = q.Order("created_at ASC, id ASC").Limit(limit)
	if err := q.Scan(&rows).Error; err != nil {
		return FeedbackExportFlushResult{}, fmt.Errorf("query pending feedback exports: %w", err)
	}

	var attempted, sent, failed int
	for _, row := range rows {
		attempted++

		var payloadData interface{}
		if len(row.PayloadSnapshot) > 0 {
			_ = json.Unmarshal(row.PayloadSnapshot, &payloadData)
		}

		bundle := &FeedbackTraceBundle{
			CompanyID: row.CompanyID,
			TraceID:   row.ID,
			ExportID:  row.ExportID,
			Data:      payloadData,
		}

		_, err := s.shareClient.UploadTraceBundle(bundle)
		if err != nil {
			reason := truncateFeedbackFailureReason(err)
			s.db.WithContext(ctx).Table("feedback_exports").
				Where("id = ?", row.ID).
				Updates(map[string]interface{}{
					"status":            "failed",
					"attempt_count":     row.AttemptCount + 1,
					"last_attempted_at": now,
					"failure_reason":    reason,
					"updated_at":        now,
				})
			failed++
			continue
		}

		s.db.WithContext(ctx).Table("feedback_exports").
			Where("id = ?", row.ID).
			Updates(map[string]interface{}{
				"status":            "sent",
				"attempt_count":     row.AttemptCount + 1,
				"last_attempted_at": now,
				"exported_at":       now,
				"failure_reason":    nil,
				"updated_at":        now,
			})
		sent++
	}

	return FeedbackExportFlushResult{
		Attempted: attempted,
		Sent:      sent,
		Failed:    failed,
	}, nil
}

// applyFeedbackExportFilters applies optional CompanyID and TraceID filters.
func applyFeedbackExportFilters(q *gorm.DB, opts *FeedbackExportFlushOptions) *gorm.DB {
	if opts == nil {
		return q
	}
	if opts.CompanyID != nil {
		q = q.Where("company_id = ?", *opts.CompanyID)
	}
	if opts.TraceID != nil {
		q = q.Where("id = ?", *opts.TraceID)
	}
	return q
}

// truncateFeedbackFailureReason converts an error to a short reason string,
// matching the behaviour of the Node.js truncateFailureReason helper.
func truncateFeedbackFailureReason(err error) string {
	msg := strings.TrimSpace(err.Error())
	if len(msg) > 1_000 {
		msg = msg[:1_000]
	}
	if msg == "" {
		msg = "Feedback export failed"
	}
	return msg
}

// StartFeedbackExportFlusher launches a background goroutine that calls
// FlushPendingFeedbackTraces every intervalMs milliseconds, mirroring the
// Node.js 5-second FEEDBACK_EXPORT_FLUSH_INTERVAL_MS timer.
// The goroutine stops when ctx is cancelled.
func StartFeedbackExportFlusher(ctx context.Context, svc *FeedbackExportService, intervalMs int) {
	if intervalMs <= 0 {
		intervalMs = 5_000
	}
	ticker := time.NewTicker(time.Duration(intervalMs) * time.Millisecond)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				result, err := svc.FlushPendingFeedbackTraces(ctx, nil)
				if err != nil {
					slog.Error("feedback export flush failed", "error", err)
				} else if result.Attempted > 0 {
					slog.Info("feedback export flush", "attempted", result.Attempted, "sent", result.Sent, "failed", result.Failed)
				}
			}
		}
	}()
}
