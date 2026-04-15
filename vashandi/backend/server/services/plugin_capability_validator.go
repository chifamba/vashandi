package services

import (
	"encoding/json"
	"fmt"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
)

type PluginCapabilityValidator struct{}

func NewPluginCapabilityValidator() *PluginCapabilityValidator {
	return &PluginCapabilityValidator{}
}

func (v *PluginCapabilityValidator) AssertOperation(manifest map[string]interface{}, operation string) error {
	// In a real implementation, this would check if the operation is in the manifest capabilities.
	// For now, we stub it to allow.
	return nil
}

type CapabilityScopedInvoker interface {
	Invoke(operation string, fn func() (interface{}, error)) (interface{}, error)
}

type DefaultCapabilityScopedInvoker struct {
	manifest  map[string]interface{}
	validator *PluginCapabilityValidator
}

func NewCapabilityScopedInvoker(plugin *models.Plugin, validator *PluginCapabilityValidator) CapabilityScopedInvoker {
	var manifest map[string]interface{}
	if plugin != nil && plugin.ManifestJSON != nil {
		json.Unmarshal(plugin.ManifestJSON, &manifest)
	}
	return &DefaultCapabilityScopedInvoker{
		manifest:  manifest,
		validator: validator,
	}
}

func (i *DefaultCapabilityScopedInvoker) Invoke(operation string, fn func() (interface{}, error)) (interface{}, error) {
	if err := i.validator.AssertOperation(i.manifest, operation); err != nil {
		return nil, fmt.Errorf("capability check failed: %w", err)
	}
	return fn()
}
