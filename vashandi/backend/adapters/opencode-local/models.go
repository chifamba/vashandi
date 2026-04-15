package opencodelocal

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os/user"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	modelsCacheTTL      = 60 * time.Second
	modelsDiscoveryTimeout = 20 * time.Second
)

// AdapterModel represents a single model returned by `opencode models`.
type AdapterModel struct {
	ID    string
	Label string
}

// modelsCache stores cached model discovery results.
type modelsCache struct {
	mu      sync.Mutex
	entries map[string]*modelsCacheEntry
}

type modelsCacheEntry struct {
	models    []AdapterModel
	expiresAt time.Time
}

var globalModelsCache = &modelsCache{
	entries: make(map[string]*modelsCacheEntry),
}

// volatileEnvKeyPrefixes are prefixes that indicate a volatile env key that
// should be excluded from cache-key computation.
var volatileEnvKeyPrefixes = []string{"PAPERCLIP_", "npm_", "NPM_"}

// volatileEnvKeyExact is the set of exact volatile key names.
var volatileEnvKeyExact = map[string]bool{
	"PWD": true, "OLDPWD": true, "SHLVL": true, "_": true,
	"TERM_SESSION_ID": true, "HOME": true,
}

func isVolatileEnvKey(key string) bool {
	if volatileEnvKeyExact[key] {
		return true
	}
	for _, prefix := range volatileEnvKeyPrefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

func hashValue(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h)
}

func discoveryCacheKey(command, cwd string, env map[string]string) string {
	type kv struct{ k, v string }
	var pairs []kv
	for k, v := range env {
		if !isVolatileEnvKey(k) {
			pairs = append(pairs, kv{k, hashValue(v)})
		}
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].k < pairs[j].k })
	var sb strings.Builder
	sb.WriteString(command)
	sb.WriteByte('\n')
	sb.WriteString(cwd)
	for _, p := range pairs {
		sb.WriteByte('\n')
		sb.WriteString(p.k)
		sb.WriteByte('=')
		sb.WriteString(p.v)
	}
	return sb.String()
}

func (c *modelsCache) get(key string) ([]AdapterModel, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok || time.Now().After(entry.expiresAt) {
		delete(c.entries, key)
		return nil, false
	}
	return entry.models, true
}

func (c *modelsCache) set(key string, models []AdapterModel) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Prune expired entries.
	now := time.Now()
	for k, v := range c.entries {
		if now.After(v.expiresAt) {
			delete(c.entries, k)
		}
	}
	c.entries[key] = &modelsCacheEntry{
		models:    models,
		expiresAt: now.Add(modelsCacheTTL),
	}
}

// ResetModelsCacheForTests clears the global models cache. Intended for tests only.
func ResetModelsCacheForTests() {
	globalModelsCache.mu.Lock()
	defer globalModelsCache.mu.Unlock()
	globalModelsCache.entries = make(map[string]*modelsCacheEntry)
}

