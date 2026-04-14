package routes

import (
"encoding/json"
"net/http"

"github.com/chifamba/vashandi/vashandi/backend/db/models"
"github.com/go-chi/chi/v5"
"gorm.io/gorm"
)

func ListAdaptersHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
builtin := []map[string]string{
{"type": "claude", "name": "Claude (Anthropic)"},
{"type": "codex", "name": "Codex (OpenAI)"},
{"type": "gemini", "name": "Gemini (Google)"},
{"type": "cursor", "name": "Cursor"},
{"type": "windsurf", "name": "Windsurf"},
{"type": "aider", "name": "Aider"},
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

w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(map[string]interface{}{
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
