package routes

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

const failedRunsLookbackHours = 24

func SidebarBadgesHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(computeBadgeCounts(r.Context(), db, companyID)) //nolint:errcheck
	}
}

// computeBadgeCountsWithLookback is the underlying query used by both the REST
// and SSE badge handlers. lookbackHours controls how far back to search for
// failed runs.
func computeBadgeCountsWithLookback(ctx context.Context, db *gorm.DB, companyID string, lookbackHours int) map[string]int64 {
	var pendingApprovals, pendingJoinRequests, failedRuns, openIssues int64

	db.WithContext(ctx).Model(&models.Approval{}).
		Where("company_id = ? AND status = ?", companyID, "pending").
		Count(&pendingApprovals)

	db.WithContext(ctx).Model(&models.JoinRequest{}).
		Where("company_id = ? AND status = ?", companyID, "pending_approval").
		Count(&pendingJoinRequests)

	cutoff := time.Now().Add(-time.Duration(lookbackHours) * time.Hour)
	db.WithContext(ctx).Model(&models.HeartbeatRun{}).
		Where("company_id = ? AND status = ? AND created_at > ?", companyID, "failed", cutoff).
		Count(&failedRuns)

	db.WithContext(ctx).Model(&models.Issue{}).
		Where("company_id = ? AND status NOT IN ?", companyID, []string{"done", "cancelled"}).
		Count(&openIssues)

	return map[string]int64{
		"pendingApprovals":    pendingApprovals,
		"pendingJoinRequests": pendingJoinRequests,
		"failedRuns":          failedRuns,
		"openIssues":          openIssues,
	}
}

