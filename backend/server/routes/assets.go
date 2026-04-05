package routes

import (
	"encoding/json"
	"net/http"
	"fmt"

	"github.com/chifamba/paperclip/backend/db/models"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// In a real application, storage would be handled by a storage interface.
// For now, this is a simplified version of the assets port since we don't have
// the full Go storage and image processing libraries mapped yet (like DOMPurify and sharp equivalents).

// CreateAssetHandler creates a new asset record (metadata only for this stub)
func CreateAssetHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		companyID := chi.URLParam(r, "companyId")

		// Simplified: We just read the metadata payload instead of doing multipart/form-data.
		// In a complete port, we would parse multipart form, validate sizes,
		// stream to an S3/Local disk adapter, and then save the asset record.
		var asset models.Asset
		if err := json.NewDecoder(r.Body).Decode(&asset); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid payload"})
			return
		}

		asset.CompanyID = companyID

		// Set some dummy values that would normally come from the storage provider
		if asset.Provider == "" {
			asset.Provider = "local_disk"
		}
		if asset.ObjectKey == "" {
			asset.ObjectKey = fmt.Sprintf("assets/%s/dummy_key", companyID)
		}

		if err := db.Create(&asset).Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to create asset"})
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(asset)
	}
}

// GetAssetContentHandler serves the asset content
func GetAssetContentHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		assetID := chi.URLParam(r, "assetId")

		var asset models.Asset
		if err := db.Where("id = ?", assetID).First(&asset).Error; err != nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Asset not found"})
			return
		}

		// Simplified: We don't have the storage provider to stream from.
		// We'll just return the metadata as JSON for now, or a dummy string.
		w.Header().Set("Content-Type", asset.ContentType)
		filename := "asset"
		if asset.OriginalFilename != nil {
			filename = *asset.OriginalFilename
		}
		w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", filename))

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Dummy asset content for: " + asset.ObjectKey))
	}
}
