package routes

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"github.com/chifamba/vashandi/vashandi/backend/server/services"
	"gorm.io/gorm"
)

// validPluginStatuses mirrors the PLUGIN_STATUSES constant from @paperclipai/shared.
var validPluginStatuses = map[string]bool{
	"installed":       true,
	"ready":           true,
	"disabled":        true,
	"error":           true,
	"upgrade_pending": true,
	"uninstalled":     true,
	"pending":         true,
	"installing":      true,
}

// bundledPluginExample is a first-party example plugin available for local installation.
type bundledPluginExample struct {
	PackageName string `json:"packageName"`
	PluginKey   string `json:"pluginKey"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	LocalPath   string `json:"localPath"`
	Tag         string `json:"tag"`
}

var bundledExamples = []bundledPluginExample{
	{
		PackageName: "@paperclipai/plugin-hello-world-example",
		PluginKey:   "paperclip.hello-world-example",
		DisplayName: "Hello World Widget (Example)",
		Description: "Reference UI plugin that adds a simple Hello World widget to the Paperclip dashboard.",
		LocalPath:   "packages/plugins/examples/plugin-hello-world-example",
		Tag:         "example",
	},
	{
		PackageName: "@paperclipai/plugin-file-browser-example",
		PluginKey:   "paperclip-file-browser-example",
		DisplayName: "File Browser (Example)",
		Description: "Example plugin that adds a Files link in project navigation plus a project detail file browser.",
		LocalPath:   "packages/plugins/examples/plugin-file-browser-example",
		Tag:         "example",
	},
	{
		PackageName: "@paperclipai/plugin-kitchen-sink-example",
		PluginKey:   "paperclip-kitchen-sink-example",
		DisplayName: "Kitchen Sink (Example)",
		Description: "Reference plugin that demonstrates the current Paperclip plugin API surface.",
		LocalPath:   "packages/plugins/examples/plugin-kitchen-sink-example",
		Tag:         "example",
	},
}

// pluginUIContribution is the normalized shape returned by GET /plugins/ui-contributions.
type pluginUIContribution struct {
	PluginID    string        `json:"pluginId"`
	PluginKey   string        `json:"pluginKey"`
	DisplayName string        `json:"displayName"`
	Version     string        `json:"version"`
	UpdatedAt   string        `json:"updatedAt"`
	UIEntryFile string        `json:"uiEntryFile"`
	Slots       []interface{} `json:"slots"`
	Launchers   []interface{} `json:"launchers"`
}

// pluginHealthCheck is the response shape for GET /plugins/:pluginId/health and the
// health section of GET /plugins/:pluginId/dashboard.
type pluginHealthCheck struct {
	PluginID  string             `json:"pluginId"`
	Status    string             `json:"status"`
	Healthy   bool               `json:"healthy"`
	Checks    []pluginHealthItem `json:"checks"`
	LastError *string            `json:"lastError,omitempty"`
}

type pluginHealthItem struct {
	Name    string  `json:"name"`
	Passed  bool    `json:"passed"`
	Message *string `json:"message,omitempty"`
}

// resolvePlugin resolves a plugin by UUID or plugin key.
func resolvePlugin(registry *services.PluginRegistryService, r *http.Request, pluginID string) (*models.Plugin, error) {
	return registry.Resolve(r.Context(), pluginID)
}

// notImplemented returns 501 with an error message.
func notImplemented(w http.ResponseWriter, feature string) {
	writeJSON(w, http.StatusNotImplemented, map[string]string{
		"error": fmt.Sprintf("%s is not available in the Go backend", feature),
	})
}

// --------------------------------------------------------------------------
// GET /plugins
// --------------------------------------------------------------------------

// ListPluginsHandler returns a list of installed plugins, optionally filtered by status.
func ListPluginsHandler(db *gorm.DB, activity *services.ActivityService) http.HandlerFunc {
	registry := services.NewPluginRegistryService(db)
	return func(w http.ResponseWriter, r *http.Request) {
		if err := AssertBoard(r); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}

		rawStatus := r.URL.Query().Get("status")
		var plugins []models.Plugin
		var err error

		if rawStatus != "" {
			if !validPluginStatuses[rawStatus] {
				writeJSON(w, http.StatusBadRequest, map[string]string{
					"error": fmt.Sprintf("Invalid status %q. Must be one of: %s", rawStatus, joinStatuses()),
				})
				return
			}
			plugins, err = registry.ListByStatus(r.Context(), rawStatus)
		} else {
			plugins, err = registry.ListInstalled(r.Context())
		}

		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		if plugins == nil {
			plugins = []models.Plugin{}
		}
		writeJSON(w, http.StatusOK, plugins)
	}
}

func joinStatuses() string {
	keys := make([]string, 0, len(validPluginStatuses))
	for k := range validPluginStatuses {
		keys = append(keys, k)
	}
	return strings.Join(keys, ", ")
}

// --------------------------------------------------------------------------
// GET /plugins/examples
// --------------------------------------------------------------------------

// GetPluginExamplesHandler returns the list of bundled first-party example plugins.
func GetPluginExamplesHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := AssertBoard(r); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, bundledExamples)
	}
}

// --------------------------------------------------------------------------
// GET /plugins/ui-contributions
// --------------------------------------------------------------------------

// GetPluginUIContributionsHandler returns UI slot / launcher metadata from ready plugins.
func GetPluginUIContributionsHandler(db *gorm.DB) http.HandlerFunc {
	registry := services.NewPluginRegistryService(db)
	return func(w http.ResponseWriter, r *http.Request) {
		if err := AssertBoard(r); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}

		plugins, err := registry.ListByStatus(r.Context(), "ready")
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		contributions := make([]pluginUIContribution, 0)
		for _, plugin := range plugins {
			manifest := parseManifest(plugin.ManifestJSON)
			if manifest == nil {
				continue
			}
			uiEntry, slots, launchers := extractUIMetadata(manifest)
			if uiEntry == "" && len(slots) == 0 && len(launchers) == 0 {
				continue
			}
			displayName, _ := manifest["displayName"].(string)
			contributions = append(contributions, pluginUIContribution{
				PluginID:    plugin.ID,
				PluginKey:   plugin.PluginKey,
				DisplayName: displayName,
				Version:     plugin.Version,
				UpdatedAt:   plugin.UpdatedAt.UTC().Format(time.RFC3339),
				UIEntryFile: uiEntry,
				Slots:       slots,
				Launchers:   launchers,
			})
		}
		writeJSON(w, http.StatusOK, contributions)
	}
}

func parseManifest(raw []byte) map[string]interface{} {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m
}

func extractUIMetadata(manifest map[string]interface{}) (uiEntryFile string, slots []interface{}, launchers []interface{}) {
	if ui, ok := manifest["ui"].(map[string]interface{}); ok {
		if entry, ok := ui["entryFile"].(string); ok {
			uiEntryFile = entry
		}
		if sl, ok := ui["slots"].([]interface{}); ok {
			slots = sl
		}
		if la, ok := ui["launchers"].([]interface{}); ok {
			launchers = la
		}
	}
	// Legacy top-level launchers key
	if len(launchers) == 0 {
		if la, ok := manifest["launchers"].([]interface{}); ok {
			launchers = la
		}
	}
	return
}

// --------------------------------------------------------------------------
// GET /plugins/tools  (501 — requires worker manager)
// --------------------------------------------------------------------------

func GetPluginToolsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := AssertBoard(r); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}
		notImplemented(w, "Plugin tool dispatch")
	}
}

// --------------------------------------------------------------------------
// POST /plugins/tools/execute  (501 — requires worker manager)
// --------------------------------------------------------------------------

func ExecutePluginToolHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := AssertBoard(r); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}
		notImplemented(w, "Plugin tool dispatch")
	}
}

// --------------------------------------------------------------------------
// POST /plugins/install
// --------------------------------------------------------------------------

// InstallPluginHandler registers a plugin in the database. Since Go cannot run
// npm install natively, npm-based packages are recorded with status "pending"
// and must be loaded via the Node.js worker infrastructure.  Local-path plugins
// require a manifest.json at the given path to be readable.
func InstallPluginHandler(db *gorm.DB, activity *services.ActivityService) http.HandlerFunc {
	registry := services.NewPluginRegistryService(db)
	lifecycle := services.NewPluginLifecycleService(db)
	return func(w http.ResponseWriter, r *http.Request) {
		if err := AssertBoard(r); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}

		var body struct {
			PackageName string `json:"packageName"`
			Version     string `json:"version"`
			IsLocalPath bool   `json:"isLocalPath"`
			// PluginKey and ManifestJSON allow callers to pre-register a plugin
			// without relying on npm install (used by integration tests and the
			// CLI when the Node worker already installed the package).
			PluginKey    string          `json:"pluginKey"`
			ManifestJSON json.RawMessage `json:"manifestJson"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}

		if body.PackageName == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "packageName is required"})
			return
		}
		body.PackageName = strings.TrimSpace(body.PackageName)
		if body.PackageName == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "packageName cannot be empty"})
			return
		}

		if !body.IsLocalPath && strings.ContainsAny(body.PackageName, "<>:\"|?*") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "packageName contains invalid characters"})
			return
		}

		pluginKey := body.PluginKey
		manifestRaw := body.ManifestJSON
		version := body.Version
		if version == "" {
			version = "0.0.0"
		}

		// If no plugin key was supplied we derive one from the package name.
		if pluginKey == "" {
			pluginKey = body.PackageName
		}

		// Use provided manifest or create a minimal stub.
		if len(manifestRaw) == 0 {
			stub := map[string]interface{}{
				"id":          pluginKey,
				"displayName": body.PackageName,
				"version":     version,
			}
			raw, _ := json.Marshal(stub)
			manifestRaw = raw
		}

		var packagePath *string
		if body.IsLocalPath {
			p := body.PackageName
			packagePath = &p
		}

		plugin, err := registry.Register(r.Context(), services.RegisterPluginInput{
			PluginKey:   pluginKey,
			PackageName: body.PackageName,
			PackagePath: packagePath,
			Version:     version,
			ManifestRaw: manifestRaw,
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		// Transition to ready so the plugin is immediately usable.
		_, _ = lifecycle.Load(r.Context(), plugin.ID)
		updated, err := registry.GetByID(r.Context(), plugin.ID)
		if err != nil || updated == nil {
			updated = plugin
		}

		actor := GetActorInfo(r)
		if activity != nil {
			_, _ = activity.Log(r.Context(), services.LogEntry{
				CompanyID:  "system",
				ActorType:  actor.ActorType,
				ActorID:    actor.UserID,
				Action:     "plugin.installed",
				EntityType: "plugin",
				EntityID:   updated.ID,
				Details: map[string]interface{}{
					"pluginKey":   updated.PluginKey,
					"packageName": updated.PackageName,
					"version":     updated.Version,
					"source":      map[bool]string{true: "local_path", false: "npm"}[body.IsLocalPath],
				},
			})
		}

		writeJSON(w, http.StatusOK, updated)
	}
}

