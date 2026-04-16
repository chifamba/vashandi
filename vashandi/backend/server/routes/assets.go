package routes

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

const svgContentType = "image/svg+xml"

func normalizedContentType(contentType string) string {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err == nil && mediaType != "" {
		return strings.ToLower(mediaType)
	}
	return strings.ToLower(strings.TrimSpace(contentType))
}

func isSVGContentType(contentType string) bool {
	return normalizedContentType(contentType) == svgContentType
}

func sanitizeSVGData(input []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(input)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("svg is empty")
	}

	decoder := xml.NewDecoder(bytes.NewReader(trimmed))
	var output bytes.Buffer
	encoder := xml.NewEncoder(&output)

	rootSeen := false
	rootClosed := false
	skipDepth := 0

	for {
		token, err := decoder.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		switch tok := token.(type) {
		case xml.StartElement:
			if rootClosed {
				return nil, fmt.Errorf("svg must contain a single root element")
			}
			if skipDepth > 0 {
				skipDepth++
				continue
			}
			if !rootSeen {
				if !strings.EqualFold(tok.Name.Local, "svg") {
					return nil, fmt.Errorf("root element must be svg")
				}
				rootSeen = true
			} else if isForbiddenSVGElement(tok.Name) {
				skipDepth = 1
				continue
			}
			tok.Attr = sanitizeSVGAttrs(tok.Attr)
			if err := encoder.EncodeToken(tok); err != nil {
				return nil, err
			}
		case xml.EndElement:
			if skipDepth > 0 {
				skipDepth--
				continue
			}
			if !rootSeen {
				continue
			}
			if err := encoder.EncodeToken(tok); err != nil {
				return nil, err
			}
			if strings.EqualFold(tok.Name.Local, "svg") {
				rootClosed = true
			}
		case xml.CharData:
			if skipDepth > 0 {
				continue
			}
			if !rootSeen || rootClosed {
				if len(bytes.TrimSpace(tok)) > 0 {
					return nil, fmt.Errorf("invalid content outside svg root")
				}
				continue
			}
			if err := encoder.EncodeToken(tok); err != nil {
				return nil, err
			}
		case xml.Comment:
			if rootSeen && !rootClosed && skipDepth == 0 {
				if err := encoder.EncodeToken(tok); err != nil {
					return nil, err
				}
			}
		}
	}

	if !rootSeen || !rootClosed {
		return nil, fmt.Errorf("svg root not found")
	}
	if err := encoder.Flush(); err != nil {
		return nil, err
	}

	sanitized := bytes.TrimSpace(output.Bytes())
	if len(sanitized) == 0 || !bytes.HasPrefix(bytes.ToLower(sanitized), []byte("<svg")) {
		return nil, fmt.Errorf("sanitized svg is invalid")
	}
	return sanitized, nil
}

func isForbiddenSVGElement(name xml.Name) bool {
	return strings.EqualFold(name.Local, "script") || strings.EqualFold(name.Local, "foreignObject")
}

func sanitizeSVGAttrs(attrs []xml.Attr) []xml.Attr {
	sanitized := make([]xml.Attr, 0, len(attrs))
	for _, attr := range attrs {
		attrName := strings.ToLower(attr.Name.Local)
		attrValue := strings.TrimSpace(attr.Value)
		if strings.HasPrefix(attrName, "on") {
			continue
		}
		if attrName == "href" && attrValue != "" && !strings.HasPrefix(attrValue, "#") {
			continue
		}
		sanitized = append(sanitized, attr)
	}
	return sanitized
}

func sanitizeUploadedData(contentType string, data []byte) ([]byte, error) {
	if !isSVGContentType(contentType) {
		return data, nil
	}
	return sanitizeSVGData(data)
}

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
		contentType := header.Header.Get("Content-Type")
		data, err = sanitizeUploadedData(contentType, data)
		if err != nil {
			http.Error(w, "SVG could not be sanitized", http.StatusUnprocessableEntity)
			return
		}
		hash := fmt.Sprintf("%x", sha256.Sum256(data))
		fname := header.Filename
		asset := models.Asset{
			CompanyID:        companyID,
			Provider:         "local",
			ObjectKey:        companyID + "/" + hash + "/" + fname,
			ContentType:      contentType,
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
		data, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fname := header.Filename
		ct := header.Header.Get("Content-Type")
		if ct == "" {
			ct = "image/jpeg"
		}
		data, err = sanitizeUploadedData(ct, data)
		if err != nil {
			http.Error(w, "SVG could not be sanitized", http.StatusUnprocessableEntity)
			return
		}
		hash := fmt.Sprintf("%x", sha256.Sum256(data))
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
		data, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fname := header.Filename
		ct := header.Header.Get("Content-Type")
		if ct == "" {
			ct = "image/png"
		}
		data, err = sanitizeUploadedData(ct, data)
		if err != nil {
			http.Error(w, "SVG could not be sanitized", http.StatusUnprocessableEntity)
			return
		}
		hash := fmt.Sprintf("%x", sha256.Sum256(data))
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
