package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"gorm.io/gorm"

	"github.com/chifamba/vashandi/vashandi/backend/server/realtime"
	"github.com/chifamba/vashandi/vashandi/backend/server/routes"
	"github.com/chifamba/vashandi/vashandi/backend/server/services"
	"github.com/chifamba/vashandi/vashandi/backend/shared"
	"github.com/chifamba/vashandi/vashandi/backend/shared/telemetry"
)

// RouterOptions configures the router beyond the basic service dependencies.
type RouterOptions struct {
	// DeploymentMode is "local_trusted" (default) or "authenticated".
	DeploymentMode string

	// DeploymentExposure is "private" or "public".  Combined with
	// DeploymentMode == "authenticated" it enables the private hostname guard.
	DeploymentExposure string

	// AllowedHostnames is the operator-configured list of hostnames permitted
	// when the private hostname guard is active.
	AllowedHostnames []string

	// BindHost is the address the HTTP server listens on.  It is automatically
	// added to the private hostname allow set when non-empty and not "0.0.0.0".
	BindHost string

	// AuthHandler is an optional handler mounted at /api/auth/* to serve
	// BetterAuth endpoints (sign-in, sign-out, etc.).  When nil those paths
	// return 501 Not Implemented.
	AuthHandler http.Handler

	// BetterAuthSecret verifies signed better-auth cookies for session routes.
	BetterAuthSecret string

	// Hub is the shared realtime event hub used for WebSocket live events and
	// SSE badge streams. A new Hub is created automatically when nil.
	Hub *realtime.Hub

	// Telemetry is the active telemetry client. When non-nil, route handlers
	// emit events to the ingest endpoint. Pass nil to disable tracking.
	Telemetry *telemetry.Client

	// PluginWorkerManager manages Node.js plugin worker processes. When nil,
	// the worker-dependent routes return 501 Not Implemented.
	PluginWorkerManager *services.PluginWorkerManager

	// PluginStreamBus receives stream notifications from plugin workers for
	// SSE fan-out. When nil, the stream bridge route returns 501.
	PluginStreamBus *services.PluginStreamBus

	// PluginJobScheduler manages scheduled plugin jobs.
	PluginJobScheduler *services.PluginJobScheduler

	// PluginJobStore handles DB operations for plugin jobs.
	PluginJobStore *services.PluginJobStore

	// PluginLifecycleService manages plugin state transitions.
	PluginLifecycleService *services.PluginLifecycleService

	// DatabaseBackup configuration
	DatabaseBackup shared.DatabaseBackupConfig

	// PluginEventBus handles namespaced event routing for plugins.
	PluginEventBus *services.PluginEventBus
	// PluginCapabilityValidator enforces least-privilege for plugins.
	PluginCapabilityValidator *services.PluginCapabilityValidator
	// PluginToolDispatcher handles discovery and execution of plugin tools.
	PluginToolDispatcher *services.PluginToolDispatcher

	// InstanceSettings manages instance-wide configuration.
	InstanceSettings *services.InstanceSettingsService

	// AdapterPluginStore is the on-disk registry of user-installed external
	// adapter packages (~/.paperclip/adapter-plugins.json). When non-nil,
	// ListAdaptersHandler will include these entries alongside the built-ins.
	AdapterPluginStore *services.AdapterPluginStore

	// RuntimeManager manages workspace runtime services and startup rehydration.
	RuntimeManager *services.WorkspaceRuntimeManager
}

