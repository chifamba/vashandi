package pilocal

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"
)

const modelsCacheTTL = 60 * time.Second

// AdapterModel is a discovered Pi model.
type AdapterModel struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// DiscoverInput holds parameters for model discovery.
type DiscoverInput struct {
	// Command is the pi executable; defaults to "pi" or PAPERCLIP_PI_COMMAND env var.
	Command string
	// Cwd is the working directory for the discovery subprocess.
	Cwd string
	// Env holds additional environment variables merged over os.Environ().
	Env map[string]string
}

// modelsCacheEntry is one TTL-bounded entry in the in-process models cache.
type modelsCacheEntry struct {
	expiresAt time.Time
	models    []AdapterModel
}

var (
	modelsCache   = map[string]modelsCacheEntry{}
	modelsCacheMu sync.Mutex
)

// resolvePiCommand returns the effective pi command name.
func resolvePiCommand(input string) string {
	if input != "" {
		return input
	}
	if v := strings.TrimSpace(os.Getenv("PAPERCLIP_PI_COMMAND")); v != "" {
		return v
	}
	return "pi"
}

// parseModelsOutput converts `pi --list-models` stdout/stderr into a slice of
// AdapterModel entries.  Pi outputs a columnar table; rows with 2+ whitespace-
// separated columns are parsed as "provider   model ...".
func parseModelsOutput(output string) []AdapterModel {
	var parsed []AdapterModel
	lines := strings.Split(output, "\n")

	startIndex := 0
	if len(lines) > 0 {
		hdr := strings.ToLower(lines[0])
		if strings.Contains(hdr, "provider") || strings.Contains(hdr, "model") {
			startIndex = 1
		}
	}

	for i := startIndex; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		// Split by 2+ spaces to handle the columnar format.
		parts := splitByDoubleSpace(line)
		if len(parts) < 2 {
			continue
		}
		provider := strings.TrimSpace(parts[0])
		model := strings.TrimSpace(parts[1])
		if provider == "" || model == "" {
			continue
		}
		if provider == "provider" && model == "model" {
			continue // skip repeated header
		}
		id := provider + "/" + model
		parsed = append(parsed, AdapterModel{ID: id, Label: id})
	}
	return parsed
}

// splitByDoubleSpace splits a string on runs of 2 or more spaces.
func splitByDoubleSpace(s string) []string {
	var parts []string
	var cur strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == ' ' {
			// count consecutive spaces
			j := i
			for j < len(s) && s[j] == ' ' {
				j++
			}
			if j-i >= 2 {
				parts = append(parts, cur.String())
				cur.Reset()
			} else {
				cur.WriteByte(s[i])
			}
			i = j
		} else {
			cur.WriteByte(s[i])
			i++
		}
	}
	if cur.Len() > 0 {
		parts = append(parts, cur.String())
	}
	return parts
}

func dedupeModels(models []AdapterModel) []AdapterModel {
	seen := map[string]struct{}{}
	out := make([]AdapterModel, 0, len(models))
	for _, m := range models {
		id := strings.TrimSpace(m.ID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
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

// volatileEnvPrefixes are env key prefixes excluded from the cache key so that
// per-invocation PAPERCLIP_RUN_ID etc. do not invalidate the cache constantly.
var volatileEnvPrefixes = []string{"PAPERCLIP_", "npm_", "NPM_"}
var volatileEnvExact = map[string]bool{
	"PWD": true, "OLDPWD": true, "SHLVL": true, "_": true, "TERM_SESSION_ID": true,
}

func isVolatileEnvKey(key string) bool {
	if volatileEnvExact[key] {
		return true
	}
	for _, prefix := range volatileEnvPrefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

func hashValue(s string) string {
	sum := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", sum)
}

func discoveryCacheKey(command, cwd string, env map[string]string) string {
	type kv struct{ k, v string }
	var pairs []kv
	for k, v := range env {
		if isVolatileEnvKey(k) {
			continue
		}
		pairs = append(pairs, kv{k, hashValue(v)})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].k < pairs[j].k })
	var sb strings.Builder
	sb.WriteString(command)
	sb.WriteByte('\n')
	sb.WriteString(cwd)
	sb.WriteByte('\n')
	for _, p := range pairs {
		sb.WriteString(p.k)
		sb.WriteByte('=')
		sb.WriteString(p.v)
		sb.WriteByte('\n')
	}
	return sb.String()
}

func pruneExpiredModelsCache(now time.Time) {
	for k, v := range modelsCache {
		if !v.expiresAt.After(now) {
			delete(modelsCache, k)
		}
	}
}

// DiscoverPiModels runs `pi --list-models` and returns available models.
func DiscoverPiModels(inp DiscoverInput) ([]AdapterModel, error) {
	command := resolvePiCommand(inp.Command)
	cwd := inp.Cwd
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			cwd = "."
		}
	}

	// Build runtime env: os.Environ() merged with inp.Env.
	runtimeEnv := mergeEnv(nil, inp.Env)

	cmd := exec.Command(command, "--list-models")
	cmd.Dir = cwd
	cmd.Env = runtimeEnv

	// timeout: 20 s (same as Node.js)
	done := make(chan struct{})
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("`%s --list-models` could not start: %w", command, err)
	}

	timer := time.AfterFunc(20*time.Second, func() {
		_ = cmd.Process.Kill()
	})

	err := cmd.Wait()
	timer.Stop()
	close(done)

	if err != nil {
		// Check if we were killed (timeout)
		if cmd.ProcessState != nil && !cmd.ProcessState.Exited() {
			return nil, fmt.Errorf("`%s --list-models` timed out", command)
		}
		detail := firstNonEmptyLine(stderr.String())
		if detail == "" {
			detail = firstNonEmptyLine(stdout.String())
		}
		if detail != "" {
			return nil, fmt.Errorf("`%s --list-models` failed: %s", command, detail)
		}
		return nil, fmt.Errorf("`%s --list-models` failed", command)
	}
	_ = done

	// Pi outputs model list to stderr, fall back to stdout for older versions.
	output := stderr.String()
	if output == "" {
		output = stdout.String()
	}
	return sortModels(dedupeModels(parseModelsOutput(output))), nil
}

