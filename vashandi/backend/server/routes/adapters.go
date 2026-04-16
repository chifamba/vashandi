package routes

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"github.com/chifamba/vashandi/vashandi/backend/server/services"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// ListAdaptersHandler returns all available adapters: built-in types, DB-backed
// plugin adapters, and user-installed external adapters from the AdapterPluginStore.
// store may be nil, in which case external adapters are silently omitted.
func ListAdaptersHandler(db *gorm.DB, store *services.AdapterPluginStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		builtin := []map[string]string{
			{"type": "claude", "name": "Claude (Anthropic)"},
			{"type": "claude_local", "name": "Claude (local CLI)"},
			{"type": "codex", "name": "Codex (OpenAI)"},
			{"type": "codex_local", "name": "Codex (local CLI)"},
			{"type": "gemini", "name": "Gemini (Google)"},
			{"type": "cursor", "name": "Cursor"},
			{"type": "cursor_local", "name": "Cursor (local CLI)"},
			{"type": "windsurf", "name": "Windsurf"},
			{"type": "aider", "name": "Aider"},
			{"type": "opencode_local", "name": "OpenCode (local)"},
			{"type": "pi_local", "name": "PI (local CLI)"},
			{"type": "openclaw_gateway", "name": "OpenClaw Gateway"},
		}

		var plugins []models.Plugin
		db.WithContext(r.Context()).Where("status = ?", "installed").Find(&plugins)
		pluginAdapters := make([]map[string]string, 0, len(plugins))
		for _, p := range plugins {
			pluginAdapters = append(pluginAdapters, map[string]string{
				"type": "plugin:" + p.PluginKey,
				"name": p.PackageName,
			})
		}

		// Append user-installed external adapters from the on-disk registry.
		if store != nil {
			entries, err := store.List()
			if err != nil {
				slog.Warn("adapter plugin store: List failed", "error", err)
			} else {
				for _, e := range entries {
					if !e.Disabled {
						pluginAdapters = append(pluginAdapters, map[string]string{
							"type": e.Type,
							"name": e.PackageName,
						})
					}
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
			"builtin": builtin,
			"plugins": pluginAdapters,
		})
	}
}

func PauseAdapterHandler() http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
_ = chi.URLParam(r, "adapterType")
w.WriteHeader(http.StatusOK)
}
}

func UpdateAdapterHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
adapterType := chi.URLParam(r, "type")
var body map[string]interface{}
if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
http.Error(w, err.Error(), http.StatusBadRequest)
return
}
var plugin models.Plugin
if err := db.WithContext(r.Context()).Where("plugin_key = ?", adapterType).First(&plugin).Error; err != nil {
http.Error(w, "Not found", http.StatusNotFound)
return
}
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
}
}

func DeleteAdapterHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
adapterType := chi.URLParam(r, "type")
db.WithContext(r.Context()).Model(&models.Plugin{}).
Where("plugin_key = ?", adapterType).
Update("status", "uninstalled")
w.WriteHeader(http.StatusNoContent)
}
}

// InstallAdapterHandler handles POST /adapters/install
func InstallAdapterHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok", "body": body})
	}
}

// OverrideAdapterHandler handles PATCH /adapters/:type/override
func OverrideAdapterHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adapterType := chi.URLParam(r, "type")
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"adapterType": adapterType, "status": "overridden"})
	}
}

// ReloadAdapterHandler handles POST /adapters/:type/reload
func ReloadAdapterHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adapterType := chi.URLParam(r, "type")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"adapterType": adapterType, "status": "reloaded"})
	}
}

// ReinstallAdapterHandler handles POST /adapters/:type/reinstall
func ReinstallAdapterHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adapterType := chi.URLParam(r, "type")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"adapterType": adapterType, "status": "reinstalled"})
	}
}

// GetAdapterConfigSchemaHandler handles GET /adapters/:type/config-schema
func GetAdapterConfigSchemaHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adapterType := chi.URLParam(r, "type")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"adapterType": adapterType,
			"schema":      map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		})
	}
}

