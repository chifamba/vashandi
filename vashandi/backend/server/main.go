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
	Scheduler  *services.RoutineSchedulerService
	LiveEvents *realtime.Hub
}

func NewApp(db *gorm.DB, routerOpts RouterOptions) *App {
	activitySvc := services.NewActivityService(db)
	secretsSvc := services.NewSecretService(db, activitySvc)
	opsSvc := services.NewWorkspaceOperationService(db)
	heartbeatSvc := services.NewHeartbeatService(db, secretsSvc, activitySvc, opsSvc, nil, nil)

	// Create the shared event hub and inject it into the router options and
	// the heartbeat service so that run-status changes are broadcast to all
	// connected WebSocket and SSE clients.
	hub := realtime.NewHub()
	routerOpts.Hub = hub
	heartbeatSvc.Notify = hub.Publish

	issueSvc := services.NewIssueService(db, activitySvc)
	schedulerSvc := services.NewRoutineSchedulerService(db, heartbeatSvc, issueSvc, activitySvc)

	r := SetupRouter(db, activitySvc, secretsSvc, heartbeatSvc, routerOpts)

	return &App{
		Router:     r,
		DB:         db,
		Heartbeat:  heartbeatSvc,
		Scheduler:  schedulerSvc,
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

	InitTelemetry(cfg.Telemetry)

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
		DeploymentMode:     cfg.Server.DeploymentMode,
		DeploymentExposure: cfg.Server.Exposure,
		AllowedHostnames:   cfg.Server.AllowedHostnames,
		BindHost:           cfg.Server.Host,
		Telemetry:          GetTelemetryClient(),
	})

	// Startup Recovery
	if app.Heartbeat != nil {
		slog.Info("Running startup heartbeat recovery")
		if err := app.Heartbeat.ReapOrphanedRuns(context.Background()); err != nil {
			slog.Error("Startup recovery failed", "error", err)
		}
	}

	// Start the routine cron scheduler.
	if app.Scheduler != nil {
		slog.Info("Starting routine cron scheduler")
		services.StartRoutineScheduler(context.Background(), app.Scheduler, 60_000)
	}

	if err := app.Start(cfg.Server.Port); err != nil {
		slog.Error("Server failed", "error", err)
		os.Exit(1)
	}
}
