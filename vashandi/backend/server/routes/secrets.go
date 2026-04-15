package routes

import (
	"encoding/json"
	"net/http"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"github.com/go-chi/chi/v5"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func ListSecretProvidersHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		providers := []string{"local_encrypted", "aws_secrets_manager", "hashicorp_vault"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(providers)
	}
}

func ListSecretsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")
		if !requireBoard(w, r) {
			return
		}
		if !requireCompanyAccess(w, r, companyID) {
			return
		}
		var secrets []models.CompanySecret
		db.WithContext(r.Context()).Where("company_id = ?", companyID).Order("name ASC").Find(&secrets)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(secrets)
	}
}

func CreateSecretHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")
		if !requireBoard(w, r) {
			return
		}
		if !requireCompanyAccess(w, r, companyID) {
			return
		}
		var body struct {
			Name        string `json:"name"`
			Provider    string `json:"provider"`
			Description string `json:"description"`
			Value       string `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		desc := body.Description
		secret := models.CompanySecret{
			CompanyID:   companyID,
			Name:        body.Name,
			Provider:    body.Provider,
			Description: &desc,
		}
		if secret.Provider == "" {
			secret.Provider = "local_encrypted"
		}
		if err := db.WithContext(r.Context()).Create(&secret).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if body.Value != "" {
			material, err := json.Marshal(map[string]string{"value": body.Value})
			if err != nil {
				http.Error(w, "failed to encode secret material", http.StatusInternalServerError)
				return
			}
			version := models.CompanySecretVersion{
				SecretID: secret.ID,
				Version:  1,
				Material: datatypes.JSON(material),
			}
			db.WithContext(r.Context()).Create(&version)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(secret)
	}
}

func RotateSecretHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if !requireBoard(w, r) {
			return
		}
		var secret models.CompanySecret
		if err := db.WithContext(r.Context()).First(&secret, "id = ?", id).Error; err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		if !requireCompanyAccess(w, r, secret.CompanyID) {
			return
		}
		var body struct {
			Value string `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		material, err := json.Marshal(map[string]string{"value": body.Value})
		if err != nil {
			http.Error(w, "failed to encode secret material", http.StatusInternalServerError)
			return
		}
		newVersion := secret.LatestVersion + 1
		version := models.CompanySecretVersion{
			SecretID: secret.ID,
			Version:  newVersion,
			Material: datatypes.JSON(material),
		}
		db.WithContext(r.Context()).Create(&version)
		secret.LatestVersion = newVersion
		db.WithContext(r.Context()).Save(&secret)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int{"version": newVersion})
	}
}

func UpdateSecretHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if !requireBoard(w, r) {
			return
		}
		var secret models.CompanySecret
		if err := db.WithContext(r.Context()).First(&secret, "id = ?", id).Error; err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		if !requireCompanyAccess(w, r, secret.CompanyID) {
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&secret); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		db.WithContext(r.Context()).Save(&secret)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(secret)
	}
}

func DeleteSecretHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if !requireBoard(w, r) {
			return
		}
		var secret models.CompanySecret
		if err := db.WithContext(r.Context()).First(&secret, "id = ?", id).Error; err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		if !requireCompanyAccess(w, r, secret.CompanyID) {
			return
		}
		db.WithContext(r.Context()).Delete(&models.CompanySecret{}, "id = ?", id)
		w.WriteHeader(http.StatusNoContent)
	}
}