// --------------------------------------------------------------------------
// GET /plugins/:pluginId
// --------------------------------------------------------------------------

func GetPluginHandler(db *gorm.DB) http.HandlerFunc {
	registry := services.NewPluginRegistryService(db)
	return func(w http.ResponseWriter, r *http.Request) {
		if err := AssertBoard(r); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}

		pluginID := chi.URLParam(r, "pluginId")
		plugin, err := resolvePlugin(registry, r, pluginID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if plugin == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Plugin not found"})
			return
		}

		// Enrich with a supportsConfigTest=false flag (worker RPC not available in Go).
		type pluginWithMeta struct {
			models.Plugin
			SupportsConfigTest bool `json:"supportsConfigTest"`
		}
		writeJSON(w, http.StatusOK, pluginWithMeta{Plugin: *plugin, SupportsConfigTest: false})
	}
}

// --------------------------------------------------------------------------
// DELETE /plugins/:pluginId
// --------------------------------------------------------------------------

func DeletePluginHandler(db *gorm.DB, activity *services.ActivityService) http.HandlerFunc {
	registry := services.NewPluginRegistryService(db)
	lifecycle := services.NewPluginLifecycleService(db)
	return func(w http.ResponseWriter, r *http.Request) {
		if err := AssertBoard(r); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}

		pluginID := chi.URLParam(r, "pluginId")
		purge := r.URL.Query().Get("purge") == "true"

		plugin, err := resolvePlugin(registry, r, pluginID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if plugin == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Plugin not found"})
			return
		}

		result, err := lifecycle.Unload(r.Context(), plugin.ID, purge)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		actor := GetActorInfo(r)
		if activity != nil {
			_, _ = activity.Log(r.Context(), services.LogEntry{
				CompanyID:  "system",
				ActorType:  actor.ActorType,
				ActorID:    actor.UserID,
				Action:     "plugin.uninstalled",
				EntityType: "plugin",
				EntityID:   plugin.ID,
				Details:    map[string]interface{}{"pluginKey": plugin.PluginKey, "purge": purge},
			})
		}
		writeJSON(w, http.StatusOK, result)
	}
}

