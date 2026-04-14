package routes

import (
"encoding/json"
"net/http"

"github.com/chifamba/vashandi/vashandi/backend/db/models"
"github.com/go-chi/chi/v5"
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
