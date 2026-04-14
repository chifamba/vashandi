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
	r.Use(ActorMiddleware(db))

	// Routes
	r.Get("/health", routes.HealthHandler(db))

	// Company Routes
	r.Get("/companies", routes.ListCompaniesHandler(db))
	r.Post("/companies", routes.CreateCompanyHandler(db, secretsSvc, heartbeatSvc.Memory))
	r.Get("/companies/{id}", routes.GetCompanyHandler(db))
	r.Patch("/companies/{id}", routes.UpdateCompanyHandler(db))
	r.Delete("/companies/{id}", routes.DeleteCompanyHandler(db))
	r.Patch("/companies/{id}/archive", routes.ArchiveCompanyHandler(db, heartbeatSvc.Memory))
	r.Patch("/companies/{id}/branding", routes.UpdateCompanyBrandingHandler(db))
	r.Get("/companies/stats", routes.GetCompanyStatsHandler(db))

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

		// Issue extended routes
		api.Post("/issues/{id}/release", issueRoutes.ReleaseIssueHandler)
		api.Get("/companies/{companyId}/labels", routes.ListIssueLabelsHandler(db))
		api.Post("/companies/{companyId}/labels", routes.CreateLabelHandler(db))
		api.Delete("/labels/{labelId}", routes.DeleteLabelHandler(db))
		api.Post("/issues/{id}/read", issueRoutes.MarkIssueReadHandler)
		api.Delete("/issues/{id}/read", issueRoutes.UnmarkIssueReadHandler)
		api.Post("/issues/{id}/inbox-archive", issueRoutes.ArchiveIssueInboxHandler)
		api.Delete("/issues/{id}/inbox-archive", issueRoutes.UnarchiveIssueInboxHandler)
		api.Get("/issues/{id}/approvals", issueRoutes.ListIssueApprovalsHandler)
		api.Post("/issues/{id}/approvals", issueRoutes.LinkIssueApprovalHandler)
		api.Delete("/issues/{id}/approvals/{approvalId}", issueRoutes.UnlinkIssueApprovalHandler)
		api.Get("/issues/{id}/attachments", issueRoutes.ListIssueAttachmentsHandler)
		api.Delete("/attachments/{attachmentId}", routes.DeleteAttachmentHandler(db))
		api.Get("/issues/{id}/feedback-votes", issueRoutes.ListIssueFeedbackVotesHandler)
		api.Post("/issues/{id}/feedback-votes", issueRoutes.UpsertIssueFeedbackVoteHandler)
		api.Get("/issues/{id}/documents", issueRoutes.ListIssueDocumentsHandler)
		api.Get("/issues/{id}/documents/{key}", issueRoutes.GetIssueDocumentHandler)
		api.Put("/issues/{id}/documents/{key}", issueRoutes.UpsertIssueDocumentHandler)
		api.Delete("/issues/{id}/documents/{key}", issueRoutes.DeleteIssueDocumentHandler)
		api.Patch("/work-products/{id}", routes.UpdateWorkProductHandler(db))
		api.Delete("/work-products/{id}", routes.DeleteWorkProductHandler(db))
		api.Get("/issues/{id}/comments/{commentId}", issueRoutes.GetIssueCommentHandler)

		// Issue Activity & Run Routes
		api.Get("/issues/{id}/activity", routes.ListIssueActivityHandler(db))
		api.Get("/issues/{id}/runs", routes.ListIssueRunsHandler(db))

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
		api.Post("/companies/{companyId}/agents", routes.CreateAgentHandler(db, heartbeatSvc.Memory))
		api.Get("/agents/{id}", routes.GetAgentHandler(db))
		api.Patch("/agents/{id}", routes.UpdateAgentHandler(db))
		api.Delete("/agents/{id}", routes.DeleteAgentHandler(db, heartbeatSvc.Memory))
		api.Post("/agents/{id}/pause", routes.PauseAgentHandler(db))
		api.Post("/agents/{id}/resume", routes.ResumeAgentHandler(db))
		api.Post("/agents/{id}/terminate", routes.TerminateAgentHandler(db))
		api.Post("/agents/{id}/wakeup", routes.WakeupAgentHandler(db))
		api.Get("/agents/{id}/runtime-state", routes.GetAgentRuntimeStateHandler(db))
		api.Post("/agents/{id}/runtime-state/reset-session", routes.ResetAgentSessionHandler(db))
		api.Get("/agents/{id}/task-sessions", routes.GetAgentTaskSessionsHandler(db))
		api.Get("/agents/{id}/config-revisions", routes.ListConfigRevisionsHandler(db))
		api.Get("/agents/{id}/config-revisions/{revisionId}", routes.GetConfigRevisionHandler(db))
		api.Post("/agents/{id}/config-revisions/{revisionId}/rollback", routes.RollbackConfigRevisionHandler(db))
		api.Get("/agents/{id}/keys", routes.GetAgentAPIKeysHandler(db))
		api.Post("/agents/{id}/keys", routes.CreateAgentAPIKeyHandler(db))
		api.Delete("/agents/{id}/keys/{keyId}", routes.RevokeAgentAPIKeyHandler(db))

		// Heartbeat Run Routes
		api.Get("/companies/{companyId}/heartbeat-runs", routes.ListCompanyHeartbeatRunsHandler(db))
		api.Get("/heartbeat-runs/{runId}", routes.GetHeartbeatRunHandler(db))
		api.Post("/heartbeat-runs/{runId}/cancel", routes.CancelHeartbeatRunHandler(db))
		api.Get("/heartbeat-runs/{runId}/workspace-operations", routes.GetHeartbeatRunWorkspaceOperationsHandler(db))

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

		// Board-claim Routes
		api.Get("/board-claim/{token}", routes.BoardClaimTokenHandler(db))
		api.Post("/board-claim/{token}/claim", routes.ClaimBoardTokenHandler(db))

		// Admin user access Routes
		api.Get("/admin/users/{userId}/company-access", routes.GetUserCompanyAccessHandler(db))
		api.Put("/admin/users/{userId}/company-access", routes.UpdateUserCompanyAccessHandler(db))

		// Adapter Routes
		api.Get("/adapters", routes.ListAdaptersHandler(db))
		api.Post("/adapters/{adapterType}/pause", routes.PauseAdapterHandler())
		api.Patch("/adapters/{type}", routes.UpdateAdapterHandler(db))
		api.Delete("/adapters/{type}", routes.DeleteAdapterHandler(db))

		// Approval Routes
		api.Get("/companies/{companyId}/approvals", routes.ListApprovalsHandler(db))
		api.Post("/companies/{companyId}/approvals", routes.CreateApprovalHandler(db))
		api.Get("/approvals/{id}", routes.GetApprovalHandler(db))
		api.Get("/approvals/{id}/issues", routes.GetApprovalIssuesHandler(db))
		api.Post("/approvals/{id}/resubmit", routes.ResubmitApprovalHandler(db))
		api.Get("/approvals/{id}/comments", routes.GetApprovalCommentsHandler(db))
		api.Post("/approvals/{id}/approve", routes.ApproveHandler(db, heartbeatSvc))
		api.Post("/approvals/{id}/reject", routes.RejectHandler(db))
		api.Post("/approvals/{id}/comments", routes.AddApprovalCommentHandler(db))

		// Asset Routes
		api.Post("/companies/{companyId}/assets", routes.UploadAssetHandler(db))
		api.Get("/assets/{id}", routes.GetAssetHandler(db))

		// Company Skills Routes
		api.Get("/companies/{companyId}/skills", routes.ListCompanySkillsHandler(db))
		api.Post("/companies/{companyId}/skills", routes.CreateCompanySkillHandler(db))
		api.Get("/companies/{companyId}/skills/{skillId}", routes.GetCompanySkillHandler(db))
		api.Delete("/companies/{companyId}/skills/{skillId}", routes.DeleteCompanySkillHandler(db))
		api.Patch("/skills/{id}", routes.UpdateCompanySkillHandler(db))

		// Cost Routes
		api.Post("/companies/{companyId}/cost-events", routes.CreateCostEventHandler(db, costSvc))
		api.Post("/companies/{companyId}/finance-events", routes.CreateFinanceEventHandler(db))
		api.Get("/companies/{companyId}/costs/summary", routes.GetCostSummaryHandler(db))
		api.Get("/companies/{companyId}/costs/by-agent", routes.GetCostsByAgentHandler(db))
		api.Get("/companies/{companyId}/costs/by-provider", routes.GetCostsByProviderHandler(db))
		api.Get("/companies/{companyId}/costs/by-biller", routes.GetCostsByBillerHandler(db))
		api.Get("/companies/{companyId}/budgets/overview", routes.GetBudgetOverviewHandler(db))
		api.Patch("/agents/{agentId}/budgets", routes.UpdateAgentBudgetHandler(db))
		api.Put("/companies/{companyId}/budget-policy", routes.UpdateBudgetPolicyHandler(db))

		// Execution Workspace Routes
		api.Get("/companies/{companyId}/execution-workspaces", routes.ListExecutionWorkspacesHandler(db))
		api.Get("/execution-workspaces/{id}", routes.GetExecutionWorkspaceHandler(db))
		api.Patch("/execution-workspaces/{id}", routes.UpdateExecutionWorkspaceHandler(db))
		api.Get("/execution-workspaces/{id}/close-readiness", routes.GetWorkspaceCloseReadinessHandler(db))
		api.Get("/execution-workspaces/{id}/workspace-operations", routes.GetWorkspaceWorkspaceOperationsHandler(db))

		// Inbox Dismissal Routes
		api.Get("/companies/{companyId}/inbox-dismissals", routes.ListInboxDismissalsHandler(db))
		api.Post("/companies/{companyId}/inbox-dismissals", routes.CreateInboxDismissalHandler(db))

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
		api.Delete("/projects/{id}", routes.DeleteProjectHandler(db))
		api.Get("/projects/{id}/workspaces", routes.ListProjectWorkspacesHandler(db))
		api.Post("/projects/{id}/workspaces", routes.CreateProjectWorkspaceHandler(db))
		api.Patch("/projects/{id}/workspaces/{workspaceId}", routes.UpdateProjectWorkspaceHandler(db))
		api.Delete("/projects/{id}/workspaces/{workspaceId}", routes.DeleteProjectWorkspaceHandler(db))
		api.Post("/projects/{id}/workspaces/{workspaceId}/runtime-services/{action}", routes.ProjectWorkspaceRuntimeServicesHandler())

		// Routine Routes
		api.Get("/companies/{companyId}/routines", routes.ListRoutinesHandler(db))
		api.Post("/companies/{companyId}/routines", routes.CreateRoutineHandler(db))
		api.Get("/routines/{id}", routes.GetRoutineHandler(db))
		api.Patch("/routines/{id}", routes.UpdateRoutineHandler(db))
		api.Delete("/routines/{id}", routes.DeleteRoutineHandler(db))
		api.Post("/routines/{id}/triggers", routes.CreateRoutineTriggerHandler(db))
		api.Post("/routines/{id}/run", routes.RunRoutineNowHandler(db))
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
		api.Get("/companies/{companyId}/sidebar-badges/stream", routes.SidebarBadgesSSEHandler())

		// Company Export/Import
		api.Post("/companies/{companyId}/exports", routes.ExportCompanyHandler())
		api.Post("/companies/{companyId}/imports/apply", routes.ImportCompanyHandler())

		// Heartbeat Run SSE Events
		api.Get("/heartbeat-runs/{runId}/events", routes.HeartbeatRunEventsSSEHandler())
	})

	return r
}
