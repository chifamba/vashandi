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

	// Initialize services
	activitySvc := services.NewActivityService(db)
	issueRoutes := routes.NewIssueRoutes(db, activitySvc)
	
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

	// API v1 Routes
	r.Route("/api/v1", func(api chi.Router) {
		// Heartbeat Routes
		api.Route("/heartbeat", func(h chi.Router) {
			h.Post("/wakeup", routes.HeartbeatWakeupHandler(db, activitySvc))
			h.Get("/runs", routes.ListHeartbeatRunsHandler(db))
		})

		// Plugin Routes
		api.Get("/plugins", routes.ListPluginsHandler(db, activitySvc))

		// Issue Routes
		api.Get("/companies/{companyId}/issues", issueRoutes.ListIssuesHandler)
		api.Post("/companies/{companyId}/issues", issueRoutes.CreateIssueHandler)
		api.Get("/issues/{id}", issueRoutes.GetIssueHandler)
		api.Patch("/issues/{id}/status", issueRoutes.TransitionIssueHandler)

		// Activity Routes
		api.Get("/companies/{companyId}/activity", func(w http.ResponseWriter, r *http.Request) {
			companyID := chi.URLParam(r, "companyId")
			list, err := activitySvc.List(r.Context(), services.ActivityFilters{CompanyID: companyID})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(list)
		})

		// Agent Routes
		api.Get("/companies/{companyId}/agents", routes.ListAgentsHandler(db))
		api.Get("/agents/{id}", routes.GetAgentHandler(db))
		api.Post("/companies/{companyId}/agents", routes.CreateAgentHandler(db))
		api.Delete("/agents/{id}", routes.DeleteAgentHandler(db))

		// MCP Governance Routes
		api.Get("/companies/{companyId}/mcp/tools", routes.MCPToolsHandler(db))
		api.Get("/companies/{companyId}/mcp/profiles", routes.MCPProfilesHandler(db))
		api.Get("/agents/{agentId}/mcp-tools", routes.AgentMCPToolsHandler(db))

		// Memory Service Routes
		api.Get("/companies/{companyId}/memory/bindings", routes.MemoryBindingsHandler(db))
		api.Get("/companies/{companyId}/memory/operations", routes.MemoryOperationsHandler(db))

		// Teams Routes
		api.Get("/companies/{companyId}/teams", routes.TeamsHandler(db))
		api.Get("/teams/{teamId}", routes.TeamHandler(db))

		// Dashboard Routes
		api.Get("/companies/{companyId}/dashboard", routes.DashboardHandler(db))

		// Platform Metrics
		api.Get("/platform/metrics", routes.PlatformMetricsHandler(db))

		// Goals Routes
		api.Get("/companies/{companyId}/goals", routes.ListGoalsHandler(db))
		api.Post("/companies/{companyId}/goals", routes.CreateGoalHandler(db))
		api.Get("/goals/{id}", routes.GetGoalHandler(db))
		api.Patch("/goals/{id}", routes.UpdateGoalHandler(db))
		api.Delete("/goals/{id}", routes.DeleteGoalHandler(db))

		// Context Routes
		api.Post("/companies/{companyId}/context/hydrate", routes.PreRunHydrationHandler(db))
		api.Post("/companies/{companyId}/context/capture", routes.PostRunCaptureHandler(db))

		// Curator Routes
		api.Get("/companies/{companyId}/curator/proposals", routes.ListProposalsHandler(db))
		api.Post("/companies/{companyId}/curator/proposals/{proposalId}/approve", routes.ApproveProposalHandler(db))

		// Chat Routes
		api.Post("/companies/{companyId}/chat", routes.CeoChatIngestionHandler(db))
	})

	return r
}
