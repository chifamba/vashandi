package routes

import (
	"encoding/json"
	"net/http"

	"gorm.io/gorm"
)

// The Plugins API manages installing, configuring, executing tools, and removing agent plugins.
// Since plugins rely heavily on Node.js process management and JSON-RPC over stdio, the Go port
// will return 501 Not Implemented until Phase 5 (Adapters and Plugins Architecture).

func PluginsNotImplementedHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotImplemented)
		json.NewEncoder(w).Encode(map[string]string{"error": "Plugin execution and management is pending Phase 5 (Go Plugin Architecture port)"})
	}
}

// These are placeholders to wire up the router for Phase 3 completion.
func ListPluginsHandler(db *gorm.DB) http.HandlerFunc { return PluginsNotImplementedHandler() }
func ListPluginExamplesHandler(db *gorm.DB) http.HandlerFunc { return PluginsNotImplementedHandler() }
func ListPluginUiContributionsHandler(db *gorm.DB) http.HandlerFunc { return PluginsNotImplementedHandler() }
func ListPluginToolsHandler(db *gorm.DB) http.HandlerFunc { return PluginsNotImplementedHandler() }
func ExecutePluginToolHandler(db *gorm.DB) http.HandlerFunc { return PluginsNotImplementedHandler() }
func InstallPluginHandler(db *gorm.DB) http.HandlerFunc { return PluginsNotImplementedHandler() }
func GetPluginHandler(db *gorm.DB) http.HandlerFunc { return PluginsNotImplementedHandler() }
func DeletePluginHandler(db *gorm.DB) http.HandlerFunc { return PluginsNotImplementedHandler() }
func EnablePluginHandler(db *gorm.DB) http.HandlerFunc { return PluginsNotImplementedHandler() }
func DisablePluginHandler(db *gorm.DB) http.HandlerFunc { return PluginsNotImplementedHandler() }
func GetPluginHealthHandler(db *gorm.DB) http.HandlerFunc { return PluginsNotImplementedHandler() }
func GetPluginLogsHandler(db *gorm.DB) http.HandlerFunc { return PluginsNotImplementedHandler() }
func UpgradePluginHandler(db *gorm.DB) http.HandlerFunc { return PluginsNotImplementedHandler() }
func GetPluginConfigHandler(db *gorm.DB) http.HandlerFunc { return PluginsNotImplementedHandler() }
func UpdatePluginConfigHandler(db *gorm.DB) http.HandlerFunc { return PluginsNotImplementedHandler() }
func TestPluginConfigHandler(db *gorm.DB) http.HandlerFunc { return PluginsNotImplementedHandler() }
func ListPluginJobsHandler(db *gorm.DB) http.HandlerFunc { return PluginsNotImplementedHandler() }
func ListPluginJobRunsHandler(db *gorm.DB) http.HandlerFunc { return PluginsNotImplementedHandler() }
func TriggerPluginJobHandler(db *gorm.DB) http.HandlerFunc { return PluginsNotImplementedHandler() }
func PluginWebhookHandler(db *gorm.DB) http.HandlerFunc { return PluginsNotImplementedHandler() }
func GetPluginDashboardHandler(db *gorm.DB) http.HandlerFunc { return PluginsNotImplementedHandler() }