// SetupRouter initializes the chi router with common middleware and routes
func SetupRouter(db *gorm.DB, activitySvc *services.ActivityService, secretsSvc *services.SecretService, heartbeatSvc *services.HeartbeatService, opts RouterOptions) *chi.Mux {
	hub := opts.Hub
	if hub == nil {
		hub = realtime.NewHub()
	}
	deploymentMode := opts.DeploymentMode
	tc := opts.Telemetry

	r := chi.NewRouter()

	issueRoutes := routes.NewIssueRoutes(db, activitySvc)
	feedbackSvc := services.NewFeedbackService(db)
	costSvc := heartbeatSvc.Costs
	if costSvc == nil {
		costSvc = services.NewCostService(db)
	}
	if heartbeatSvc.BudgetEnforcementHook != nil {
		costSvc.BudgetEnforcementHook = heartbeatSvc.BudgetEnforcementHook
	}
	runtimeMgr := opts.RuntimeManager
	if runtimeMgr == nil {
		runtimeMgr = services.NewWorkspaceRuntimeManager(db)
	}

	// A good base middleware stack
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)

	// Use advanced structured logging middleware if available, fall back to chi.Logger
	loggingMw := DefaultLoggingMiddleware()
	if loggingMw != nil {
		r.Use(loggingMw.Handler)
	} else {
		r.Use(middleware.Logger)
	}

	// Use recoverer with telemetry integration
	if tc != nil {
		r.Use(RecovererWithTelemetry(tc))
	} else {
		r.Use(middleware.Recoverer)
	}

	// Error handler middleware for catching validation errors and panic recovery
	r.Use(ErrorHandlerMiddleware(ErrorHandlerConfig{
		Telemetry: tc,
	}))

	// CORS must be registered before routes so that preflight OPTIONS requests
	// are handled correctly.  AllowedOrigins uses "*" with credential support
	// via the reflected-origin behaviour of the cors package.
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "X-Paperclip-Run-Id"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Private hostname guard runs before auth so blocked requests never touch
	// the database.
	r.Use(PrivateHostnameGuard(PrivateHostnameGuardOptions{
		Enabled:          opts.DeploymentMode == "authenticated" && opts.DeploymentExposure == "private",
		AllowedHostnames: opts.AllowedHostnames,
		BindHost:         opts.BindHost,
	}))

	r.Use(ActorMiddleware(db, AuthMiddlewareOptions{
		DeploymentMode:   opts.DeploymentMode,
		BetterAuthSecret: opts.BetterAuthSecret,
	}))

	// Board mutation guard runs after auth so it can inspect the actor type and source.
	r.Use(BoardMutationGuard)

	// Auth routes — registered before the main /api route block so that more-specific
	// paths (get-session) take precedence over the wildcard catch-all.
	r.Get("/api/auth/get-session", routes.GetSessionHandler(db, routes.GetSessionHandlerOptions{
		DeploymentMode:   opts.DeploymentMode,
		BetterAuthSecret: opts.BetterAuthSecret,
	}))
	if opts.AuthHandler != nil {
		r.Handle("/api/auth/*", opts.AuthHandler)
	} else {
		r.HandleFunc("/api/auth/*", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotImplemented)
			json.NewEncoder(w).Encode(map[string]string{"error": "auth handler not configured"}) //nolint:errcheck
		})
	}

	// Plugin UI static
	r.Get("/_plugins/{pluginId}/ui/*", routes.PluginUIStaticHandler(db))

	// Root-level health check (backwards compatibility for Docker health checks, etc.)
	r.Get("/health", routes.HealthHandler(db, routes.HealthHandlerOptions{
		DeploymentMode:         opts.DeploymentMode,
		DeploymentExposure:     opts.DeploymentExposure,
		AuthReady:              true,
		CompanyDeletionEnabled: true,
	}))

	// API Routes - all routes under /api to match UI client expectations
	r.Route("/api", func(api chi.Router) {
		// Health check
		api.Get("/health", routes.HealthHandler(db, routes.HealthHandlerOptions{
			DeploymentMode:         opts.DeploymentMode,
			DeploymentExposure:     opts.DeploymentExposure,
			AuthReady:              true,
			CompanyDeletionEnabled: true,
		}))

		// Company Routes
		api.Get("/companies", routes.ListCompaniesHandler(db))
		api.Post("/companies", routes.CreateCompanyHandler(db, secretsSvc, heartbeatSvc.Memory))
		api.Get("/companies/{id}", routes.GetCompanyHandler(db))
		api.Patch("/companies/{id}", routes.UpdateCompanyHandler(db))
		api.Delete("/companies/{id}", routes.DeleteCompanyHandler(db))
		api.Patch("/companies/{id}/archive", routes.ArchiveCompanyHandler(db, heartbeatSvc.Memory))
		api.Patch("/companies/{id}/branding", routes.UpdateCompanyBrandingHandler(db))
		api.Get("/companies/stats", routes.GetCompanyStatsHandler(db))

		// Live-events WebSocket — GET /api/companies/{companyId}/events/ws
		// Path matches the Node.js server and the UI client expectations.
		// Auth is handled inside the handler (bearer token, ?token= query param, or local_trusted mode).
		resolveCookie := func(r *http.Request, db *gorm.DB) (routes.ActorInfo, bool) {
			return ResolveSessionCookieActor(r, db, opts.BetterAuthSecret)
		}
		api.Get("/companies/{companyId}/events/ws", hub.LiveEventsHandler(db, deploymentMode, resolveCookie))

		// Heartbeat Routes
		api.Route("/heartbeat", func(h chi.Router) {
			h.Post("/wakeup", routes.HeartbeatWakeupHandler(heartbeatSvc))
			h.Get("/runs", routes.ListHeartbeatRunsHandler(db))
		})

		// Plugin Routes
		// Static sub-paths must be registered before parameterized :pluginId routes.
		api.Get("/plugins", routes.ListPluginsHandler(db, activitySvc))
		api.Get("/plugins/examples", routes.GetPluginExamplesHandler())
		api.Get("/plugins/ui-contributions", routes.GetPluginUIContributionsHandler(db))
		api.Get("/plugins/tools", routes.GetPluginToolsHandler(opts.PluginToolDispatcher))
		api.Post("/plugins/tools/execute", routes.ExecutePluginToolHandler(opts.PluginToolDispatcher, activitySvc))
		api.Post("/plugins/install", routes.InstallPluginHandler(db, activitySvc, opts.PluginLifecycleService, opts.PluginCapabilityValidator))
		// Per-plugin routes
		api.Get("/plugins/{pluginId}", routes.GetPluginHandler(db))
		api.Delete("/plugins/{pluginId}", routes.DeletePluginHandler(db, activitySvc, opts.PluginLifecycleService))
		api.Post("/plugins/{pluginId}/enable", routes.EnablePluginHandler(db, activitySvc, opts.PluginLifecycleService))
		api.Post("/plugins/{pluginId}/disable", routes.DisablePluginHandler(db, activitySvc, opts.PluginLifecycleService))
		api.Get("/plugins/{pluginId}/health", routes.GetPluginHealthHandler(db))
		api.Get("/plugins/{pluginId}/logs", routes.GetPluginLogsHandler(db))
		api.Post("/plugins/{pluginId}/upgrade", routes.UpgradePluginHandler(db, activitySvc, opts.PluginLifecycleService))
		api.Get("/plugins/{pluginId}/config", routes.GetPluginConfigHandler(db))
		api.Post("/plugins/{pluginId}/config", routes.SetPluginConfigHandler(db, activitySvc))
		api.Post("/plugins/{pluginId}/config/test", routes.TestPluginConfigHandler(db, opts.PluginWorkerManager))
		api.Post("/plugins/{pluginId}/bridge/data", routes.PluginBridgeDataHandler(db, opts.PluginWorkerManager))
		api.Post("/plugins/{pluginId}/bridge/action", routes.PluginBridgeActionHandler(db, opts.PluginWorkerManager))
		api.Get("/plugins/{pluginId}/bridge/stream/{channel}", routes.PluginBridgeStreamHandler(db, opts.PluginWorkerManager, opts.PluginStreamBus))
		api.Post("/plugins/{pluginId}/data/{key}", routes.PluginDataByKeyHandler(db, opts.PluginWorkerManager))
		api.Post("/plugins/{pluginId}/actions/{key}", routes.PluginActionByKeyHandler(db, opts.PluginWorkerManager))
		api.Get("/plugins/{pluginId}/jobs", routes.GetPluginJobsHandler(db))
		api.Get("/plugins/{pluginId}/jobs/{jobId}/runs", routes.GetPluginJobRunsHandler(db))
		api.Post("/plugins/{pluginId}/jobs/{jobId}/trigger", routes.TriggerPluginJobHandler(db, opts.PluginJobScheduler))
		api.Post("/plugins/{pluginId}/webhooks/{endpointKey}", routes.WebhookIngestionHandler(db, opts.PluginWorkerManager))
		api.Get("/plugins/{pluginId}/dashboard", routes.GetPluginDashboardHandler(db, opts.PluginWorkerManager))

		// Issue Routes
		api.Get("/issues", issueRoutes.ListAllIssuesHandler)
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
		api.Get("/issues/{id}/feedback-traces", routes.ListIssueFeedbackTracesHandler(feedbackSvc))
		api.Get("/feedback-traces/{traceId}", routes.GetFeedbackTraceHandler(feedbackSvc))
		api.Get("/feedback-traces/{traceId}/bundle", routes.GetFeedbackTraceBundleHandler(feedbackSvc))
		api.Get("/companies/{companyId}/feedback-traces", routes.ListCompanyFeedbackTracesHandler(feedbackSvc))
		api.Get("/issues/{id}/documents", issueRoutes.ListIssueDocumentsHandler)
		api.Get("/issues/{id}/documents/{key}", issueRoutes.GetIssueDocumentHandler)
		api.Get("/issues/{id}/documents/{key}/revisions", issueRoutes.ListIssueDocumentRevisionsHandler)
		api.Post("/issues/{id}/documents/{key}/revisions/{revisionId}/restore", issueRoutes.RestoreIssueDocumentRevisionHandler)
		api.Put("/issues/{id}/documents/{key}", issueRoutes.UpsertIssueDocumentHandler)
		api.Delete("/issues/{id}/documents/{key}", issueRoutes.DeleteIssueDocumentHandler)
		api.Get("/issues/{id}/document-payload", issueRoutes.GetIssueDocumentPayloadHandler)
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
		api.Get("/activity/{id}", routes.GetActivityHandler(db))
		api.Post("/companies/{companyId}/activity", routes.CreateActivityHandler(db))

		// Agent Routes
		api.Get("/companies/{companyId}/agents", routes.ListAgentsHandler(db))
		api.Post("/companies/{companyId}/agents", routes.CreateAgentHandler(db, heartbeatSvc.Memory, tc))
		api.Post("/companies/{companyId}/agent-hires", routes.CreateAgentHireHandler(db))
		api.Get("/agents/me", routes.GetAgentMeHandler(db))
		api.Get("/agents/me/inbox-lite", routes.GetAgentMeInboxLiteHandler(db))
		api.Get("/agents/me/inbox/mine", routes.GetAgentMeInboxMineHandler(db))
		api.Get("/agents/{id}", routes.GetAgentHandler(db))
		api.Patch("/agents/{id}", routes.UpdateAgentHandler(db))
		api.Delete("/agents/{id}", routes.DeleteAgentHandler(db, heartbeatSvc.Memory))
		api.Post("/agents/{id}/pause", routes.PauseAgentHandler(db))
		api.Post("/agents/{id}/resume", routes.ResumeAgentHandler(db))
		api.Post("/agents/{id}/terminate", routes.TerminateAgentHandler(db))
		api.Post("/agents/{id}/wakeup", routes.WakeupAgentHandler(db))
		api.Post("/agents/{id}/heartbeat/invoke", routes.InvokeAgentHeartbeatHandler(db))
		api.Post("/agents/{id}/claude-login", routes.AgentClaudeLoginHandler(db))
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
		api.Get("/companies/{companyId}/context", routes.ListContextOperationsHandler(db))
		api.Get("/companies/{companyId}/context/{operation}", routes.GetContextOperationHandler(db))
		api.Post("/companies/{companyId}/context/hydrate", routes.PreRunHydrationHandler(db))
		api.Post("/companies/{companyId}/context/capture", routes.PostRunCaptureHandler(db))

		// Curator Routes
		api.Get("/companies/{companyId}/curator/proposals", routes.ListProposalsHandler(db))
		api.Post("/companies/{companyId}/curator/proposals/{proposalId}/approve", routes.ApproveProposalHandler(db, activitySvc))

		// Chat Routes
		api.Post("/companies/{companyId}/chat", routes.CeoChatIngestionHandler(db))

		// Access/Auth Routes
		api.Post("/invites/accept", routes.InviteAcceptHandler(db))
		api.Post("/invites/{token}/accept", routes.InviteAcceptByPathHandler(db))
		api.Get("/invites/{token}", routes.GetInviteHandler(db))
		api.Get("/invites/{token}/onboarding", routes.GetInviteOnboardingHandler(db))
		api.Get("/invites/{token}/onboarding.txt", routes.GetInviteOnboardingTextHandler(db))
		api.Get("/invites/{token}/test-resolution", routes.GetInviteTestResolutionHandler(db))
		api.Post("/invites/{inviteId}/revoke", routes.RevokeInviteHandler(db))
		api.Post("/companies/{companyId}/invites", routes.CreateCompanyInviteHandler(db))
		api.Post("/companies/{companyId}/openclaw/invite-prompt", routes.OpenClawInvitePromptHandler(db))
		// cli-auth: register both singular (legacy) and plural (canonical) paths
		api.Post("/cli-auth/challenge", routes.CLIAuthChallengeHandler(db))
		api.Post("/cli-auth/challenges", routes.CLIAuthChallengeHandler(db))
		api.Get("/cli-auth/challenges/{id}", routes.GetCLIAuthChallengeStatusHandler(db))
		api.Post("/cli-auth/challenges/{id}/approve", routes.ApproveCLIAuthChallengeHandler(db))
		api.Post("/cli-auth/challenges/{id}/cancel", routes.CancelCLIAuthChallengeHandler(db))
		api.Get("/cli-auth/resolve/{token}", routes.ResolveCLIAuthHandler(db))
		api.Get("/cli-auth/me", routes.GetCLIAuthMeHandler(db))
		api.Post("/cli-auth/revoke-current", routes.RevokeCLIAuthCurrentHandler(db))
		api.Get("/companies/{companyId}/join-requests", routes.ListJoinRequestsHandler(db))
		api.Post("/join-requests/{id}/claim", routes.ClaimJoinRequestHandler(db))
		api.Post("/companies/{companyId}/join-requests/{requestId}/approve", routes.ApproveJoinRequestHandler(db))
		api.Post("/companies/{companyId}/join-requests/{requestId}/reject", routes.RejectJoinRequestHandler(db))
		api.Post("/join-requests/{requestId}/claim-api-key", routes.ClaimJoinRequestAPIKeyHandler(db))
		api.Patch("/member-roles/{id}", routes.UpdateMemberPermissionsHandler(db))
		api.Get("/companies/{companyId}/members", routes.ListCompanyMembersHandler(db))
		api.Patch("/companies/{companyId}/members/{userId}", routes.UpdateCompanyMemberHandler(db))
		api.Post("/companies/{companyId}/members/{userId}/remove", routes.RemoveCompanyMemberHandler(db))
		api.Get("/llms/skills.txt", routes.ListSkillsHandler())
		api.Get("/skills/available", routes.ListSkillsHandler())
		api.Get("/skills/index", routes.ListSkillsHandler())
		api.Get("/skills/{skillName}", routes.GetSkillByNameHandler())

		// Board-claim Routes
		api.Get("/board-claim/{token}", routes.BoardClaimTokenHandler(db))
		api.Post("/board-claim/{token}/claim", routes.ClaimBoardTokenHandler(db))

		// Admin user access Routes
		api.Get("/admin/users/{userId}/company-access", routes.GetUserCompanyAccessHandler(db))
		api.Put("/admin/users/{userId}/company-access", routes.UpdateUserCompanyAccessHandler(db))
		api.Post("/admin/users/{userId}/promote-instance-admin", routes.PromoteInstanceAdminHandler(db))
		api.Post("/admin/users/{userId}/demote-instance-admin", routes.DemoteInstanceAdminHandler(db))

		// Adapter Routes
		api.Get("/adapters", routes.ListAdaptersHandler(db, opts.AdapterPluginStore))
		api.Post("/adapters/install", routes.InstallAdapterHandler(db))
		api.Post("/adapters/{adapterType}/pause", routes.PauseAdapterHandler())
		api.Post("/adapters/{type}/reload", routes.ReloadAdapterHandler(db))
		api.Post("/adapters/{type}/reinstall", routes.ReinstallAdapterHandler(db))
		api.Get("/adapters/{type}/config-schema", routes.GetAdapterConfigSchemaHandler(db))
		api.Get("/adapters/{type}/ui-parser.js", routes.GetAdapterUIParserHandler(db))
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
		api.Post("/approvals/{id}/request-revision", routes.RequestRevisionHandler(db, activitySvc))
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
		api.Post("/execution-workspaces/{id}/runtime-services/{action}", routes.ExecutionWorkspaceRuntimeServicesHandler(db, runtimeMgr))

		// Inbox Dismissal Routes
		api.Get("/companies/{companyId}/inbox-dismissals", routes.ListInboxDismissalsHandler(db))
		api.Post("/companies/{companyId}/inbox-dismissals", routes.CreateInboxDismissalHandler(db))

		// Instance Settings Routes
		api.Get("/settings/general", routes.GetGeneralSettingsHandler(opts.InstanceSettings))
		api.Patch("/settings/general", routes.UpdateGeneralSettingsHandler(opts.InstanceSettings))
		api.Get("/settings/experimental", routes.GetExperimentalSettingsHandler(opts.InstanceSettings))
		api.Patch("/settings/experimental", routes.UpdateExperimentalSettingsHandler(opts.InstanceSettings))
		api.Get("/instance/settings/general", routes.GetGeneralSettingsHandler(opts.InstanceSettings))
		api.Patch("/instance/settings/general", routes.UpdateGeneralSettingsHandler(opts.InstanceSettings))
		api.Get("/instance/settings/experimental", routes.GetExperimentalSettingsHandler(opts.InstanceSettings))
		api.Patch("/instance/settings/experimental", routes.UpdateExperimentalSettingsHandler(opts.InstanceSettings))

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
		api.Post("/projects/{id}/workspaces/{workspaceId}/runtime-services/{action}", routes.ProjectWorkspaceRuntimeServicesHandler(db, runtimeMgr))

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
		api.Get("/companies/{companyId}/sidebar-badges/stream", routes.SidebarBadgesSSEHandler(db, hub.Subscribe))

		// Company Export/Import
		api.Post("/companies/{companyId}/exports/preview", routes.PreviewExportCompanyHandler(db))
		api.Post("/companies/{companyId}/exports", routes.ExportCompanyHandler(db))
		api.Post("/companies/{companyId}/export", routes.ExportCompanyHandler(db)) // UI compatibility alias
		api.Post("/companies/{companyId}/imports/preview", routes.PreviewImportCompanyHandler(db))
		api.Post("/companies/{companyId}/imports/apply", routes.ImportCompanyHandler(db))
		// Global import routes (no companyId in path - board only)
		api.Post("/companies/import/preview", routes.GlobalImportPreviewHandler(db))
		api.Post("/companies/import", routes.GlobalImportHandler(db))

		// Heartbeat Run Events (REST endpoint — returns stored events from DB)
		api.Get("/heartbeat-runs/{runId}/events", routes.ListHeartbeatRunEventsHandler(db))
	})

	return r
}
