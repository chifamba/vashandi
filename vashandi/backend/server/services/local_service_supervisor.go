package services

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/shared"
)

// LocalServiceRegistryRecord is stored on disk (one JSON file per service) so that
// the supervisor can re-adopt a process that survived a server restart.
type LocalServiceRegistryRecord struct {
	Version         int                    `json:"version"`
	ServiceKey      string                 `json:"serviceKey"`
	ProfileKind     string                 `json:"profileKind"`
	ServiceName     string                 `json:"serviceName"`
	Command         string                 `json:"command"`
	Cwd             string                 `json:"cwd"`
	EnvFingerprint  string                 `json:"envFingerprint"`
	Port            *int                   `json:"port"`
	URL             *string                `json:"url"`
	PID             int                    `json:"pid"`
	ProcessGroupID  *int                   `json:"processGroupId"`
	Provider        string                 `json:"provider"`
	RuntimeServiceID *string               `json:"runtimeServiceId"`
	ReuseKey        *string                `json:"reuseKey"`
	StartedAt       string                 `json:"startedAt"`
	LastSeenAt      string                 `json:"lastSeenAt"`
	Metadata        map[string]interface{} `json:"metadata"`
}

// LocalServiceIdentityInput is used to compute a stable service key.
type LocalServiceIdentityInput struct {
	ProfileKind    string
	ServiceName    string
	Cwd            string
	Command        string
	EnvFingerprint string
	Port           *int
	Scope          map[string]interface{}
}

func getRuntimeServicesDir() string {
	return filepath.Join(shared.ResolvePaperclipInstanceRoot(), "runtime-services")
}

func getRuntimeServiceRegistryPath(serviceKey string) string {
	return filepath.Join(getRuntimeServicesDir(), serviceKey+".json")
}

// stableStringify produces a deterministic JSON-like string for hashing.
func stableStringify(v interface{}) string {
	if v == nil {
		return "null"
	}
	switch val := v.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			keyJSON, _ := json.Marshal(k)
			parts = append(parts, fmt.Sprintf("%s:%s", keyJSON, stableStringify(val[k])))
		}
		return "{" + strings.Join(parts, ",") + "}"
	case []interface{}:
		parts := make([]string, 0, len(val))
		for _, item := range val {
			parts = append(parts, stableStringify(item))
		}
		return "[" + strings.Join(parts, ",") + "]"
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

func sanitizeServiceKeySegment(value, fallback string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	for _, r := range normalized {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			builder.WriteRune(r)
		} else {
			builder.WriteRune('-')
		}
	}
	result := strings.Trim(builder.String(), "-")
	// collapse repeated dashes
	for strings.Contains(result, "--") {
		result = strings.ReplaceAll(result, "--", "-")
	}
	if result == "" {
		return fallback
	}
	return result
}

// CreateLocalServiceKey computes a stable 24-hex-char digest for a service identity.
func CreateLocalServiceKey(input LocalServiceIdentityInput) string {
	scopeVal := map[string]interface{}(nil)
	if input.Scope != nil {
		scopeVal = input.Scope
	}

	var portVal interface{} = nil
	if input.Port != nil {
		portVal = *input.Port
	}

	payload := map[string]interface{}{
		"profileKind":    input.ProfileKind,
		"serviceName":    input.ServiceName,
		"cwd":            filepath.Clean(input.Cwd),
		"command":        input.Command,
		"envFingerprint": input.EnvFingerprint,
		"port":           portVal,
		"scope":          scopeVal,
	}
	digest := sha256.Sum256([]byte(stableStringify(payload)))
	hex := fmt.Sprintf("%x", digest)[:24]
	return fmt.Sprintf("%s-%s-%s",
		sanitizeServiceKeySegment(input.ProfileKind, "service"),
		sanitizeServiceKeySegment(input.ServiceName, "service"),
		hex,
	)
}

// WriteLocalServiceRegistryRecord writes (or overwrites) a registry file.
func WriteLocalServiceRegistryRecord(record *LocalServiceRegistryRecord) error {
	dir := getRuntimeServicesDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(getRuntimeServiceRegistryPath(record.ServiceKey), b, 0o644)
}

// RemoveLocalServiceRegistryRecord removes the registry file if it exists.
func RemoveLocalServiceRegistryRecord(serviceKey string) error {
	err := os.Remove(getRuntimeServiceRegistryPath(serviceKey))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// ReadLocalServiceRegistryRecord reads and validates a registry record.
func ReadLocalServiceRegistryRecord(serviceKey string) (*LocalServiceRegistryRecord, error) {
	return safeReadRegistryRecord(getRuntimeServiceRegistryPath(serviceKey))
}

func safeReadRegistryRecord(filePath string) (*LocalServiceRegistryRecord, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var rec LocalServiceRegistryRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, nil
	}
	if rec.Version != 1 || rec.ServiceKey == "" || rec.PID <= 0 {
		return nil, nil
	}
	return &rec, nil
}

// ListLocalServiceRegistryRecords lists all valid records, optionally filtered.
func ListLocalServiceRegistryRecords(profileKind string) ([]*LocalServiceRegistryRecord, error) {
	dir := getRuntimeServicesDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var records []*LocalServiceRegistryRecord
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		rec, _ := safeReadRegistryRecord(filepath.Join(dir, entry.Name()))
		if rec == nil {
			continue
		}
		if profileKind != "" && rec.ProfileKind != profileKind {
			continue
		}
		records = append(records, rec)
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].ServiceKey < records[j].ServiceKey
	})
	return records, nil
}

