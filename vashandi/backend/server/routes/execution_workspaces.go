package routes

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"github.com/chifamba/vashandi/vashandi/backend/server/services"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func ListExecutionWorkspacesHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
companyID := chi.URLParam(r, "companyId")
q := db.WithContext(r.Context()).Where("company_id = ?", companyID)
if projectID := r.URL.Query().Get("projectId"); projectID != "" {
q = q.Where("project_id = ?", projectID)
}
if status := r.URL.Query().Get("status"); status != "" {
q = q.Where("status = ?", status)
}
var workspaces []models.ExecutionWorkspace
q.Order("last_used_at DESC").Find(&workspaces)
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(workspaces)
}
}

func GetExecutionWorkspaceHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
id := chi.URLParam(r, "id")
var ws models.ExecutionWorkspace
if err := db.WithContext(r.Context()).First(&ws, "id = ?", id).Error; err != nil {
http.Error(w, "Not found", http.StatusNotFound)
return
}
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(ws)
}
}

func UpdateExecutionWorkspaceHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
id := chi.URLParam(r, "id")
var ws models.ExecutionWorkspace
if err := db.WithContext(r.Context()).First(&ws, "id = ?", id).Error; err != nil {
http.Error(w, "Not found", http.StatusNotFound)
return
}
if err := json.NewDecoder(r.Body).Decode(&ws); err != nil {
http.Error(w, err.Error(), http.StatusBadRequest)
return
}
db.WithContext(r.Context()).Save(&ws)
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(ws)
}
}

func GetWorkspaceCloseReadinessHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(map[string]interface{}{
"ready":  true,
"reason": nil,
})
}
}

func GetWorkspaceWorkspaceOperationsHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
id := chi.URLParam(r, "id")
var operations []models.WorkspaceOperation
db.WithContext(r.Context()).Where("execution_workspace_id = ?", id).Find(&operations)
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(operations)
}
}

