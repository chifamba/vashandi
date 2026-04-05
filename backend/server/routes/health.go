package routes

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/chifamba/paperclip/backend/db/models"
	"gorm.io/gorm"
)

type HealthResponse struct {
	Status                string                 `json:"status"`
	Version               string                 `json:"version"`
	DeploymentMode        string                 `json:"deploymentMode,omitempty"`
	DeploymentExposure    string                 `json:"deploymentExposure,omitempty"`
	AuthReady             bool                   `json:"authReady,omitempty"`
	BootstrapStatus       string                 `json:"bootstrapStatus,omitempty"`
	BootstrapInviteActive bool                   `json:"bootstrapInviteActive,omitempty"`
	Features              map[string]interface{} `json:"features,omitempty"`
	Error                 string                 `json:"error,omitempty"`
}

type HealthOpts struct {
	DeploymentMode         string
	DeploymentExposure     string
	AuthReady              bool
	CompanyDeletionEnabled bool
}

var serverVersion = "0.0.0-dev"

func init() {
	// Attempt to read version from environment if provided during build/runtime
	if v := os.Getenv("PAPERCLIP_VERSION"); v != "" {
		serverVersion = v
	}
}

// HealthHandler returns an http.HandlerFunc that performs health checks
func HealthHandler(db *gorm.DB, opts HealthOpts) http.HandlerFunc {
	// Defaults if not provided
	if opts.DeploymentMode == "" {
		opts.DeploymentMode = "local_trusted"
	}
	if opts.DeploymentExposure == "" {
		opts.DeploymentExposure = "private"
	}
	if !opts.AuthReady {
		opts.AuthReady = true // Setting default true as per TS
	}

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if db == nil {
			json.NewEncoder(w).Encode(HealthResponse{
				Status:  "ok",
				Version: serverVersion,
			})
			return
		}

		// Check database connection
		var result int
		if err := db.Raw("SELECT 1").Scan(&result).Error; err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(HealthResponse{
				Status:  "unhealthy",
				Version: serverVersion,
				Error:   "database_unreachable",
			})
			return
		}

		bootstrapStatus := "ready"
		bootstrapInviteActive := false

		if opts.DeploymentMode == "authenticated" {
			var roleCount int64
			db.Model(&models.InstanceUserRole{}).Where("role = ?", "instance_admin").Count(&roleCount)

			if roleCount > 0 {
				bootstrapStatus = "ready"
			} else {
				bootstrapStatus = "bootstrap_pending"

				var inviteCount int64
				db.Model(&models.Invite{}).
					Where("invite_type = ?", "bootstrap_ceo").
					Where("revoked_at IS NULL").
					Where("accepted_at IS NULL").
					Where("expires_at > NOW()").
					Count(&inviteCount)

				bootstrapInviteActive = inviteCount > 0
			}
		}

		json.NewEncoder(w).Encode(HealthResponse{
			Status:                "ok",
			Version:               serverVersion,
			DeploymentMode:        opts.DeploymentMode,
			DeploymentExposure:    opts.DeploymentExposure,
			AuthReady:             opts.AuthReady,
			BootstrapStatus:       bootstrapStatus,
			BootstrapInviteActive: bootstrapInviteActive,
			Features: map[string]interface{}{
				"companyDeletionEnabled": opts.CompanyDeletionEnabled,
			},
		})
	}
}
