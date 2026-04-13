package routes

import (
	"encoding/json"
	"net/http"

	"github.com/chifamba/vashandi/vashandi/backend/server/services"
	"gorm.io/gorm"
)

// ListPluginsHandler returns a list of installed plugins
func ListPluginsHandler(db *gorm.DB) http.HandlerFunc {
	service := services.NewPluginService(db)
	return func(w http.ResponseWriter, r *http.Request) {
		plugins, err := service.ListPlugins(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(plugins)
	}
}
