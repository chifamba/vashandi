package routes

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/chifamba/paperclip/backend/db/models"
)

// MemoryBindingsHandler serves the memory bindings for a company
func MemoryBindingsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		companyID := chi.URLParam(r, "companyId")

		if companyID == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "companyId is required"})
			return
		}

		var bindings []models.MemoryBinding
		db.Where("company_id = ?", companyID).Find(&bindings)

		json.NewEncoder(w).Encode(bindings)
	}
}

// MemoryOperationsHandler serves the memory operations for a company
func MemoryOperationsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		companyID := chi.URLParam(r, "companyId")

		if companyID == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "companyId is required"})
			return
		}

		var operations []models.MemoryOperation
		db.Where("company_id = ?", companyID).Find(&operations)

		json.NewEncoder(w).Encode(operations)
	}
}