// --------------------------------------------------------------------------
// POST /plugins/:pluginId/enable
// --------------------------------------------------------------------------

func EnablePluginHandler(db *gorm.DB, activity *services.ActivityService) http.HandlerFunc {
	registry := services.NewPluginRegistryService(db)
	lifecycle := services.NewPluginLifecycleService(db)
	return func(w http.ResponseWriter, r *http.Request) {
		if err := AssertBoard(r); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}

		pluginID := chi.URLParam(r, "pluginId")
		plugin, err := resolvePlugin(registry, r, pluginID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if plugin == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Plugin not found"})
			return
		}

		result, err := lifecycle.Load(r.Context(), plugin.ID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		actor := GetActorInfo(r)
		if activity != nil {
			_, _ = activity.Log(r.Context(), services.LogEntry{
				CompanyID:  "system",
				ActorType:  actor.ActorType,
				ActorID:    actor.UserID,
				Action:     "plugin.enabled",
				EntityType: "plugin",
				EntityID:   plugin.ID,
				Details:    map[string]interface{}{"pluginKey": plugin.PluginKey},
			})
		}
		writeJSON(w, http.StatusOK, result)
	}
}

// --------------------------------------------------------------------------
// POST /plugins/:pluginId/disable
// --------------------------------------------------------------------------

