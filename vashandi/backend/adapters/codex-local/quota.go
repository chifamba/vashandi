package codexlocal

import (
	"context"
	"encoding/json"
	"os/exec"
)

type QuotaStatus struct {
	TotalTokens int `json:"totalTokens"`
}

// ReadCodexQuotaStatus simplisticly calls `codex quota status` if available.
func ReadCodexQuotaStatus(ctx context.Context, command string, env []string) (*QuotaStatus, error) {
	cmd := exec.CommandContext(ctx, command, "quota", "status", "--json")
	cmd.Env = env
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var status QuotaStatus
	if err := json.Unmarshal(output, &status); err != nil {
		return nil, err
	}
	return &status, nil
}
