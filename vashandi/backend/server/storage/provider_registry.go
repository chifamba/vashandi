package storage

import (
	"context"

	"github.com/chifamba/vashandi/vashandi/backend/shared"
)

func NewProviderFromConfig(ctx context.Context, cfg shared.StorageConfig) (Provider, error) {
	if cfg.Provider == "s3" {
		return NewS3Provider(ctx, cfg.S3)
	}
	return NewLocalDiskProvider(cfg.LocalDisk.BaseDir), nil
}

func NewServiceFromConfig(ctx context.Context, cfg shared.StorageConfig) (*Service, error) {
	provider, err := NewProviderFromConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return NewService(provider), nil
}