// ExecutionWorkspaceRuntimeServicesHandler handles POST /execution-workspaces/:id/runtime-services/:action
func ExecutionWorkspaceRuntimeServicesHandler(db *gorm.DB, rtMgr *services.WorkspaceRuntimeManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		action := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "action")))

		if action != "start" && action != "stop" && action != "restart" {
			http.Error(w, `{"error":"Runtime service action not found"}`, http.StatusNotFound)
			return
		}

		var ws models.ExecutionWorkspace
		if err := db.WithContext(r.Context()).First(&ws, "id = ?", id).Error; err != nil {
			http.Error(w, `{"error":"Execution workspace not found"}`, http.StatusNotFound)
			return
		}

		if ws.Cwd == nil || *ws.Cwd == "" {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
				"error": "Execution workspace needs a local path before Paperclip can manage local runtime services",
			})
			return
		}
		workspaceCwd := *ws.Cwd

		// Read the runtime config from the execution workspace metadata, with a
		// fallback to the project workspace's runtimeConfig.
		effectiveRuntimeConfig := services.ReadExecutionWorkspaceRuntimeConfig(ws.Metadata)
		if effectiveRuntimeConfig == nil && ws.ProjectWorkspaceID != nil {
			var pw models.ProjectWorkspace
			if err := db.WithContext(r.Context()).First(&pw, "id = ?", *ws.ProjectWorkspaceID).Error; err == nil {
				effectiveRuntimeConfig = services.ReadProjectWorkspaceRuntimeConfig(pw.Metadata)
			}
		}

		if (action == "start" || action == "restart") && effectiveRuntimeConfig == nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
				"error": "Execution workspace has no runtime service configuration or inherited project workspace default",
			})
			return
		}

		actor := GetActorInfo(r)

		// Create a workspace operation to record this action.
		opSvc := services.NewWorkspaceOperationService(db)
		recorder := opSvc.CreateRecorder(ws.CompanyID, nil, &ws.ID)
		phase := "workspace_teardown"
		if action != "stop" {
			phase = "workspace_provision"
		}
		cmdStr := "workspace runtime " + action
		op, opErr := recorder.Begin(r.Context(), phase, &cmdStr)

		var stdout, stderr []string
		var runtimeServiceCount int
		var runErr error

		if action == "stop" || action == "restart" {
			if rtMgr != nil {
				count, err := rtMgr.StopRuntimeServicesForExecutionWorkspace(r.Context(), ws.ID, workspaceCwd)
				if err != nil {
					runErr = err
				} else {
					runtimeServiceCount = count
				}
			} else {
				// Mark any persisted services as stopped when no in-memory manager is available.
				db.WithContext(r.Context()).
					Model(&models.WorkspaceRuntimeService{}).
					Where("execution_workspace_id = ? AND status IN ('starting','running')", ws.ID).
					Updates(map[string]interface{}{
						"status":        "stopped",
						"health_status": "unknown",
						"stopped_at":    time.Now(),
						"last_used_at":  time.Now(),
						"updated_at":    time.Now(),
					})
			}
		}

		if (action == "start" || action == "restart") && runErr == nil {
			if rtMgr != nil {
				onLog := func(stream, chunk string) {
					if stream == "stdout" {
						stdout = append(stdout, chunk)
					} else {
						stderr = append(stderr, chunk)
					}
				}
				var ownerAgentID *string
				if actor.AgentID != "" {
					ownerAgentID = &actor.AgentID
				}
				refs, err := rtMgr.StartRuntimeServices(r.Context(), services.StartRuntimeServicesInput{
					CompanyID:            ws.CompanyID,
					ProjectID:            &ws.ProjectID,
					ProjectWorkspaceID:   ws.ProjectWorkspaceID,
					ExecutionWorkspaceID: &ws.ID,
					WorkspaceCwd:         workspaceCwd,
					RuntimeConfig:        effectiveRuntimeConfig,
					OwnerAgentID:         ownerAgentID,
				}, onLog)
				if err != nil {
					runErr = err
				} else {
					runtimeServiceCount = len(refs)
				}
			}
		}

		// Finish the workspace operation record.
		if op != nil && opErr == nil {
			exitCode := 0
			if runErr != nil {
				exitCode = 1
			}
			_ = recorder.Finish(r.Context(), op.ID, exitCode, runErr)
		}

		if runErr != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": runErr.Error()})
			return
		}

		// Update the workspace metadata with the desired state.
		desiredState := "running"
		if action == "stop" {
			desiredState = "stopped"
		}
		updateMetadataDesiredState(db, r, ws.ID, desiredState)

		// Log activity (best-effort; do not fail the request on log errors).
		actorType := actor.ActorType
		if actorType == "" {
			actorType = "system"
		}
		actorID := actor.UserID
		if actor.IsAgent {
			actorID = actor.AgentID
		}
		var agentIDPtr *string
		if actor.AgentID != "" {
			agentIDPtr = &actor.AgentID
		}
		logActivityEntry(db, r, models.ActivityLog{
			ID:         uuid.New().String(),
			CompanyID:  ws.CompanyID,
			ActorType:  actorType,
			ActorID:    actorID,
			AgentID:    agentIDPtr,
			Action:     "execution_workspace.runtime_" + action,
			EntityType: "execution_workspace",
			EntityID:   ws.ID,
		})

		// Reload the workspace for the response.
		var updatedWS models.ExecutionWorkspace
		if err := db.WithContext(r.Context()).First(&updatedWS, "id = ?", id).Error; err != nil {
			updatedWS = ws
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"workspace": updatedWS,
			"operation": op,
			"stdout":    strings.Join(stdout, ""),
			"stderr":    strings.Join(stderr, ""),
			"status":    "ok",
			"action":    action,
			"runtimeServiceCount": runtimeServiceCount,
		})
	}
}
