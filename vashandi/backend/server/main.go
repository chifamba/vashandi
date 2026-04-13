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

	"github.com/chifamba/vashandi/vashandi/backend/server/services"
	"github.com/chifamba/vashandi/vashandi/backend/server/routes"
)

type App struct {
	Router *chi.Mux
	DB     *gorm.DB
}

func NewApp(db *gorm.DB) *App {
	r := SetupRouter(db)
	
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

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
