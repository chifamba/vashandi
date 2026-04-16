package routes

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"github.com/chifamba/vashandi/vashandi/backend/server/services"
	"github.com/go-chi/chi/v5"
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
// GET /plugins/tools
// --------------------------------------------------------------------------

// agentToolDescriptor is the agent-facing tool shape returned by GET /plugins/tools.
type agentToolDescriptor struct {
	Name             string                 `json:"name"`
	DisplayName      string                 `json:"displayName"`
	Description      string                 `json:"description"`
	ParametersSchema map[string]interface{} `json:"parametersSchema"`
	PluginID         string                 `json:"pluginId"`
}

// GetPluginToolsHandler lists all plugin-contributed tools.
func GetPluginToolsHandler(dispatcher *services.PluginToolDispatcher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := AssertBoard(r); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}

		tools, err := dispatcher.ListToolsForAgent(r.Context(), "", "")
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, tools)
	}
}

// --------------------------------------------------------------------------
// POST /plugins/tools/execute
// --------------------------------------------------------------------------

// ExecutePluginToolHandler dispatches a tool-execution call to the appropriate
// plugin worker via the dispatcher.
type pluginToolExecutor interface {
	ExecuteTool(ctx context.Context, namespacedName string, parameters interface{}, runContext interface{}) (interface{}, error)
}

