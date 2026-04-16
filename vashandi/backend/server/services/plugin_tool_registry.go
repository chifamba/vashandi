package services

import (
	"encoding/json"
	"fmt"
	"sync"
)

/**
 * RegisteredTool combines the manifest-level declaration with routing metadata.
 */
type RegisteredTool struct {
	PluginID         string                 `json:"pluginId"`
	PluginDbID       string                 `json:"pluginDbId"`
	Name             string                 `json:"name"`
	NamespacedName   string                 `json:"namespacedName"`
	DisplayName      string                 `json:"displayName"`
	Description      string                 `json:"description"`
	ParametersSchema map[string]interface{} `json:"parametersSchema"`
}

type PluginToolRegistry struct {
	byNamespace map[string]*RegisteredTool
	byPlugin    map[string][]string // Map of pluginID -> []NamespacedNames
	mu          sync.RWMutex
}

func NewPluginToolRegistry() *PluginToolRegistry {
	return &PluginToolRegistry{
		byNamespace: make(map[string]*RegisteredTool),
		byPlugin:    make(map[string][]string),
	}
}

func (r *PluginToolRegistry) RegisterPlugin(pluginKey string, manifestJSON []byte, pluginDbID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 1. Parse manifest
	var m struct {
		Tools []struct {
			Name             string                 `json:"name"`
			DisplayName      string                 `json:"displayName"`
			Description      string                 `json:"description"`
			ParametersSchema map[string]interface{} `json:"parametersSchema"`
		} `json:"tools"`
	}

	if err := json.Unmarshal(manifestJSON, &m); err != nil {
		// Log error and skip if manifest is invalid
		fmt.Printf("Error registering tools for plugin %s: failed to parse manifest\n", pluginKey)
		return
	}

	// 2. Unregister previous tools
	r.unregisterNoLock(pluginKey)

	// 3. Register new tools
	for _, decl := range m.Tools {
		namespacedName := fmt.Sprintf("%s:%s", pluginKey, decl.Name)
		tool := &RegisteredTool{
			PluginID:         pluginKey,
			PluginDbID:       pluginDbID,
			Name:             decl.Name,
			NamespacedName:   namespacedName,
			DisplayName:      decl.DisplayName,
			Description:      decl.Description,
			ParametersSchema: decl.ParametersSchema,
		}

		if tool.DisplayName == "" {
			tool.DisplayName = tool.Name
		}
		if tool.ParametersSchema == nil {
			tool.ParametersSchema = map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			}
		}

		r.byNamespace[namespacedName] = tool
		r.byPlugin[pluginKey] = append(r.byPlugin[pluginKey], namespacedName)
	}
}

func (r *PluginToolRegistry) UnregisterPlugin(pluginKey string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.unregisterNoLock(pluginKey)
}

func (r *PluginToolRegistry) unregisterNoLock(pluginKey string) {
	names := r.byPlugin[pluginKey]
	for _, name := range names {
		delete(r.byNamespace, name)
	}
	delete(r.byPlugin, pluginKey)
}

func (r *PluginToolRegistry) GetTool(namespacedName string) *RegisteredTool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.byNamespace[namespacedName]
}

func (r *PluginToolRegistry) ListTools(pluginKey string) []*RegisteredTool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*RegisteredTool
	if pluginKey != "" {
		names := r.byPlugin[pluginKey]
		for _, name := range names {
			if t, ok := r.byNamespace[name]; ok {
				result = append(result, t)
			}
		}
	} else {
		for _, t := range r.byNamespace {
			result = append(result, t)
		}
	}
	return result
}
