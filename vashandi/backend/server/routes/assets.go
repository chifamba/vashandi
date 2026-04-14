package routes

import (
"crypto/sha256"
"encoding/json"
"fmt"
"io"
"net/http"

"github.com/chifamba/vashandi/vashandi/backend/db/models"
"github.com/go-chi/chi/v5"
"gorm.io/gorm"
)

func UploadAssetHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
companyID := chi.URLParam(r, "companyId")
if err := r.ParseMultipartForm(10 << 20); err != nil {
http.Error(w, err.Error(), http.StatusBadRequest)
return
}
file, header, err := r.FormFile("file")
if err != nil {
http.Error(w, err.Error(), http.StatusBadRequest)
return
}
defer file.Close()

data, err := io.ReadAll(file)
if err != nil {
http.Error(w, err.Error(), http.StatusInternalServerError)
return
}
hash := fmt.Sprintf("%x", sha256.Sum256(data))
fname := header.Filename
asset := models.Asset{
CompanyID:        companyID,
Provider:         "local",
ObjectKey:        companyID + "/" + hash + "/" + fname,
ContentType:      header.Header.Get("Content-Type"),
ByteSize:         len(data),
Sha256:           hash,
OriginalFilename: &fname,
}
if err := db.WithContext(r.Context()).Create(&asset).Error; err != nil {
http.Error(w, err.Error(), http.StatusInternalServerError)
return
}
w.Header().Set("Content-Type", "application/json")
w.WriteHeader(http.StatusCreated)
json.NewEncoder(w).Encode(asset)
}
}

func GetAssetHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
id := chi.URLParam(r, "id")
var asset models.Asset
if err := db.WithContext(r.Context()).First(&asset, "id = ?", id).Error; err != nil {
http.Error(w, "Not found", http.StatusNotFound)
return
}
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(asset)
}
}

// UploadImageAssetHandler handles POST /companies/:companyId/assets/images
func UploadImageAssetHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer file.Close()
		data, _ := io.ReadAll(file)
		hash := fmt.Sprintf("%x", sha256.Sum256(data))
		fname := header.Filename
		ct := header.Header.Get("Content-Type")
		if ct == "" {
			ct = "image/jpeg"
		}
		asset := models.Asset{
			CompanyID:        companyID,
			Provider:         "local",
			ObjectKey:        companyID + "/images/" + hash + "/" + fname,
			ContentType:      ct,
			ByteSize:         len(data),
			Sha256:           hash,
			OriginalFilename: &fname,
		}
		if err := db.WithContext(r.Context()).Create(&asset).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(asset)
	}
}

// UploadCompanyLogoHandler handles POST /companies/:companyId/logo
func UploadCompanyLogoHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer file.Close()
		data, _ := io.ReadAll(file)
		hash := fmt.Sprintf("%x", sha256.Sum256(data))
		fname := header.Filename
		ct := header.Header.Get("Content-Type")
		if ct == "" {
			ct = "image/png"
		}
		asset := models.Asset{
			CompanyID:        companyID,
			Provider:         "local",
			ObjectKey:        companyID + "/logo/" + hash + "/" + fname,
			ContentType:      ct,
			ByteSize:         len(data),
			Sha256:           hash,
			OriginalFilename: &fname,
		}
		if err := db.WithContext(r.Context()).Create(&asset).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		logo := models.CompanyLogo{
			CompanyID: companyID,
			AssetID:   asset.ID,
		}
		db.WithContext(r.Context()).
			Where("company_id = ?", companyID).
			Assign(models.CompanyLogo{AssetID: asset.ID}).
			FirstOrCreate(&logo)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"logo":  logo,
			"asset": asset,
		})
	}
}

// GetAssetContentHandler handles GET /assets/:assetId/content
func GetAssetContentHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		assetID := chi.URLParam(r, "assetId")
		var asset models.Asset
		if err := db.WithContext(r.Context()).First(&asset, "id = ?", assetID).Error; err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		// Return asset metadata; actual file serving requires storage backend integration
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(asset)
	}
}
