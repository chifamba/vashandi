package services

// NormalizedAgentPermissions holds the resolved permission set for an agent.
type NormalizedAgentPermissions struct {
	CanCreateAgents bool `json:"canCreateAgents"`
}

// DefaultPermissionsForRole returns the default permission set for an agent role.
// Mirrors the TypeScript defaultPermissionsForRole function.
func DefaultPermissionsForRole(role string) NormalizedAgentPermissions {
	return NormalizedAgentPermissions{
		CanCreateAgents: role == "ceo",
	}
}

// NormalizeAgentPermissions resolves the final permission set for an agent by
// merging explicit permission overrides with role-based defaults.
// Mirrors the TypeScript normalizeAgentPermissions function.
func NormalizeAgentPermissions(permissions interface{}, role string) NormalizedAgentPermissions {
	defaults := DefaultPermissionsForRole(role)

	record, ok := permissions.(map[string]interface{})
	if !ok || record == nil {
		return defaults
	}

	result := defaults
	if v, exists := record["canCreateAgents"]; exists {
		if b, isBool := v.(bool); isBool {
			result.CanCreateAgents = b
		}
	}
	return result
}
