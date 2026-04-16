package routes

import (
	"encoding/json"
	"net/http"

	"github.com/chifamba/vashandi/vashandi/backend/server/services"
)

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func requireBoard(w http.ResponseWriter, r *http.Request) bool {
	if err := AssertBoard(r); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return false
	}
	return true
}

func requireCompanyAccess(w http.ResponseWriter, r *http.Request, companyID string) bool {
	if err := AssertCompanyAccess(r, companyID); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return false
	}
	return true
}

func requireInstanceAdmin(w http.ResponseWriter, r *http.Request) bool {
	if err := AssertInstanceAdmin(r); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return false
	}
	return true
}

func GetGeneralSettingsHandler(settingsSvc *services.InstanceSettingsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireBoard(w, r) {
			return
		}
		settings, err := settingsSvc.GetGeneral(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, settings)
	}
}

func UpdateGeneralSettingsHandler(settingsSvc *services.InstanceSettingsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireInstanceAdmin(w, r) {
			return
		}
		var patch map[string]any
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		settings, err := settingsSvc.UpdateGeneral(r.Context(), patch)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, settings)
	}
}

func GetExperimentalSettingsHandler(settingsSvc *services.InstanceSettingsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireBoard(w, r) {
			return
		}
		settings, err := settingsSvc.GetExperimental(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, settings)
	}
}

func UpdateExperimentalSettingsHandler(settingsSvc *services.InstanceSettingsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireInstanceAdmin(w, r) {
			return
		}
		var patch map[string]any
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		settings, err := settingsSvc.UpdateExperimental(r.Context(), patch)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, settings)
	}
}