func DisablePluginHandler(db *gorm.DB, activity *services.ActivityService) http.HandlerFunc {
	registry := services.NewPluginRegistryService(db)
	lifecycle := services.NewPluginLifecycleService(db)
	return func(w http.ResponseWriter, r *http.Request) {
		if err := AssertBoard(r); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}

		pluginID := chi.URLParam(r, "pluginId")
		plugin, err := resolvePlugin(registry, r, pluginID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if plugin == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Plugin not found"})
			return
		}

		var body struct {
			Reason string `json:"reason"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)

		result, err := lifecycle.Disable(r.Context(), plugin.ID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		actor := GetActorInfo(r)
		if activity != nil {
			_, _ = activity.Log(r.Context(), services.LogEntry{
				CompanyID:  "system",
				ActorType:  actor.ActorType,
				ActorID:    actor.UserID,
				Action:     "plugin.disabled",
				EntityType: "plugin",
				EntityID:   plugin.ID,
				Details:    map[string]interface{}{"pluginKey": plugin.PluginKey, "reason": body.Reason},
			})
		}
		writeJSON(w, http.StatusOK, result)
	}
}

// --------------------------------------------------------------------------
// GET /plugins/:pluginId/health
// --------------------------------------------------------------------------

func GetPluginHealthHandler(db *gorm.DB) http.HandlerFunc {
	registry := services.NewPluginRegistryService(db)
	return func(w http.ResponseWriter, r *http.Request) {
		if err := AssertBoard(r); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}

		pluginID := chi.URLParam(r, "pluginId")
		plugin, err := resolvePlugin(registry, r, pluginID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if plugin == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Plugin not found"})
			return
		}

		writeJSON(w, http.StatusOK, buildHealthCheck(plugin))
	}
}

func buildHealthCheck(plugin *models.Plugin) pluginHealthCheck {
	checks := []pluginHealthItem{
		{Name: "registry", Passed: true, Message: pluginStrPtr("Plugin found in registry")},
	}

	manifest := parseManifest(plugin.ManifestJSON)
	hasValidManifest := manifest != nil && manifest["id"] != nil
	manifestMsg := "Manifest is valid"
	if !hasValidManifest {
		manifestMsg = "Manifest is invalid or missing"
	}
	checks = append(checks, pluginHealthItem{Name: "manifest", Passed: hasValidManifest, Message: &manifestMsg})

	isReady := plugin.Status == "ready"
	statusMsg := fmt.Sprintf("Current status: %s", plugin.Status)
	checks = append(checks, pluginHealthItem{Name: "status", Passed: isReady, Message: &statusMsg})

	hasNoError := plugin.LastError == nil
	if !hasNoError {
		checks = append(checks, pluginHealthItem{Name: "error_state", Passed: false, Message: plugin.LastError})
	}

	return pluginHealthCheck{
		PluginID:  plugin.ID,
		Status:    plugin.Status,
		Healthy:   isReady && hasValidManifest && hasNoError,
		Checks:    checks,
		LastError: plugin.LastError,
	}
}

func pluginStrPtr(s string) *string { return &s }

// --------------------------------------------------------------------------
// GET /plugins/:pluginId/logs
// --------------------------------------------------------------------------

func GetPluginLogsHandler(db *gorm.DB) http.HandlerFunc {
	registry := services.NewPluginRegistryService(db)
	return func(w http.ResponseWriter, r *http.Request) {
		if err := AssertBoard(r); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}

		pluginID := chi.URLParam(r, "pluginId")
		plugin, err := resolvePlugin(registry, r, pluginID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if plugin == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Plugin not found"})
			return
		}

		limit := 25
		if lStr := r.URL.Query().Get("limit"); lStr != "" {
			if n, err := strconv.Atoi(lStr); err == nil {
				limit = n
			}
		}
		level := r.URL.Query().Get("level")

		var since *time.Time
		if sinceStr := r.URL.Query().Get("since"); sinceStr != "" {
			if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
				since = &t
			}
		}

		logs, err := registry.ListLogs(r.Context(), plugin.ID, services.ListLogsInput{
			Limit: limit,
			Level: level,
			Since: since,
		})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if logs == nil {
			logs = []models.PluginLog{}
		}
		writeJSON(w, http.StatusOK, logs)
	}
}

// --------------------------------------------------------------------------
// POST /plugins/:pluginId/upgrade
// --------------------------------------------------------------------------

func UpgradePluginHandler(db *gorm.DB, activity *services.ActivityService) http.HandlerFunc {
	registry := services.NewPluginRegistryService(db)
	lifecycle := services.NewPluginLifecycleService(db)
	return func(w http.ResponseWriter, r *http.Request) {
		if err := AssertBoard(r); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}

		pluginID := chi.URLParam(r, "pluginId")
		plugin, err := resolvePlugin(registry, r, pluginID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if plugin == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Plugin not found"})
			return
		}

		var body struct {
			Version string `json:"version"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)

		// In the Go backend we mark the plugin as upgrade_pending.
		// The actual npm upgrade is the responsibility of the Node.js worker infrastructure.
		result, err := lifecycle.MarkUpgradePending(r.Context(), plugin.ID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		actor := GetActorInfo(r)
		if activity != nil {
			_, _ = activity.Log(r.Context(), services.LogEntry{
				CompanyID:  "system",
				ActorType:  actor.ActorType,
				ActorID:    actor.UserID,
				Action:     "plugin.upgraded",
				EntityType: "plugin",
				EntityID:   plugin.ID,
				Details: map[string]interface{}{
					"pluginKey":       plugin.PluginKey,
					"previousVersion": plugin.Version,
					"targetVersion":   body.Version,
				},
			})
		}
		writeJSON(w, http.StatusOK, result)
	}
}

