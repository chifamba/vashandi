package routes

import (
"net/http"
"os"
"path/filepath"
"strings"
"time"

"github.com/chifamba/vashandi/vashandi/backend/db/models"
"github.com/go-chi/chi/v5"
"gorm.io/gorm"
)

func PluginUIStaticHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
pluginID := chi.URLParam(r, "pluginId")
filePath := chi.URLParam(r, "*")

if strings.Contains(filePath, "..") {
http.Error(w, "Forbidden", http.StatusForbidden)
return
}

var plugin models.Plugin
if err := db.WithContext(r.Context()).First(&plugin, "id = ? OR plugin_key = ?", pluginID, pluginID).Error; err != nil {
http.Error(w, "Plugin not found", http.StatusNotFound)
return
}
if plugin.PackagePath == nil {
http.Error(w, "Plugin has no UI package path", http.StatusNotFound)
return
}

baseDir := filepath.Join(*plugin.PackagePath, "ui", "dist")
cleaned := filepath.Join(baseDir, filepath.Clean("/"+filePath))
rel, err := filepath.Rel(baseDir, cleaned)
if err != nil || rel == ".." || len(rel) >= 2 && rel[:3] == "../" {
http.Error(w, "Forbidden", http.StatusForbidden)
return
}
fullPath := cleaned
f, err := os.Open(fullPath)
if err != nil {
http.Error(w, "File not found", http.StatusNotFound)
return
}
defer f.Close()

info, err := f.Stat()
if err != nil {
http.Error(w, "Internal error", http.StatusInternalServerError)
return
}

etag := info.ModTime().Format(time.RFC3339Nano)
w.Header().Set("ETag", `"`+etag+`"`)

ext := strings.ToLower(filepath.Ext(filePath))
contentType := "application/octet-stream"
switch ext {
case ".html":
contentType = "text/html"
case ".js":
contentType = "application/javascript"
case ".css":
contentType = "text/css"
case ".png":
contentType = "image/png"
case ".svg":
contentType = "image/svg+xml"
case ".json":
contentType = "application/json"
}
w.Header().Set("Content-Type", contentType)
http.ServeContent(w, r, filePath, info.ModTime(), f)
}
}
