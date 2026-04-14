package routes

import (
"encoding/json"
"net/http"
"time"

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

func DeleteRoutineHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
id := chi.URLParam(r, "id")
if err := db.WithContext(r.Context()).Where("id = ?", id).Delete(&models.Routine{}).Error; err != nil {
http.Error(w, err.Error(), http.StatusInternalServerError)
return
}
w.WriteHeader(http.StatusNoContent)
}
}

func CreateRoutineTriggerHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		routineID := chi.URLParam(r, "id")
		var routine models.Routine
		if err := db.WithContext(r.Context()).First(&routine, "id = ?", routineID).Error; err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		var trigger models.RoutineTrigger
		if err := json.NewDecoder(r.Body).Decode(&trigger); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		trigger.RoutineID = routineID
		trigger.CompanyID = routine.CompanyID
		if err := db.WithContext(r.Context()).Create(&trigger).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(trigger)
	}
}

func UpdateRoutineTriggerHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "triggerId")
		var trigger models.RoutineTrigger
		if err := db.WithContext(r.Context()).First(&trigger, "id = ?", id).Error; err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		var updates map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := db.WithContext(r.Context()).Model(&trigger).Updates(updates).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(trigger)
	}
}

func DeleteRoutineTriggerHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "triggerId")
		if err := db.WithContext(r.Context()).Delete(&models.RoutineTrigger{}, "id = ?", id).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func FirePublicRoutineTriggerHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		publicID := chi.URLParam(r, "publicId")
		var trigger models.RoutineTrigger
		if err := db.WithContext(r.Context()).First(&trigger, "public_id = ? AND enabled = true", publicID).Error; err != nil {
			http.Error(w, "Trigger not found", http.StatusNotFound)
			return
		}
		now := time.Now()
		db.WithContext(r.Context()).Model(&trigger).Update("last_fired_at", now)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"fired": true, "triggerId": trigger.ID})
	}
}

func RunRoutineNowHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var routine models.Routine
		if err := db.WithContext(r.Context()).First(&routine, "id = ?", id).Error; err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		now := time.Now()
		routine.LastTriggeredAt = &now
		db.WithContext(r.Context()).Save(&routine)
		w.WriteHeader(http.StatusAccepted)
	}
}
