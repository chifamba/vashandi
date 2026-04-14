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
