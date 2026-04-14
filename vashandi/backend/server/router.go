package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"gorm.io/gorm"

	"github.com/chifamba/vashandi/vashandi/backend/server/routes"
	"github.com/chifamba/vashandi/vashandi/backend/server/services"
)

// SetupRouter initializes the chi router with common middleware and routes
func SetupRouter(db *gorm.DB, activitySvc *services.ActivityService, secretsSvc *services.SecretService, heartbeatSvc *services.HeartbeatService) *chi.Mux {
	r := chi.NewRouter()

	issueRoutes := routes.NewIssueRoutes(db, activitySvc)
	costSvc := services.NewCostService(db)

	// A good base middleware stack
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Routes
	r.Get("/health", routes.HealthHandler(db))

	// Company Routes
	r.Get("/companies", routes.ListCompaniesHandler(db))
	r.Post("/companies", routes.CreateCompanyHandler(db, secretsSvc, heartbeatSvc.Memory))
	r.Get("/companies/{id}", routes.GetCompanyHandler(db))
	r.Patch("/companies/{id}/archive", routes.ArchiveCompanyHandler(db, heartbeatSvc.Memory))

	// Plugin UI static
	r.Get("/_plugins/{pluginId}/ui/*", routes.PluginUIStaticHandler(db))

	// API v1 Routes
	r.Route("/api/v1", func(api chi.Router) {
		// Heartbeat Routes
		api.Route("/heartbeat", func(h chi.Router) {
			h.Post("/wakeup", routes.HeartbeatWakeupHandler(heartbeatSvc))
			h.Get("/runs", routes.ListHeartbeatRunsHandler(db))
		})

		// Plugin Routes
		api.Get("/plugins", routes.ListPluginsHandler(db, activitySvc))

		// Issue Routes
		api.Get("/companies/{companyId}/issues", issueRoutes.ListIssuesHandler)
		api.Post("/companies/{companyId}/issues", issueRoutes.CreateIssueHandler)
		api.Post("/companies/{companyId}/issues/bulk", issueRoutes.BulkUpdateIssuesHandler)
		api.Get("/issues/{id}", issueRoutes.GetIssueHandler)
		api.Patch("/issues/{id}", issueRoutes.UpdateIssueHandler)
		api.Delete("/issues/{id}", issueRoutes.DeleteIssueHandler)
		api.Patch("/issues/{id}/status", issueRoutes.TransitionIssueHandler)
		api.Post("/issues/{id}/handoff", routes.HandoffIssueHandler(db))
		api.Post("/issues/{id}/checkout", routes.IssueCheckoutHandler(db, services.NewIssueService(db, activitySvc), heartbeatSvc, activitySvc))
		api.Get("/issues/{id}/comments", issueRoutes.ListIssueCommentsHandler)
		api.Post("/issues/{id}/comments", issueRoutes.AddIssueCommentHandler)
		api.Get("/issues/{id}/work-products", issueRoutes.ListWorkProductsHandler)
		api.Post("/issues/{id}/work-products", issueRoutes.CreateWorkProductHandler)

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
		api.Post("/companies/{companyId}/agents", routes.CreateAgentHandler(db, heartbeatSvc.Memory))
		api.Delete("/agents/{id}", routes.DeleteAgentHandler(db, heartbeatSvc.Memory))

		// MCP Governance Routes
		api.Get("/companies/{companyId}/mcp/tools", routes.MCPToolsHandler(db))
		api.Get("/companies/{companyId}/mcp/profiles", routes.MCPProfilesHandler(db))
		api.Get("/agents/{agentId}/mcp-tools", routes.AgentMCPToolsHandler(db))

		// Memory Service Routes
		api.Get("/companies/{companyId}/memory/bindings", routes.MemoryBindingsHandler(db))
		api.Get("/companies/{companyId}/memory/operations", routes.MemoryOperationsHandler(db))
		api.Get("/companies/{companyId}/memory/audit/export", routes.ExportAuditHandler(db))

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
		api.Post("/companies/{companyId}/curator/proposals/{proposalId}/approve", routes.ApproveProposalHandler(db, activitySvc))

		// Chat Routes
		api.Post("/companies/{companyId}/chat", routes.CeoChatIngestionHandler(db))

		// Access/Auth Routes
		api.Post("/invites/accept", routes.InviteAcceptHandler(db))
		api.Post("/cli-auth/challenge", routes.CLIAuthChallengeHandler(db))
		api.Get("/cli-auth/resolve/{token}", routes.ResolveCLIAuthHandler(db))
		api.Get("/companies/{companyId}/join-requests", routes.ListJoinRequestsHandler(db))
		api.Post("/join-requests/{id}/claim", routes.ClaimJoinRequestHandler(db))
		api.Patch("/member-roles/{id}", routes.UpdateMemberPermissionsHandler(db))
		api.Get("/llms/skills.txt", routes.ListSkillsHandler())

		// Adapter Routes
		api.Get("/adapters", routes.ListAdaptersHandler(db))
		api.Post("/adapters/{adapterType}/pause", routes.PauseAdapterHandler())

		// Approval Routes
		api.Get("/companies/{companyId}/approvals", routes.ListApprovalsHandler(db))
		api.Post("/companies/{companyId}/approvals", routes.CreateApprovalHandler(db))
		api.Post("/approvals/{id}/approve", routes.ApproveHandler(db, heartbeatSvc))
		api.Post("/approvals/{id}/reject", routes.RejectHandler(db))
		api.Post("/approvals/{id}/comments", routes.AddApprovalCommentHandler(db))

		// Asset Routes
		api.Post("/companies/{companyId}/assets", routes.UploadAssetHandler(db))
		api.Get("/assets/{id}", routes.GetAssetHandler(db))

		// Company Skills Routes
		api.Get("/companies/{companyId}/skills", routes.ListCompanySkillsHandler(db))
		api.Post("/companies/{companyId}/skills", routes.CreateCompanySkillHandler(db))
		api.Patch("/skills/{id}", routes.UpdateCompanySkillHandler(db))

		// Cost Routes
		api.Post("/companies/{companyId}/cost-events", routes.CreateCostEventHandler(db, costSvc))
		api.Get("/companies/{companyId}/costs/summary", routes.GetCostSummaryHandler(db))
		api.Get("/companies/{companyId}/costs/by-agent", routes.GetCostsByAgentHandler(db))
		api.Put("/companies/{companyId}/budget-policy", routes.UpdateBudgetPolicyHandler(db))

		// Execution Workspace Routes
		api.Get("/companies/{companyId}/execution-workspaces", routes.ListExecutionWorkspacesHandler(db))
		api.Get("/execution-workspaces/{id}", routes.GetExecutionWorkspaceHandler(db))
		api.Patch("/execution-workspaces/{id}", routes.UpdateExecutionWorkspaceHandler(db))

		// Inbox Dismissal Routes
		api.Get("/companies/{companyId}/inbox-dismissals", routes.ListInboxDismissalsHandler())
		api.Post("/companies/{companyId}/inbox-dismissals", routes.CreateInboxDismissalHandler())

		// Instance Settings Routes
		api.Get("/settings/general", routes.GetGeneralSettingsHandler(db))
		api.Patch("/settings/general", routes.UpdateGeneralSettingsHandler(db))
		api.Get("/settings/experimental", routes.GetExperimentalSettingsHandler(db))
		api.Patch("/settings/experimental", routes.UpdateExperimentalSettingsHandler(db))

		// LLM Routes
		api.Get("/llms/configuration", routes.ListAgentConfigurationHandler())
		api.Get("/llms/icons", routes.ListAgentIconsHandler())
		api.Get("/llms/adapters/{adapterType}", routes.GetAdapterConfigurationHandler())

		// Org Chart SVG
		api.Get("/companies/{companyId}/org-chart.svg", routes.OrgChartSVGHandler(db))

		// Project Routes
		api.Get("/companies/{companyId}/projects", routes.ListProjectsHandler(db))
		api.Post("/companies/{companyId}/projects", routes.CreateProjectHandler(db))
		api.Get("/projects/{id}", routes.GetProjectHandler(db))
		api.Patch("/projects/{id}", routes.UpdateProjectHandler(db))

		// Routine Routes
		api.Get("/companies/{companyId}/routines", routes.ListRoutinesHandler(db))
		api.Post("/companies/{companyId}/routines", routes.CreateRoutineHandler(db))
		api.Get("/routines/{id}", routes.GetRoutineHandler(db))
		api.Patch("/routines/{id}", routes.UpdateRoutineHandler(db))
		api.Get("/routines/{id}/runs", routes.ListRoutineRunsHandler(db))

		// Secret Routes
		api.Get("/secret-providers", routes.ListSecretProvidersHandler())
		api.Get("/companies/{companyId}/secrets", routes.ListSecretsHandler(db))
		api.Post("/companies/{companyId}/secrets", routes.CreateSecretHandler(db))
		api.Post("/secrets/{id}/rotate", routes.RotateSecretHandler(db))
		api.Patch("/secrets/{id}", routes.UpdateSecretHandler(db))
		api.Delete("/secrets/{id}", routes.DeleteSecretHandler(db))

		// Sidebar Badges
		api.Get("/companies/{companyId}/sidebar-badges", routes.SidebarBadgesHandler(db))
	})

	return r
}
