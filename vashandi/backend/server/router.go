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
		api.Get("/issues/{id}/heartbeat-context", issueRoutes.GetIssueHeartbeatContextHandler)
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
		api.Post("/companies/{companyId}/issues/{issueId}/attachments", routes.UploadIssueAttachmentHandler(db))
		api.Get("/attachments/{attachmentId}/content", routes.GetAttachmentContentHandler(db))
		api.Delete("/attachments/{attachmentId}", routes.DeleteAttachmentHandler(db))
		api.Get("/issues/{id}/feedback-votes", issueRoutes.ListIssueFeedbackVotesHandler)
		api.Post("/issues/{id}/feedback-votes", issueRoutes.UpsertIssueFeedbackVoteHandler)
		api.Get("/issues/{id}/documents", issueRoutes.ListIssueDocumentsHandler)
		api.Get("/issues/{id}/documents/{key}", issueRoutes.GetIssueDocumentHandler)
		api.Get("/issues/{id}/documents/{key}/revisions", issueRoutes.ListIssueDocumentRevisionsHandler)
		api.Put("/issues/{id}/documents/{key}", issueRoutes.UpsertIssueDocumentHandler)
		api.Delete("/issues/{id}/documents/{key}", issueRoutes.DeleteIssueDocumentHandler)
		api.Patch("/work-products/{id}", routes.UpdateWorkProductHandler(db))
		api.Delete("/work-products/{id}", routes.DeleteWorkProductHandler(db))
		api.Get("/issues/{id}/comments/{commentId}", issueRoutes.GetIssueCommentHandler)

		// Issue Activity & Run Routes
		api.Get("/issues/{id}/activity", routes.ListIssueActivityHandler(db))
		api.Get("/issues/{id}/runs", routes.ListIssueRunsHandler(db))
		api.Get("/issues/{issueId}/live-runs", routes.GetIssueLiveRunsHandler(db))
		api.Get("/issues/{issueId}/active-run", routes.GetIssueActiveRunHandler(db))

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
		api.Post("/companies/{companyId}/activity", routes.CreateActivityHandler(db))

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
		api.Get("/agents/{id}/skills", routes.GetAgentSkillsHandler(db))
		api.Post("/agents/{id}/skills/sync", routes.SyncAgentSkillsHandler(db))
		api.Get("/agents/{id}/configuration", routes.GetAgentConfigurationHandler(db))
		api.Get("/agents/{id}/instructions-bundle", routes.GetAgentInstructionsBundleHandler(db))
		api.Patch("/agents/{id}/instructions-bundle", routes.PatchAgentInstructionsBundleHandler(db))
		api.Get("/agents/{id}/instructions-bundle/file", routes.GetAgentInstructionsBundleFileHandler(db))
		api.Put("/agents/{id}/instructions-bundle/file", routes.PutAgentInstructionsBundleFileHandler(db))
		api.Delete("/agents/{id}/instructions-bundle/file", routes.DeleteAgentInstructionsBundleFileHandler(db))
		api.Patch("/agents/{id}/instructions-path", routes.PatchAgentInstructionsPathHandler(db))
		api.Patch("/agents/{id}/permissions", routes.UpdateAgentPermissionsHandler(db))
		api.Get("/companies/{companyId}/adapters/{type}/models", routes.GetAdapterModelsHandler(db))
		api.Get("/companies/{companyId}/adapters/{type}/detect-model", routes.DetectAdapterModelHandler(db))
		api.Get("/companies/{companyId}/agent-configurations", routes.GetCompanyAgentConfigurationsHandler(db))
		api.Get("/instance/scheduler-heartbeats", routes.GetSchedulerHeartbeatsHandler(db))

		// Heartbeat Run Routes
		api.Get("/companies/{companyId}/heartbeat-runs", routes.ListCompanyHeartbeatRunsHandler(db))
		api.Get("/companies/{companyId}/live-runs", routes.GetCompanyLiveRunsHandler(db))
		api.Get("/heartbeat-runs/{runId}", routes.GetHeartbeatRunHandler(db))
		api.Post("/heartbeat-runs/{runId}/cancel", routes.CancelHeartbeatRunHandler(db))
		api.Get("/heartbeat-runs/{runId}/workspace-operations", routes.GetHeartbeatRunWorkspaceOperationsHandler(db))
		api.Get("/heartbeat-runs/{runId}/log", routes.GetHeartbeatRunLogHandler(db))
		api.Get("/heartbeat-runs/{runId}/issues", routes.ListHeartbeatRunIssuesHandler(db))
		api.Get("/workspace-operations/{operationId}/log", routes.GetWorkspaceOperationLogHandler(db))

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
		api.Get("/invites/{token}", routes.GetInviteHandler(db))
		api.Get("/invites/{token}/onboarding", routes.GetInviteOnboardingHandler(db))
		api.Post("/invites/{inviteId}/revoke", routes.RevokeInviteHandler(db))
		api.Post("/cli-auth/challenge", routes.CLIAuthChallengeHandler(db))
		api.Get("/cli-auth/resolve/{token}", routes.ResolveCLIAuthHandler(db))
		api.Get("/cli-auth/me", routes.GetCLIAuthMeHandler(db))
		api.Get("/companies/{companyId}/join-requests", routes.ListJoinRequestsHandler(db))
		api.Post("/join-requests/{id}/claim", routes.ClaimJoinRequestHandler(db))
		api.Patch("/member-roles/{id}", routes.UpdateMemberPermissionsHandler(db))
		api.Get("/companies/{companyId}/members", routes.ListCompanyMembersHandler(db))
		api.Patch("/companies/{companyId}/members/{userId}", routes.UpdateCompanyMemberHandler(db))
		api.Post("/companies/{companyId}/members/{userId}/remove", routes.RemoveCompanyMemberHandler(db))
		api.Get("/llms/skills.txt", routes.ListSkillsHandler())
		api.Get("/skills/available", routes.ListSkillsHandler())
		api.Get("/skills/index", routes.ListSkillsHandler())

		// Board-claim Routes
		api.Get("/board-claim/{token}", routes.BoardClaimTokenHandler(db))
		api.Post("/board-claim/{token}/claim", routes.ClaimBoardTokenHandler(db))

		// Admin user access Routes
		api.Get("/admin/users/{userId}/company-access", routes.GetUserCompanyAccessHandler(db))
		api.Put("/admin/users/{userId}/company-access", routes.UpdateUserCompanyAccessHandler(db))

		// Adapter Routes
		api.Get("/adapters", routes.ListAdaptersHandler(db))
		api.Post("/adapters/install", routes.InstallAdapterHandler(db))
		api.Post("/adapters/{adapterType}/pause", routes.PauseAdapterHandler())
		api.Post("/adapters/{type}/reload", routes.ReloadAdapterHandler(db))
		api.Post("/adapters/{type}/reinstall", routes.ReinstallAdapterHandler(db))
		api.Get("/adapters/{type}/config-schema", routes.GetAdapterConfigSchemaHandler(db))
		api.Patch("/adapters/{type}", routes.UpdateAdapterHandler(db))
		api.Patch("/adapters/{type}/override", routes.OverrideAdapterHandler(db))
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
		api.Post("/companies/{companyId}/assets/images", routes.UploadImageAssetHandler(db))
		api.Post("/companies/{companyId}/logo", routes.UploadCompanyLogoHandler(db))
		api.Get("/assets/{id}", routes.GetAssetHandler(db))
		api.Get("/assets/{assetId}/content", routes.GetAssetContentHandler(db))

		// Company Skills Routes
		api.Get("/companies/{companyId}/skills", routes.ListCompanySkillsHandler(db))
		api.Post("/companies/{companyId}/skills", routes.CreateCompanySkillHandler(db))
		api.Get("/companies/{companyId}/skills/{skillId}", routes.GetCompanySkillHandler(db))
		api.Get("/companies/{companyId}/skills/{skillId}/update-status", routes.GetCompanySkillUpdateStatusHandler(db))
		api.Get("/companies/{companyId}/skills/{skillId}/files", routes.GetCompanySkillFilesHandler(db))
		api.Post("/companies/{companyId}/skills/{skillId}/install-update", routes.InstallUpdateCompanySkillHandler(db))
		api.Delete("/companies/{companyId}/skills/{skillId}", routes.DeleteCompanySkillHandler(db))
		api.Patch("/skills/{id}", routes.UpdateCompanySkillHandler(db))

		// Cost Routes
		api.Post("/companies/{companyId}/cost-events", routes.CreateCostEventHandler(db, costSvc))
		api.Post("/companies/{companyId}/finance-events", routes.CreateFinanceEventHandler(db))
		api.Get("/companies/{companyId}/costs/summary", routes.GetCostSummaryHandler(db))
		api.Get("/companies/{companyId}/costs/by-agent", routes.GetCostsByAgentHandler(db))
		api.Get("/companies/{companyId}/costs/by-agent-model", routes.GetCostsByAgentModelHandler(db))
		api.Get("/companies/{companyId}/costs/by-provider", routes.GetCostsByProviderHandler(db))
		api.Get("/companies/{companyId}/costs/by-biller", routes.GetCostsByBillerHandler(db))
		api.Get("/companies/{companyId}/costs/by-project", routes.GetCostsByProjectHandler(db))
		api.Get("/companies/{companyId}/costs/finance-summary", routes.GetFinanceSummaryHandler(db))
		api.Get("/companies/{companyId}/costs/finance-by-biller", routes.GetFinanceByBillerHandler(db))
		api.Get("/companies/{companyId}/costs/finance-by-kind", routes.GetFinanceByKindHandler(db))
		api.Get("/companies/{companyId}/costs/finance-events", routes.GetFinanceEventsHandler(db))
		api.Get("/companies/{companyId}/costs/window-spend", routes.GetWindowSpendHandler(db))
		api.Get("/companies/{companyId}/costs/quota-windows", routes.GetQuotaWindowsHandler(db))
		api.Get("/companies/{companyId}/budgets/overview", routes.GetBudgetOverviewHandler(db))
		api.Patch("/companies/{companyId}/budgets", routes.PatchCompanyBudgetsHandler(db))
		api.Patch("/agents/{agentId}/budgets", routes.UpdateAgentBudgetHandler(db))
		api.Put("/companies/{companyId}/budget-policy", routes.UpdateBudgetPolicyHandler(db))

		// Execution Workspace Routes
		api.Get("/companies/{companyId}/execution-workspaces", routes.ListExecutionWorkspacesHandler(db))
		api.Get("/execution-workspaces/{id}", routes.GetExecutionWorkspaceHandler(db))
		api.Patch("/execution-workspaces/{id}", routes.UpdateExecutionWorkspaceHandler(db))
		api.Get("/execution-workspaces/{id}/close-readiness", routes.GetWorkspaceCloseReadinessHandler(db))
		api.Get("/execution-workspaces/{id}/workspace-operations", routes.GetWorkspaceWorkspaceOperationsHandler(db))
		api.Post("/execution-workspaces/{id}/runtime-services/{action}", routes.ExecutionWorkspaceRuntimeServicesHandler(db))

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
		api.Get("/llms/agent-configuration.txt", routes.ListAgentConfigurationHandler())
		api.Get("/llms/icons", routes.ListAgentIconsHandler())
		api.Get("/llms/agent-icons.txt", routes.ListAgentIconsHandler())
		api.Get("/llms/adapters/{adapterType}", routes.GetAdapterConfigurationHandler())
		api.Get("/llms/agent-configuration/{adapterType}.txt", routes.GetAdapterConfigurationHandler())

		// Org Chart SVG
		api.Get("/companies/{companyId}/org-chart.svg", routes.OrgChartSVGHandler(db))
		api.Get("/companies/{companyId}/org", routes.OrgChartSVGHandler(db))
		api.Get("/companies/{companyId}/org.svg", routes.OrgChartSVGHandler(db))
		api.Get("/companies/{companyId}/org.png", routes.OrgChartPNGHandler(db))

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
		api.Patch("/routine-triggers/{triggerId}", routes.UpdateRoutineTriggerHandler(db))
		api.Delete("/routine-triggers/{triggerId}", routes.DeleteRoutineTriggerHandler(db))
		api.Post("/routine-triggers/public/{publicId}/fire", routes.FirePublicRoutineTriggerHandler(db))
		api.Post("/routines/{id}/run", routes.RunRoutineNowHandler(db))
		api.Get("/routines/{id}/runs", routes.ListRoutineRunsHandler(db))

		// Secret Routes
		api.Get("/secret-providers", routes.ListSecretProvidersHandler())
		api.Get("/companies/{companyId}/secret-providers", routes.ListSecretProvidersHandler())
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