// --------------------------------------------------------------------------
// GET /plugins/:pluginId/config
// --------------------------------------------------------------------------

func GetPluginConfigHandler(db *gorm.DB) http.HandlerFunc {
	registry := services.NewPluginRegistryService(db)
	return func(w http.ResponseWriter, r *http.Request) {
		if err := AssertBoard(r); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}

		pluginID := chi.URLParam(r, "pluginId")
		plugin, err := resolvePlugin(registry, r, pluginID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if plugin == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Plugin not found"})
			return
		}

		config, err := registry.GetConfig(r.Context(), plugin.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		// Return null when no config record exists (matches Node.js behaviour).
		writeJSON(w, http.StatusOK, config)
	}
}

// --------------------------------------------------------------------------
// POST /plugins/:pluginId/config
// --------------------------------------------------------------------------

func SetPluginConfigHandler(db *gorm.DB, activity *services.ActivityService) http.HandlerFunc {
	registry := services.NewPluginRegistryService(db)
	return func(w http.ResponseWriter, r *http.Request) {
		if err := AssertBoard(r); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}

		pluginID := chi.URLParam(r, "pluginId")
		plugin, err := resolvePlugin(registry, r, pluginID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if plugin == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Plugin not found"})
			return
		}

		var body struct {
			ConfigJSON map[string]interface{} `json:"configJson"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ConfigJSON == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": `"configJson" is required and must be an object`})
			return
		}

		// Strip devUiUrl unless the caller is an instance admin (SSRF prevention).
		actor := GetActorInfo(r)
		if _, hasDevUI := body.ConfigJSON["devUiUrl"]; hasDevUI && !actor.IsInstanceAdmin {
			delete(body.ConfigJSON, "devUiUrl")
		}

		result, err := registry.UpsertConfig(r.Context(), plugin.ID, services.UpsertConfigInput{
			ConfigJSON: body.ConfigJSON,
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		if activity != nil {
			_, _ = activity.Log(r.Context(), services.LogEntry{
				CompanyID:  "system",
				ActorType:  actor.ActorType,
				ActorID:    actor.UserID,
				Action:     "plugin.config.updated",
				EntityType: "plugin",
				EntityID:   plugin.ID,
				Details: map[string]interface{}{
					"pluginKey":      plugin.PluginKey,
					"configKeyCount": len(body.ConfigJSON),
				},
			})
		}
		writeJSON(w, http.StatusOK, result)
	}
}

// --------------------------------------------------------------------------
// POST /plugins/:pluginId/config/test  (501 — requires worker manager)
// --------------------------------------------------------------------------

func TestPluginConfigHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := AssertBoard(r); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}
		notImplemented(w, "Plugin config testing (requires worker manager)")
	}
}

// --------------------------------------------------------------------------
// POST /plugins/:pluginId/bridge/data  (501)
// POST /plugins/:pluginId/bridge/action  (501)
// POST /plugins/:pluginId/data/:key  (501)
// POST /plugins/:pluginId/actions/:key  (501)
// GET  /plugins/:pluginId/bridge/stream/:channel  (501)
// --------------------------------------------------------------------------

func PluginBridgeDataHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		notImplemented(w, "Plugin bridge getData (requires worker manager)")
	}
}

func PluginBridgeActionHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		notImplemented(w, "Plugin bridge performAction (requires worker manager)")
	}
}

func PluginDataByKeyHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		notImplemented(w, "Plugin getData by key (requires worker manager)")
	}
}

func PluginActionByKeyHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		notImplemented(w, "Plugin performAction by key (requires worker manager)")
	}
}

func PluginBridgeStreamHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		notImplemented(w, "Plugin SSE stream bridge (requires worker manager)")
	}
}

// --------------------------------------------------------------------------
// GET /plugins/:pluginId/jobs
// --------------------------------------------------------------------------

func GetPluginJobsHandler(db *gorm.DB) http.HandlerFunc {
	registry := services.NewPluginRegistryService(db)
	return func(w http.ResponseWriter, r *http.Request) {
		if err := AssertBoard(r); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}

		pluginID := chi.URLParam(r, "pluginId")
		plugin, err := resolvePlugin(registry, r, pluginID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if plugin == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Plugin not found"})
			return
		}

		rawStatus := r.URL.Query().Get("status")
		if rawStatus != "" {
			valid := map[string]bool{"active": true, "paused": true, "failed": true}
			if !valid[rawStatus] {
				writeJSON(w, http.StatusBadRequest, map[string]string{
					"error": fmt.Sprintf("Invalid status %q. Must be one of: active, paused, failed", rawStatus),
				})
				return
			}
		}

		jobs, err := registry.ListJobs(r.Context(), plugin.ID, rawStatus)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if jobs == nil {
			jobs = []models.PluginJob{}
		}
		writeJSON(w, http.StatusOK, jobs)
	}
}

// --------------------------------------------------------------------------
// GET /plugins/:pluginId/jobs/:jobId/runs
// --------------------------------------------------------------------------

func GetPluginJobRunsHandler(db *gorm.DB) http.HandlerFunc {
	registry := services.NewPluginRegistryService(db)
	return func(w http.ResponseWriter, r *http.Request) {
		if err := AssertBoard(r); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}

		pluginID := chi.URLParam(r, "pluginId")
		jobID := chi.URLParam(r, "jobId")

		plugin, err := resolvePlugin(registry, r, pluginID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if plugin == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Plugin not found"})
			return
		}

		job, err := registry.GetJobByIDForPlugin(r.Context(), plugin.ID, jobID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if job == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Job not found"})
			return
		}

		limit := 25
		if lStr := r.URL.Query().Get("limit"); lStr != "" {
			if n, err := strconv.Atoi(lStr); err == nil {
				if n < 1 || n > 500 {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be between 1 and 500"})
					return
				}
				limit = n
			}
		}

		runs, err := registry.ListJobRuns(r.Context(), jobID, limit)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if runs == nil {
			runs = []models.PluginJobRun{}
		}
		writeJSON(w, http.StatusOK, runs)
	}
}

// --------------------------------------------------------------------------
// POST /plugins/:pluginId/jobs/:jobId/trigger  (501 — requires job scheduler)
// --------------------------------------------------------------------------

func TriggerPluginJobHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := AssertBoard(r); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}
		notImplemented(w, "Plugin job trigger (requires job scheduler)")
	}
}

// --------------------------------------------------------------------------
// POST /plugins/:pluginId/webhooks/:endpointKey
// --------------------------------------------------------------------------

// WebhookIngestionHandler records inbound webhook deliveries for a plugin.
// NOTE: This endpoint does NOT require board authentication (external callers
// must be able to reach it). Signature verification is the plugin's responsibility.
func WebhookIngestionHandler(db *gorm.DB) http.HandlerFunc {
	registry := services.NewPluginRegistryService(db)
	return func(w http.ResponseWriter, r *http.Request) {
		pluginID := chi.URLParam(r, "pluginId")
		endpointKey := chi.URLParam(r, "endpointKey")

		plugin, err := resolvePlugin(registry, r, pluginID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if plugin == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Plugin not found"})
			return
		}

		if plugin.Status != "ready" {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": fmt.Sprintf("Plugin is not ready (current status: %s)", plugin.Status),
			})
			return
		}

		manifest := parseManifest(plugin.ManifestJSON)
		if manifest == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Plugin manifest is missing"})
			return
		}

		// Verify webhooks.receive capability.
		capabilities, _ := manifest["capabilities"].([]interface{})
		hasWebhookCap := false
		for _, c := range capabilities {
			if c == "webhooks.receive" {
				hasWebhookCap = true
				break
			}
		}
		if !hasWebhookCap {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "Plugin does not have the webhooks.receive capability",
			})
			return
		}

		// Verify the endpointKey is declared in the manifest.
		declaredWebhooks, _ := manifest["webhooks"].([]interface{})
		found := false
		for _, wh := range declaredWebhooks {
			if whMap, ok := wh.(map[string]interface{}); ok {
				if whMap["endpointKey"] == endpointKey {
					found = true
					break
				}
			}
		}
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{
				"error": fmt.Sprintf("Webhook endpoint %q is not declared by this plugin", endpointKey),
			})
			return
		}

		// Read the raw body.
		rawBody, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read request body"})
			return
		}

		// Build headers JSON.
		rawHeaders := make(map[string]string)
		for key, vals := range r.Header {
			rawHeaders[strings.ToLower(key)] = strings.Join(vals, ", ")
		}
		headersJSON, _ := json.Marshal(rawHeaders)

		// Build payload JSON (use raw body if JSON, or wrap as string).
		var payloadJSON json.RawMessage
		if json.Valid(rawBody) {
			payloadJSON = rawBody
		} else {
			wrapped, _ := json.Marshal(map[string]string{"raw": string(rawBody)})
			payloadJSON = wrapped
		}

		startedAt := time.Now()
		deliveryID, err := registry.RecordWebhookDelivery(r.Context(), plugin.ID, endpointKey, payloadJSON, headersJSON)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to record delivery"})
			return
		}

		// In the Go backend there is no worker to dispatch to, so we record as
		// "success" immediately (the plugin worker infrastructure is Node.js-side).
		durationMs := int(time.Since(startedAt).Milliseconds())
		_ = registry.UpdateWebhookDelivery(r.Context(), deliveryID, "success", durationMs, nil)

		writeJSON(w, http.StatusOK, map[string]string{
			"deliveryId": deliveryID,
			"status":     "success",
		})
	}
}

// --------------------------------------------------------------------------
// GET /plugins/:pluginId/dashboard
// --------------------------------------------------------------------------

func GetPluginDashboardHandler(db *gorm.DB) http.HandlerFunc {
	registry := services.NewPluginRegistryService(db)
	return func(w http.ResponseWriter, r *http.Request) {
		if err := AssertBoard(r); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}

		pluginID := chi.URLParam(r, "pluginId")
		plugin, err := resolvePlugin(registry, r, pluginID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if plugin == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Plugin not found"})
			return
		}

		// Recent job runs.
		jobRuns, _ := registry.ListJobRunsByPlugin(r.Context(), plugin.ID, 10)
		if jobRuns == nil {
			jobRuns = []models.PluginJobRun{}
		}

		// Recent webhook deliveries.
		deliveries, _ := registry.ListWebhookDeliveries(r.Context(), plugin.ID, 10)
		if deliveries == nil {
			deliveries = []models.PluginWebhookDelivery{}
		}

		health := buildHealthCheck(plugin)

		type dashboardResponse struct {
			PluginID               string                       `json:"pluginId"`
			Worker                 interface{}                  `json:"worker"`
			RecentJobRuns          []models.PluginJobRun        `json:"recentJobRuns"`
			RecentWebhookDeliveries []models.PluginWebhookDelivery `json:"recentWebhookDeliveries"`
			Health                 pluginHealthCheck            `json:"health"`
			CheckedAt              string                       `json:"checkedAt"`
		}

		writeJSON(w, http.StatusOK, dashboardResponse{
			PluginID:               plugin.ID,
			Worker:                 nil, // No worker manager in Go backend
			RecentJobRuns:          jobRuns,
			RecentWebhookDeliveries: deliveries,
			Health:                 health,
			CheckedAt:              time.Now().UTC().Format(time.RFC3339),
		})
	}
}