func dedupeModels(models []AdapterModel) []AdapterModel {
	seen := make(map[string]bool)
	var out []AdapterModel
	for _, m := range models {
		id := strings.TrimSpace(m.ID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		label := strings.TrimSpace(m.Label)
		if label == "" {
			label = id
		}
		out = append(out, AdapterModel{ID: id, Label: label})
	}
	return out
}

func sortModels(models []AdapterModel) []AdapterModel {
	sorted := make([]AdapterModel, len(models))
	copy(sorted, models)
	sort.Slice(sorted, func(i, j int) bool {
		return strings.ToLower(sorted[i].ID) < strings.ToLower(sorted[j].ID)
	})
	return sorted
}

func parseModelsOutput(stdout string) []AdapterModel {
	var models []AdapterModel
	for _, raw := range strings.Split(stdout, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		// First whitespace-delimited token must contain a slash (provider/model).
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		firstToken := fields[0]
		slashIdx := strings.Index(firstToken, "/")
		if slashIdx < 0 {
			continue
		}
		provider := strings.TrimSpace(firstToken[:slashIdx])
		model := strings.TrimSpace(firstToken[slashIdx+1:])
		if provider == "" || model == "" {
			continue
		}
		id := provider + "/" + model
		models = append(models, AdapterModel{ID: id, Label: id})
	}
	return dedupeModels(models)
}

func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

// resolveOpenCodeCommand returns the opencode command to use, preferring
// the provided value over the default "opencode".
func resolveOpenCodeCommand(command string) string {
	if command == "" {
		return "opencode"
	}
	return command
}

// resolveUserHome returns the home directory of the currently running user,
// falling back to the OS-level home when os/user is unavailable.
func resolveUserHome() string {
	u, err := user.Current()
	if err != nil {
		return ""
	}
	return u.HomeDir
}

// DiscoverOpenCodeModels runs `opencode models` and parses the output.
func DiscoverOpenCodeModels(ctx context.Context, command, cwd string, env map[string]string) ([]AdapterModel, error) {
	cmd := resolveOpenCodeCommand(command)
	home := resolveUserHome()
	mergedEnv := make(map[string]string, len(env))
	for k, v := range env {
		mergedEnv[k] = v
	}
	if home != "" {
		mergedEnv["HOME"] = home
	}
	mergedEnv["OPENCODE_DISABLE_PROJECT_CONFIG"] = "true"

	timeoutCtx, cancel := context.WithTimeout(ctx, modelsDiscoveryTimeout)
	defer cancel()

	proc, err := runProcess(timeoutCtx, cmd, []string{"models"}, cwd, mergedEnv, "")
	if err != nil {
		return nil, err
	}

	if proc.TimedOut {
		return nil, fmt.Errorf("`opencode models` timed out after %.0fs", modelsDiscoveryTimeout.Seconds())
	}
	if proc.ExitCode != 0 {
		detail := firstNonEmptyLine(proc.Stderr)
		if detail == "" {
			detail = firstNonEmptyLine(proc.Stdout)
		}
		if detail != "" {
			return nil, fmt.Errorf("`opencode models` failed: %s", detail)
		}
		return nil, fmt.Errorf("`opencode models` failed")
	}

	return sortModels(parseModelsOutput(proc.Stdout)), nil
}

// DiscoverOpenCodeModelsCached is like DiscoverOpenCodeModels but uses a
// process-wide TTL cache keyed on command, cwd, and non-volatile env vars.
func DiscoverOpenCodeModelsCached(ctx context.Context, command, cwd string, env map[string]string) ([]AdapterModel, error) {
	cmd := resolveOpenCodeCommand(command)
	key := discoveryCacheKey(cmd, cwd, env)
	if cached, ok := globalModelsCache.get(key); ok {
		return cached, nil
	}
	models, err := DiscoverOpenCodeModels(ctx, command, cwd, env)
	if err != nil {
		return nil, err
	}
	globalModelsCache.set(key, models)
	return models, nil
}

// EnsureOpenCodeModelConfiguredAndAvailable validates that model is non-empty
// and present in the list returned by DiscoverOpenCodeModelsCached. Returns
// the full model list on success.
func EnsureOpenCodeModelConfiguredAndAvailable(ctx context.Context, model, command, cwd string, env map[string]string) ([]AdapterModel, error) {
	if strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("OpenCode requires `adapterConfig.model` in provider/model format")
	}

	models, err := DiscoverOpenCodeModelsCached(ctx, command, cwd, env)
	if err != nil {
		return nil, err
	}

	if len(models) == 0 {
		return nil, fmt.Errorf("OpenCode returned no models. Run `opencode models` and verify provider auth")
	}

	for _, m := range models {
		if m.ID == model {
			return models, nil
		}
	}

	limit := 12
	if len(models) < limit {
		limit = len(models)
	}
	sampleIDs := make([]string, limit)
	for i := 0; i < limit; i++ {
		sampleIDs[i] = models[i].ID
	}
	sample := strings.Join(sampleIDs, ", ")
	suffix := ""
	if len(models) > 12 {
		suffix = ", ..."
	}
	return nil, fmt.Errorf("configured OpenCode model is unavailable: %s. Available models: %s%s", model, sample, suffix)
}

// ListOpenCodeModels returns discovered models or an empty slice on failure.
func ListOpenCodeModels(ctx context.Context) []AdapterModel {
	models, err := DiscoverOpenCodeModelsCached(ctx, "", "", nil)
	if err != nil {
		return nil
	}
	return models
}

// DefaultModels returns the static list of well-known OpenCode models.
func DefaultModels() []AdapterModel {
	return []AdapterModel{
		{ID: "openai/gpt-5.2-codex", Label: "openai/gpt-5.2-codex"},
		{ID: "openai/gpt-5.4", Label: "openai/gpt-5.4"},
		{ID: "openai/gpt-5.2", Label: "openai/gpt-5.2"},
		{ID: "openai/gpt-5.1-codex-max", Label: "openai/gpt-5.1-codex-max"},
		{ID: "openai/gpt-5.1-codex-mini", Label: "openai/gpt-5.1-codex-mini"},
	}
}
