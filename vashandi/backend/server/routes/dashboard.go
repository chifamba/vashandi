package routes

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// DashboardSummary represents the structure of the dashboard summary response
type DashboardSummary struct {
	TotalAgents          int64   `json:"totalAgents"`
	ActiveAgents         int64   `json:"activeAgents"`
	RunningAgents        int64   `json:"runningAgents"`
	PausedAgents         int64   `json:"pausedAgents"`
	ErrorAgents          int64   `json:"errorAgents"`
	TotalIssues          int64   `json:"totalIssues"`
	OpenIssues           int64   `json:"openIssues"`
	InProgressIssues     int64   `json:"inProgressIssues"`
	BlockedIssues        int64   `json:"blockedIssues"`
	DoneIssues           int64   `json:"doneIssues"`
	PendingApprovals     int64   `json:"pendingApprovals"`
	MTDSpend             float64 `json:"mtdSpend"`
	BudgetUtilization    float64 `json:"budgetUtilization"`
	MemoryOperationCount int64   `json:"memoryOperationCount"`
	MemoryHitRate        float64 `json:"memoryHitRate"`
	MCPInvocationCount   int64   `json:"mcpInvocationCount"`
}

// DashboardHandler returns an http.HandlerFunc that serves the dashboard summary for a company
func DashboardHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		companyID := chi.URLParam(r, "companyId")

		if companyID == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "companyId is required"})
			return
		}

		if err := AssertCompanyAccess(r, companyID); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}

		var summary DashboardSummary

		db.Table("agents").Where("company_id = ?", companyID).Count(&summary.TotalAgents)
		db.Table("agents").Where("company_id = ? AND status = ?", companyID, "active").Count(&summary.ActiveAgents)
		db.Table("agents").Where("company_id = ? AND status = ?", companyID, "running").Count(&summary.RunningAgents)
		db.Table("agents").Where("company_id = ? AND status = ?", companyID, "paused").Count(&summary.PausedAgents)
		db.Table("agents").Where("company_id = ? AND status = ?", companyID, "error").Count(&summary.ErrorAgents)
		db.Table("issues").Where("company_id = ?", companyID).Count(&summary.TotalIssues)
		db.Table("issues").Where("company_id = ? AND status = ?", companyID, "open").Count(&summary.OpenIssues)
		db.Table("issues").Where("company_id = ? AND status = ?", companyID, "in_progress").Count(&summary.InProgressIssues)
		db.Table("issues").Where("company_id = ? AND status = ?", companyID, "blocked").Count(&summary.BlockedIssues)
		db.Table("issues").Where("company_id = ? AND status = ?", companyID, "done").Count(&summary.DoneIssues)
		db.Table("approvals").Where("company_id = ? AND status = ?", companyID, "pending").Count(&summary.PendingApprovals)

		// V3.1 Platform Observability Dashboarding metric mockups:
		// MTDSpend and BudgetUtilization would typically require complex queries across finance/cost events tables
		var spend float64
		db.Table("cost_events").Where("company_id = ?", companyID).Select("sum(amount)").Scan(&spend)
		summary.MTDSpend = spend

		var limit float64
		db.Table("budget_policies").Where("company_id = ?", companyID).Select("sum(limit_amount)").Scan(&limit)
		if limit > 0 {
			summary.BudgetUtilization = spend / limit
		}

		db.Table("memory_operations").Where("company_id = ?", companyID).Count(&summary.MemoryOperationCount)

		var memHitCount int64
		db.Table("memory_operations").Where("company_id = ? AND success = ?", companyID, true).Count(&memHitCount)
		if summary.MemoryOperationCount > 0 {
			summary.MemoryHitRate = float64(memHitCount) / float64(summary.MemoryOperationCount)
		}

		db.Table("activity_log").Where("company_id = ? AND action = ?", companyID, "mcp_tool_invoked").Count(&summary.MCPInvocationCount)

		json.NewEncoder(w).Encode(summary)
	}
}

type PlatformMetrics struct {
	TotalAgents   int64   `json:"totalAgents"`
	ActiveRuns    int64   `json:"activeRuns"`
	TotalSpendMTD float64 `json:"totalSpendMTD"`
	ErrorRate     float64 `json:"errorRate"`
}

// PlatformMetricsHandler serves cross-company aggregated metrics
func PlatformMetricsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if err := AssertBoard(r); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}

		var metrics PlatformMetrics

		db.Table("agents").Count(&metrics.TotalAgents)
		db.Table("heartbeat_runs").Where("status IN ?", []string{"active", "running"}).Count(&metrics.ActiveRuns)

		var spend float64
		db.Table("cost_events").Select("sum(amount)").Scan(&spend)
		metrics.TotalSpendMTD = spend

		var totalRuns int64
		db.Table("heartbeat_runs").Count(&totalRuns)
		var errorRuns int64
		db.Table("heartbeat_runs").Where("status = ?", "error").Count(&errorRuns)
		if totalRuns > 0 {
			metrics.ErrorRate = float64(errorRuns) / float64(totalRuns)
		}

		json.NewEncoder(w).Encode(metrics)
	}
}
