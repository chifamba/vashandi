package routes

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
)

func setupCostsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&costs_test=1"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	for _, tbl := range []string{"finance_events", "budget_policies", "cost_events"} {
		db.Exec("DROP TABLE IF EXISTS " + tbl)
	}
	db.Exec(`CREATE TABLE cost_events (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		agent_id text NOT NULL,
		issue_id text,
		project_id text,
		goal_id text,
		heartbeat_run_id text,
		billing_code text,
		provider text NOT NULL,
		biller text NOT NULL DEFAULT 'unknown',
		billing_type text NOT NULL DEFAULT 'unknown',
		model text NOT NULL,
		input_tokens integer NOT NULL DEFAULT 0,
		cached_input_tokens integer NOT NULL DEFAULT 0,
		output_tokens integer NOT NULL DEFAULT 0,
		cost_cents integer NOT NULL DEFAULT 0,
		occurred_at datetime,
		created_at datetime
	)`)
	db.Exec(`CREATE TABLE budget_policies (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		scope_type text NOT NULL,
		scope_id text NOT NULL,
		metric text NOT NULL DEFAULT 'billed_cents',
		window_kind text NOT NULL DEFAULT 'monthly',
		amount integer NOT NULL DEFAULT 0,
		warn_percent integer NOT NULL DEFAULT 80,
		hard_stop_enabled boolean NOT NULL DEFAULT 1,
		notify_enabled boolean NOT NULL DEFAULT 1,
		is_active boolean NOT NULL DEFAULT 1,
		created_by_user_id text,
		updated_by_user_id text,
		created_at datetime,
		updated_at datetime
	)`)
	db.Exec(`CREATE TABLE finance_events (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		agent_id text,
		issue_id text,
		project_id text,
		goal_id text,
		heartbeat_run_id text,
		cost_event_id text,
		billing_code text,
		description text,
		event_kind text NOT NULL,
		direction text NOT NULL DEFAULT 'debit',
		biller text NOT NULL DEFAULT 'unknown',
		provider text,
		execution_adapter_type text,
		pricing_tier text,
		region text,
		model text,
		quantity integer,
		unit text,
		amount_cents integer NOT NULL DEFAULT 0,
		currency text NOT NULL DEFAULT 'USD',
		estimated boolean NOT NULL DEFAULT 0,
		external_invoice_id text,
		metadata_json text,
		occurred_at datetime,
		created_at datetime
	)`)
	return db
}

func TestGetCostSummaryHandler(t *testing.T) {
	db := setupCostsTestDB(t)
	db.Exec("INSERT INTO cost_events (id, company_id, agent_id, provider, model, cost_cents, occurred_at) VALUES ('ce1', 'comp-a', 'a1', 'openai', 'gpt-4', 100, '2026-01-15T00:00:00Z')")
	db.Exec("INSERT INTO cost_events (id, company_id, agent_id, provider, model, cost_cents, occurred_at) VALUES ('ce2', 'comp-a', 'a1', 'openai', 'gpt-4', 200, '2026-01-16T00:00:00Z')")
	db.Exec("INSERT INTO cost_events (id, company_id, agent_id, provider, model, cost_cents, occurred_at) VALUES ('ce3', 'comp-b', 'a2', 'openai', 'gpt-4', 500, '2026-01-15T00:00:00Z')")

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/costs/summary", GetCostSummaryHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-a/costs/summary", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result map[string]interface{}
	json.NewDecoder(w.Body).Decode(&result)
	totalCents := int64(result["totalCostCents"].(float64))
	if totalCents != 300 {
		t.Errorf("expected 300 total cost cents, got %d", totalCents)
	}
}

func TestGetCostsByAgentHandler(t *testing.T) {
	db := setupCostsTestDB(t)
	db.Exec("INSERT INTO cost_events (id, company_id, agent_id, provider, model, cost_cents, occurred_at) VALUES ('ce1', 'comp-a', 'a1', 'openai', 'gpt-4', 100, '2026-01-15')")
	db.Exec("INSERT INTO cost_events (id, company_id, agent_id, provider, model, cost_cents, occurred_at) VALUES ('ce2', 'comp-a', 'a1', 'openai', 'gpt-4', 50, '2026-01-16')")
	db.Exec("INSERT INTO cost_events (id, company_id, agent_id, provider, model, cost_cents, occurred_at) VALUES ('ce3', 'comp-a', 'a2', 'openai', 'gpt-4', 200, '2026-01-15')")

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/costs/by-agent", GetCostsByAgentHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-a/costs/by-agent", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var results []struct {
		AgentID   string `json:"agentId"`
		CostCents int64  `json:"costCents"`
	}
	json.NewDecoder(w.Body).Decode(&results)
	if len(results) != 2 {
		t.Errorf("expected 2 agent cost rows, got %d", len(results))
	}
}

