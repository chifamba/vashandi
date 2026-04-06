package server

import (
	"github.com/chifamba/paperclip/backend/server/realtime"
	"gorm.io/gorm"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
		)

type App struct {
	Router *chi.Mux
}

func NewApp(db *gorm.DB) *App {
	rtManager := realtime.NewManager()
	go rtManager.Run()

	r := SetupRouter(db, rtManager)

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

	var db *gorm.DB // Stubbed: In a real implementation this is passed in from Run or Config
	app := NewApp(db)
	if err := app.Start(cfg.Server.Port); err != nil {
		slog.Error("Server failed", "error", err)
		os.Exit(1)
	}
}