func ExecutePluginToolHandler(dispatcher pluginToolExecutor, activity *services.ActivityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := AssertBoard(r); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}

		var body struct {
			Tool       string                 `json:"tool"`
			Parameters map[string]interface{} `json:"parameters"`
			RunContext map[string]interface{} `json:"runContext"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}
		if body.Tool == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": `"tool" is required`})
			return
		}

		companyID, _ := body.RunContext["companyId"].(string)
		agentID, _ := body.RunContext["agentId"].(string)
		runID, _ := body.RunContext["runId"].(string)

		res, err := dispatcher.ExecuteTool(r.Context(), body.Tool, body.Parameters, body.RunContext)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}

		if activity != nil && strings.TrimSpace(companyID) != "" {
			actor := GetActorInfo(r)
			var agentIDPtr *string
			if strings.TrimSpace(agentID) != "" {
				agentIDPtr = &agentID
			}
			var runIDPtr *string
			if strings.TrimSpace(runID) != "" {
				runIDPtr = &runID
			}
			_, _ = activity.Log(r.Context(), services.LogEntry{
				CompanyID:  companyID,
				ActorType:  actor.ActorType,
				ActorID:    actor.UserID,
				Action:     "mcp_tool_invoked",
				EntityType: "plugin_tool",
				EntityID:   body.Tool,
				AgentID:    agentIDPtr,
				RunID:      runIDPtr,
				Details: map[string]interface{}{
					"tool":       body.Tool,
					"parameters": body.Parameters,
					"runContext": body.RunContext,
				},
			})
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		// If the dispatcher returned raw bytes (it does currently since it wraps wm.Call), we write them.
		if raw, ok := res.([]byte); ok {
			_, _ = w.Write(raw)
		} else {
			_ = json.NewEncoder(w).Encode(res)
		}
	}
}

// --------------------------------------------------------------------------
// POST /plugins/install
// --------------------------------------------------------------------------

// InstallPluginHandler registers a plugin in the database. Since Go cannot run
// npm install natively, npm-based packages are recorded with status "pending"
// and must be loaded via the Node.js worker infrastructure.  Local-path plugins
// require a manifest.json at the given path to be readable.
func InstallPluginHandler(db *gorm.DB, activity *services.ActivityService, lifecycle *services.PluginLifecycleService, validator *services.PluginCapabilityValidator) http.HandlerFunc {
	registry := services.NewPluginRegistryService(db)
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

		if validator != nil && len(body.ManifestJSON) > 0 {
			var manifestV1 services.PluginManifestV1
			if err := json.Unmarshal(body.ManifestJSON, &manifestV1); err == nil {
				if err := validator.ValidateManifestCapabilities(&manifestV1); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("Manifest validation failed: %v", err)})
					return
				}
			}
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

func DeletePluginHandler(db *gorm.DB, activity *services.ActivityService, lifecycle *services.PluginLifecycleService) http.HandlerFunc {
	registry := services.NewPluginRegistryService(db)
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

func EnablePluginHandler(db *gorm.DB, activity *services.ActivityService, lifecycle *services.PluginLifecycleService) http.HandlerFunc {
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

func DisablePluginHandler(db *gorm.DB, activity *services.ActivityService, lifecycle *services.PluginLifecycleService) http.HandlerFunc {
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

func UpgradePluginHandler(db *gorm.DB, activity *services.ActivityService, lifecycle *services.PluginLifecycleService) http.HandlerFunc {
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
// POST /plugins/:pluginId/config/test
// --------------------------------------------------------------------------

// pluginBridgeError is the standard error envelope for bridge RPC failures.
type pluginBridgeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// TestPluginConfigHandler validates a plugin configuration by forwarding it to
// the plugin worker's validateConfig RPC method.
func TestPluginConfigHandler(db *gorm.DB, wm *services.PluginWorkerManager) http.HandlerFunc {
	registry := services.NewPluginRegistryService(db)
	return func(w http.ResponseWriter, r *http.Request) {
		if err := AssertBoard(r); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}
		if wm == nil {
			writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "Plugin bridge is not enabled"})
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
		if plugin.Status != "ready" {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": fmt.Sprintf("Plugin is not ready (current status: %s)", plugin.Status),
			})
			return
		}

		var body struct {
			ConfigJSON map[string]interface{} `json:"configJson"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ConfigJSON == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": `"configJson" is required and must be an object`})
			return
		}

		// Check if the worker supports validateConfig.
		handle := wm.GetWorker(plugin.ID)
		if handle != nil && !handle.SupportsMethod("validateConfig") {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"valid":     false,
				"supported": false,
				"message":   "This plugin does not support configuration testing.",
			})
			return
		}

		params := map[string]interface{}{"config": body.ConfigJSON}
		raw, callErr := wm.Call(r.Context(), plugin.ID, "validateConfig", params, 30*time.Second)
		if callErr != nil {
			writeJSON(w, http.StatusBadGateway, pluginBridgeError{Code: "WORKER_UNAVAILABLE", Message: callErr.Error()})
			return
		}

		var result struct {
			OK       bool     `json:"ok"`
			Warnings []string `json:"warnings"`
			Errors   []string `json:"errors"`
		}
		if err := json.Unmarshal(raw, &result); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Invalid response from worker"})
			return
		}
		if result.OK {
			resp := map[string]interface{}{"valid": true}
			if len(result.Warnings) > 0 {
				resp["message"] = "Warnings: " + strings.Join(result.Warnings, "; ")
			}
			writeJSON(w, http.StatusOK, resp)
		} else {
			msg := "Configuration validation failed."
			if len(result.Errors) > 0 {
				msg = strings.Join(result.Errors, "; ")
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"valid": false, "message": msg})
		}
	}
}

// --------------------------------------------------------------------------
// POST /plugins/:pluginId/bridge/data
// POST /plugins/:pluginId/bridge/action
// POST /plugins/:pluginId/data/:key
// POST /plugins/:pluginId/actions/:key
// GET  /plugins/:pluginId/bridge/stream/:channel
// --------------------------------------------------------------------------

