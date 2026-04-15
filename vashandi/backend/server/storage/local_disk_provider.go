package storage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

type LocalDiskProvider struct {
	baseDir string
}

func NewLocalDiskProvider(baseDir string) *LocalDiskProvider {
	return &LocalDiskProvider{baseDir: filepath.Clean(baseDir)}
}

func (p *LocalDiskProvider) ProviderID() string {
	return "local_disk"
}

func normalizeObjectKey(objectKey string) (string, error) {
	normalized := strings.ReplaceAll(strings.TrimSpace(objectKey), "\\", "/")
	if normalized == "" || strings.HasPrefix(normalized, "/") {
		return "", newStatusError(400, "invalid object key")
	}
	parts := strings.Split(normalized, "/")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if part == "." || part == ".." {
			return "", newStatusError(400, "invalid object key")
		}
		out = append(out, part)
	}
	if len(out) == 0 {
		return "", newStatusError(400, "invalid object key")
	}
	return strings.Join(out, "/"), nil
}

func (p *LocalDiskProvider) resolvePath(objectKey string) (string, error) {
	normalizedKey, err := normalizeObjectKey(objectKey)
	if err != nil {
		return "", err
	}
	resolved := filepath.Clean(filepath.Join(p.baseDir, normalizedKey))
	base := filepath.Clean(p.baseDir)
	if resolved != base && !strings.HasPrefix(resolved, base+string(os.PathSeparator)) {
		return "", newStatusError(400, "invalid object key path")
	}
	return resolved, nil
}

func (p *LocalDiskProvider) PutObject(_ context.Context, input PutObjectInput) error {
	targetPath, err := p.resolvePath(input.ObjectKey)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	tempFile, err := os.CreateTemp(filepath.Dir(targetPath), filepath.Base(targetPath)+".tmp-*")
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)
	if _, err := tempFile.Write(input.Body); err != nil {
		tempFile.Close()
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, targetPath)
}

func (p *LocalDiskProvider) GetObject(_ context.Context, input GetObjectInput) (*GetObjectResult, error) {
	filePath, err := p.resolvePath(input.ObjectKey)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(filePath)
	if err != nil || !info.Mode().IsRegular() {
		return nil, newStatusError(404, "object not found")
	}
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	lastModified := info.ModTime()
	return &GetObjectResult{
		Stream:        file,
		ContentLength: info.Size(),
		LastModified:  &lastModified,
	}, nil
}

func (p *LocalDiskProvider) HeadObject(_ context.Context, input HeadObjectInput) (*HeadObjectResult, error) {
	filePath, err := p.resolvePath(input.ObjectKey)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(filePath)
	if err != nil || !info.Mode().IsRegular() {
		return &HeadObjectResult{Exists: false}, nil
	}
	lastModified := info.ModTime()
	return &HeadObjectResult{
		Exists:        true,
		ContentLength: info.Size(),
		LastModified:  &lastModified,
	}, nil
}

func (p *LocalDiskProvider) DeleteObject(_ context.Context, input DeleteObjectInput) error {
	filePath, err := p.resolvePath(input.ObjectKey)
	if err != nil {
		return err
	}
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