// DiscoverPiModelsCached is like DiscoverPiModels but caches results for 60 s.
func DiscoverPiModelsCached(inp DiscoverInput) ([]AdapterModel, error) {
	command := resolvePiCommand(inp.Command)
	cwd := inp.Cwd
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			cwd = "."
		}
	}

	key := discoveryCacheKey(command, cwd, inp.Env)

	modelsCacheMu.Lock()
	now := time.Now()
	pruneExpiredModelsCache(now)
	if entry, ok := modelsCache[key]; ok && entry.expiresAt.After(now) {
		result := entry.models
		modelsCacheMu.Unlock()
		return result, nil
	}
	modelsCacheMu.Unlock()

	models, err := DiscoverPiModels(inp)
	if err != nil {
		return nil, err
	}

	modelsCacheMu.Lock()
	modelsCache[key] = modelsCacheEntry{
		expiresAt: time.Now().Add(modelsCacheTTL),
		models:    models,
	}
	modelsCacheMu.Unlock()

	return models, nil
}

// EnsurePiModelConfiguredAndAvailable verifies that a non-empty model string is
// set and that the model appears in the discovered model list.
func EnsurePiModelConfiguredAndAvailable(model string, inp DiscoverInput) ([]AdapterModel, error) {
	if strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("Pi requires `adapterConfig.model` in provider/model format")
	}

	models, err := DiscoverPiModelsCached(inp)
	if err != nil {
		return nil, err
	}

	if len(models) == 0 {
		return nil, fmt.Errorf("Pi returned no models. Run `pi --list-models` and verify provider auth")
	}

	for _, m := range models {
		if m.ID == model {
			return models, nil
		}
	}

	sample := make([]string, 0, 12)
	for i, m := range models {
		if i >= 12 {
			break
		}
		sample = append(sample, m.ID)
	}
	suffix := ""
	if len(models) > 12 {
		suffix = ", ..."
	}
	return nil, fmt.Errorf("configured Pi model is unavailable: %s. Available models: %s%s",
		model, strings.Join(sample, ", "), suffix)
}

// ListPiModels returns discovered models, returning an empty slice on error.
func ListPiModels() []AdapterModel {
	models, err := DiscoverPiModelsCached(DiscoverInput{})
	if err != nil {
		return nil
	}
	return models
}

// ResetModelsCacheForTests clears the in-process models cache (test helper).
func ResetModelsCacheForTests() {
	modelsCacheMu.Lock()
	modelsCache = map[string]modelsCacheEntry{}
	modelsCacheMu.Unlock()
}

// firstNonEmptyLine returns the first trimmed non-empty line from text.
func firstNonEmptyLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return t
		}
	}
	return ""
}

// mergeEnv builds an env-string slice by layering base (os.Environ()) with
// extra entries.  extra values win; duplicates from base are omitted.
func mergeEnv(base []string, extra map[string]string) []string {
	if base == nil {
		base = os.Environ()
	}
	// Remove base keys that are overridden by extra.
	overrides := make(map[string]struct{}, len(extra))
	for k := range extra {
		overrides[strings.ToUpper(k)] = struct{}{}
	}

	result := make([]string, 0, len(base)+len(extra))
	for _, entry := range base {
		idx := strings.IndexByte(entry, '=')
		if idx < 0 {
			result = append(result, entry)
			continue
		}
		key := strings.ToUpper(entry[:idx])
		if _, skip := overrides[key]; !skip {
			result = append(result, entry)
		}
	}
	for k, v := range extra {
		result = append(result, k+"="+v)
	}
	return result
}
