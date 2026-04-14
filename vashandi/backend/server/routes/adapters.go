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
