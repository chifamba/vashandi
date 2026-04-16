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
	Router               *chi.Mux
	DB                   *gorm.DB
	Heartbeat            *services.HeartbeatService
	Scheduler            *services.RoutineSchedulerService
	LiveEvents           *realtime.Hub
	PluginWorkerManager  *services.PluginWorkerManager
	PluginJobScheduler   *services.PluginJobScheduler
	PluginJobCoordinator *services.PluginJobCoordinator
	PluginHostServices   *services.PluginHostServices
	DatabaseBackup       *services.DatabaseBackupService
}

func NewApp(db *gorm.DB, routerOpts RouterOptions) *App {
	activitySvc := services.NewActivityService(db)
	secretsSvc := services.NewSecretService(db, activitySvc)
	opsSvc := services.NewWorkspaceOperationService(db)
	issueSvc := services.NewIssueService(db, activitySvc)
	goalSvc := services.NewGoalService(db)
	workspaceSvc := services.NewWorkspaceService(db)
	costSvc := services.NewCostService(db)
	registrySvc := services.NewPluginRegistryService(db)
	stateSvc := services.NewPluginStateStore(db)
	instanceSettingsSvc := services.NewInstanceSettingsService(db)
	routerOpts.InstanceSettings = instanceSettingsSvc

	// Create the adapter plugin store so user-installed external adapters are
	// discovered at startup via ~/.paperclip/adapter-plugins.json.
	adapterPluginStore := services.NewAdapterPluginStore()
	routerOpts.AdapterPluginStore = adapterPluginStore

	eventBus := services.NewPluginEventBus()
	capabilityValidator := services.NewPluginCapabilityValidator()
	pluginSecretsHandler := services.NewPluginSecretsHandler(db, secretsSvc, registrySvc, capabilityValidator)
	toolRegistry := services.NewPluginToolRegistry()
	toolDispatcher := services.NewPluginToolDispatcher(db, toolRegistry, nil) // Will inject wm below

	heartbeatSvc := services.NewHeartbeatService(db, secretsSvc, activitySvc, opsSvc, nil, nil)
	heartbeatSvc.Workspaces = workspaceSvc
	heartbeatSvc.Costs = costSvc
	heartbeatSvc.EventBus = eventBus

	// Create the shared event hub and inject it into the router options and
	// the heartbeat service so that run-status changes are broadcast to all
	// connected WebSocket and SSE clients.
	hub := realtime.NewHub()
	routerOpts.Hub = hub
	heartbeatSvc.Notify = hub.Publish

	// Create PluginHostServices
	hostServices := services.NewPluginHostServices(services.PluginHostServicesOptions{
		DB:             db,
		Issues:         issueSvc,
		Goals:          goalSvc,
		Heartbeat:      heartbeatSvc,
		Activity:       activitySvc,
		Costs:          costSvc,
		Secrets:        secretsSvc,
		Registry:       registrySvc,
		State:          stateSvc,
		EventBus:       eventBus,
		Telemetry:      routerOpts.Telemetry,
		Validator:      capabilityValidator,
		SecretsHandler: pluginSecretsHandler,
	})

	// Create the plugin stream bus (SSE fan-out for worker stream notifications).
	streamBus := services.NewPluginStreamBus()
	routerOpts.PluginStreamBus = streamBus

	// Create the plugin worker manager that spawns Node.js plugin processes.
	pluginWorkerManager := services.NewPluginWorkerManager(services.PluginWorkerManagerOptions{
		DB:                  db,
		StreamBus:           streamBus,
		EventBus:            eventBus,
		HostServices:        hostServices,
		CapabilityValidator: capabilityValidator,
		DefaultSandboxConfig: &services.PluginSandboxConfig{
			TimeoutMs:     2000,
			MemoryLimitMb: 512,
		},
	})
	routerOpts.PluginWorkerManager = pluginWorkerManager
	routerOpts.PluginCapabilityValidator = capabilityValidator
	toolDispatcher.WorkerManager = pluginWorkerManager
	routerOpts.PluginToolDispatcher = toolDispatcher

	schedulerSvc := services.NewRoutineSchedulerService(db, heartbeatSvc, issueSvc, activitySvc)

	routerOpts.PluginEventBus = eventBus

	// Create shared plugin lifecycle service.
	lifecycleSvc := services.NewPluginLifecycleService(db, pluginWorkerManager, eventBus, toolRegistry)
	routerOpts.PluginLifecycleService = lifecycleSvc

	// Initialize Plugin Job Scheduler stack.
	jobStore := services.NewPluginJobStore(db)
	jobScheduler := services.NewPluginJobScheduler(db, jobStore, pluginWorkerManager)
	jobCoordinator := services.NewPluginJobCoordinator(jobStore, jobScheduler, registrySvc, lifecycleSvc)

	routerOpts.PluginJobStore = jobStore
	routerOpts.PluginJobScheduler = jobScheduler

	// Create and wire the database backup service.
	backupSvc := services.NewDatabaseBackupService(
		db,
		instanceSettingsSvc,
		routerOpts.DatabaseBackup.Dir,
		routerOpts.DatabaseBackup.IntervalMinutes,
		routerOpts.DatabaseBackup.Enabled,
	)

	r := SetupRouter(db, activitySvc, secretsSvc, heartbeatSvc, routerOpts)

	return &App{
		Router:               r,
		DB:                   db,
		Heartbeat:            heartbeatSvc,
		Scheduler:            schedulerSvc,
		LiveEvents:           hub,
		PluginWorkerManager:  pluginWorkerManager,
		PluginJobScheduler:   jobScheduler,
		PluginJobCoordinator: jobCoordinator,
		PluginHostServices:   hostServices,
		DatabaseBackup:       backupSvc,
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

	authHandler := NewBetterAuthHandler(db)
	app := NewApp(db, RouterOptions{
		DeploymentMode:     cfg.Server.DeploymentMode,
		DeploymentExposure: cfg.Server.Exposure,
		AllowedHostnames:   cfg.Server.AllowedHostnames,
		BindHost:           cfg.Server.Host,
		AuthHandler:        authHandler,
		Telemetry:          GetTelemetryClient(),
		DatabaseBackup:     cfg.Database.Backup,
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
	if app.Heartbeat != nil {
		slog.Info("Starting heartbeat timer scheduler")
		services.StartHeartbeatScheduler(context.Background(), app.Heartbeat, 60_000)
	}

	// Start plugin workers for all ready plugins.
	if app.PluginWorkerManager != nil {
		slog.Info("Starting plugin workers for ready plugins")
		go app.PluginWorkerManager.StartReadyPlugins(context.Background())
	}

	// Start the plugin dev watcher (only active when PLUGIN_DEV_WATCH=true).
	// Queries installed local-path plugins so file changes auto-reload workers.
	if app.PluginWorkerManager != nil {
		var localPlugins []struct {
			ID          string  `gorm:"column:id"`
			PackagePath *string `gorm:"column:package_path"`
		}
		if err := db.WithContext(context.Background()).
			Table("plugins").
			Select("id, package_path").
			Where("status = ? AND package_path IS NOT NULL", "installed").
			Scan(&localPlugins).Error; err != nil {
			slog.Warn("plugin-dev-watcher: could not query local-path plugins", "error", err)
		} else {
			pluginDirs := make(map[string]string, len(localPlugins))
			for _, p := range localPlugins {
				if p.PackagePath != nil && *p.PackagePath != "" {
					pluginDirs[p.ID] = *p.PackagePath
				}
			}
			services.StartPluginDevWatcher(context.Background(), pluginDirs, app.PluginWorkerManager)
		}
	}

	// Start plugin host services log flusher.
	if app.PluginHostServices != nil {
		slog.Info("Starting plugin host services log flusher")
		app.PluginHostServices.StartLogFlusher(context.Background())
	}

	// Start plugin job scheduler and coordinator.
	if app.PluginJobScheduler != nil && app.PluginJobCoordinator != nil {
		slog.Info("Starting plugin job scheduler and coordinator")
		app.PluginJobCoordinator.Start()
		app.PluginJobScheduler.Start(context.Background())
	}

	// Start database backup and retention service.
	if app.DatabaseBackup != nil {
		go app.DatabaseBackup.Start(context.Background())
	}

	// Start the feedback export flusher (mirrors the Node.js 5-second timer).
	feedbackShareClient := services.NewFeedbackTraceShareClientFromEnv()
	feedbackExportSvc := services.NewFeedbackExportService(db, feedbackShareClient)
	if result, err := feedbackExportSvc.FlushPendingFeedbackTraces(context.Background(), nil); err != nil {
		slog.Error("Initial feedback export flush failed", "error", err)
	} else if result.Attempted > 0 {
		slog.Info("Initial feedback export flush", "attempted", result.Attempted, "sent", result.Sent, "failed", result.Failed)
	}
	slog.Info("Starting feedback export flusher")
	services.StartFeedbackExportFlusher(context.Background(), feedbackExportSvc, 5_000)

	slog.Info("Starting plugin log retention service")
	services.StartPluginLogRetention(context.Background(), db, 7, 1)

	if err := app.Start(cfg.Server.Port); err != nil {
		slog.Error("Server failed", "error", err)
		os.Exit(1)
	}
}
