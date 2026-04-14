package routes

import (
"encoding/json"
"net/http"

"github.com/chifamba/vashandi/vashandi/backend/db/models"
"github.com/go-chi/chi/v5"
"gorm.io/gorm"
)

func ListCompanySkillsHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
companyID := chi.URLParam(r, "companyId")
var skills []models.CompanySkill
db.WithContext(r.Context()).Where("company_id = ?", companyID).Order("name ASC").Find(&skills)
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(skills)
}
}

func CreateCompanySkillHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
companyID := chi.URLParam(r, "companyId")
var skill models.CompanySkill
if err := json.NewDecoder(r.Body).Decode(&skill); err != nil {
http.Error(w, err.Error(), http.StatusBadRequest)
return
}
skill.CompanyID = companyID
if err := db.WithContext(r.Context()).Create(&skill).Error; err != nil {
http.Error(w, err.Error(), http.StatusInternalServerError)
return
}
w.Header().Set("Content-Type", "application/json")
w.WriteHeader(http.StatusCreated)
json.NewEncoder(w).Encode(skill)
}
}

func UpdateCompanySkillHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
id := chi.URLParam(r, "id")
var skill models.CompanySkill
if err := db.WithContext(r.Context()).First(&skill, "id = ?", id).Error; err != nil {
http.Error(w, "Not found", http.StatusNotFound)
return
}
if err := json.NewDecoder(r.Body).Decode(&skill); err != nil {
http.Error(w, err.Error(), http.StatusBadRequest)
return
}
db.WithContext(r.Context()).Save(&skill)
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(skill)
}
}
