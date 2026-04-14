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

func CreateFinanceEventHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
companyID := chi.URLParam(r, "companyId")
var event models.FinanceEvent
if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
http.Error(w, err.Error(), http.StatusBadRequest)
return
}
event.CompanyID = companyID
if err := db.WithContext(r.Context()).Create(&event).Error; err != nil {
http.Error(w, err.Error(), http.StatusInternalServerError)
return
}
w.Header().Set("Content-Type", "application/json")
w.WriteHeader(http.StatusCreated)
json.NewEncoder(w).Encode(event)
}
}

func GetCostsByProviderHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
companyID := chi.URLParam(r, "companyId")
var results []struct {
Provider   string `json:"provider"`
TotalCents int64  `json:"totalCents"`
}
db.WithContext(r.Context()).Model(&models.CostEvent{}).
Where("company_id = ?", companyID).
Select("provider, SUM(cost_cents) as total_cents").
Group("provider").
Scan(&results)
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(results)
}
}

func GetCostsByBillerHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
companyID := chi.URLParam(r, "companyId")
var results []struct {
Biller     string `json:"biller"`
TotalCents int64  `json:"totalCents"`
}
db.WithContext(r.Context()).Model(&models.CostEvent{}).
Where("company_id = ?", companyID).
Select("biller, SUM(cost_cents) as total_cents").
Group("biller").
Scan(&results)
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(results)
}
}

func GetBudgetOverviewHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
companyID := chi.URLParam(r, "companyId")
var policies []models.BudgetPolicy
db.WithContext(r.Context()).Where("company_id = ?", companyID).Find(&policies)
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(policies)
}
}

func UpdateAgentBudgetHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
agentID := chi.URLParam(r, "agentId")
var policy models.BudgetPolicy
if err := json.NewDecoder(r.Body).Decode(&policy); err != nil {
http.Error(w, err.Error(), http.StatusBadRequest)
return
}
policy.ScopeType = "agent"
policy.ScopeID = agentID
result := db.WithContext(r.Context()).
Where("scope_type = ? AND scope_id = ?", "agent", agentID).
FirstOrCreate(&policy)
if result.Error != nil {
http.Error(w, result.Error.Error(), http.StatusInternalServerError)
return
}
db.WithContext(r.Context()).Save(&policy)
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(policy)
}
}

func PatchCompanyBudgetsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")
		var policy models.BudgetPolicy
		if err := json.NewDecoder(r.Body).Decode(&policy); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		policy.ScopeType = "company"
		policy.ScopeID = companyID
		result := db.WithContext(r.Context()).
			Where("scope_type = ? AND scope_id = ?", "company", companyID).
			FirstOrCreate(&policy)
		if result.Error != nil {
			http.Error(w, result.Error.Error(), http.StatusInternalServerError)
			return
		}
		db.WithContext(r.Context()).Save(&policy)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(policy)
	}
}

func GetCostsByAgentModelHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")
		var results []struct {
			AgentID   string `json:"agentId"`
			Model     string `json:"model"`
			TotalCents int64 `json:"totalCents"`
		}
		db.WithContext(r.Context()).Model(&models.CostEvent{}).
			Where("company_id = ?", companyID).
			Select("agent_id, model, SUM(cost_cents) as total_cents").
			Group("agent_id, model").
			Scan(&results)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	}
}

func GetCostsByProjectHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")
		var results []struct {
			ProjectID  *string `json:"projectId"`
			TotalCents int64   `json:"totalCents"`
		}
		db.WithContext(r.Context()).Model(&models.CostEvent{}).
			Where("company_id = ?", companyID).
			Select("project_id, SUM(cost_cents) as total_cents").
			Group("project_id").
			Scan(&results)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	}
}

func GetFinanceSummaryHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")
		var totalDebit, totalCredit int64
		db.WithContext(r.Context()).Model(&models.FinanceEvent{}).
			Where("company_id = ? AND direction = ?", companyID, "debit").
			Select("COALESCE(SUM(amount_cents), 0)").Scan(&totalDebit)
		db.WithContext(r.Context()).Model(&models.FinanceEvent{}).
			Where("company_id = ? AND direction = ?", companyID, "credit").
			Select("COALESCE(SUM(amount_cents), 0)").Scan(&totalCredit)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"totalDebitCents":  totalDebit,
			"totalCreditCents": totalCredit,
			"netCents":         totalDebit - totalCredit,
		})
	}
}

func GetFinanceByBillerHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")
		var results []struct {
			Biller     string `json:"biller"`
			TotalCents int64  `json:"totalCents"`
		}
		db.WithContext(r.Context()).Model(&models.FinanceEvent{}).
			Where("company_id = ?", companyID).
			Select("biller, SUM(amount_cents) as total_cents").
			Group("biller").
			Scan(&results)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	}
}

func GetFinanceByKindHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")
		var results []struct {
			EventKind  string `json:"eventKind"`
			TotalCents int64  `json:"totalCents"`
		}
		db.WithContext(r.Context()).Model(&models.FinanceEvent{}).
			Where("company_id = ?", companyID).
			Select("event_kind, SUM(amount_cents) as total_cents").
			Group("event_kind").
			Scan(&results)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	}
}

func GetFinanceEventsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")
		var events []models.FinanceEvent
		db.WithContext(r.Context()).
			Where("company_id = ?", companyID).
			Order("occurred_at DESC").
			Limit(200).
			Find(&events)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(events)
	}
}

func GetWindowSpendHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")
		windowDays := 30
		var totalCents int64
		db.WithContext(r.Context()).Model(&models.CostEvent{}).
			Where("company_id = ? AND occurred_at >= NOW() - INTERVAL '? days'", companyID, windowDays).
			Select("COALESCE(SUM(cost_cents), 0)").Scan(&totalCents)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"windowDays": windowDays,
			"totalCents": totalCents,
		})
	}
}

func GetQuotaWindowsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")
		var policies []models.BudgetPolicy
		db.WithContext(r.Context()).
			Where("scope_type IN ('company','agent') AND (scope_id = ? OR scope_id IN (SELECT id::text FROM agents WHERE company_id = ?))", companyID, companyID).
			Find(&policies)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(policies)
	}
}
