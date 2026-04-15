package server

import (
	"path/filepath"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/shared"
	"github.com/chifamba/vashandi/vashandi/backend/shared/telemetry"
)

// serverVersion is the canonical version string reported in telemetry payloads.
// It intentionally matches the value used in routes/health.go.
const serverVersion = "0.0.0-dev"

var globalTelemetryClient *telemetry.Client

// InitTelemetry initialises the global telemetry client from the supplied
// config. Subsequent calls are no-ops and return the already-initialised
// client. It mirrors initTelemetry() in server/src/telemetry.ts.
func InitTelemetry(cfg shared.TelemetryConfig) *telemetry.Client {
	if globalTelemetryClient != nil {
		return globalTelemetryClient
	}

	enabled := cfg.Enabled
	config := telemetry.ResolveConfig(&enabled)
	if !config.Enabled {
		return nil
	}

	stateDir := filepath.Join(shared.ResolvePaperclipInstanceRoot(), "telemetry")
	tc := telemetry.NewClient(config, func() telemetry.State {
		return telemetry.LoadOrCreateState(stateDir, serverVersion)
	}, serverVersion)
	tc.StartPeriodicFlush(60 * time.Second)

	globalTelemetryClient = tc
	return tc
}

// GetTelemetryClient returns the active global telemetry client.
// Returns nil when telemetry is disabled or has not been initialised.
func GetTelemetryClient() *telemetry.Client {
	return globalTelemetryClient
}
