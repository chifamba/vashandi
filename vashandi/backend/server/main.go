package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/chifamba/vashandi/vashandi/backend/server/realtime"
	"github.com/chifamba/vashandi/vashandi/backend/server/services"
	"github.com/chifamba/vashandi/vashandi/backend/shared"
)
type App struct {
	Router     *chi.Mux
	DB         *gorm.DB
	Heartbeat  *services.HeartbeatService
	LiveEvents *realtime.Hub
}

func NewApp(db *gorm.DB, routerOpts RouterOptions) *App {
	activitySvc := services.NewActivityService(db)
	secretsSvc := services.NewSecretService(db, activitySvc)
	opsSvc := services.NewWorkspaceOperationService(db)
	heartbeatSvc := services.NewHeartbeatService(db, secretsSvc, activitySvc, opsSvc, nil, nil)

	// Create the live-events hub and inject it so SetupRouter and App share the same instance.
	hub := realtime.NewHub()
	routerOpts.Hub = hub

	r := SetupRouter(db, activitySvc, secretsSvc, heartbeatSvc, routerOpts)
	
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	return &App{
		Router:     r,
		DB:         db,
		Heartbeat:  heartbeatSvc,
		LiveEvents: hub,
	}
}

func (a *App) Start(port int) error {
	addr := fmt.Sprintf(":%d", port)
	slog.Info("Starting server", "port", port)
	return http.ListenAndServe(addr, a.Router)
}

// LoadConfig reads the PaperclipConfig from the instance root config.json.
// The DATABASE_URL environment variable overrides the config's database connection string.
func LoadConfig() (*shared.PaperclipConfig, error) {
	instanceRoot := shared.ResolvePaperclipInstanceRoot()
	configFile := filepath.Join(instanceRoot, "config.json")
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("could not read config file %s: %w", configFile, err)
	}
	var cfg shared.PaperclipConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("could not parse config file: %w", err)
	}
	return &cfg, nil
}

func Run() {
	cfg, err := LoadConfig()
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	// "ui-only" mode: serve the pre-built frontend SPA with no API routes and
	// no database connection.  This lets operators run the UI as a standalone
	// static file server while the API runs in a separate process.
	if cfg.Server.UIMode == shared.UIModeUIOnly {
		runUIOnly(cfg)
		return
	}

	dsn := cfg.Database.ConnectionString
	if envDSN := os.Getenv("DATABASE_URL"); envDSN != "" {
		dsn = envDSN
	}
	if dsn == "" {
		slog.Error("No database connection string configured; set database.connectionString in config or DATABASE_URL env var")
		os.Exit(1)
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		slog.Error("Failed to connect to database", "error", err)
		os.Exit(1)
	}

	app := NewApp(db, RouterOptions{
		DeploymentMode: cfg.Server.DeploymentMode,
		UIHandler:      newUIHandlerFromConfig(cfg.Server.UIMode),
	})

	// Startup Recovery
	if app.Heartbeat != nil {
		slog.Info("Running startup heartbeat recovery")
		if err := app.Heartbeat.ReapOrphanedRuns(context.Background()); err != nil {
			slog.Error("Startup recovery failed", "error", err)
		}
	}

	if err := app.Start(cfg.Server.Port); err != nil {
		slog.Error("Server failed", "error", err)
		os.Exit(1)
	}
}

// runUIOnly starts a lightweight static-file server for the pre-built frontend
// SPA.  No API routes and no database connection are required.
func runUIOnly(cfg *shared.PaperclipConfig) {
	uiHandler := newUIHandlerFromConfig(shared.UIModeUIOnly)
	if uiHandler == nil {
		slog.Error("ui-only mode: no UI assets found; cannot start")
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.Handle("/", uiHandler)

	var handler http.Handler = mux
	handler = cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "HEAD", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Content-Type"},
		AllowCredentials: false,
		MaxAge:           300,
	})(handler)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	slog.Info("Starting UI-only server", "port", cfg.Server.Port)
	if err := http.ListenAndServe(addr, handler); err != nil {
		slog.Error("UI-only server failed", "error", err)
		os.Exit(1)
	}
}
