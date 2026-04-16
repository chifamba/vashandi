package routes

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"gorm.io/gorm"
)

const (
	instanceAdminRole = "instance_admin"
	bootstrapCEOType  = "bootstrap_ceo"
)

type HealthResponse struct {
	Status             string                 `json:"status"`
	Version            string                 `json:"version"`
	DeploymentMode     string                 `json:"deploymentMode,omitempty"`
	DeploymentExposure string                 `json:"deploymentExposure,omitempty"`
	AuthReady          bool                   `json:"authReady,omitempty"`
	BootstrapStatus    string                 `json:"bootstrapStatus,omitempty"`
	BootstrapInvite    bool                   `json:"bootstrapInviteActive"`
	Features           map[string]interface{} `json:"features,omitempty"`
	Error              string                 `json:"error,omitempty"`
}

type HealthHandlerOptions struct {
	DeploymentMode         string
	DeploymentExposure     string
	AuthReady              bool
	CompanyDeletionEnabled bool
}

// HealthHandler returns an http.HandlerFunc that performs health checks
func HealthHandler(db *gorm.DB, options ...HealthHandlerOptions) http.HandlerFunc {
	opts := HealthHandlerOptions{
		DeploymentMode:         "local_trusted",
		DeploymentExposure:     "private",
		AuthReady:              true,
		CompanyDeletionEnabled: true,
	}
	if len(options) > 0 {
		opts = options[0]
	}

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

		bootstrapStatus := "ready"
		bootstrapInviteActive := false
		if opts.DeploymentMode == "authenticated" {
			var adminCount int64
			if err := db.Model(&models.InstanceUserRole{}).
				Where("role = ?", instanceAdminRole).
				Count(&adminCount).Error; err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				json.NewEncoder(w).Encode(HealthResponse{
					Status:  "unhealthy",
					Version: "0.0.0-dev",
					Error:   "database_query_failed",
				})
				return
			}
			if adminCount == 0 {
				bootstrapStatus = "bootstrap_pending"
				var inviteCount int64
				now := time.Now()
				if err := db.Model(&models.Invite{}).
					Where("invite_type = ?", bootstrapCEOType).
					Where("revoked_at IS NULL").
					Where("accepted_at IS NULL").
					Where("expires_at > ?", now).
					Count(&inviteCount).Error; err != nil {
					w.WriteHeader(http.StatusServiceUnavailable)
					json.NewEncoder(w).Encode(HealthResponse{
						Status:  "unhealthy",
						Version: "0.0.0-dev",
						Error:   "database_query_failed",
					})
					return
				}
				bootstrapInviteActive = inviteCount > 0
			}
		}

		json.NewEncoder(w).Encode(HealthResponse{
			Status:             "ok",
			Version:            "0.0.0-dev",
			DeploymentMode:     opts.DeploymentMode,
			DeploymentExposure: opts.DeploymentExposure,
			AuthReady:          opts.AuthReady,
			BootstrapStatus:    bootstrapStatus,
			BootstrapInvite:    bootstrapInviteActive,
			Features: map[string]interface{}{
				"companyDeletionEnabled": opts.CompanyDeletionEnabled,
			},
		})
	}
}
