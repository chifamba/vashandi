package server

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"gorm.io/gorm"

	"github.com/chifamba/paperclip/backend/server/routes"
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
	r.Get("/health", routes.HealthHandler(db, routes.HealthOpts{}))

	// Dashboard Routes
	r.Get("/companies/{companyId}/dashboard", routes.DashboardHandler(db))

	// Activity Routes
	r.Get("/companies/{companyId}/activity", routes.ListActivityHandler(db))
	r.Post("/companies/{companyId}/activity", routes.CreateActivityHandler(db))

	// Goals Routes
	r.Get("/companies/{companyId}/goals", routes.ListGoalsHandler(db))
	r.Post("/companies/{companyId}/goals", routes.CreateGoalHandler(db))
	r.Get("/goals/{id}", routes.GetGoalHandler(db))
	r.Patch("/goals/{id}", routes.UpdateGoalHandler(db))
	r.Delete("/goals/{id}", routes.DeleteGoalHandler(db))

	// Companies Routes
	r.Get("/companies", routes.ListCompaniesHandler(db))
	r.Get("/companies/stats", routes.CompanyStatsHandler(db))
	r.Post("/companies", routes.CreateCompanyHandler(db))
	r.Get("/companies/{id}", routes.GetCompanyHandler(db))
	r.Patch("/companies/{id}", routes.UpdateCompanyHandler(db))
	r.Delete("/companies/{id}", routes.DeleteCompanyHandler(db))
	r.Post("/companies/{id}/archive", routes.ArchiveCompanyHandler(db))
	r.Get("/companies/{companyId}/feedback-traces", routes.ListFeedbackTracesHandler(db))
	r.Get("/companies/{companyId}/export", routes.ExportCompanyHandler(db))
	r.Post("/companies/import", routes.ImportCompanyHandler(db))

	// Costs Routes
	r.Post("/companies/{companyId}/costs/events", routes.ReportCostHandler(db))
	r.Get("/companies/{companyId}/costs/summary", routes.CostSummaryHandler(db))

	// Projects Routes
	r.Get("/companies/{companyId}/projects", routes.ListProjectsHandler(db))
	r.Post("/companies/{companyId}/projects", routes.CreateProjectHandler(db))
	r.Get("/projects/{id}", routes.GetProjectHandler(db))
	r.Patch("/projects/{id}", routes.UpdateProjectHandler(db))
	r.Delete("/projects/{id}", routes.DeleteProjectHandler(db))
	r.Post("/projects/{id}/archive", routes.ArchiveProjectHandler(db))
	r.Get("/projects/{id}/workspaces", routes.ListProjectWorkspacesHandler(db))
	r.Post("/projects/{id}/workspaces", routes.CreateProjectWorkspaceHandler(db))

	// Approvals Routes
	r.Get("/companies/{companyId}/approvals", routes.ListApprovalsHandler(db))
	r.Post("/companies/{companyId}/approvals", routes.CreateApprovalHandler(db))
	r.Get("/approvals/{id}", routes.GetApprovalHandler(db))
	r.Post("/approvals/{id}/approve", routes.ApproveApprovalHandler(db))
	r.Post("/approvals/{id}/reject", routes.RejectApprovalHandler(db))

	// Agents Routes
	r.Get("/companies/{companyId}/agents", routes.ListAgentsHandler(db))
	r.Post("/companies/{companyId}/agents", routes.CreateAgentHandler(db))
	r.Get("/agents/{id}", routes.GetAgentHandler(db))
	r.Patch("/agents/{id}", routes.UpdateAgentHandler(db))
	r.Delete("/agents/{id}", routes.DeleteAgentHandler(db))
	r.Post("/agents/{id}/pause", routes.PauseAgentHandler(db))
	r.Post("/agents/{id}/resume", routes.ResumeAgentHandler(db))
	r.Post("/agents/{id}/terminate", routes.TerminateAgentHandler(db))
	r.Get("/agents/{id}/keys", routes.ListAgentKeysHandler(db))
	r.Post("/agents/{id}/keys", routes.CreateAgentKeyHandler(db))
	r.Delete("/keys/{id}", routes.RevokeAgentKeyHandler(db))

	// Issues Routes
	r.Get("/companies/{companyId}/issues", routes.ListIssuesHandler(db))
	r.Post("/companies/{companyId}/issues", routes.CreateIssueHandler(db))
	r.Get("/issues/{id}", routes.GetIssueHandler(db))
	r.Patch("/issues/{id}", routes.UpdateIssueHandler(db))
	r.Delete("/issues/{id}", routes.DeleteIssueHandler(db))
	r.Get("/issues/{id}/comments", routes.ListIssueCommentsHandler(db))
	r.Post("/issues/{id}/comments", routes.CreateIssueCommentHandler(db))

	// Assets Routes
	r.Post("/companies/{companyId}/assets", routes.CreateAssetHandler(db))
	r.Get("/assets/{assetId}/content", routes.GetAssetContentHandler(db))

	// Routines Routes
	r.Get("/companies/{companyId}/routines", routes.ListRoutinesHandler(db))
	r.Post("/companies/{companyId}/routines", routes.CreateRoutineHandler(db))
	r.Get("/routines/{id}", routes.GetRoutineHandler(db))
	r.Patch("/routines/{id}", routes.UpdateRoutineHandler(db))
	r.Delete("/routines/{id}", routes.DeleteRoutineHandler(db))

	// Company Skills Routes
	r.Get("/companies/{companyId}/skills", routes.ListCompanySkillsHandler(db))
	r.Post("/companies/{companyId}/skills", routes.CreateCompanySkillHandler(db))
	r.Delete("/skills/{id}", routes.DeleteCompanySkillHandler(db))

	// Secrets Routes
	r.Get("/companies/{companyId}/secrets", routes.ListCompanySecretsHandler(db))
	r.Post("/companies/{companyId}/secrets", routes.CreateCompanySecretHandler(db))
	r.Delete("/secrets/{id}", routes.DeleteCompanySecretHandler(db))

	return r
}
