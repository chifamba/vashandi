package routes

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/chifamba/paperclip/backend/server/services"
)

type ChatRequest struct {
	Message string `json:"message"`
}

func IngestChatHandler(db *gorm.DB) http.HandlerFunc {
	adapter := services.NewOpenBrainAdapter()
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")

		var req ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		lowerMsg := strings.ToLower(req.Message)
		// Basic NLP keyword matching for strategy context
		if strings.Contains(lowerMsg, "strategy") || strings.Contains(lowerMsg, "goal") || strings.Contains(lowerMsg, "priority") || strings.Contains(lowerMsg, "vision") {
			metadata := map[string]string{
				"source": "ceo_chat",
				"type":   "strategy",
			}

			err := adapter.IngestMemory(r.Context(), companyID, req.Message, metadata)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{"status": "ingested_as_strategy"})
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ignored_low_value"})
	}
}
