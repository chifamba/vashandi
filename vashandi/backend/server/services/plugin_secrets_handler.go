package services

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"gorm.io/gorm"
)

var uuidRegex = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type SecretMeta struct {
	ID       string `json:"id"`
	Key      string `json:"key"`
	Provider string `json:"provider"`
}

type PluginSecretsHandler struct {
	db        *gorm.DB
	secrets   *SecretService
	registry  *PluginRegistryService
	validator *PluginCapabilityValidator

	// Rate limiting: pluginID -> last resolution timestamps
	attempts   map[string][]time.Time
	attemptsMu sync.Mutex
}

func NewPluginSecretsHandler(db *gorm.DB, secrets *SecretService, registry *PluginRegistryService, validator *PluginCapabilityValidator) *PluginSecretsHandler {
	return &PluginSecretsHandler{
		db:        db,
		secrets:   secrets,
		registry:  registry,
		validator: validator,
		attempts:  make(map[string][]time.Time),
	}
}

func (h *PluginSecretsHandler) checkRateLimit(pluginID string) bool {
	h.attemptsMu.Lock()
	defer h.attemptsMu.Unlock()

	now := time.Now()
	window := now.Add(-1 * time.Minute)

	// Clean up old attempts
	var recent []time.Time
	for _, t := range h.attempts[pluginID] {
		if t.After(window) {
			recent = append(recent, t)
		}
	}

	if len(recent) >= 30 {
		return false
	}

	recent = append(recent, now)
	h.attempts[pluginID] = recent
	return true
}

func (h *PluginSecretsHandler) ResolveSecret(ctx context.Context, pluginID, companyID, secretID string) (string, error) {
	// 1. Rate limiting
	if !h.checkRateLimit(pluginID) {
		return "", fmt.Errorf("rate limit exceeded for secret resolution")
	}

	// 2. Validate UUID format
	if !uuidRegex.MatchString(secretID) {
		return "", fmt.Errorf("invalid secret reference: %s", secretID)
	}

	// 3. Load manifest and check capabilities
	var plugin models.Plugin
	if err := h.db.WithContext(ctx).First(&plugin, "id = ?", pluginID).Error; err != nil {
		return "", fmt.Errorf("plugin not found")
	}

	var manifest PluginManifestV1
	if err := json.Unmarshal(plugin.ManifestJSON, &manifest); err != nil {
		return "", fmt.Errorf("failed to parse plugin manifest")
	}

	if err := h.validator.CheckOperation(&manifest, "secrets.resolve"); err != nil {
		return "", err
	}

	// 4. Scope check: Only allow secrets referenced in this plugin's config
	allowedRefs, err := h.getAllowedRefs(ctx, pluginID, &manifest)
	if err != nil {
		return "", err
	}

	if !allowedRefs[secretID] {
		// Generic not found error to avoid leaking existence
		return "", fmt.Errorf("secret not found")
	}

	// 5. Resolve via core SecretService
	// Note: If companyID is empty, we must ensure the core service doesn't bypass safety.
	// However, since we already verified the secret is in the plugin's config,
	// and the config is associated with the plugin (which belongs to the platform),
	// we use the secret's associated company ID if provided, otherwise we resolve by global UUID.
	
	val, err := h.secrets.ResolveSecretValue(ctx, companyID, secretID, "latest")
	if err != nil {
		// Sanitize error
		return "", fmt.Errorf("secret not found")
	}

	return val, nil
}

func (h *PluginSecretsHandler) ListAvailableSecrets(ctx context.Context, pluginID, companyID string) ([]SecretMeta, error) {
	var plugin models.Plugin
	if err := h.db.WithContext(ctx).First(&plugin, "id = ?", pluginID).Error; err != nil {
		return nil, fmt.Errorf("plugin not found")
	}

	var manifest PluginManifestV1
	if err := json.Unmarshal(plugin.ManifestJSON, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse plugin manifest")
	}

	allowedRefs, err := h.getAllowedRefs(ctx, pluginID, &manifest)
	if err != nil {
		return nil, err
	}

	var secretIDs []string
	for id := range allowedRefs {
		secretIDs = append(secretIDs, id)
	}

	if len(secretIDs) == 0 {
		return []SecretMeta{}, nil
	}

	var secrets []models.CompanySecret
	if err := h.db.WithContext(ctx).Where("id IN ?", secretIDs).Find(&secrets).Error; err != nil {
		return nil, err
	}

	meta := make([]SecretMeta, 0, len(secrets))
	for _, s := range secrets {
		meta = append(meta, SecretMeta{
			ID:       s.ID,
			Key:      s.Name,
			Provider: s.Provider,
		})
	}

	return meta, nil
}

func (h *PluginSecretsHandler) getAllowedRefs(ctx context.Context, pluginID string, manifest *PluginManifestV1) (map[string]bool, error) {
	var configRow models.PluginConfig
	if err := h.db.WithContext(ctx).Where("plugin_id = ?", pluginID).First(&configRow).Error; err != nil {
		return map[string]bool{}, nil
	}

	var config map[string]interface{}
	if err := json.Unmarshal(configRow.ConfigJSON, &config); err != nil {
		return nil, err
	}

	refs := make(map[string]bool)
	h.extractRefs(config, manifest.InstanceConfigSchema, refs)
	return refs, nil
}

func (h *PluginSecretsHandler) extractRefs(config map[string]interface{}, schema map[string]interface{}, refs map[string]bool) {
	// If schema defines secret-ref paths, use them.
	secretPaths := h.collectSecretRefPaths(schema)
	if len(secretPaths) > 0 {
		for _, path := range secretPaths {
			val := h.getValueAtPath(config, path)
			if s, ok := val.(string); ok && uuidRegex.MatchString(s) {
				refs[s] = true
			}
		}
		return
	}

	// Fallback: scan everything for UUIDs
	h.scanForUuids(config, refs)
}

func (h *PluginSecretsHandler) collectSecretRefPaths(schema map[string]interface{}) []string {
	var paths []string
	var walk func(node map[string]interface{}, prefix string)
	walk = func(node map[string]interface{}, prefix string) {
		props, _ := node["properties"].(map[string]interface{})
		if props == nil {
			return
		}
		for key, val := range props {
			prop, _ := val.(map[string]interface{})
			if prop == nil {
				continue
			}
			path := key
			if prefix != "" {
				path = prefix + "." + key
			}

			if fmt.Sprintf("%v", prop["format"]) == "secret-ref" {
				paths = append(paths, path)
			}

			if fmt.Sprintf("%v", prop["type"]) == "object" {
				walk(prop, path)
			}
		}
	}
	walk(schema, "")
	return paths
}

func (h *PluginSecretsHandler) getValueAtPath(data map[string]interface{}, path string) interface{} {
	parts := strings.Split(path, ".")
	var current interface{} = data
	for _, part := range parts {
		if m, ok := current.(map[string]interface{}); ok {
			current = m[part]
		} else {
			return nil
		}
	}
	return current
}

func (h *PluginSecretsHandler) scanForUuids(data interface{}, refs map[string]bool) {
	switch v := data.(type) {
	case string:
		if uuidRegex.MatchString(v) {
			refs[v] = true
		}
	case map[string]interface{}:
		for _, val := range v {
			h.scanForUuids(val, refs)
		}
	case []interface{}:
		for _, val := range v {
			h.scanForUuids(val, refs)
		}
	}
}
