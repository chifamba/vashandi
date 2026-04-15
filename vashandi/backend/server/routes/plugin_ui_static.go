package routes

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

var contentHashedPluginAssetPattern = regexp.MustCompile(`[.-][a-fA-F0-9]{8,}\.\w+$`)

var pluginAssetContentTypes = map[string]string{
	".css":   "text/css; charset=utf-8",
	".eot":   "application/vnd.ms-fontobject",
	".gif":   "image/gif",
	".html":  "text/html; charset=utf-8",
	".ico":   "image/x-icon",
	".jpeg":  "image/jpeg",
	".jpg":   "image/jpeg",
	".js":    "application/javascript; charset=utf-8",
	".json":  "application/json; charset=utf-8",
	".map":   "application/json; charset=utf-8",
	".mjs":   "application/javascript; charset=utf-8",
	".png":   "image/png",
	".svg":   "image/svg+xml",
	".txt":   "text/plain; charset=utf-8",
	".ttf":   "font/ttf",
	".webp":  "image/webp",
	".woff":  "font/woff",
	".woff2": "font/woff2",
}

func PluginUIStaticHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pluginID := chi.URLParam(r, "pluginId")
		filePath := chi.URLParam(r, "*")
		if strings.TrimSpace(filePath) == "" {
			http.Error(w, "File path is required", http.StatusBadRequest)
			return
		}
		if strings.Contains(filePath, "..") {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		var plugin models.Plugin
		if err := db.WithContext(r.Context()).First(&plugin, "id = ? OR plugin_key = ?", pluginID, pluginID).Error; err != nil {
			http.Error(w, "Plugin not found", http.StatusNotFound)
			return
		}
		if plugin.Status != "ready" {
			http.Error(w, "Plugin UI is not available", http.StatusForbidden)
			return
		}
		if plugin.PackagePath == nil || strings.TrimSpace(*plugin.PackagePath) == "" {
			http.Error(w, "Plugin has no UI package path", http.StatusNotFound)
			return
		}

		uiEntrypoint, ok := pluginUIEntrypoint(plugin.ManifestJSON)
		if !ok {
			http.Error(w, "Plugin does not declare a UI bundle", http.StatusNotFound)
			return
		}

		uiDir, err := resolvePluginUIDir(*plugin.PackagePath, uiEntrypoint)
		if err != nil {
			http.Error(w, "Plugin UI directory not found", http.StatusNotFound)
			return
		}

		requestedPath := filepath.Join(uiDir, filepath.Clean(string(filepath.Separator)+filePath))
		realUIDir, err := filepath.EvalSymlinks(uiDir)
		if err != nil {
			http.Error(w, "Plugin UI directory not found", http.StatusNotFound)
			return
		}
		realFilePath, err := filepath.EvalSymlinks(requestedPath)
		if err != nil {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}

		rel, err := filepath.Rel(realUIDir, realFilePath)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		info, err := os.Stat(realFilePath)
		if err != nil || !info.Mode().IsRegular() {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}

		if contentHashedPluginAssetPattern.MatchString(filepath.Base(realFilePath)) {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=0, must-revalidate")
			etag := computePluginAssetETag(info.Size(), info.ModTime().UnixMilli())
			w.Header().Set("ETag", etag)
			if r.Header.Get("If-None-Match") == etag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}

		ext := strings.ToLower(filepath.Ext(realFilePath))
		contentType := pluginAssetContentTypes[ext]
		if contentType == "" {
			contentType = mime.TypeByExtension(ext)
		}
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")

		f, err := os.Open(realFilePath)
		if err != nil {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}
		defer f.Close()

		http.ServeContent(w, r, filepath.Base(realFilePath), info.ModTime(), f)
	}
}

func pluginUIEntrypoint(manifestJSON []byte) (string, bool) {
	if len(manifestJSON) == 0 {
		return "", false
	}

	var manifest map[string]any
	if err := json.Unmarshal(manifestJSON, &manifest); err != nil {
		return "", false
	}

	entrypoints, ok := manifest["entrypoints"].(map[string]any)
	if !ok {
		return "", false
	}
	uiValue, ok := entrypoints["ui"].(string)
	if !ok || strings.TrimSpace(uiValue) == "" {
		return "", false
	}
	return uiValue, true
}

func resolvePluginUIDir(packagePath string, uiEntrypoint string) (string, error) {
	root := filepath.Clean(packagePath)
	resolved := filepath.Clean(filepath.Join(root, filepath.FromSlash(uiEntrypoint)))
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("ui path escapes package root")
	}

	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("ui dir not found")
	}
	return resolved, nil
}

func computePluginAssetETag(size int64, mtimeMs int64) string {
	sum := md5.Sum([]byte(fmt.Sprintf("v2:%d-%d", size, mtimeMs)))
	return fmt.Sprintf(`"%x"`, sum[:8])
}
