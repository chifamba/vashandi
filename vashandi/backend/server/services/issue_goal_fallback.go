package services

// ResolveIssueGoalID returns the effective goal ID for an issue, applying
// project-level and default-goal fallback logic.
// Mirrors the TypeScript resolveIssueGoalId function.
func ResolveIssueGoalID(
	goalID *string,
	projectID *string,
	projectGoalID *string,
	defaultGoalID *string,
) *string {
	if goalID != nil && *goalID != "" {
		return goalID
	}
	if projectID != nil && *projectID != "" {
		return projectGoalID
	}
	return defaultGoalID
}

// ResolveNextIssueGoalID computes the new goal ID after a patch is applied to
// an issue's project / goal fields. Mirrors resolveNextIssueGoalId in TypeScript.
//
// Parameters ending in "Current" hold the pre-patch values.
// Parameters without the suffix hold the requested patch values (nil = not changing).
func ResolveNextIssueGoalID(
	currentProjectID *string,
	currentGoalID *string,
	currentProjectGoalID *string,
	patchProjectID *string,      // nil = unchanged
	patchGoalID *string,         // nil = unchanged (use sentinel notProvided = true)
	goalIDProvided bool,         // true when patchGoalID was explicitly set (even to nil)
	patchProjectGoalID *string,  // nil = unchanged
	projectGoalIDProvided bool,  // true when patchProjectGoalID was explicitly set
	defaultGoalID *string,
) *string {
	// Determine the target project after the patch
	targetProjectID := currentProjectID
	if patchProjectID != nil {
		targetProjectID = patchProjectID
	}

	// Determine the target project-goal after the patch
	var targetProjectGoalID *string
	if projectGoalIDProvided {
		targetProjectGoalID = patchProjectGoalID
	} else if targetProjectID != nil && *targetProjectID != "" {
		targetProjectGoalID = currentProjectGoalID
	}
	// else: project is being cleared → project goal doesn't apply

	resolveFallback := func(projID, projGoalID *string) *string {
		if projID != nil && *projID != "" {
			return projGoalID
		}
		return defaultGoalID
	}

	// Explicit goal override
	if goalIDProvided {
		if patchGoalID != nil && *patchGoalID != "" {
			return patchGoalID
		}
		return resolveFallback(targetProjectID, targetProjectGoalID)
	}

	// No explicit goal: re-derive from project context
	currentFallback := resolveFallback(currentProjectID, currentProjectGoalID)
	nextFallback := resolveFallback(targetProjectID, targetProjectGoalID)

	if currentGoalID == nil || *currentGoalID == "" {
		return nextFallback
	}

	// If the current goal matches the current derived fallback, follow the new fallback
	if currentFallback != nil && *currentGoalID == *currentFallback {
		return nextFallback
	}

	// Otherwise, keep the explicit goal as-is
	return currentGoalID
}
