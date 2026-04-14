package routes

import (
"encoding/json"
"net/http"

"github.com/chifamba/vashandi/vashandi/backend/db/models"
"github.com/chifamba/vashandi/vashandi/backend/server/services"
"github.com/go-chi/chi/v5"
"gorm.io/gorm"
)

// ShouldWakeAssigneeOnCheckout returns true if the issue should trigger a heartbeat wakeup on checkout.
func ShouldWakeAssigneeOnCheckout(issue *models.Issue, actorAgentID string) bool {
if issue == nil {
return false
}
if issue.AssigneeAgentID == nil {
return false
}
if actorAgentID != "" && *issue.AssigneeAgentID == actorAgentID {
return false
}
return issue.Status == "in_progress"
}

// IssueCheckoutHandler handles atomic issue checkout and optional heartbeat wakeup.
func IssueCheckoutHandler(db *gorm.DB, issueSvc *services.IssueService, heartbeatSvc *services.HeartbeatService, activitySvc *services.ActivityService) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
id := chi.URLParam(r, "id")
companyID := chi.URLParam(r, "companyId")
var body struct {
RunID       string `json:"runId"`
ActorAgentID string `json:"actorAgentId"`
}
if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
http.Error(w, err.Error(), http.StatusBadRequest)
return
}
if err := issueSvc.Checkout(r.Context(), id, companyID, body.RunID); err != nil {
http.Error(w, err.Error(), http.StatusConflict)
return
}
var issue models.Issue
db.WithContext(r.Context()).First(&issue, "id = ?", id)
if heartbeatSvc != nil && ShouldWakeAssigneeOnCheckout(&issue, body.ActorAgentID) {
_, _ = heartbeatSvc.Wakeup(r.Context(), companyID, *issue.AssigneeAgentID, services.WakeupOptions{})
}
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(issue)
}
}
