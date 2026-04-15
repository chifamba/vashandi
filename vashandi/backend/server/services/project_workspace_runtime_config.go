package services

// ProjectWorkspaceRuntimeConfig holds the runtime configuration embedded in a
// project workspace's metadata JSON.
// Mirrors the TypeScript ProjectWorkspaceRuntimeConfig shared type.
type ProjectWorkspaceRuntimeConfig struct {
	WorkspaceRuntime map[string]interface{} `json:"workspaceRuntime"`
	DesiredState     string                 `json:"desiredState"` // "running" | "stopped" | ""
}

func isValidDesiredState(v interface{}) (string, bool) {
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	return s, s == "running" || s == "stopped"
}

func cloneMapRecord(v interface{}) map[string]interface{} {
	m, ok := v.(map[string]interface{})
	if !ok || m == nil {
		return nil
	}
	out := make(map[string]interface{}, len(m))
	for k, val := range m {
		out[k] = val
	}
	return out
}

// ParseProjectWorkspaceRuntimeConfig extracts the runtime config from workspace metadata.
// Returns nil if metadata is nil or contains no runtime config.
// Mirrors the TypeScript readProjectWorkspaceRuntimeConfig function.
func ParseProjectWorkspaceRuntimeConfig(metadata map[string]interface{}) *ProjectWorkspaceRuntimeConfig {
	if metadata == nil {
		return nil
	}
	rawConfig, ok := metadata["runtimeConfig"].(map[string]interface{})
	if !ok || rawConfig == nil {
		return nil
	}

	cfg := &ProjectWorkspaceRuntimeConfig{}

	if ws := cloneMapRecord(rawConfig["workspaceRuntime"]); ws != nil {
		cfg.WorkspaceRuntime = ws
	}
	if ds, ok := isValidDesiredState(rawConfig["desiredState"]); ok {
		cfg.DesiredState = ds
	}

	if cfg.WorkspaceRuntime == nil && cfg.DesiredState == "" {
		return nil
	}
	return cfg
}

// MergeProjectWorkspaceRuntimeConfig applies a partial config patch to workspace metadata.
// When patch is nil, the runtimeConfig key is removed from metadata.
// Returns the new merged metadata, or nil if the result would be empty.
// Mirrors the TypeScript mergeProjectWorkspaceRuntimeConfig function.
func MergeProjectWorkspaceRuntimeConfig(
	metadata map[string]interface{},
	patch *ProjectWorkspaceRuntimeConfig,
) map[string]interface{} {
	nextMetadata := make(map[string]interface{})
	for k, v := range metadata {
		nextMetadata[k] = v
	}

	if patch == nil {
		delete(nextMetadata, "runtimeConfig")
		if len(nextMetadata) == 0 {
			return nil
		}
		return nextMetadata
	}

	current := ParseProjectWorkspaceRuntimeConfig(metadata)
	if current == nil {
		current = &ProjectWorkspaceRuntimeConfig{}
	}

	next := ProjectWorkspaceRuntimeConfig{
		WorkspaceRuntime: current.WorkspaceRuntime,
		DesiredState:     current.DesiredState,
	}

	// Apply patch fields
	if patch.WorkspaceRuntime != nil {
		next.WorkspaceRuntime = cloneMapRecord(patch.WorkspaceRuntime)
	} else if patch.WorkspaceRuntime == nil && patch.DesiredState != "" {
		// Only workspace runtime is being cleared when it is explicitly nil
		// and DesiredState is being set (partial patch)
		// Keep current workspace runtime in this case — caller passes nil to clear.
		// The zero-value nil map already signals "no change" above.
	}

	if patch.DesiredState != "" {
		if _, ok := isValidDesiredState(patch.DesiredState); ok {
			next.DesiredState = patch.DesiredState
		}
	}

	if next.WorkspaceRuntime == nil && next.DesiredState == "" {
		delete(nextMetadata, "runtimeConfig")
	} else {
		runtimeConfig := make(map[string]interface{})
		if next.WorkspaceRuntime != nil {
			runtimeConfig["workspaceRuntime"] = next.WorkspaceRuntime
		}
		if next.DesiredState != "" {
			runtimeConfig["desiredState"] = next.DesiredState
		}
		nextMetadata["runtimeConfig"] = runtimeConfig
	}

	if len(nextMetadata) == 0 {
		return nil
	}
	return nextMetadata
}