// PluginBridgeDataHandler proxies a getData call from the UI to the plugin worker.
func PluginBridgeDataHandler(db *gorm.DB, wm *services.PluginWorkerManager) http.HandlerFunc {
	registry := services.NewPluginRegistryService(db)
	return func(w http.ResponseWriter, r *http.Request) {
		if wm == nil {
			writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "Plugin bridge is not enabled"})
			return
		}
		pluginID := chi.URLParam(r, "pluginId")
		plugin, bridgeErr, status := resolvePluginForBridge(registry, r, pluginID)
		if bridgeErr != nil {
			writeJSON(w, status, bridgeErr)
			return
		}

		var body struct {
			Key               string                 `json:"key"`
			CompanyID         string                 `json:"companyId"`
			Params            map[string]interface{} `json:"params"`
			RenderEnvironment interface{}            `json:"renderEnvironment"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Key == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": `"key" is required and must be a string`})
			return
		}

		params := map[string]interface{}{
			"key":               body.Key,
			"params":            orEmptyMap(body.Params),
			"renderEnvironment": body.RenderEnvironment,
		}
		proxyBridgeCall(w, r.Context(), wm, plugin.ID, "getData", params)
	}
}

// PluginBridgeActionHandler proxies a performAction call from the UI to the plugin worker.
func PluginBridgeActionHandler(db *gorm.DB, wm *services.PluginWorkerManager) http.HandlerFunc {
	registry := services.NewPluginRegistryService(db)
	return func(w http.ResponseWriter, r *http.Request) {
		if wm == nil {
			writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "Plugin bridge is not enabled"})
			return
		}
		pluginID := chi.URLParam(r, "pluginId")
		plugin, bridgeErr, status := resolvePluginForBridge(registry, r, pluginID)
		if bridgeErr != nil {
			writeJSON(w, status, bridgeErr)
			return
		}

		var body struct {
			Key               string                 `json:"key"`
			CompanyID         string                 `json:"companyId"`
			Params            map[string]interface{} `json:"params"`
			RenderEnvironment interface{}            `json:"renderEnvironment"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Key == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": `"key" is required and must be a string`})
			return
		}

		params := map[string]interface{}{
			"key":               body.Key,
			"params":            orEmptyMap(body.Params),
			"renderEnvironment": body.RenderEnvironment,
		}
		proxyBridgeCall(w, r.Context(), wm, plugin.ID, "performAction", params)
	}
}

// PluginDataByKeyHandler proxies a getData call with the key as a URL parameter.
func PluginDataByKeyHandler(db *gorm.DB, wm *services.PluginWorkerManager) http.HandlerFunc {
	registry := services.NewPluginRegistryService(db)
	return func(w http.ResponseWriter, r *http.Request) {
		if wm == nil {
			writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "Plugin bridge is not enabled"})
			return
		}
		pluginID := chi.URLParam(r, "pluginId")
		key := chi.URLParam(r, "key")
		plugin, bridgeErr, status := resolvePluginForBridge(registry, r, pluginID)
		if bridgeErr != nil {
			writeJSON(w, status, bridgeErr)
			return
		}

		var body struct {
			CompanyID         string                 `json:"companyId"`
			Params            map[string]interface{} `json:"params"`
			RenderEnvironment interface{}            `json:"renderEnvironment"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)

		params := map[string]interface{}{
			"key":               key,
			"params":            orEmptyMap(body.Params),
			"renderEnvironment": body.RenderEnvironment,
		}
		proxyBridgeCall(w, r.Context(), wm, plugin.ID, "getData", params)
	}
}

// PluginActionByKeyHandler proxies a performAction call with the key as a URL parameter.
func PluginActionByKeyHandler(db *gorm.DB, wm *services.PluginWorkerManager) http.HandlerFunc {
	registry := services.NewPluginRegistryService(db)
	return func(w http.ResponseWriter, r *http.Request) {
		if wm == nil {
			writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "Plugin bridge is not enabled"})
			return
		}
		pluginID := chi.URLParam(r, "pluginId")
		key := chi.URLParam(r, "key")
		plugin, bridgeErr, status := resolvePluginForBridge(registry, r, pluginID)
		if bridgeErr != nil {
			writeJSON(w, status, bridgeErr)
			return
		}

		var body struct {
			CompanyID         string                 `json:"companyId"`
			Params            map[string]interface{} `json:"params"`
			RenderEnvironment interface{}            `json:"renderEnvironment"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)

		params := map[string]interface{}{
			"key":               key,
			"params":            orEmptyMap(body.Params),
			"renderEnvironment": body.RenderEnvironment,
		}
		proxyBridgeCall(w, r.Context(), wm, plugin.ID, "performAction", params)
	}
}

