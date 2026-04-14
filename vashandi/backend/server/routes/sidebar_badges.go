package routes

import (
"encoding/json"
"net/http"
"time"

"github.com/chifamba/vashandi/vashandi/backend/db/models"
"github.com/go-chi/chi/v5"
"gorm.io/gorm"
)

func SidebarBadgesHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
companyID := chi.URLParam(r, "companyId")

var pendingApprovals, pendingJoinRequests, failedRuns int64

db.WithContext(r.Context()).Model(&models.Approval{}).
Where("company_id = ? AND status = ?", companyID, "pending").
Count(&pendingApprovals)

db.WithContext(r.Context()).Model(&models.JoinRequest{}).
Where("company_id = ? AND status = ?", companyID, "pending_approval").
Count(&pendingJoinRequests)

cutoff := time.Now().Add(-24 * time.Hour)
db.WithContext(r.Context()).Model(&models.HeartbeatRun{}).
Where("company_id = ? AND status = ? AND created_at > ?", companyID, "failed", cutoff).
Count(&failedRuns)

w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(map[string]int64{
"pendingApprovals":    pendingApprovals,
"pendingJoinRequests": pendingJoinRequests,
"failedRuns":          failedRuns,
})
}
}
