package services

import (
	"testing"
	"testing/fstest"
)

func TestResolveDefaultAgentInstructionsBundleRole(t *testing.T) {
	if role := ResolveDefaultAgentInstructionsBundleRole("ceo"); role != DefaultAgentBundleRoleCEO {
		t.Errorf("expected ceo role, got %s", role)
	}

	if role := ResolveDefaultAgentInstructionsBundleRole("unknown"); role != DefaultAgentBundleRoleDefault {
		t.Errorf("expected default role for unknown, got %s", role)
	}
}

func TestLoadDefaultAgentInstructionsBundle(t *testing.T) {
	// Create an in-memory filesystem
	mockFS := fstest.MapFS{
		"onboarding-assets/default/AGENTS.md": &fstest.MapFile{Data: []byte("default agents instructions")},
		"onboarding-assets/ceo/AGENTS.md":     &fstest.MapFile{Data: []byte("ceo agents")},
		"onboarding-assets/ceo/HEARTBEAT.md":  &fstest.MapFile{Data: []byte("ceo heartbeat")},
		"onboarding-assets/ceo/SOUL.md":       &fstest.MapFile{Data: []byte("ceo soul")},
		"onboarding-assets/ceo/TOOLS.md":      &fstest.MapFile{Data: []byte("ceo tools")},
	}

	// 1. Default Role
	bundle, err := LoadDefaultAgentInstructionsBundle(mockFS, "onboarding-assets", "general")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bundle) != 1 {
		t.Errorf("expected 1 file for default bundle, got %d", len(bundle))
	}
	if bundle["AGENTS.md"] != "default agents instructions" {
		t.Errorf("invalid file content: %s", bundle["AGENTS.md"])
	}

	// 2. CEO Role
	bundle, err = LoadDefaultAgentInstructionsBundle(mockFS, "onboarding-assets", "ceo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bundle) != 4 {
		t.Errorf("expected 4 files for ceo bundle, got %d", len(bundle))
	}
	if bundle["SOUL.md"] != "ceo soul" {
		t.Errorf("invalid file content: %s", bundle["SOUL.md"])
	}

	// 3. Missing file error
	badFS := fstest.MapFS{
		"onboarding-assets/default/MISSING.md": &fstest.MapFile{Data: []byte("")}, // wrong file
	}
	_, err = LoadDefaultAgentInstructionsBundle(badFS, "onboarding-assets", "default")
	if err == nil {
		t.Errorf("expected error for missing file")
	}
}