func TestGetBudgetOverviewHandler(t *testing.T) {
	db := setupCostsTestDB(t)
	db.Exec("INSERT INTO budget_policies (id, company_id, scope_type, scope_id, amount) VALUES ('bp1', 'comp-a', 'company', 'comp-a', 10000)")
	db.Exec("INSERT INTO budget_policies (id, company_id, scope_type, scope_id, amount) VALUES ('bp2', 'comp-b', 'company', 'comp-b', 5000)")

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/budgets", GetBudgetOverviewHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-a/budgets", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var policies []models.BudgetPolicy
	json.NewDecoder(w.Body).Decode(&policies)
	if len(policies) != 1 {
		t.Errorf("expected 1 budget policy for comp-a, got %d", len(policies))
	}
}

func TestUpdateBudgetPolicyHandler(t *testing.T) {
	db := setupCostsTestDB(t)

	router := chi.NewRouter()
	router.Put("/companies/{companyId}/budgets", UpdateBudgetPolicyHandler(db))

	body, _ := json.Marshal(map[string]interface{}{
		"scopeType":  "company",
		"scopeId":    "comp-xyz",
		"metric":     "billed_cents",
		"windowKind": "monthly",
		"amount":     10000,
	})
	req := httptest.NewRequest(http.MethodPut, "/companies/comp-xyz/budgets", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestUpdateBudgetPolicyHandler_BadBody(t *testing.T) {
	db := setupCostsTestDB(t)

	router := chi.NewRouter()
	router.Put("/companies/{companyId}/budgets", UpdateBudgetPolicyHandler(db))

	req := httptest.NewRequest(http.MethodPut, "/companies/comp-1/budgets", bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestGetCostsByProviderHandler(t *testing.T) {
	db := setupCostsTestDB(t)
	db.Exec("INSERT INTO cost_events (id, company_id, agent_id, provider, model, cost_cents, occurred_at) VALUES ('ce1', 'comp-a', 'a1', 'openai', 'gpt-4', 100, '2026-01-15')")
	db.Exec("INSERT INTO cost_events (id, company_id, agent_id, provider, model, cost_cents, occurred_at) VALUES ('ce2', 'comp-a', 'a1', 'anthropic', 'claude', 200, '2026-01-15')")

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/costs/by-provider", GetCostsByProviderHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-a/costs/by-provider", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var results []struct {
		Provider   string `json:"provider"`
		TotalCents int64  `json:"totalCents"`
	}
	json.NewDecoder(w.Body).Decode(&results)
	if len(results) != 2 {
		t.Errorf("expected 2 provider cost rows, got %d", len(results))
	}
}

func TestGetFinanceEventsHandler(t *testing.T) {
	db := setupCostsTestDB(t)
	db.Exec("INSERT INTO finance_events (id, company_id, event_kind, direction, biller, amount_cents, occurred_at) VALUES ('fe1', 'comp-a', 'api_usage', 'debit', 'openai', 100, '2026-01-15')")
	db.Exec("INSERT INTO finance_events (id, company_id, event_kind, direction, biller, amount_cents, occurred_at) VALUES ('fe2', 'comp-b', 'api_usage', 'debit', 'openai', 200, '2026-01-15')")

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/finance/events", GetFinanceEventsHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-a/finance/events", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var events []models.FinanceEvent
	json.NewDecoder(w.Body).Decode(&events)
	if len(events) != 1 {
		t.Errorf("expected 1 finance event for comp-a, got %d", len(events))
	}
}

func TestGetFinanceSummaryHandler(t *testing.T) {
	db := setupCostsTestDB(t)
	db.Exec("INSERT INTO finance_events (id, company_id, event_kind, direction, biller, amount_cents, occurred_at) VALUES ('fe1', 'comp-a', 'api_usage', 'debit', 'openai', 300, '2026-01-15')")
	db.Exec("INSERT INTO finance_events (id, company_id, event_kind, direction, biller, amount_cents, occurred_at) VALUES ('fe2', 'comp-a', 'refund', 'credit', 'openai', 100, '2026-01-16')")

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/finance/summary", GetFinanceSummaryHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-a/finance/summary", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result map[string]interface{}
	json.NewDecoder(w.Body).Decode(&result)
	totalDebit := int64(result["totalDebitCents"].(float64))
	totalCredit := int64(result["totalCreditCents"].(float64))
	if totalDebit != 300 {
		t.Errorf("expected 300 debit cents, got %d", totalDebit)
	}
	if totalCredit != 100 {
		t.Errorf("expected 100 credit cents, got %d", totalCredit)
	}
}
