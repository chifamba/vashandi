package routes
import (
	"encoding/json"
	"net/http"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)
func RegisterContextRoutes(r chi.Router) {
	r.Post("/triggers/run_start", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "run_start_forwarded"})
	})
	r.Post("/triggers/run_complete", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "run_complete_forwarded"})
	})
	r.Post("/triggers/checkout", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "checkout_forwarded"})
	})
}
func PreRunHydrationHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "run_start_forwarded"})
	}
}
func PostRunCaptureHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "run_complete_forwarded"})
	}
}