// GetAdapterUIParserHandler handles GET /adapters/:type/ui-parser.js
//
// Serves the self-contained UI parser JS bundle for an external adapter type.
// External adapter packages may export a "./ui-parser" entry in their
// package.json pointing to a zero-dependency ESM module. The UI fetches this
// script on demand when rendering run transcripts for adapter types that have
// no built-in parser.
//
// Returns 404 when the adapter has no package_path or no "./ui-parser" export.
// Validates that the resolved file path stays within the package directory to
// prevent path traversal attacks.
func GetAdapterUIParserHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adapterType := chi.URLParam(r, "type")

		// External adapter types are stored with plugin_key equal to the bare
		// type name; the "plugin:" prefix is a UI-side convention.
		pluginKey := strings.TrimPrefix(adapterType, "plugin:")

		var plugin models.Plugin
		if err := db.WithContext(r.Context()).
			Where("plugin_key = ? AND status != ?", pluginKey, "uninstalled").
			First(&plugin).Error; err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"No UI parser available for adapter %q"}`, adapterType), http.StatusNotFound)
			return
		}

		if plugin.PackagePath == nil || strings.TrimSpace(*plugin.PackagePath) == "" {
			http.Error(w, fmt.Sprintf(`{"error":"No UI parser available for adapter %q"}`, adapterType), http.StatusNotFound)
			return
		}

		source, err := extractAdapterUIParserSource(*plugin.PackagePath)
		if err != nil || source == "" {
			http.Error(w, fmt.Sprintf(`{"error":"No UI parser available for adapter %q"}`, adapterType), http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(source))
	}
}

// supportedAdapterUiParserContract is the only major version the server accepts.
const supportedAdapterUiParserContract = "1"

// extractAdapterUIParserSource reads the adapter package.json at packagePath,
// resolves the "./ui-parser" export, validates the contract version, and returns
// the JS source. Returns ("", nil) when no ui-parser export is declared.
func extractAdapterUIParserSource(packagePath string) (string, error) {
	packageDir := filepath.Clean(packagePath)

	pkgJSONPath := filepath.Join(packageDir, "package.json")
	data, err := os.ReadFile(pkgJSONPath)
	if err != nil {
		return "", err
	}

	var pkg map[string]interface{}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return "", err
	}

	exports, ok := pkg["exports"].(map[string]interface{})
	if !ok {
		return "", nil
	}
	uiParserExp, ok := exports["./ui-parser"]
	if !ok {
		return "", nil
	}

	// Validate contract version when declared; warn but proceed when absent.
	if paperclip, ok := pkg["paperclip"].(map[string]interface{}); ok {
		if contractVersion, ok := paperclip["adapterUiParser"].(string); ok {
			major := strings.SplitN(contractVersion, ".", 2)[0]
			if major != supportedAdapterUiParserContract {
				return "", fmt.Errorf("adapter declares unsupported UI parser contract version %q (supported: %s.x)", contractVersion, supportedAdapterUiParserContract)
			}
		}
	}

	// Resolve the concrete file path from the export entry.
	var uiParserFile string
	switch v := uiParserExp.(type) {
	case string:
		uiParserFile = v
	case map[string]interface{}:
		if imp, ok := v["import"].(string); ok {
			uiParserFile = imp
		} else if def, ok := v["default"].(string); ok {
			uiParserFile = def
		}
	}
	if uiParserFile == "" {
		return "", nil
	}

	// Security: ensure the resolved path stays within the package directory.
	resolvedPath := filepath.Clean(filepath.Join(packageDir, uiParserFile))
	rel, err := filepath.Rel(packageDir, resolvedPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("ui-parser path escapes package directory")
	}

	// Resolve symlinks and re-check containment.
	realPackageDir, err := filepath.EvalSymlinks(packageDir)
	if err != nil {
		return "", err
	}
	realFilePath, err := filepath.EvalSymlinks(resolvedPath)
	if err != nil {
		return "", err
	}
	rel2, err := filepath.Rel(realPackageDir, realFilePath)
	if err != nil || rel2 == ".." || strings.HasPrefix(rel2, ".."+string(filepath.Separator)) || filepath.IsAbs(rel2) {
		return "", fmt.Errorf("ui-parser symlink escapes package directory")
	}

	content, err := os.ReadFile(realFilePath)
	if err != nil {
		return "", err
	}
	return string(content), nil
}
