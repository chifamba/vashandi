package routes

import (
"encoding/json"
"net/http"

"github.com/chifamba/vashandi/vashandi/backend/db/models"
"github.com/go-chi/chi/v5"
"gorm.io/gorm"
)

func ListRoutinesHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
companyID := chi.URLParam(r, "companyId")
var routines []models.Routine
db.WithContext(r.Context()).Where("company_id = ?", companyID).Order("created_at DESC").Find(&routines)
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(routines)
}
}

func GetRoutineHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
id := chi.URLParam(r, "id")
var routine models.Routine
if err := db.WithContext(r.Context()).First(&routine, "id = ?", id).Error; err != nil {
http.Error(w, "Not found", http.StatusNotFound)
return
}
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(routine)
}
}

func CreateRoutineHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
companyID := chi.URLParam(r, "companyId")
var routine models.Routine
if err := json.NewDecoder(r.Body).Decode(&routine); err != nil {
http.Error(w, err.Error(), http.StatusBadRequest)
return
}
routine.CompanyID = companyID
if err := db.WithContext(r.Context()).Create(&routine).Error; err != nil {
http.Error(w, err.Error(), http.StatusInternalServerError)
return
}
w.Header().Set("Content-Type", "application/json")
w.WriteHeader(http.StatusCreated)
json.NewEncoder(w).Encode(routine)
}
}

func UpdateRoutineHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
id := chi.URLParam(r, "id")
var routine models.Routine
if err := db.WithContext(r.Context()).First(&routine, "id = ?", id).Error; err != nil {
http.Error(w, "Not found", http.StatusNotFound)
return
}
if err := json.NewDecoder(r.Body).Decode(&routine); err != nil {
http.Error(w, err.Error(), http.StatusBadRequest)
return
}
db.WithContext(r.Context()).Save(&routine)
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(routine)
}
}

func ListRoutineRunsHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
routineID := chi.URLParam(r, "id")
var runs []models.HeartbeatRun
db.WithContext(r.Context()).
Where("trigger_detail = ?", routineID).
Order("created_at DESC").Find(&runs)
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(runs)
}
}
