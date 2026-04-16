package opencodelocal

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Supported default models as referenced by OpenCode. OpenCode dynamically
// queries its backend, but these are reasonable fallback constants.
const (
	ModelGPT4o        = "gpt-4o"
	ModelClaudeSonnet = "claude-3-5-sonnet-20241022"
)

type AdapterModel struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

func DiscoverOpenCodeModels(ctx context.Context, command string, env []string) ([]AdapterModel, error) {
	if command == "" {
		command = "opencode"
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, "models")
	cmd.Env = env

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("opencode models failed: %w", err)
	}

	return ParseModelsOutput(string(out)), nil
}

func ParseModelsOutput(stdout string) []AdapterModel {
	var models []AdapterModel
	seen := make(map[string]bool)

	lines := strings.Split(stdout, "\n")
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}

		firstToken := parts[0]
		if !strings.Contains(firstToken, "/") {
			continue
		}

		idx := strings.Index(firstToken, "/")
		provider := strings.TrimSpace(firstToken[:idx])
		model := strings.TrimSpace(firstToken[idx+1:])

		if provider == "" || model == "" {
			continue
		}

		id := fmt.Sprintf("%s/%s", provider, model)
		if seen[id] {
			continue
		}
		seen[id] = true
		models = append(models, AdapterModel{ID: id, Label: id})
	}
	return models
}