// PluginBridgeStreamHandler is an SSE endpoint that fans out stream events from
// the plugin worker to the connected UI client.
func PluginBridgeStreamHandler(db *gorm.DB, wm *services.PluginWorkerManager, streamBus *services.PluginStreamBus) http.HandlerFunc {
	registry := services.NewPluginRegistryService(db)
	return func(w http.ResponseWriter, r *http.Request) {
		if err := AssertBoard(r); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}
		if wm == nil || streamBus == nil {
			writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "Plugin stream bridge is not enabled"})
			return
		}

		pluginID := chi.URLParam(r, "pluginId")
		channel := chi.URLParam(r, "channel")
		companyID := r.URL.Query().Get("companyId")
		if companyID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": `"companyId" query parameter is required`})
			return
		}

		plugin, err := resolvePlugin(registry, r, pluginID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if plugin == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Plugin not found"})
			return
		}

		// SSE Setup
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		// Subscribe to stream bus
		events, cancel := streamBus.Subscribe(plugin.ID, channel, companyID)
		defer cancel()

		// Send initial connection comment
		_, _ = fmt.Fprint(w, ":ok\n\n")
		flusher.Flush()

		// Send initial open signal
		fmt.Fprintf(w, "event: %s\ndata: {}\n\n", services.StreamEventOpen)
		flusher.Flush()

		for {
			select {
			case <-r.Context().Done():
				return
			case event, ok := <-events:
				if !ok {
					return
				}
				data, _ := json.Marshal(event.Data)
				if event.Type != services.StreamEventMessage {
					fmt.Fprintf(w, "event: %s\n", string(event.Type))
				}
				fmt.Fprintf(w, "data: %s\n\n", string(data))
				flusher.Flush()

				if event.Type == services.StreamEventClose {
					return
				}
			}
		}
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
// POST /plugins/:pluginId/jobs/:jobId/trigger
// --------------------------------------------------------------------------

func TriggerPluginJobHandler(db *gorm.DB, scheduler *services.PluginJobScheduler) http.HandlerFunc {
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

		runID, err := scheduler.TriggerJob(r.Context(), jobID, "manual")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"jobId": jobID,
			"runId": runID,
		})
	}
}

// --------------------------------------------------------------------------
// POST /plugins/:pluginId/webhooks/:endpointKey
// --------------------------------------------------------------------------

func WebhookIngestionHandler(db *gorm.DB, wm *services.PluginWorkerManager) http.HandlerFunc {
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
		var parsedBody interface{}
		if json.Valid(rawBody) {
			payloadJSON = rawBody
			_ = json.Unmarshal(rawBody, &parsedBody)
		} else {
			wrapped, _ := json.Marshal(map[string]string{"raw": string(rawBody)})
			payloadJSON = wrapped
			parsedBody = map[string]string{"raw": string(rawBody)}
		}

		startedAt := time.Now()
		deliveryID, err := registry.RecordWebhookDelivery(r.Context(), plugin.ID, endpointKey, payloadJSON, headersJSON)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to record delivery"})
			return
		}

		// Dispatch to the worker via handleWebhook RPC.
		var workerErr error
		if wm != nil && wm.IsRunning(plugin.ID) {
			params := map[string]interface{}{
				"endpointKey": endpointKey,
				"headers":     rawHeaders,
				"rawBody":     string(rawBody),
				"parsedBody":  parsedBody,
				"requestId":   deliveryID, // Use deliveryID as requestId
			}
			_, workerErr = wm.Call(r.Context(), plugin.ID, "handleWebhook", params, 30*time.Second)
		} else {
			workerErr = fmt.Errorf("plugin worker not available")
		}

		durationMs := int(time.Since(startedAt).Milliseconds())
		status := "success"
		var errMsg *string
		if workerErr != nil {
			status = "failed"
			s := workerErr.Error()
			errMsg = &s
		}

		_ = registry.UpdateWebhookDelivery(r.Context(), deliveryID, status, durationMs, errMsg)

		if workerErr != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{
				"deliveryId": deliveryID,
				"status":     status,
				"error":      *errMsg,
			})
		} else {
			writeJSON(w, http.StatusOK, map[string]string{
				"deliveryId": deliveryID,
				"status":     status,
			})
		}
	}
}

