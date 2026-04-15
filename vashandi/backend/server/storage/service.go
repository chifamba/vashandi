package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

const maxSegmentLength = 120

func sanitizeSegment(value string) string {
	replacer := strings.NewReplacer(
		" ", "_",
		"/", "_",
		"\\", "_",
	)
	cleaned := replacer.Replace(strings.TrimSpace(value))
	var builder strings.Builder
	lastUnderscore := false
	for _, r := range cleaned {
		valid := (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '.' || r == '_' || r == '-'
		if !valid {
			r = '_'
		}
		if r == '_' {
			if lastUnderscore {
				continue
			}
			lastUnderscore = true
		} else {
			lastUnderscore = false
		}
		builder.WriteRune(r)
		if builder.Len() >= maxSegmentLength {
			break
		}
	}
	result := strings.Trim(builder.String(), "_")
	if result == "" {
		return "file"
	}
	return result
}

func normalizeNamespace(namespace string) string {
	parts := strings.Split(namespace, "/")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, sanitizeSegment(part))
	}
	if len(out) == 0 {
		return "misc"
	}
	return strings.Join(out, "/")
}

func splitFilename(filename string) (string, string) {
	base := strings.TrimSpace(filepath.Base(filename))
	if base == "." || base == string(filepath.Separator) || base == "" {
		return "file", ""
	}
	ext := strings.ToLower(filepath.Ext(base))
	stem := base[:len(base)-len(ext)]
	if stem == "" {
		stem = "file"
	}
	var extBuilder strings.Builder
	for _, r := range ext {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' {
			extBuilder.WriteRune(r)
		}
		if extBuilder.Len() >= 16 {
			break
		}
	}
	return sanitizeSegment(stem), extBuilder.String()
}

func ensureCompanyPrefix(companyID, objectKey string) error {
	expectedPrefix := companyID + "/"
	if !strings.HasPrefix(objectKey, expectedPrefix) {
		return newStatusError(403, "object does not belong to company")
	}
	if strings.Contains(objectKey, "..") {
		return newStatusError(400, "invalid object key")
	}
	return nil
}

func hashBytes(input []byte) string {
	sum := sha256.Sum256(input)
	return hex.EncodeToString(sum[:])
}

func buildObjectKey(companyID, namespace, originalFilename string, now time.Time) string {
	ns := normalizeNamespace(namespace)
	year, month, day := now.UTC().Date()
	stem, ext := splitFilename(originalFilename)
	filename := fmt.Sprintf("%s-%s%s", uuid.NewString(), stem, ext)
	return fmt.Sprintf("%s/%s/%04d/%02d/%02d/%s", companyID, ns, year, month, day, filename)
}

func validatePutFileInput(input PutFileInput) error {
	if strings.TrimSpace(input.CompanyID) == "" {
		return newStatusError(422, "companyId is required")
	}
	if strings.TrimSpace(input.Namespace) == "" {
		return newStatusError(422, "namespace is required")
	}
	if strings.TrimSpace(input.ContentType) == "" {
		return newStatusError(422, "contentType is required")
	}
	if len(input.Body) == 0 {
		return newStatusError(422, "file is empty")
	}
	return nil
}

func (s *Service) PutFile(ctx context.Context, input PutFileInput) (*PutFileResult, error) {
	if err := validatePutFileInput(input); err != nil {
		return nil, err
	}
	objectKey := buildObjectKey(input.CompanyID, input.Namespace, input.OriginalFilename, time.Now())
	contentType := strings.ToLower(strings.TrimSpace(input.ContentType))
	if err := s.provider.PutObject(ctx, PutObjectInput{
		ObjectKey:     objectKey,
		Body:          input.Body,
		ContentType:   contentType,
		ContentLength: int64(len(input.Body)),
	}); err != nil {
		return nil, err
	}
	return &PutFileResult{
		Provider:         s.provider.ProviderID(),
		ObjectKey:        objectKey,
		ContentType:      contentType,
		ByteSize:         int64(len(input.Body)),
		SHA256:           hashBytes(input.Body),
		OriginalFilename: input.OriginalFilename,
	}, nil
}

func (s *Service) GetObject(ctx context.Context, companyID, objectKey string) (*GetObjectResult, error) {
	if err := ensureCompanyPrefix(companyID, objectKey); err != nil {
		return nil, err
	}
	return s.provider.GetObject(ctx, GetObjectInput{ObjectKey: objectKey})
}

func (s *Service) HeadObject(ctx context.Context, companyID, objectKey string) (*HeadObjectResult, error) {
	if err := ensureCompanyPrefix(companyID, objectKey); err != nil {
		return nil, err
	}
	return s.provider.HeadObject(ctx, HeadObjectInput{ObjectKey: objectKey})
}

func (s *Service) DeleteObject(ctx context.Context, companyID, objectKey string) error {
	if err := ensureCompanyPrefix(companyID, objectKey); err != nil {
		return err
	}
	return s.provider.DeleteObject(ctx, DeleteObjectInput{ObjectKey: objectKey})
}
