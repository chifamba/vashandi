package routes

import (
	"encoding/json"
	"net/http"

	"gorm.io/gorm"
)

type HealthResponse struct {
	Status             string                 `json:"status"`
	Version            string                 `json:"version"`
	DeploymentMode     string                 `json:"deploymentMode,omitempty"`
	DeploymentExposure string                 `json:"deploymentExposure,omitempty"`
	AuthReady          bool                   `json:"authReady,omitempty"`
	BootstrapStatus    string                 `json:"bootstrapStatus,omitempty"`
	Features           map[string]interface{} `json:"features,omitempty"`
	Error              string                 `json:"error,omitempty"`
}

// HealthHandler returns an http.HandlerFunc that performs health checks
func HealthHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if db == nil {
			json.NewEncoder(w).Encode(HealthResponse{
				Status:  "ok",
				Version: "0.0.0-dev", // Hardcoded for now until versioning is ported
			})
			return
		}

		// Check database connection
		var result int
		if err := db.Raw("SELECT 1").Scan(&result).Error; err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(HealthResponse{
				Status:  "unhealthy",
				Version: "0.0.0-dev",
				Error:   "database_unreachable",
			})
			return
		}

		// Assuming default authenticated mode for response structure based on TS default args equivalent check
		json.NewEncoder(w).Encode(HealthResponse{
			Status:             "ok",
			Version:            "0.0.0-dev",
			DeploymentMode:     "local_trusted",
			DeploymentExposure: "private",
			AuthReady:          true,
			BootstrapStatus:    "ready", // Stubbing bootstrap logic for now as instance roles are needed
			Features: map[string]interface{}{
				"companyDeletionEnabled": true,
			},
		})
	}
}
