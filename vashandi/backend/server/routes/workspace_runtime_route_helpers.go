package routes

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"gorm.io/gorm"
)

// updateMetadataDesiredState sets metadata.config.desiredState on an execution
// workspace. Errors are silently swallowed because this is a best-effort update.
func updateMetadataDesiredState(db *gorm.DB, r *http.Request, workspaceID, desiredState string) {
	var ws models.ExecutionWorkspace
	if err := db.WithContext(r.Context()).Select("id, metadata").First(&ws, "id = ?", workspaceID).Error; err != nil {
		return
	}

	// Parse the existing metadata.
	meta := map[string]interface{}{}
	if ws.Metadata != nil {
		_ = json.Unmarshal(ws.Metadata, &meta)
	}

	// Merge desiredState under metadata.config.
	cfg, _ := meta["config"].(map[string]interface{})
	if cfg == nil {
		cfg = map[string]interface{}{}
	}
	cfg["desiredState"] = desiredState
	meta["config"] = cfg

	b, _ := json.Marshal(meta)
	db.WithContext(r.Context()).Model(&models.ExecutionWorkspace{}).
		Where("id = ?", workspaceID).
		Updates(map[string]interface{}{
			"metadata":   string(b),
			"updated_at": time.Now(),
		})
}

// updateProjectWorkspaceDesiredState sets metadata.runtimeConfig.desiredState on
// a project workspace. Errors are silently swallowed.
func updateProjectWorkspaceDesiredState(db *gorm.DB, r *http.Request, workspaceID, desiredState string) {
	var pw models.ProjectWorkspace
	if err := db.WithContext(r.Context()).Select("id, metadata").First(&pw, "id = ?", workspaceID).Error; err != nil {
		return
	}

	meta := map[string]interface{}{}
	if pw.Metadata != nil {
		_ = json.Unmarshal(pw.Metadata, &meta)
	}

	rc, _ := meta["runtimeConfig"].(map[string]interface{})
	if rc == nil {
		rc = map[string]interface{}{}
	}
	rc["desiredState"] = desiredState
	meta["runtimeConfig"] = rc

	b, _ := json.Marshal(meta)
	db.WithContext(r.Context()).Model(&models.ProjectWorkspace{}).
		Where("id = ?", workspaceID).
		Updates(map[string]interface{}{
			"metadata":   string(b),
			"updated_at": time.Now(),
		})
}

// logActivityEntry persists an activity log entry; errors are silently ignored.
func logActivityEntry(db *gorm.DB, r *http.Request, entry models.ActivityLog) {
	entry.CreatedAt = time.Now()
	_ = db.WithContext(r.Context()).Create(&entry).Error
}
