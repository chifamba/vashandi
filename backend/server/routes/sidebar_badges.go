package routes

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// ListSidebarBadgesHandler lists sidebar badges for a company
func ListSidebarBadgesHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		companyID := chi.URLParam(r, "companyId")

		// The TypeScript version heavily aggregates from issues and join requests.
		// For the Go port without the full aggregation service, we return a basic struct
		// to maintain API parity structurally.
		type SidebarBadges struct {
			JoinRequests int `json:"joinRequests"`
			Issues       int `json:"issues"`
		}

		var badges SidebarBadges

		// Count pending join requests
		var joinRequestCount int64
		db.Table("join_requests").Where("company_id = ? AND status = 'pending'", companyID).Count(&joinRequestCount)
		badges.JoinRequests = int(joinRequestCount)

		// Count actionable issues
		var issuesCount int64
		db.Table("issues").Where("company_id = ? AND status NOT IN ('done', 'archived')", companyID).Count(&issuesCount)
		badges.Issues = int(issuesCount)

		json.NewEncoder(w).Encode(badges)
	}
}
