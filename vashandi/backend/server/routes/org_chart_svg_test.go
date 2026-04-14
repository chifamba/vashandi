package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupOrgChartTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&org_chart_test=1"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.Exec("DROP TABLE IF EXISTS agents")
	db.Exec(`CREATE TABLE agents (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		name text NOT NULL,
		role text NOT NULL DEFAULT 'general',
		title text,
		icon text,
		status text NOT NULL DEFAULT 'idle',
		reports_to text,
		capabilities text,
		adapter_type text NOT NULL DEFAULT 'process',
		adapter_config text NOT NULL DEFAULT '{}',
		runtime_config text NOT NULL DEFAULT '{}',
		budget_monthly_cents integer NOT NULL DEFAULT 0,
		spent_monthly_cents integer NOT NULL DEFAULT 0,
		pause_reason text,
		paused_at datetime,
		permissions text NOT NULL DEFAULT '{}',
		last_heartbeat_at datetime,
		metadata text,
		created_at datetime,
		updated_at datetime,
		deleted_at datetime
	)`)
	return db
}

func TestOrgChartSVGHandler_EmptyCompany(t *testing.T) {
	db := setupOrgChartTestDB(t)

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/org-chart.svg", OrgChartSVGHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/c1/org-chart.svg", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "image/svg+xml" {
		t.Errorf("expected Content-Type image/svg+xml, got %s", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "<svg") {
		t.Errorf("expected SVG output, got %s", body)
	}
}

func TestOrgChartSVGHandler_SingleAgent(t *testing.T) {
	db := setupOrgChartTestDB(t)
	db.Exec("INSERT INTO agents (id, company_id, name, role, adapter_type, adapter_config, runtime_config, permissions) VALUES ('a1', 'c1', 'CEO Bot', 'ceo', 'process', '{}', '{}', '{}')")

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/org-chart.svg", OrgChartSVGHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/c1/org-chart.svg", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "CEO Bot") {
		t.Errorf("expected agent name in SVG, got %s", body)
	}
}

func TestOrgChartSVGHandler_Hierarchy(t *testing.T) {
	db := setupOrgChartTestDB(t)
	db.Exec("INSERT INTO agents (id, company_id, name, role, adapter_type, adapter_config, runtime_config, permissions) VALUES ('ceo', 'c1', 'CEO', 'ceo', 'process', '{}', '{}', '{}')")
	reportsTo := "ceo"
	db.Exec("INSERT INTO agents (id, company_id, name, role, reports_to, adapter_type, adapter_config, runtime_config, permissions) VALUES ('dev', 'c1', 'Developer', 'general', ?, 'process', '{}', '{}', '{}')", reportsTo)

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/org-chart.svg", OrgChartSVGHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/c1/org-chart.svg", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "CEO") {
		t.Errorf("expected CEO in SVG")
	}
	if !strings.Contains(body, "Developer") {
		t.Errorf("expected Developer in SVG")
	}
	// Should contain an edge line between parent and child
	if !strings.Contains(body, "<line") {
		t.Errorf("expected edge <line> element in SVG for hierarchy")
	}
}

func TestOrgChartSVGHandler_CompanyScoping(t *testing.T) {
	db := setupOrgChartTestDB(t)
	db.Exec("INSERT INTO agents (id, company_id, name, role, adapter_type, adapter_config, runtime_config, permissions) VALUES ('a1', 'c1', 'Alpha', 'general', 'process', '{}', '{}', '{}')")
	db.Exec("INSERT INTO agents (id, company_id, name, role, adapter_type, adapter_config, runtime_config, permissions) VALUES ('a2', 'c2', 'Beta', 'general', 'process', '{}', '{}', '{}')")

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/org-chart.svg", OrgChartSVGHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/c1/org-chart.svg", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "Alpha") {
		t.Errorf("expected Alpha agent from c1")
	}
	if strings.Contains(body, "Beta") {
		t.Errorf("should not contain Beta agent from c2")
	}
}

func TestOrgChartSVGHandler_NebulaStyle(t *testing.T) {
	db := setupOrgChartTestDB(t)
	db.Exec("INSERT INTO agents (id, company_id, name, role, adapter_type, adapter_config, runtime_config, permissions) VALUES ('a1', 'c1', 'Agent', 'general', 'process', '{}', '{}', '{}')")

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/org-chart.svg", OrgChartSVGHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/c1/org-chart.svg?style=nebula", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	body := w.Body.String()
	// Nebula style uses dark background
	if !strings.Contains(body, "#0f0c29") {
		t.Errorf("expected nebula bg color #0f0c29 in SVG")
	}
}

func TestOrgChartSVGHandler_LongNameTruncation(t *testing.T) {
	db := setupOrgChartTestDB(t)
	longName := "A Very Long Agent Name That Exceeds Twenty Characters"
	db.Exec("INSERT INTO agents (id, company_id, name, role, adapter_type, adapter_config, runtime_config, permissions) VALUES ('a1', 'c1', ?, 'general', 'process', '{}', '{}', '{}')", longName)

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/org-chart.svg", OrgChartSVGHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/c1/org-chart.svg", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	body := w.Body.String()
	// Name should be truncated to 20 chars + "..."
	if strings.Contains(body, longName) {
		t.Errorf("expected long name to be truncated, but found full name")
	}
	if !strings.Contains(body, "...") {
		t.Errorf("expected truncated name with ellipsis")
	}
}

func TestOrgChartPNGHandler_FallsBackToSVG(t *testing.T) {
	db := setupOrgChartTestDB(t)

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/org-chart.png", OrgChartPNGHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/c1/org-chart.png", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "image/svg+xml" {
		t.Errorf("PNG handler should fallback to SVG, got Content-Type %s", ct)
	}
}

func TestHtmlEscape(t *testing.T) {
	tests := []struct {
		input, expected string
	}{
		{"hello", "hello"},
		{"<script>alert('xss')</script>", "&lt;script&gt;alert('xss')&lt;/script&gt;"},
		{"a & b", "a &amp; b"},
		{"", ""},
	}
	for _, tc := range tests {
		got := htmlEscape(tc.input)
		if got != tc.expected {
			t.Errorf("htmlEscape(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}
