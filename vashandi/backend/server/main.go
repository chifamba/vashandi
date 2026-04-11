package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

type App struct {
	Router *chi.Mux
}

func NewApp() *App {
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

	// Example group for API
	r.Route("/api/v1", func(r chi.Router) {
		// e.g. r.Use(ActorMiddleware)
		r.Get("/companies", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[]`))
		})
	})

	return &App{
		Router: r,
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

	app := NewApp()
	if err := app.Start(cfg.Server.Port); err != nil {
		slog.Error("Server failed", "error", err)
		os.Exit(1)
	}
}