// TouchLocalServiceRegistryRecord updates the lastSeenAt timestamp and optional fields.
func TouchLocalServiceRegistryRecord(serviceKey string, patch *LocalServiceRegistryRecord) (*LocalServiceRegistryRecord, error) {
	existing, err := ReadLocalServiceRegistryRecord(serviceKey)
	if err != nil || existing == nil {
		return nil, err
	}
	existing.LastSeenAt = time.Now().UTC().Format(time.RFC3339)
	if patch != nil {
		if patch.RuntimeServiceID != nil {
			existing.RuntimeServiceID = patch.RuntimeServiceID
		}
		if patch.Port != nil {
			existing.Port = patch.Port
		}
		if patch.URL != nil {
			existing.URL = patch.URL
		}
	}
	if err := WriteLocalServiceRegistryRecord(existing); err != nil {
		return nil, err
	}
	return existing, nil
}

// IsPidAlive returns true if the process is running.
func IsPidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess always succeeds; use signal 0 to check existence.
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

// IsLikelyMatchingCommand checks that the running process at the given PID looks
// like the recorded command (best-effort, only on non-Windows systems).
func IsLikelyMatchingCommand(record *LocalServiceRegistryRecord) bool {
	if record == nil {
		return false
	}
	out, err := exec.Command("ps", "-o", "command=", "-p", fmt.Sprintf("%d", record.PID)).Output()
	if err != nil {
		return true // assume match when we can't check
	}
	commandLine := strings.TrimSpace(string(out))
	if commandLine == "" {
		return false
	}
	normalize := func(s string) string {
		s = strings.ReplaceAll(s, `"`, "")
		s = strings.ReplaceAll(s, `'`, "")
		fields := strings.Fields(s)
		return strings.Join(fields, " ")
	}
	return strings.Contains(normalize(commandLine), normalize(record.Command)) ||
		strings.Contains(commandLine, record.ServiceName)
}

// FindAdoptableLocalService returns a valid registry record if the service is
// still alive and matches the expected identity, otherwise removes the stale
// record and returns nil.
func FindAdoptableLocalService(input LocalServiceIdentityInput) (*LocalServiceRegistryRecord, error) {
	serviceKey := CreateLocalServiceKey(input)
	record, err := ReadLocalServiceRegistryRecord(serviceKey)
	if err != nil || record == nil {
		return nil, err
	}

	if !IsPidAlive(record.PID) {
		_ = RemoveLocalServiceRegistryRecord(serviceKey)
		return nil, nil
	}
	if !IsLikelyMatchingCommand(record) {
		_ = RemoveLocalServiceRegistryRecord(serviceKey)
		return nil, nil
	}
	if input.Command != "" && record.Command != input.Command {
		return nil, nil
	}
	if input.Cwd != "" && filepath.Clean(record.Cwd) != filepath.Clean(input.Cwd) {
		return nil, nil
	}
	if input.EnvFingerprint != "" && record.EnvFingerprint != input.EnvFingerprint {
		return nil, nil
	}
	if input.Port != nil && record.Port != nil && *input.Port != *record.Port {
		return nil, nil
	}
	return record, nil
}

// TerminateLocalService sends SIGTERM to the process (or process group on Unix),
// waits up to forceAfterMs, then sends SIGKILL if still alive.
func TerminateLocalService(pid, processGroupID int, forceAfterMs int) {
	if pid <= 0 {
		return
	}
	if forceAfterMs <= 0 {
		forceAfterMs = 2000
	}
	deadline := time.Now().Add(time.Duration(forceAfterMs) * time.Millisecond)

	// Try to terminate the process group first (so children are also killed).
	if processGroupID > 0 {
		_ = syscall.Kill(-processGroupID, syscall.SIGTERM)
	} else {
		proc, _ := os.FindProcess(pid)
		if proc != nil {
			_ = proc.Signal(syscall.SIGTERM)
		}
	}

	// Poll until dead or deadline.
	for time.Now().Before(deadline) {
		if !IsPidAlive(pid) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !IsPidAlive(pid) {
		return
	}

	// Force-kill.
	if processGroupID > 0 {
		_ = syscall.Kill(-processGroupID, syscall.SIGKILL)
	} else {
		proc, _ := os.FindProcess(pid)
		if proc != nil {
			_ = proc.Signal(syscall.SIGKILL)
		}
	}
}

// ReadLocalServicePortOwner returns the PID of the process listening on the
// given port, or 0 if none is found. Uses lsof on Unix.
func ReadLocalServicePortOwner(port int) int {
	if port <= 0 {
		return 0
	}
	out, err := exec.Command("lsof", "-nPiTCP", fmt.Sprintf(":%d", port), "-sTCP:LISTEN", "-t").Output()
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(out), "\n") {
		var pid int
		if _, err := fmt.Sscanf(strings.TrimSpace(line), "%d", &pid); err == nil && pid > 0 {
			return pid
		}
	}
	return 0
}

// SanitizeRuntimeServiceBaseEnv returns a copy of the environment with
// PAPERCLIP_ variables and database credentials stripped.
func SanitizeRuntimeServiceBaseEnv(env []string) []string {
	result := make([]string, 0, len(env))
	for _, kv := range env {
		key := kv
		if idx := strings.IndexByte(kv, '='); idx >= 0 {
			key = kv[:idx]
		}
		if strings.HasPrefix(key, "PAPERCLIP_") {
			continue
		}
		switch key {
		case "DATABASE_URL", "npm_config_tailscale_auth", "npm_config_authenticated_private":
			continue
		}
		result = append(result, kv)
	}
	return result
}
