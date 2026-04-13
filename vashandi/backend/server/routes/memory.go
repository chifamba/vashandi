package routes

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
	"github.com/chifamba/vashandi/vashandi/backend/server/services"
)

// MemoryBindingsHandler returns memory bindings for a company
func MemoryBindingsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Mock implementation for parity
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]interface{}{})
	}
}

// MemoryOperationsHandler returns memory operations for a company
func MemoryOperationsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Mock implementation for parity
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]interface{}{})
	}
}

// ExportAuditHandler exports the audit log from OpenBrain
func ExportAuditHandler(db *gorm.DB) http.HandlerFunc {
	adapter := services.NewOpenBrainAdapter()
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")
		format := r.URL.Query().Get("format")
		if format == "" {
			format = "jsonld"
		}

		data, contentType, err := adapter.ExportAudit(r.Context(), companyID, format)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", contentType)
		w.Write(data)
	}
}
