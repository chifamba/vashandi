package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"gorm.io/gorm"

	"github.com/chifamba/vashandi/vashandi/backend/server/routes"
)

type App struct {
	Router *chi.Mux
	DB     *gorm.DB
}

func NewApp(db *gorm.DB) *App {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// API Group
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/companies", routes.ListCompaniesHandler(db))
		r.Get("/companies/{id}", routes.GetCompanyHandler(db))
		r.Post("/companies", routes.CreateCompanyHandler(db))

		// Heartbeat Routes
		r.Route("/heartbeat", func(r chi.Router) {
			r.Post("/wakeup", routes.HeartbeatWakeupHandler(db))
			r.Get("/runs", routes.ListHeartbeatRunsHandler(db))
		})

		// Plugin Routes
		r.Get("/plugins", routes.ListPluginsHandler(db))
	})

	return &App{
		Router: r,
		DB:     db,
	}
}

func (a *App) Start(port int) error {
	addr := fmt.Sprintf(":%d", port)
	slog.Info("Starting server", "port", port)
	return http.ListenAndServe(addr, a.Router)
}

func Run() {
	cfg, err := LoadConfig()
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	// Initialize DB (simplified for now, assumes DB setup logic elsewhere)
	// In production, this would use cfg.Database connection details
	var db *gorm.DB 

	app := NewApp(db)
	if err := app.Start(cfg.Server.Port); err != nil {
		slog.Error("Server failed", "error", err)
		os.Exit(1)
	}
}
