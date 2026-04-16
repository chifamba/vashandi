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
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"github.com/chifamba/vashandi/vashandi/backend/shared"
	"gorm.io/gorm"
)

type SecretService struct {
	DB       *gorm.DB
	Activity *ActivityService
}

func formatPlainBinding(value interface{}) map[string]interface{} {
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
			normalizedEnv[key] = formatPlainBinding(binding)
		case map[string]interface{}:
			bindingType, _ := binding["type"].(string)
			switch bindingType {
			case "", "plain":
				normalizedEnv[key] = formatPlainBinding(binding["value"])
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

// ResolveSecretReference resolves a secret reference (either a string ID or a secret_ref object).
func (s *SecretService) ResolveSecretReference(ctx context.Context, companyID string, ref interface{}) (string, error) {
	switch v := ref.(type) {
	case string:
		return s.ResolveSecretValue(ctx, companyID, v, "latest")
	case map[string]interface{}:
		if t, _ := v["type"].(string); t == "secret_ref" {
			secretID, _ := v["secretId"].(string)
			version := v["version"]
			return s.ResolveSecretValue(ctx, companyID, secretID, version)
		}
		return "", fmt.Errorf("invalid secret_ref object")
	default:
		return "", fmt.Errorf("invalid secret reference type: %T", ref)
	}
}

// SecretsService is an alias for SecretService to match the routines service interface.
type SecretsService = SecretService

// CreateSecretInput contains input for creating a secret.
type CreateSecretInput struct {
	Name        string `json:"name"`
	Provider    string `json:"provider"`
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
}

// Create creates a new secret.
func (s *SecretService) Create(ctx context.Context, companyID string, input CreateSecretInput, actor Actor) (*models.CompanySecret, error) {
	// Check for duplicate name
	var existing models.CompanySecret
	if err := s.DB.WithContext(ctx).
		Where("company_id = ? AND name = ?", companyID, input.Name).
		First(&existing).Error; err == nil {
		return nil, fmt.Errorf("secret already exists: %s", input.Name)
	}

	// Encrypt the value for local_encrypted provider
	material := make(map[string]interface{})
	valueSha256 := ""
	if input.Provider == "local_encrypted" {
		encrypted, err := shared.EncryptLocalSecret(input.Value)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt secret: %w", err)
		}
		material["encrypted"] = encrypted

		// Compute SHA256 of the value
		hash := sha256.Sum256([]byte(input.Value))
		valueSha256 = hex.EncodeToString(hash[:])
	}

	materialJSON, _ := json.Marshal(material)

	// Create secret record
	secret := &models.CompanySecret{
		CompanyID:     companyID,
		Name:          input.Name,
		Provider:      input.Provider,
		Description:   nilIfEmpty(input.Description),
		LatestVersion: 1,
	}
	if actor.AgentID != nil {
		secret.CreatedByAgentID = actor.AgentID
	}
	if actor.UserID != nil {
		secret.CreatedByUserID = actor.UserID
	}

	if err := s.DB.WithContext(ctx).Create(secret).Error; err != nil {
		return nil, err
	}

	// Create first version
	version := &models.CompanySecretVersion{
		SecretID:    secret.ID,
		Version:     1,
		Material:    materialJSON,
		ValueSha256: valueSha256,
	}
	if actor.AgentID != nil {
		version.CreatedByAgentID = actor.AgentID
	}
	if actor.UserID != nil {
		version.CreatedByUserID = actor.UserID
	}

	if err := s.DB.WithContext(ctx).Create(version).Error; err != nil {
		return nil, err
	}

	return secret, nil
}

// Rotate rotates a secret to a new value.
func (s *SecretService) Rotate(ctx context.Context, secretID, newValue string, actor Actor) (*models.CompanySecret, error) {
	var secret models.CompanySecret
	if err := s.DB.WithContext(ctx).First(&secret, "id = ?", secretID).Error; err != nil {
		return nil, fmt.Errorf("secret not found")
	}

	nextVersion := secret.LatestVersion + 1

	// Encrypt the value for local_encrypted provider
	material := make(map[string]interface{})
	valueSha256 := ""
	if secret.Provider == "local_encrypted" {
		encrypted, err := shared.EncryptLocalSecret(newValue)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt secret: %w", err)
		}
		material["encrypted"] = encrypted

		hash := sha256.Sum256([]byte(newValue))
		valueSha256 = hex.EncodeToString(hash[:])
	}

	materialJSON, _ := json.Marshal(material)

	// Create new version
	version := &models.CompanySecretVersion{
		SecretID:    secret.ID,
		Version:     nextVersion,
		Material:    materialJSON,
		ValueSha256: valueSha256,
	}
	if actor.AgentID != nil {
		version.CreatedByAgentID = actor.AgentID
	}
	if actor.UserID != nil {
		version.CreatedByUserID = actor.UserID
	}

	if err := s.DB.WithContext(ctx).Create(version).Error; err != nil {
		return nil, err
	}

	// Update secret with new version
	if err := s.DB.WithContext(ctx).Model(&secret).Updates(map[string]interface{}{
		"latest_version": nextVersion,
		"updated_at":     time.Now(),
	}).Error; err != nil {
		return nil, err
	}

	s.DB.WithContext(ctx).First(&secret, "id = ?", secretID)
	return &secret, nil
}

// ResolveValue is an alias for ResolveSecretValue for use by RoutineService.
func (s *SecretService) ResolveValue(ctx context.Context, companyID, secretID string) (string, error) {
	return s.ResolveSecretValue(ctx, companyID, secretID, "latest")
}

// Actor represents an entity performing actions.
type Actor struct {
	AgentID *string
	UserID  *string
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
