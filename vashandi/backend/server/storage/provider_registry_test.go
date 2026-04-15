package storage

import (
	"context"
	"net/http"
	"testing"

	"github.com/chifamba/vashandi/vashandi/backend/shared"
)

func TestNewProviderFromConfig_LocalDisk(t *testing.T) {
	provider, err := NewProviderFromConfig(context.Background(), shared.StorageConfig{
		Provider: "local_disk",
		LocalDisk: shared.StorageLocalDiskConfig{
			BaseDir: t.TempDir(),
		},
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	if got := provider.ProviderID(); got != "local_disk" {
		t.Fatalf("expected local_disk provider, got %q", got)
	}
}

func TestNewProviderFromConfig_S3RequiresBucket(t *testing.T) {
	_, err := NewProviderFromConfig(context.Background(), shared.StorageConfig{
		Provider: "s3",
		S3: shared.StorageS3Config{
			Region: "us-east-1",
		},
	})
	if statusCode(err) != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got err=%v", err)
	}
}
