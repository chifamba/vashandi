package routes

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

type ContextOperationDescriptor struct {
	Name     string `json:"name"`
	Method   string `json:"method"`
	Endpoint string `json:"endpoint"`
	Status   string `json:"status"`
}

func listContextOperations(companyID string) []ContextOperationDescriptor {
	return []ContextOperationDescriptor{
		{
			Name:     "hydrate",
			Method:   http.MethodPost,
			Endpoint: "/api/v1/companies/" + companyID + "/context/hydrate",
			Status:   "available",
		},
		{
			Name:     "capture",
			Method:   http.MethodPost,
			Endpoint: "/api/v1/companies/" + companyID + "/context/capture",
			Status:   "available",
		},
	}
}

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

func ListContextOperationsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = db
		companyID := chi.URLParam(r, "companyId")
		if companyID == "" {
			http.Error(w, "companyId is required", http.StatusBadRequest)
			return
		}
		if err := AssertCompanyAccess(r, companyID); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, listContextOperations(companyID))
	}
}

func GetContextOperationHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = db
		companyID := chi.URLParam(r, "companyId")
		if companyID == "" {
			http.Error(w, "companyId is required", http.StatusBadRequest)
			return
		}
		if err := AssertCompanyAccess(r, companyID); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}

		operationName := chi.URLParam(r, "operation")
		for _, descriptor := range listContextOperations(companyID) {
			if descriptor.Name == operationName {
				writeJSON(w, http.StatusOK, descriptor)
				return
			}
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "context operation not found"})
	}
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
