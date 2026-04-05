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
	r.Get("/health", routes.HealthHandler(db))

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
	r.Post("/companies", routes.CreateCompanyHandler(db))
	r.Get("/companies/{id}", routes.GetCompanyHandler(db))
	r.Patch("/companies/{id}", routes.UpdateCompanyHandler(db))
	r.Delete("/companies/{id}", routes.DeleteCompanyHandler(db))

	// Costs Routes
	r.Post("/companies/{companyId}/costs/events", routes.ReportCostHandler(db))
	r.Get("/companies/{companyId}/costs/summary", routes.CostSummaryHandler(db))

	// Projects Routes
	r.Get("/companies/{companyId}/projects", routes.ListProjectsHandler(db))
	r.Post("/companies/{companyId}/projects", routes.CreateProjectHandler(db))
	r.Get("/projects/{id}", routes.GetProjectHandler(db))
	r.Patch("/projects/{id}", routes.UpdateProjectHandler(db))
	r.Delete("/projects/{id}", routes.DeleteProjectHandler(db))

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

	return r
}
