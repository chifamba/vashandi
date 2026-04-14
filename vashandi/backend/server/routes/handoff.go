package routes

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
	"github.com/chifamba/vashandi/vashandi/backend/db/models"
)

type HandoffRequest struct {
	TargetAgentID   string `json:"targetAgentId"`
	HandoffMarkdown string `json:"handoffMarkdown"`
}

// HandoffIssueHandler transfers control of an issue to another agent.
func HandoffIssueHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		issueID := chi.URLParam(r, "id")

		var req HandoffRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// 1. Fetch Issue
		var issue models.Issue
		if err := db.Where("id = ?", issueID).First(&issue).Error; err != nil {
			http.Error(w, "Issue not found", http.StatusNotFound)
			return
		}

		// 2. Fetch Latest Run to attach handoff markdown
		var latestRun models.HeartbeatRun
		if err := db.Where("agent_id = ? AND task_id = ?", issue.AssigneeAgentID, issueID).
			Order("created_at desc").First(&latestRun).Error; err == nil {
			latestRun.HandoffMarkdown = &req.HandoffMarkdown
			db.Save(&latestRun)
		}

		// 3. Update Issue Assignee
		issue.AssigneeAgentID = &req.TargetAgentID
		if err := db.Save(&issue).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// 4. Create New Run for Target Agent
		newRun := models.HeartbeatRun{
			CompanyID:        issue.CompanyID,
			AgentID:          req.TargetAgentID,
			TaskID:           issue.ID,
			InvocationSource: "handoff",
			Status:           "queued",
			ContextSnapshot:  latestRun.ContextSnapshot, // Pass context over
		}
		if err := db.Create(&newRun).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status":   "handoff_completed",
			"newRunId": newRun.ID,
		})
	}
}
