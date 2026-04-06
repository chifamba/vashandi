package routes

import (
	"net/http"
	"fmt"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// GetLLMConfigTxtHandler returns generic llm agent config in txt
func GetLLMConfigTxtHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Agent configuration instructions for LLMs."))
	}
}

// GetLLMIconsTxtHandler returns agent icons config in txt
func GetLLMIconsTxtHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Agent icons instructions for LLMs."))
	}
}

// GetLLMAdapterConfigTxtHandler returns specific adapter agent config in txt
func GetLLMAdapterConfigTxtHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adapterType := chi.URLParam(r, "adapterType")
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf("Agent configuration instructions for adapter type %s.", adapterType)))
	}
}
