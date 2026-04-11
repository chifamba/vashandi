package routes

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// DashboardSummary represents the structure of the dashboard summary response
type DashboardSummary struct {
	TotalAgents      int64 `json:"totalAgents"`
	ActiveAgents     int64 `json:"activeAgents"`
	TotalIssues      int64 `json:"totalIssues"`
	OpenIssues       int64 `json:"openIssues"`
	PendingApprovals int64 `json:"pendingApprovals"`
}

// DashboardHandler returns an http.HandlerFunc that serves the dashboard summary for a company
func DashboardHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		companyID := chi.URLParam(r, "companyId")

		// Authorization stub: In a full implementation, we'd verify the user/agent has access to companyID
		if companyID == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "companyId is required"})
			return
		}

		var summary DashboardSummary

		db.Table("agents").Where("company_id = ?", companyID).Count(&summary.TotalAgents)
		db.Table("agents").Where("company_id = ? AND status = ?", companyID, "active").Count(&summary.ActiveAgents)
		db.Table("issues").Where("company_id = ?", companyID).Count(&summary.TotalIssues)
		db.Table("issues").Where("company_id = ? AND status NOT IN ?", companyID, []string{"done", "cancelled"}).Count(&summary.OpenIssues)
		db.Table("approvals").Where("company_id = ? AND status = ?", companyID, "pending").Count(&summary.PendingApprovals)

		json.NewEncoder(w).Encode(summary)
	}
}
