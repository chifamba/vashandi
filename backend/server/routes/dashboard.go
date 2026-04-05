package routes

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/chifamba/paperclip/backend/db/models"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

type DashboardSummary struct {
	CompanyID        string                 `json:"companyId"`
	Agents           AgentCounts            `json:"agents"`
	Tasks            TaskCounts             `json:"tasks"`
	Costs            DashboardCostSummary   `json:"costs"`
	PendingApprovals int64                  `json:"pendingApprovals"`
	Budgets          DashboardBudgetOverview `json:"budgets"`
}

type AgentCounts struct {
	Active  int64 `json:"active"`
	Running int64 `json:"running"`
	Paused  int64 `json:"paused"`
	Error   int64 `json:"error"`
}

type TaskCounts struct {
	Open       int64 `json:"open"`
	InProgress int64 `json:"inProgress"`
	Blocked    int64 `json:"blocked"`
	Done       int64 `json:"done"`
}

type DashboardCostSummary struct {
	MonthSpendCents       int64   `json:"monthSpendCents"`
	MonthBudgetCents      int     `json:"monthBudgetCents"`
	MonthUtilizationPercent float64 `json:"monthUtilizationPercent"`
}

type DashboardBudgetOverview struct {
	ActiveIncidents  int `json:"activeIncidents"`
	PendingApprovals int `json:"pendingApprovals"`
	PausedAgents     int `json:"pausedAgents"`
	PausedProjects   int `json:"pausedProjects"`
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

		var company models.Company
		if err := db.Where("id = ?", companyID).First(&company).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(map[string]string{"error": "Company not found"})
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Internal server error"})
			return
		}

		type StatusCount struct {
			Status string
			Count  int64
		}

		var agentRows []StatusCount
		db.Table("agents").Select("status, count(*) as count").Where("company_id = ?", companyID).Group("status").Scan(&agentRows)

		var taskRows []StatusCount
		db.Table("issues").Select("status, count(*) as count").Where("company_id = ?", companyID).Group("status").Scan(&taskRows)

		var pendingApprovals int64
		db.Table("approvals").Where("company_id = ? AND status = ?", companyID, "pending").Count(&pendingApprovals)

		agentCounts := AgentCounts{}
		for _, row := range agentRows {
			switch row.Status {
			case "idle", "active":
				agentCounts.Active += row.Count
			case "running":
				agentCounts.Running += row.Count
			case "paused":
				agentCounts.Paused += row.Count
			case "error":
				agentCounts.Error += row.Count
			}
		}

		taskCounts := TaskCounts{}
		for _, row := range taskRows {
			switch row.Status {
			case "in_progress":
				taskCounts.InProgress += row.Count
				taskCounts.Open += row.Count
			case "blocked":
				taskCounts.Blocked += row.Count
				taskCounts.Open += row.Count
			case "done":
				taskCounts.Done += row.Count
			case "todo", "open":
				taskCounts.Open += row.Count
			}
		}

		now := time.Now()
		monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

		var monthSpend int64
		db.Table("cost_events").Where("company_id = ? AND occurred_at >= ?", companyID, monthStart).Select("COALESCE(SUM(cost_cents), 0)").Scan(&monthSpend)

		var utilization float64
		if company.BudgetMonthlyCents > 0 {
			utilization = (float64(monthSpend) / float64(company.BudgetMonthlyCents)) * 100
		}

		// Calculate full budget overview
		var activeIncidentsCount int64
		db.Table("budget_incidents").Where("company_id = ? AND resolved_at IS NULL", companyID).Count(&activeIncidentsCount)

		var pausedAgentsCount int64
		db.Table("agents").Where("company_id = ? AND status = ?", companyID, "paused").Count(&pausedAgentsCount)

		var pausedProjectsCount int64
		db.Table("projects").Where("company_id = ? AND status = ?", companyID, "paused").Count(&pausedProjectsCount)

		summary := DashboardSummary{
			CompanyID: companyID,
			Agents:    agentCounts,
			Tasks:     taskCounts,
			Costs: DashboardCostSummary{
				MonthSpendCents:       monthSpend,
				MonthBudgetCents:      company.BudgetMonthlyCents,
				MonthUtilizationPercent: utilization,
			},
			PendingApprovals: pendingApprovals,
			Budgets: DashboardBudgetOverview{
				ActiveIncidents:  int(activeIncidentsCount),
				PendingApprovals: int(pendingApprovals),
				PausedAgents:     int(pausedAgentsCount),
				PausedProjects:   int(pausedProjectsCount),
			},
		}

		json.NewEncoder(w).Encode(summary)
	}
}
