package routes

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/chifamba/paperclip/backend/server/services"
)

type HydrateRequest struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

type CaptureRequest struct {
	Text     string            `json:"text"`
	Metadata map[string]string `json:"metadata"`
}

func PreRunHydrationHandler(db *gorm.DB) http.HandlerFunc {
	adapter := services.NewOpenBrainAdapter()
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")

		var req HydrateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if req.Limit == 0 {
			req.Limit = 5
		}

		results, err := adapter.QueryMemory(r.Context(), companyID, req.Query, req.Limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	}
}

func PostRunCaptureHandler(db *gorm.DB) http.HandlerFunc {
	adapter := services.NewOpenBrainAdapter()
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")

		var req CaptureRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		err := adapter.IngestMemory(r.Context(), companyID, req.Text, req.Metadata)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
	}
}
