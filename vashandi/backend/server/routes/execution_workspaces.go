package routes

import (
"encoding/json"
"net/http"

"github.com/chifamba/vashandi/vashandi/backend/db/models"
"github.com/go-chi/chi/v5"
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
