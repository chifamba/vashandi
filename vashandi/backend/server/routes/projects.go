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

func ListProjectsHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
companyID := chi.URLParam(r, "companyId")
var projects []models.Project
db.WithContext(r.Context()).Where("company_id = ?", companyID).
Preload("LeadAgent").
Order("created_at DESC").Find(&projects)
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(projects)
}
}

func GetProjectHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
id := chi.URLParam(r, "id")
var project models.Project
if err := db.WithContext(r.Context()).Preload("LeadAgent").First(&project, "id = ?", id).Error; err != nil {
http.Error(w, "Not found", http.StatusNotFound)
return
}
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(project)
}
}

func CreateProjectHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
companyID := chi.URLParam(r, "companyId")
var project models.Project
if err := json.NewDecoder(r.Body).Decode(&project); err != nil {
http.Error(w, err.Error(), http.StatusBadRequest)
return
}
project.CompanyID = companyID
if err := db.WithContext(r.Context()).Create(&project).Error; err != nil {
http.Error(w, err.Error(), http.StatusInternalServerError)
return
}
w.Header().Set("Content-Type", "application/json")
w.WriteHeader(http.StatusCreated)
json.NewEncoder(w).Encode(project)
}
}

func UpdateProjectHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
id := chi.URLParam(r, "id")
var project models.Project
if err := db.WithContext(r.Context()).First(&project, "id = ?", id).Error; err != nil {
http.Error(w, "Not found", http.StatusNotFound)
return
}
if err := json.NewDecoder(r.Body).Decode(&project); err != nil {
http.Error(w, err.Error(), http.StatusBadRequest)
return
}
db.WithContext(r.Context()).Save(&project)
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(project)
}
}

func DeleteProjectHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
id := chi.URLParam(r, "id")
if err := db.WithContext(r.Context()).Where("id = ?", id).Delete(&models.Project{}).Error; err != nil {
http.Error(w, err.Error(), http.StatusInternalServerError)
return
}
w.WriteHeader(http.StatusNoContent)
}
}

func ListProjectWorkspacesHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
projectID := chi.URLParam(r, "id")
var workspaces []models.ProjectWorkspace
db.WithContext(r.Context()).Where("project_id = ?", projectID).Find(&workspaces)
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(workspaces)
}
}

func CreateProjectWorkspaceHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
projectID := chi.URLParam(r, "id")
var ws models.ProjectWorkspace
if err := json.NewDecoder(r.Body).Decode(&ws); err != nil {
http.Error(w, err.Error(), http.StatusBadRequest)
return
}
ws.ProjectID = projectID
var project models.Project
if err := db.WithContext(r.Context()).First(&project, "id = ?", projectID).Error; err != nil {
http.Error(w, "Project not found", http.StatusNotFound)
return
}
ws.CompanyID = project.CompanyID
if err := db.WithContext(r.Context()).Create(&ws).Error; err != nil {
http.Error(w, err.Error(), http.StatusInternalServerError)
return
}
w.Header().Set("Content-Type", "application/json")
w.WriteHeader(http.StatusCreated)
json.NewEncoder(w).Encode(ws)
}
}

func UpdateProjectWorkspaceHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
projectID := chi.URLParam(r, "id")
workspaceID := chi.URLParam(r, "workspaceId")
var ws models.ProjectWorkspace
if err := db.WithContext(r.Context()).First(&ws, "id = ? AND project_id = ?", workspaceID, projectID).Error; err != nil {
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

func DeleteProjectWorkspaceHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
projectID := chi.URLParam(r, "id")
workspaceID := chi.URLParam(r, "workspaceId")
if err := db.WithContext(r.Context()).
Where("id = ? AND project_id = ?", workspaceID, projectID).
Delete(&models.ProjectWorkspace{}).Error; err != nil {
http.Error(w, err.Error(), http.StatusInternalServerError)
return
}
w.WriteHeader(http.StatusNoContent)
}
}

// ProjectWorkspaceRuntimeServicesHandler — POST /projects/:id/workspaces/:workspaceId/runtime-services/:action
func ProjectWorkspaceRuntimeServicesHandler(db *gorm.DB, rtMgr *services.WorkspaceRuntimeManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectID := chi.URLParam(r, "id")
		workspaceID := chi.URLParam(r, "workspaceId")
		action := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "action")))

		if action != "start" && action != "stop" && action != "restart" {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Runtime service action not found"})
			return
		}

		var project models.Project
		if err := db.WithContext(r.Context()).First(&project, "id = ?", projectID).Error; err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Project not found"})
			return
		}

		var pw models.ProjectWorkspace
		if err := db.WithContext(r.Context()).First(&pw, "id = ? AND project_id = ?", workspaceID, projectID).Error; err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Project workspace not found"})
			return
		}

		if pw.Cwd == nil || *pw.Cwd == "" {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
				"error": "Project workspace needs a local path before Paperclip can manage local runtime services",
			})
			return
		}
		workspaceCwd := *pw.Cwd

		runtimeConfig := services.ReadProjectWorkspaceRuntimeConfig(pw.Metadata)
		if (action == "start" || action == "restart") && runtimeConfig == nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
				"error": "Project workspace has no runtime service configuration",
			})
			return
		}

		actor := GetActorInfo(r)

		opSvc := services.NewWorkspaceOperationService(db)
		recorder := opSvc.CreateRecorder(project.CompanyID, nil, nil)
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
				count, err := rtMgr.StopRuntimeServicesForProjectWorkspace(r.Context(), pw.ID)
				if err != nil {
					runErr = err
				} else {
					runtimeServiceCount = count
				}
			} else {
				db.WithContext(r.Context()).
					Model(&models.WorkspaceRuntimeService{}).
					Where("project_workspace_id = ? AND scope_type = 'project_workspace' AND status IN ('starting','running')", pw.ID).
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
					CompanyID:          project.CompanyID,
					ProjectID:          &project.ID,
					ProjectWorkspaceID: &pw.ID,
					WorkspaceCwd:       workspaceCwd,
					RuntimeConfig:      runtimeConfig,
					OwnerAgentID:       ownerAgentID,
				}, onLog)
				if err != nil {
					runErr = err
				} else {
					runtimeServiceCount = len(refs)
				}
			}
		}

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

		// Update the workspace desired state.
		desiredState := "running"
		if action == "stop" {
			desiredState = "stopped"
		}
		updateProjectWorkspaceDesiredState(db, r, pw.ID, desiredState)

		// Log activity (best-effort).
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
			CompanyID:  project.CompanyID,
			ActorType:  actorType,
			ActorID:    actorID,
			AgentID:    agentIDPtr,
			Action:     "project_workspace.runtime_" + action,
			EntityType: "project_workspace",
			EntityID:   pw.ID,
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"workspace": pw,
			"operation": op,
			"stdout":    strings.Join(stdout, ""),
			"stderr":    strings.Join(stderr, ""),
			"status":    "ok",
			"action":    action,
			"runtimeServiceCount": runtimeServiceCount,
		})
	}
}
