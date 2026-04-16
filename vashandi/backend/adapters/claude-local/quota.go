package claudelocal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type AuthStatus struct {
	LoggedIn         bool    `json:"loggedIn"`
	AuthMethod       *string `json:"authMethod"`
	SubscriptionType *string `json:"subscriptionType"`
}

// ReadClaudeAuthStatus calls `claude auth status`.
func ReadClaudeAuthStatus(ctx context.Context, command string) (*AuthStatus, error) {
	cmd := exec.CommandContext(ctx, command, "auth", "status")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var status AuthStatus
	if err := json.Unmarshal(output, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

type QuotaWindow struct {
	Label       string  `json:"label"`
	UsedPercent *int    `json:"usedPercent"`
	ResetsAt    *string `json:"resetsAt"`
	ValueLabel  *string `json:"valueLabel"`
	Detail      *string `json:"detail"`
}

// FetchClaudeQuota is a simplified port of the quota polling logic.
// In a real implementation, this would read credentials from ~/.claude/ and poll the API.
func FetchClaudeQuota(ctx context.Context, token string) ([]QuotaWindow, error) {
	if token == "" {
		return nil, fmt.Errorf("no token provided")
	}

	url := "https://api.anthropic.com/api/oauth/usage"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic usage api returned %d", resp.StatusCode)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}

	// Simplistic parsing of the usage response
	var windows []QuotaWindow
	if session, ok := body["five_hour"].(map[string]interface{}); ok {
		windows = append(windows, QuotaWindow{
			Label:       "Current session",
			UsedPercent: toIntPtr(session["utilization"]),
		})
	}
	
	return windows, nil
}

func toIntPtr(v interface{}) *int {
	if f, ok := v.(float64); ok {
		i := int(f * 100)
		return &i
	}
	return nil
}

func GetClaudeToken() (string, error) {
	// Heuristic: check ~/.claude/.credentials.json
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	
	paths := []string{
		filepath.Join(home, ".claude", ".credentials.json"),
		filepath.Join(home, ".claude", "credentials.json"),
	}

	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var creds map[string]interface{}
		if err := json.Unmarshal(data, &creds); err != nil {
			continue
		}
		if oauth, ok := creds["claudeAiOauth"].(map[string]interface{}); ok {
			if token, ok := oauth["accessToken"].(string); ok && token != "" {
				return token, nil
			}
		}
	}
	return "", fmt.Errorf("no claude token found")
}