// --------------------------------------------------------------------------
// GET /plugins/:pluginId/dashboard
// --------------------------------------------------------------------------

func GetPluginDashboardHandler(db *gorm.DB, wm *services.PluginWorkerManager) http.HandlerFunc {
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

		var workerInfo interface{}
		if wm != nil {
			if handle := wm.GetWorker(plugin.ID); handle != nil {
				workerInfo = handle.Diagnostics()
			}
		}

		type dashboardResponse struct {
			PluginID                string                         `json:"pluginId"`
			Worker                  interface{}                    `json:"worker"`
			RecentJobRuns           []models.PluginJobRun          `json:"recentJobRuns"`
			RecentWebhookDeliveries []models.PluginWebhookDelivery `json:"recentWebhookDeliveries"`
			Health                  pluginHealthCheck              `json:"health"`
			CheckedAt               string                         `json:"checkedAt"`
		}

		writeJSON(w, http.StatusOK, dashboardResponse{
			PluginID:                plugin.ID,
			Worker:                  workerInfo,
			RecentJobRuns:           jobRuns,
			RecentWebhookDeliveries: deliveries,
			Health:                  health,
			CheckedAt:               time.Now().UTC().Format(time.RFC3339),
		})
	}
}

// --------------------------------------------------------------------------
// Bridge helpers (shared by the bridge/data, bridge/action, data/:key, actions/:key)
// --------------------------------------------------------------------------

// resolvePluginForBridge resolves a plugin and validates it is ready for bridge calls.
// Returns (plugin, errorBody, statusCode) — if errorBody is non-nil the caller should
// write it and return.
func resolvePluginForBridge(
	registry *services.PluginRegistryService,
	r *http.Request,
	pluginID string,
) (*models.Plugin, interface{}, int) {
	plugin, err := registry.Resolve(r.Context(), pluginID)
	if err != nil {
		return nil, map[string]string{"error": err.Error()}, http.StatusInternalServerError
	}
	if plugin == nil {
		return nil, map[string]string{"error": "Plugin not found"}, http.StatusNotFound
	}
	if plugin.Status != "ready" {
		return nil, pluginBridgeError{
			Code:    "WORKER_UNAVAILABLE",
			Message: fmt.Sprintf("Plugin is not ready (current status: %s)", plugin.Status),
		}, http.StatusBadGateway
	}
	return plugin, nil, 0
}

// proxyBridgeCall calls the given RPC method on the plugin worker and writes the
// result as { data: <result> } or a bridge-error envelope on failure.
func proxyBridgeCall(w http.ResponseWriter, ctx context.Context, wm *services.PluginWorkerManager, pluginID, method string, params map[string]interface{}) {
	raw, err := wm.Call(ctx, pluginID, method, params, 30*time.Second)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, pluginBridgeError{
			Code:    "WORKER_UNAVAILABLE",
			Message: err.Error(),
		})
		return
	}
	// Wrap result in { data: ... }
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, `{"data":%s}`, string(raw))
}

// orEmptyMap returns m if non-nil, otherwise an empty map.
func orEmptyMap(m map[string]interface{}) map[string]interface{} {
	if m != nil {
		return m
	}
	return map[string]interface{}{}
}
