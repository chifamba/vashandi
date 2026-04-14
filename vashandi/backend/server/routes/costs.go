package routes

import (
"encoding/json"
"net/http"
"time"

"github.com/chifamba/vashandi/vashandi/backend/db/models"
"github.com/chifamba/vashandi/vashandi/backend/server/services"
"github.com/go-chi/chi/v5"
"gorm.io/gorm"
)

func CreateCostEventHandler(db *gorm.DB, costSvc *services.CostService) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
companyID := chi.URLParam(r, "companyId")
var event models.CostEvent
if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
http.Error(w, err.Error(), http.StatusBadRequest)
return
}
created, err := costSvc.CreateEvent(r.Context(), companyID, &event)
if err != nil {
http.Error(w, err.Error(), http.StatusInternalServerError)
return
}
w.Header().Set("Content-Type", "application/json")
w.WriteHeader(http.StatusCreated)
json.NewEncoder(w).Encode(created)
}
}

func GetCostSummaryHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
companyID := chi.URLParam(r, "companyId")
from := r.URL.Query().Get("from")
to := r.URL.Query().Get("to")

q := db.WithContext(r.Context()).Model(&models.CostEvent{}).Where("company_id = ?", companyID)
if from != "" {
if t, err := time.Parse(time.RFC3339, from); err == nil {
q = q.Where("occurred_at >= ?", t)
}
}
if to != "" {
if t, err := time.Parse(time.RFC3339, to); err == nil {
q = q.Where("occurred_at <= ?", t)
}
}

var total int64
q.Select("COALESCE(SUM(cost_cents), 0)").Scan(&total)
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(map[string]interface{}{"totalCostCents": total})
}
}

func GetCostsByAgentHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
companyID := chi.URLParam(r, "companyId")
var results []struct {
AgentID   string `json:"agentId"`
CostCents int64  `json:"costCents"`
}
db.WithContext(r.Context()).Model(&models.CostEvent{}).
Where("company_id = ?", companyID).
Select("agent_id, SUM(cost_cents) as cost_cents").
Group("agent_id").
Scan(&results)
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(results)
}
}

func UpdateBudgetPolicyHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
companyID := chi.URLParam(r, "companyId")
var policy models.BudgetPolicy
if err := json.NewDecoder(r.Body).Decode(&policy); err != nil {
http.Error(w, err.Error(), http.StatusBadRequest)
return
}
policy.CompanyID = companyID
result := db.WithContext(r.Context()).Where("company_id = ?", companyID).FirstOrCreate(&policy)
if result.Error != nil {
http.Error(w, result.Error.Error(), http.StatusInternalServerError)
return
}
db.WithContext(r.Context()).Save(&policy)
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(policy)
}
}
