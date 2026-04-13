package server

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"gorm.io/gorm"

	"github.com/chifamba/vashandi/vashandi/backend/server/routes"
)

// SetupRouter initializes the chi router with common middleware and routes
func SetupRouter(db *gorm.DB) *chi.Mux {
	r := chi.NewRouter()

	// A good base middleware stack
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Routes
	r.Get("/health", routes.HealthHandler(db))

	// Company Routes
	r.Get("/companies", routes.ListCompaniesHandler(db))
	r.Post("/companies", routes.CreateCompanyHandler(db))
	r.Get("/companies/{id}", routes.GetCompanyHandler(db))

	// Agent Routes
	r.Get("/companies/{companyId}/agents", routes.ListAgentsHandler(db))
	r.Get("/agents/{id}", routes.GetAgentHandler(db))
	r.Post("/companies/{companyId}/agents", routes.CreateAgentHandler(db))
	r.Delete("/agents/{id}", routes.DeleteAgentHandler(db))

	// MCP Governance Routes
	r.Get("/companies/{companyId}/mcp/tools", routes.MCPToolsHandler(db))
	r.Get("/companies/{companyId}/mcp/profiles", routes.MCPProfilesHandler(db))
	r.Get("/agents/{agentId}/mcp-tools", routes.AgentMCPToolsHandler(db))

	// Memory Service Routes
	r.Get("/companies/{companyId}/memory/bindings", routes.MemoryBindingsHandler(db))
	r.Get("/companies/{companyId}/memory/operations", routes.MemoryOperationsHandler(db))

	// Teams Routes
	r.Get("/companies/{companyId}/teams", routes.TeamsHandler(db))
	r.Get("/teams/{teamId}", routes.TeamHandler(db))

	// Dashboard Routes
	r.Get("/companies/{companyId}/dashboard", routes.DashboardHandler(db))

	// Platform Metrics
	r.Get("/platform/metrics", routes.PlatformMetricsHandler(db))

	// Activity Routes
	r.Get("/companies/{companyId}/activity", routes.ListActivityHandler(db))
	r.Post("/companies/{companyId}/activity", routes.CreateActivityHandler(db))

	// Goals Routes
	r.Get("/companies/{companyId}/goals", routes.ListGoalsHandler(db))
	r.Post("/companies/{companyId}/goals", routes.CreateGoalHandler(db))
	r.Get("/goals/{id}", routes.GetGoalHandler(db))
	r.Patch("/goals/{id}", routes.UpdateGoalHandler(db))
	r.Delete("/goals/{id}", routes.DeleteGoalHandler(db))

	// Context Routes
	r.Post("/companies/{companyId}/context/hydrate", routes.PreRunHydrationHandler(db))
	r.Post("/companies/{companyId}/context/capture", routes.PostRunCaptureHandler(db))

	// Curator Routes
	r.Get("/companies/{companyId}/curator/proposals", routes.ListProposalsHandler(db))
	r.Post("/companies/{companyId}/curator/proposals/{proposalId}/approve", routes.ApproveProposalHandler(db))

	// Chat Routes
	r.Post("/companies/{companyId}/chat", routes.CeoChatIngestionHandler(db))

	return r
}
