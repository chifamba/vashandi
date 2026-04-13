package services

import (
	"context"
	"fmt"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"github.com/chifamba/vashandi/vashandi/backend/shared"
	"gorm.io/gorm"
)

type SecretService struct {
	DB *gorm.DB
}

func NewSecretService(db *gorm.DB) *SecretService {
	return &SecretService{DB: db}
}

// ResolveSecretValue resolves a secret reference to its decrypted value.
func (s *SecretService) ResolveSecretValue(ctx context.Context, companyID, secretID string, version any) (string, error) {
	var secret models.CompanySecret
	if err := s.DB.WithContext(ctx).Where("id = ? AND company_id = ?", secretID, companyID).First(&secret).Error; err != nil {
		return "", fmt.Errorf("secret not found: %w", err)
	}

	targetVersion := secret.LatestVersion
	if v, ok := version.(int); ok && v > 0 {
		targetVersion = v
	}

	var versionRow models.CompanySecretVersion
	if err := s.DB.WithContext(ctx).Where("secret_id = ? AND version = ?", secret.ID, targetVersion).First(&versionRow).Error; err != nil {
		return "", fmt.Errorf("secret version not found: %w", err)
	}

	if secret.Provider == "local_encrypted" {
		return shared.DecryptLocalSecret(versionRow.Material)
	}

	return "", fmt.Errorf("unsupported secret provider: %s", secret.Provider)
}

// ResolveEnvBindings resolves a map of environment variable bindings (plain or secret_ref).
func (s *SecretService) ResolveEnvBindings(ctx context.Context, companyID string, envValue map[string]interface{}) (map[string]string, error) {
	resolved := make(map[string]string)
	for key, val := range envValue {
		binding := val.(map[string]interface{})
		bindingType := binding["type"].(string)

		if bindingType == "plain" {
			resolved[key] = fmt.Sprintf("%v", binding["value"])
		} else if bindingType == "secret_ref" {
			secretID := binding["secretId"].(string)
			version := binding["version"]
			decrypted, err := s.ResolveSecretValue(ctx, companyID, secretID, version)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve secret %s: %w", key, err)
			}
			resolved[key] = decrypted
		}
	}
	return resolved, nil
}
