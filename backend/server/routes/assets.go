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
		w.WriteHeader(http.StatusNotImplemented)
		json.NewEncoder(w).Encode(map[string]string{"error": "Asset creation is not yet implemented in the Go port (pending storage adapter)"})
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
