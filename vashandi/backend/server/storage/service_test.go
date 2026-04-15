package storage

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func readAll(t *testing.T, reader io.ReadCloser) []byte {
	t.Helper()
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read all: %v", err)
	}
	return data
}

func statusCode(err error) int {
	if err == nil {
		return 0
	}
	statusErr, ok := err.(*StatusError)
	if !ok {
		return 0
	}
	return statusErr.Status
}

func TestLocalDiskStorageProvider_RoundTripsBytesThroughStorageService(t *testing.T) {
	root := t.TempDir()
	service := NewService(NewLocalDiskProvider(root))

	stored, err := service.PutFile(context.Background(), PutFileInput{
		CompanyID:        "company-1",
		Namespace:        "issues/issue-1",
		OriginalFilename: "demo.png",
		ContentType:      "image/png",
		Body:             []byte("hello image bytes"),
	})
	if err != nil {
		t.Fatalf("put file: %v", err)
	}

	fetched, err := service.GetObject(context.Background(), "company-1", stored.ObjectKey)
	if err != nil {
		t.Fatalf("get object: %v", err)
	}

	if got := string(readAll(t, fetched.Stream)); got != "hello image bytes" {
		t.Fatalf("expected stored body, got %q", got)
	}
	if len(stored.SHA256) != 64 {
		t.Fatalf("expected sha256 hex length 64, got %d", len(stored.SHA256))
	}

	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(stored.ObjectKey))); err != nil {
		t.Fatalf("expected stored file on disk: %v", err)
	}
}

func TestLocalDiskStorageProvider_BlocksCrossCompanyObjectAccess(t *testing.T) {
	service := NewService(NewLocalDiskProvider(t.TempDir()))

	stored, err := service.PutFile(context.Background(), PutFileInput{
		CompanyID:        "company-a",
		Namespace:        "issues/issue-1",
		OriginalFilename: "demo.png",
		ContentType:      "image/png",
		Body:             []byte("hello"),
	})
	if err != nil {
		t.Fatalf("put file: %v", err)
	}

	err = nil
	_, err = service.GetObject(context.Background(), "company-b", stored.ObjectKey)
	if statusCode(err) != http.StatusForbidden {
		t.Fatalf("expected 403, got err=%v", err)
	}
}

func TestLocalDiskStorageProvider_DeleteIsIdempotent(t *testing.T) {
	service := NewService(NewLocalDiskProvider(t.TempDir()))

	stored, err := service.PutFile(context.Background(), PutFileInput{
		CompanyID:        "company-1",
		Namespace:        "issues/issue-1",
		OriginalFilename: "demo.png",
		ContentType:      "image/png",
		Body:             []byte("hello"),
	})
	if err != nil {
		t.Fatalf("put file: %v", err)
	}

	if err := service.DeleteObject(context.Background(), "company-1", stored.ObjectKey); err != nil {
		t.Fatalf("delete object first pass: %v", err)
	}
	if err := service.DeleteObject(context.Background(), "company-1", stored.ObjectKey); err != nil {
		t.Fatalf("delete object second pass: %v", err)
	}
	_, err = service.GetObject(context.Background(), "company-1", stored.ObjectKey)
	if statusCode(err) != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got err=%v", err)
	}
}
