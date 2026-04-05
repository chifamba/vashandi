package routes

import (
	"encoding/json"
	"net/http"

	"github.com/chifamba/paperclip/backend/db/models"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// ListRoutinesHandler lists routines for a company
func ListRoutinesHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		companyID := chi.URLParam(r, "companyId")

		var routines []models.Routine
		if err := db.Where("company_id = ?", companyID).Order("created_at desc").Find(&routines).Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to fetch routines"})
			return
		}

		json.NewEncoder(w).Encode(routines)
	}
}

// GetRoutineHandler fetches a single routine
func GetRoutineHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := chi.URLParam(r, "id")

		var routine models.Routine
		if err := db.Where("id = ?", id).First(&routine).Error; err != nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Routine not found"})
			return
		}

		json.NewEncoder(w).Encode(routine)
	}
}

// CreateRoutineHandler creates a new routine
func CreateRoutineHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		companyID := chi.URLParam(r, "companyId")

		var routine models.Routine
		if err := json.NewDecoder(r.Body).Decode(&routine); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid payload"})
			return
		}

		routine.CompanyID = companyID

		if err := db.Create(&routine).Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to create routine"})
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(routine)
	}
}

// UpdateRoutineHandler updates an existing routine
func UpdateRoutineHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := chi.URLParam(r, "id")

		var updates map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid payload"})
			return
		}

		var routine models.Routine
		if err := db.Where("id = ?", id).First(&routine).Error; err != nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Routine not found"})
			return
		}


		// Sanitize mass assignment
		delete(updates, "id")
		delete(updates, "company_id")
		delete(updates, "created_at")
		delete(updates, "updated_at")

		if err := db.Model(&routine).Updates(updates).Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to update routine"})
			return
		}

		json.NewEncoder(w).Encode(routine)
	}
}

// DeleteRoutineHandler deletes an routine
func DeleteRoutineHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := chi.URLParam(r, "id")

		var routine models.Routine
		if err := db.Where("id = ?", id).First(&routine).Error; err != nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Routine not found"})
			return
		}

		if err := db.Delete(&routine).Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to delete routine"})
			return
		}

		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}
}
