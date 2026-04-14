package routes

import (
"fmt"
"net/http"

"github.com/go-chi/chi/v5"
)

func ListAgentConfigurationHandler() http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
w.Header().Set("Content-Type", "text/plain")
configs := []string{
"claude: Anthropic Claude (claude-3, claude-4)",
"codex: OpenAI Codex/GPT-4 (gpt-4o, gpt-4-turbo)",
"gemini: Google Gemini (gemini-2.0-flash, gemini-1.5-pro)",
"cursor: Cursor AI Editor",
"windsurf: Windsurf AI Editor",
"aider: Aider CLI",
}
for _, c := range configs {
fmt.Fprintln(w, c)
}
}
}

func ListAgentIconsHandler() http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
w.Header().Set("Content-Type", "text/plain")
icons := []string{
"claude", "gpt", "gemini", "cursor", "windsurf", "aider",
"robot", "brain", "chip", "star", "bolt",
}
for _, i := range icons {
fmt.Fprintln(w, i)
}
}
}

func GetAdapterConfigurationHandler() http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
adapterType := chi.URLParam(r, "adapterType")
w.Header().Set("Content-Type", "text/plain")
descriptions := map[string]string{
"claude":    "Anthropic Claude: supports streaming, tool use, large context windows.",
"codex":     "OpenAI Codex/GPT-4o: supports function calling, streaming, vision.",
"gemini":    "Google Gemini: supports streaming, function calling, multimodal.",
"cursor":    "Cursor: AI code editor with diff-based workflow.",
"windsurf":  "Windsurf: AI code editor with cascade agents.",
"aider":     "Aider: CLI-based AI pair programming tool.",
}
if desc, ok := descriptions[adapterType]; ok {
fmt.Fprintln(w, desc)
} else {
http.Error(w, "Unknown adapter type: "+adapterType, http.StatusNotFound)
}
}
}
