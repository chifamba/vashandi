package services

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"github.com/chifamba/vashandi/vashandi/backend/shared"
	"gorm.io/gorm"
)

type SecretService struct {
	DB       *gorm.DB
	Activity *ActivityService
}

func createPlainBinding(value interface{}) map[string]interface{} {
	return map[string]interface{}{
		"type":  "plain",
		"value": fmt.Sprintf("%v", value),
	}
}

func NewSecretService(db *gorm.DB, activity *ActivityService) *SecretService {
	return &SecretService{
		DB:       db,
		Activity: activity,
	}
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
		binding, ok := val.(map[string]interface{})
		if !ok {
			continue
		}
		bindingType, _ := binding["type"].(string)

		if bindingType == "plain" {
			resolved[key] = fmt.Sprintf("%v", binding["value"])
		} else if bindingType == "secret_ref" {
			secretID, _ := binding["secretId"].(string)
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

func (s *SecretService) GenerateOpenBrainToken(namespaceID string, agentID string, trustTier int) (string, error) {
	signingSecret := os.Getenv("OPENBRAIN_SIGNING_SECRET")
	if signingSecret == "" {
		signingSecret = "dev_secret_token"
	}

	claims := map[string]interface{}{
		"namespaceId": namespaceID,
		"agentId":     agentID,
		"trustTier":   trustTier,
		"actorKind":   "service",
	}

	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	enc := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(signingSecret))
	mac.Write([]byte(enc))
	sig := hex.EncodeToString(mac.Sum(nil))

	return "openbrain." + enc + "." + sig, nil
}

// ResolveAdapterConfigForRuntime resolves secret references within an adapter configuration object.
func (s *SecretService) ResolveAdapterConfigForRuntime(ctx context.Context, companyID string, config map[string]interface{}) (map[string]interface{}, error) {
	// Recursive resolution for deep configs
	var resolveDeep func(input interface{}) (interface{}, error)
	resolveDeep = func(input interface{}) (interface{}, error) {
		switch v := input.(type) {
		case map[string]interface{}:
			// Check if this map is a secret reference itself
			if bType, ok := v["type"].(string); ok && bType == "secret_ref" {
				secretID, _ := v["secretId"].(string)
				version := v["version"]
				return s.ResolveSecretValue(ctx, companyID, secretID, version)
			}

			// Otherwise, recurse into children
			out := make(map[string]interface{})
			for k, val := range v {
				resolved, err := resolveDeep(val)
				if err != nil {
					return nil, err
				}
				out[k] = resolved
			}
			return out, nil
		case []interface{}:
			out := make([]interface{}, len(v))
			for i, val := range v {
				resolved, err := resolveDeep(val)
				if err != nil {
					return nil, err
				}
				out[i] = resolved
			}
			return out, nil
		default:
			return v, nil
		}
	}

	result, err := resolveDeep(config)
	if err != nil {
		return nil, err
	}
	return result.(map[string]interface{}), nil
}

// NormalizeAdapterConfigForPersistence keeps env bindings as references and validates secret ownership.
func (s *SecretService) NormalizeAdapterConfigForPersistence(ctx context.Context, companyID string, config map[string]interface{}) (map[string]interface{}, error) {
	normalized := make(map[string]interface{}, len(config))
	for key, value := range config {
		normalized[key] = value
	}

	rawEnv, ok := config["env"]
	if !ok {
		return normalized, nil
	}

	env, ok := rawEnv.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("env must be an object")
	}

	normalizedEnv := make(map[string]interface{}, len(env))
	for key, rawBinding := range env {
		switch binding := rawBinding.(type) {
		case string:
			normalizedEnv[key] = createPlainBinding(binding)
		case map[string]interface{}:
			bindingType, _ := binding["type"].(string)
			switch bindingType {
			case "", "plain":
				normalizedEnv[key] = createPlainBinding(binding["value"])
			case "secret_ref":
				secretID, _ := binding["secretId"].(string)
				if strings.TrimSpace(secretID) == "" {
					return nil, fmt.Errorf("secret_ref binding missing secretId for key %s", key)
				}
				var secret models.CompanySecret
				if err := s.DB.WithContext(ctx).
					Where("id = ? AND company_id = ?", secretID, companyID).
					First(&secret).Error; err != nil {
					return nil, fmt.Errorf("failed to validate secret %s: %w", key, err)
				}

				version := binding["version"]
				if version == nil || version == "" {
					version = "latest"
				}
				normalizedEnv[key] = map[string]interface{}{
					"type":     "secret_ref",
					"secretId": secretID,
					"version":  version,
				}
			default:
				return nil, fmt.Errorf("unsupported env binding type %q for key %s", bindingType, key)
			}
		default:
			return nil, fmt.Errorf("invalid env binding for key %s", key)
		}
	}

	normalized["env"] = normalizedEnv
	return normalized, nil
}
