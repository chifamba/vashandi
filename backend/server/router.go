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

	return r
}
