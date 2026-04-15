package services

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// runtimeServiceStatus mirrors the Node.js values.
const (
	RuntimeServiceStatusStarting = "starting"
	RuntimeServiceStatusRunning  = "running"
	RuntimeServiceStatusStopped  = "stopped"
	RuntimeServiceStatusFailed   = "failed"
)

// RuntimeServiceRecord is the in-memory representation of a running (or recently
// stopped) local runtime service.
type RuntimeServiceRecord struct {
	ID                   string
	CompanyID            string
	ProjectID            *string
	ProjectWorkspaceID   *string
	ExecutionWorkspaceID *string
	IssueID              *string
	ServiceName          string
	Status               string
	Lifecycle            string
	ScopeType            string
	ScopeID              *string
	ReuseKey             *string
	Command              string
	Cwd                  string
	Port                 *int
	URL                  *string
	Provider             string // "local_process"
	ProviderRef          *string
	OwnerAgentID         *string
	StartedByRunID       *string
	LastUsedAt           string
	StartedAt            string
	StoppedAt            *string
	StopPolicy           map[string]interface{}
	HealthStatus         string
	Reused               bool

	// internal fields
	pid            int
	processGroupID int
	serviceKey     string
	profileKind    string
	cmd            *exec.Cmd
}

// WorkspaceRuntimeServiceRef is the public representation returned by the API.
type WorkspaceRuntimeServiceRef struct {
	ID                   string                 `json:"id"`
	CompanyID            string                 `json:"companyId"`
	ProjectID            *string                `json:"projectId"`
	ProjectWorkspaceID   *string                `json:"projectWorkspaceId"`
	ExecutionWorkspaceID *string                `json:"executionWorkspaceId"`
	IssueID              *string                `json:"issueId"`
	ServiceName          string                 `json:"serviceName"`
	Status               string                 `json:"status"`
	Lifecycle            string                 `json:"lifecycle"`
	ScopeType            string                 `json:"scopeType"`
	ScopeID              *string                `json:"scopeId"`
	ReuseKey             *string                `json:"reuseKey"`
	Command              *string                `json:"command"`
	Cwd                  *string                `json:"cwd"`
	Port                 *int                   `json:"port"`
	URL                  *string                `json:"url"`
	Provider             string                 `json:"provider"`
	ProviderRef          *string                `json:"providerRef"`
	OwnerAgentID         *string                `json:"ownerAgentId"`
	StartedByRunID       *string                `json:"startedByRunId"`
	LastUsedAt           string                 `json:"lastUsedAt"`
	StartedAt            string                 `json:"startedAt"`
	StoppedAt            *string                `json:"stoppedAt"`
	StopPolicy           map[string]interface{} `json:"stopPolicy"`
	HealthStatus         string                 `json:"healthStatus"`
	Reused               bool                   `json:"reused"`
}

// WorkspaceRuntimeManager manages in-memory state for local runtime services.
// It is safe for concurrent use.
type WorkspaceRuntimeManager struct {
	mu              sync.RWMutex
	byID            map[string]*RuntimeServiceRecord
	byReuseKey      map[string]string // reuseKey → serviceID
	db              *gorm.DB
}

// NewWorkspaceRuntimeManager creates a new singleton-friendly manager.
func NewWorkspaceRuntimeManager(db *gorm.DB) *WorkspaceRuntimeManager {
	return &WorkspaceRuntimeManager{
		byID:       make(map[string]*RuntimeServiceRecord),
		byReuseKey: make(map[string]string),
		db:         db,
	}
}

// RuntimeServiceConfig holds the parsed workspaceRuntime configuration.
type RuntimeServiceConfig struct {
	Services []map[string]interface{} `json:"services"`
}

// ParseWorkspaceRuntimeConfig reads services from a raw workspaceRuntime map.
func ParseWorkspaceRuntimeConfig(raw map[string]interface{}) *RuntimeServiceConfig {
	if raw == nil {
		return nil
	}
	cfg := &RuntimeServiceConfig{}
	if svcs, ok := raw["services"].([]interface{}); ok {
		for _, item := range svcs {
			if svc, ok := item.(map[string]interface{}); ok {
				cfg.Services = append(cfg.Services, svc)
			}
		}
	}
	return cfg
}

// ReadExecutionWorkspaceRuntimeConfig extracts the workspaceRuntime config from
// an execution workspace's metadata JSONB column.
// The metadata JSON is expected to have the shape: { "config": { "workspaceRuntime": {...} } }
func ReadExecutionWorkspaceRuntimeConfig(metadataJSON datatypes.JSON) map[string]interface{} {
	if metadataJSON == nil {
		return nil
	}
	var meta map[string]interface{}
	if err := json.Unmarshal(metadataJSON, &meta); err != nil {
		return nil
	}
	cfg, _ := meta["config"].(map[string]interface{})
	if cfg == nil {
		return nil
	}
	wr, _ := cfg["workspaceRuntime"].(map[string]interface{})
	return wr
}

// ReadProjectWorkspaceRuntimeConfig extracts the workspaceRuntime config from
// a project workspace's metadata JSONB column.
// The metadata JSON is expected to have the shape: { "runtimeConfig": { "workspaceRuntime": {...} } }
func ReadProjectWorkspaceRuntimeConfig(metadataJSON datatypes.JSON) map[string]interface{} {
	if metadataJSON == nil {
		return nil
	}
	var meta map[string]interface{}
	if err := json.Unmarshal(metadataJSON, &meta); err != nil {
		return nil
	}
	rc, _ := meta["runtimeConfig"].(map[string]interface{})
	if rc == nil {
		return nil
	}
	wr, _ := rc["workspaceRuntime"].(map[string]interface{})
	return wr
}

// computeEnvFingerprint produces a stable hash of the env configuration block.
func computeEnvFingerprint(envConfig interface{}) string {
	h := sha256.Sum256([]byte(stableStringify(envConfig)))
	return fmt.Sprintf("%x", h)[:16]
}

// resolveServiceName extracts the service name from its config map.
func resolveServiceName(service map[string]interface{}) string {
	if name, ok := service["name"].(string); ok && name != "" {
		return name
	}
	return "service"
}

// resolveServiceCommand extracts the shell command from the service config.
func resolveServiceCommand(service map[string]interface{}) string {
	if cmd, ok := service["command"].(string); ok {
		return cmd
	}
	return ""
}

// resolveServiceCwd resolves the effective working directory for the service.
func resolveServiceCwd(service map[string]interface{}, workspaceCwd string) string {
	if cwd, ok := service["cwd"].(string); ok && cwd != "" {
		if filepath.IsAbs(cwd) {
			return cwd
		}
		return filepath.Join(workspaceCwd, cwd)
	}
	return workspaceCwd
}

// resolveServiceLifecycle returns "shared" or "ephemeral".
func resolveServiceLifecycle(service map[string]interface{}) string {
	if lc, ok := service["lifecycle"].(string); ok && (lc == "shared" || lc == "ephemeral") {
		return lc
	}
	return "shared"
}

// resolveReuseKey builds a stable reuse key from the service identity when a
// lifecycle of "shared" is configured and a name is present.
func resolveReuseKey(service map[string]interface{}, workspaceCwd string) *string {
	lifecycle := resolveServiceLifecycle(service)
	name := resolveServiceName(service)
	if lifecycle != "shared" || name == "" {
		return nil
	}
	h := sha256.Sum256([]byte(stableStringify(map[string]interface{}{
		"name": name,
		"cwd":  filepath.Clean(workspaceCwd),
	})))
	key := fmt.Sprintf("ws-shared-%x", h)[:32]
	return &key
}

// StartRuntimeServicesInput holds all parameters needed to start runtime services.
type StartRuntimeServicesInput struct {
	CompanyID            string
	ProjectID            *string
	ProjectWorkspaceID   *string
	ExecutionWorkspaceID *string
	IssueID              *string
	WorkspaceCwd         string
	RuntimeConfig        map[string]interface{}
	OwnerAgentID         *string
}

// StopResult captures the result of a stop operation.
type StopResult struct {
	StoppedCount int
}

// toRef converts a record to its public representation.
func (m *WorkspaceRuntimeManager) toRef(rec *RuntimeServiceRecord) WorkspaceRuntimeServiceRef {
	var cmd *string
	if rec.Command != "" {
		c := rec.Command
		cmd = &c
	}
	var cwd *string
	if rec.Cwd != "" {
		c := rec.Cwd
		cwd = &c
	}
	return WorkspaceRuntimeServiceRef{
		ID:                   rec.ID,
		CompanyID:            rec.CompanyID,
		ProjectID:            rec.ProjectID,
		ProjectWorkspaceID:   rec.ProjectWorkspaceID,
		ExecutionWorkspaceID: rec.ExecutionWorkspaceID,
		IssueID:              rec.IssueID,
		ServiceName:          rec.ServiceName,
		Status:               rec.Status,
		Lifecycle:            rec.Lifecycle,
		ScopeType:            rec.ScopeType,
		ScopeID:              rec.ScopeID,
		ReuseKey:             rec.ReuseKey,
		Command:              cmd,
		Cwd:                  cwd,
		Port:                 rec.Port,
		URL:                  rec.URL,
		Provider:             rec.Provider,
		ProviderRef:          rec.ProviderRef,
		OwnerAgentID:         rec.OwnerAgentID,
		StartedByRunID:       rec.StartedByRunID,
		LastUsedAt:           rec.LastUsedAt,
		StartedAt:            rec.StartedAt,
		StoppedAt:            rec.StoppedAt,
		StopPolicy:           rec.StopPolicy,
		HealthStatus:         rec.HealthStatus,
		Reused:               rec.Reused,
	}
}

// StartRuntimeServices starts all services defined in the runtime config. If a
// service is already running (via reuse key) it is touched and reused.
func (m *WorkspaceRuntimeManager) StartRuntimeServices(
	ctx context.Context,
	input StartRuntimeServicesInput,
	onLog func(stream, chunk string),
) ([]WorkspaceRuntimeServiceRef, error) {
	cfg := ParseWorkspaceRuntimeConfig(input.RuntimeConfig)
	if cfg == nil || len(cfg.Services) == 0 {
		return nil, nil
	}

	var refs []WorkspaceRuntimeServiceRef

	for _, svc := range cfg.Services {
		ref, err := m.startOneService(ctx, svc, input, onLog)
		if err != nil {
			// Stop already-started services on failure (best-effort).
			for _, started := range refs {
				_ = m.stopServiceByID(ctx, started.ID)
			}
			return nil, err
		}
		refs = append(refs, ref)
	}

	return refs, nil
}

// startOneService starts a single service, reusing an existing instance if possible.
func (m *WorkspaceRuntimeManager) startOneService(
	ctx context.Context,
	svc map[string]interface{},
	input StartRuntimeServicesInput,
	onLog func(stream, chunk string),
) (WorkspaceRuntimeServiceRef, error) {
	serviceName := resolveServiceName(svc)
	command := resolveServiceCommand(svc)
	if command == "" {
		return WorkspaceRuntimeServiceRef{}, fmt.Errorf("runtime service %q is missing a command", serviceName)
	}

	workspaceCwd := input.WorkspaceCwd
	if workspaceCwd == "" {
		workspaceCwd, _ = os.Getwd()
	}
	serviceCwd := resolveServiceCwd(svc, workspaceCwd)
	lifecycle := resolveServiceLifecycle(svc)
	reuseKey := resolveReuseKey(svc, workspaceCwd)
	envConfig := svc["env"]
	envFingerprint := computeEnvFingerprint(envConfig)

	// Attempt reuse of an already-running service.
	if reuseKey != nil {
		m.mu.RLock()
		existingID, hasReuseKey := m.byReuseKey[*reuseKey]
		m.mu.RUnlock()
		if hasReuseKey {
			m.mu.Lock()
			existing, stillThere := m.byID[existingID]
			if stillThere && existing.Status == RuntimeServiceStatusRunning {
				existing.LastUsedAt = time.Now().UTC().Format(time.RFC3339)
				existing.Reused = true
				m.mu.Unlock()
				_ = m.persistRecord(ctx, existing)
				return m.toRef(existing), nil
			}
			m.mu.Unlock()
		}
	}

	// Try to adopt a live process from the disk registry.
	adoptInput := LocalServiceIdentityInput{
		ProfileKind:    "workspace-runtime",
		ServiceName:    serviceName,
		Cwd:            serviceCwd,
		Command:        command,
		EnvFingerprint: envFingerprint,
		Scope:          buildServiceScope(input),
	}
	adopted, err := FindAdoptableLocalService(adoptInput)
	if err != nil {
		return WorkspaceRuntimeServiceRef{}, fmt.Errorf("failed to check for adoptable service %q: %w", serviceName, err)
	}

	if adopted != nil {
		rec := m.buildAdoptedRecord(adopted, svc, input, reuseKey, lifecycle, serviceName, command, serviceCwd, envFingerprint)
		m.mu.Lock()
		m.byID[rec.ID] = rec
		if reuseKey != nil {
			m.byReuseKey[*reuseKey] = rec.ID
		}
		m.mu.Unlock()
		_ = m.persistRecord(ctx, rec)
		return m.toRef(rec), nil
	}

	// Check port availability before spawning.
	if portVal, hasPort := resolveServicePort(svc); hasPort && portVal > 0 {
		ownerPID := ReadLocalServicePortOwner(portVal)
		if ownerPID > 0 {
			return WorkspaceRuntimeServiceRef{}, fmt.Errorf(
				"runtime service %q cannot start: port %d is already in use by pid %d",
				serviceName, portVal, ownerPID,
			)
		}
	}

	// Spawn a new process.
	rec, err := m.spawnService(ctx, svc, input, reuseKey, lifecycle, serviceName, command, serviceCwd, envFingerprint, onLog)
	if err != nil {
		return WorkspaceRuntimeServiceRef{}, err
	}

	m.mu.Lock()
	m.byID[rec.ID] = rec
	if reuseKey != nil {
		m.byReuseKey[*reuseKey] = rec.ID
	}
	m.mu.Unlock()

	_ = m.persistRecord(ctx, rec)
	return m.toRef(rec), nil
}

func buildServiceScope(input StartRuntimeServicesInput) map[string]interface{} {
	scope := map[string]interface{}{
		"projectWorkspaceId":   nil,
		"executionWorkspaceId": nil,
	}
	if input.ProjectWorkspaceID != nil {
		scope["projectWorkspaceId"] = *input.ProjectWorkspaceID
	}
	if input.ExecutionWorkspaceID != nil {
		scope["executionWorkspaceId"] = *input.ExecutionWorkspaceID
	}
	return scope
}

func resolveServicePort(svc map[string]interface{}) (int, bool) {
	portCfg, _ := svc["port"].(map[string]interface{})
	if portCfg == nil {
		return 0, false
	}
	if portType, _ := portCfg["type"].(string); portType == "auto" {
		return 0, false
	}
	if num, ok := portCfg["number"].(float64); ok && num > 0 {
		return int(num), true
	}
	return 0, false
}

func (m *WorkspaceRuntimeManager) buildAdoptedRecord(
	adopted *LocalServiceRegistryRecord,
	svc map[string]interface{},
	input StartRuntimeServicesInput,
	reuseKey *string,
	lifecycle, serviceName, command, serviceCwd, envFingerprint string,
) *RuntimeServiceRecord {
	now := time.Now().UTC().Format(time.RFC3339)
	id := uuid.New().String()
	if adopted.RuntimeServiceID != nil && *adopted.RuntimeServiceID != "" {
		id = *adopted.RuntimeServiceID
	}

	var pgid int
	if adopted.ProcessGroupID != nil {
		pgid = *adopted.ProcessGroupID
	}

	providerRef := fmt.Sprintf("%d", adopted.PID)
	scopeType := "execution_workspace"
	if input.ExecutionWorkspaceID == nil {
		scopeType = "project_workspace"
	}
	var scopeID *string
	if input.ExecutionWorkspaceID != nil {
		scopeID = input.ExecutionWorkspaceID
	} else {
		scopeID = input.ProjectWorkspaceID
	}

	stopPolicy, _ := svc["stopPolicy"].(map[string]interface{})

	return &RuntimeServiceRecord{
		ID:                   id,
		CompanyID:            input.CompanyID,
		ProjectID:            input.ProjectID,
		ProjectWorkspaceID:   input.ProjectWorkspaceID,
		ExecutionWorkspaceID: input.ExecutionWorkspaceID,
		IssueID:              input.IssueID,
		ServiceName:          serviceName,
		Status:               RuntimeServiceStatusRunning,
		Lifecycle:            lifecycle,
		ScopeType:            scopeType,
		ScopeID:              scopeID,
		ReuseKey:             reuseKey,
		Command:              command,
		Cwd:                  serviceCwd,
		Port:                 adopted.Port,
		URL:                  adopted.URL,
		Provider:             "local_process",
		ProviderRef:          &providerRef,
		OwnerAgentID:         input.OwnerAgentID,
		LastUsedAt:           now,
		StartedAt:            adopted.StartedAt,
		StopPolicy:           stopPolicy,
		HealthStatus:         "healthy",
		Reused:               true,
		pid:                  adopted.PID,
		processGroupID:       pgid,
		serviceKey:           adopted.ServiceKey,
		profileKind:          "workspace-runtime",
	}
}

func (m *WorkspaceRuntimeManager) spawnService(
	ctx context.Context,
	svc map[string]interface{},
	input StartRuntimeServicesInput,
	reuseKey *string,
	lifecycle, serviceName, command, serviceCwd, envFingerprint string,
	onLog func(stream, chunk string),
) (*RuntimeServiceRecord, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	shell := resolveShell()
	// Build process environment: inherit sanitised env + service-specific overrides.
	processEnv := SanitizeRuntimeServiceBaseEnv(os.Environ())
	if envMap, ok := svc["env"].(map[string]interface{}); ok {
		for k, v := range envMap {
			processEnv = append(processEnv, fmt.Sprintf("%s=%v", k, v))
		}
	}

	cmd := exec.CommandContext(ctx, shell, "-lc", command)
	cmd.Dir = serviceCwd
	cmd.Env = processEnv
	// Use a process group so we can kill the whole group later.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start runtime service %q: %w", serviceName, err)
	}

	pid := cmd.Process.Pid

	// Stream output asynchronously.
	if onLog != nil {
		go pipeOutput(stdout, "stdout", serviceName, onLog)
		go pipeOutput(stderr, "stderr", serviceName, onLog)
	}

	// Wait for readiness if a URL is configured.
	url := resolveServiceURL(svc)
	if url != "" {
		if err := waitForReadinessURL(ctx, url, 30*time.Second); err != nil {
			// Kill the process if readiness check times out.
			TerminateLocalService(pid, pid, 2000)
			return nil, fmt.Errorf("runtime service %q failed readiness check: %w", serviceName, err)
		}
	}

	serviceKey := CreateLocalServiceKey(LocalServiceIdentityInput{
		ProfileKind:    "workspace-runtime",
		ServiceName:    serviceName,
		Cwd:            serviceCwd,
		Command:        command,
		EnvFingerprint: envFingerprint,
		Scope:          buildServiceScope(input),
	})

	id := uuid.New().String()
	providerRef := fmt.Sprintf("%d", pid)

	scopeType := "execution_workspace"
	if input.ExecutionWorkspaceID == nil {
		scopeType = "project_workspace"
	}
	var scopeID *string
	if input.ExecutionWorkspaceID != nil {
		scopeID = input.ExecutionWorkspaceID
	} else {
		scopeID = input.ProjectWorkspaceID
	}

	stopPolicy, _ := svc["stopPolicy"].(map[string]interface{})
	var urlPtr *string
	if url != "" {
		urlPtr = &url
	}

	rec := &RuntimeServiceRecord{
		ID:                   id,
		CompanyID:            input.CompanyID,
		ProjectID:            input.ProjectID,
		ProjectWorkspaceID:   input.ProjectWorkspaceID,
		ExecutionWorkspaceID: input.ExecutionWorkspaceID,
		IssueID:              input.IssueID,
		ServiceName:          serviceName,
		Status:               RuntimeServiceStatusRunning,
		Lifecycle:            lifecycle,
		ScopeType:            scopeType,
		ScopeID:              scopeID,
		ReuseKey:             reuseKey,
		Command:              command,
		Cwd:                  serviceCwd,
		URL:                  urlPtr,
		Provider:             "local_process",
		ProviderRef:          &providerRef,
		OwnerAgentID:         input.OwnerAgentID,
		LastUsedAt:           now,
		StartedAt:            now,
		StopPolicy:           stopPolicy,
		HealthStatus:         "healthy",
		Reused:               false,
		pid:                  pid,
		processGroupID:       pid,
		serviceKey:           serviceKey,
		profileKind:          "workspace-runtime",
		cmd:                  cmd,
	}

	// Write disk registry record.
	_ = WriteLocalServiceRegistryRecord(&LocalServiceRegistryRecord{
		Version:          1,
		ServiceKey:       serviceKey,
		ProfileKind:      "workspace-runtime",
		ServiceName:      serviceName,
		Command:          command,
		Cwd:              serviceCwd,
		EnvFingerprint:   envFingerprint,
		PID:              pid,
		ProcessGroupID:   &pid,
		Provider:         "local_process",
		RuntimeServiceID: &id,
		ReuseKey:         reuseKey,
		StartedAt:        now,
		LastSeenAt:       now,
		Metadata: map[string]interface{}{
			"projectWorkspaceId":   input.ProjectWorkspaceID,
			"executionWorkspaceId": input.ExecutionWorkspaceID,
		},
	})

	// Reap the process asynchronously so we capture its exit.
	go func() {
		_ = cmd.Wait()
		m.mu.Lock()
		current, stillThere := m.byID[id]
		if stillThere {
			if current.Status == RuntimeServiceStatusRunning {
				current.Status = RuntimeServiceStatusFailed
				current.HealthStatus = "unhealthy"
				current.StoppedAt = runtimeStrPtr(time.Now().UTC().Format(time.RFC3339))
			}
			delete(m.byID, id)
			if reuseKey != nil && m.byReuseKey[*reuseKey] == id {
				delete(m.byReuseKey, *reuseKey)
			}
		}
		m.mu.Unlock()
		if stillThere && current != nil {
			_ = RemoveLocalServiceRegistryRecord(serviceKey)
			_ = m.persistRecord(context.Background(), current)
		}
	}()

	return rec, nil
}

func resolveShell() string {
	if sh := strings.TrimSpace(os.Getenv("SHELL")); sh != "" {
		return sh
	}
	return "/bin/sh"
}

func resolveServiceURL(svc map[string]interface{}) string {
	if expose, ok := svc["expose"].(map[string]interface{}); ok {
		if tmpl, ok := expose["urlTemplate"].(string); ok && tmpl != "" {
			return tmpl
		}
	}
	if readiness, ok := svc["readiness"].(map[string]interface{}); ok {
		if tmpl, ok := readiness["urlTemplate"].(string); ok && tmpl != "" {
			return tmpl
		}
	}
	return ""
}

// StopRuntimeServicesForExecutionWorkspace stops all in-memory services that
// belong to the given execution workspace, then marks any remaining persisted
// records as stopped in the database.
func (m *WorkspaceRuntimeManager) StopRuntimeServicesForExecutionWorkspace(
	ctx context.Context,
	executionWorkspaceID, workspaceCwd string,
) (int, error) {
	m.mu.Lock()
	var toStop []string
	for id, rec := range m.byID {
		if rec.ExecutionWorkspaceID != nil && *rec.ExecutionWorkspaceID == executionWorkspaceID {
			toStop = append(toStop, id)
			continue
		}
		// Also stop services whose cwd is inside the workspace directory.
		if workspaceCwd != "" && rec.Cwd != "" {
			cleaned := filepath.Clean(workspaceCwd)
			if filepath.Clean(rec.Cwd) == cleaned ||
				strings.HasPrefix(filepath.Clean(rec.Cwd), cleaned+string(os.PathSeparator)) {
				toStop = append(toStop, id)
			}
		}
	}
	m.mu.Unlock()

	for _, id := range toStop {
		_ = m.stopServiceByID(ctx, id)
	}

	// Mark any persisted records that are still showing as running.
	_ = m.markStoppedInDB(ctx, "execution_workspace_id = ? AND status IN ('starting','running')", executionWorkspaceID)

	return len(toStop), nil
}

// StopRuntimeServicesForProjectWorkspace stops all in-memory services scoped to
// the given project workspace, then marks DB records as stopped.
func (m *WorkspaceRuntimeManager) StopRuntimeServicesForProjectWorkspace(
	ctx context.Context,
	projectWorkspaceID string,
) (int, error) {
	m.mu.Lock()
	var toStop []string
	for id, rec := range m.byID {
		if rec.ProjectWorkspaceID != nil && *rec.ProjectWorkspaceID == projectWorkspaceID &&
			rec.ScopeType == "project_workspace" {
			toStop = append(toStop, id)
		}
	}
	m.mu.Unlock()

	for _, id := range toStop {
		_ = m.stopServiceByID(ctx, id)
	}

	_ = m.markStoppedInDB(ctx,
		"project_workspace_id = ? AND scope_type = 'project_workspace' AND status IN ('starting','running')",
		projectWorkspaceID,
	)

	return len(toStop), nil
}

func (m *WorkspaceRuntimeManager) stopServiceByID(ctx context.Context, id string) error {
	m.mu.Lock()
	rec, ok := m.byID[id]
	if !ok {
		m.mu.Unlock()
		return nil
	}
	rec.Status = RuntimeServiceStatusStopped
	rec.HealthStatus = "unknown"
	now := time.Now().UTC().Format(time.RFC3339)
	rec.LastUsedAt = now
	rec.StoppedAt = &now
	delete(m.byID, id)
	if rec.ReuseKey != nil && m.byReuseKey[*rec.ReuseKey] == id {
		delete(m.byReuseKey, *rec.ReuseKey)
	}
	m.mu.Unlock()

	// Terminate the process.
	pgid := rec.processGroupID
	if pgid <= 0 {
		pgid = rec.pid
	}
	if rec.pid > 0 {
		TerminateLocalService(rec.pid, pgid, 2000)
	} else if rec.ProviderRef != nil {
		var pid int
		fmt.Sscanf(*rec.ProviderRef, "%d", &pid)
		if pid > 0 {
			TerminateLocalService(pid, pid, 2000)
		}
	}

	_ = RemoveLocalServiceRegistryRecord(rec.serviceKey)
	_ = m.persistRecord(ctx, rec)
	return nil
}

func (m *WorkspaceRuntimeManager) markStoppedInDB(ctx context.Context, condition string, args ...interface{}) error {
	if m.db == nil {
		return nil
	}
	now := time.Now()
	return m.db.WithContext(ctx).
		Model(&models.WorkspaceRuntimeService{}).
		Where(condition, args...).
		Updates(map[string]interface{}{
			"status":       RuntimeServiceStatusStopped,
			"health_status": "unknown",
			"stopped_at":   now,
			"last_used_at": now,
			"updated_at":   now,
		}).Error
}

func (m *WorkspaceRuntimeManager) persistRecord(ctx context.Context, rec *RuntimeServiceRecord) error {
	if m.db == nil {
		return nil
	}

	stopPolicyJSON, _ := json.Marshal(rec.StopPolicy)
	var stoppedAt *time.Time
	if rec.StoppedAt != nil {
		t, _ := time.Parse(time.RFC3339, *rec.StoppedAt)
		stoppedAt = &t
	}
	startedAt, _ := time.Parse(time.RFC3339, rec.StartedAt)
	if startedAt.IsZero() {
		startedAt = time.Now()
	}
	lastUsedAt, _ := time.Parse(time.RFC3339, rec.LastUsedAt)
	if lastUsedAt.IsZero() {
		lastUsedAt = time.Now()
	}

	row := models.WorkspaceRuntimeService{
		ID:                   rec.ID,
		CompanyID:            rec.CompanyID,
		ProjectID:            rec.ProjectID,
		ProjectWorkspaceID:   rec.ProjectWorkspaceID,
		ExecutionWorkspaceID: rec.ExecutionWorkspaceID,
		IssueID:              rec.IssueID,
		ServiceName:          rec.ServiceName,
		Status:               rec.Status,
		Lifecycle:            rec.Lifecycle,
		ScopeType:            rec.ScopeType,
		ScopeID:              rec.ScopeID,
		ReuseKey:             rec.ReuseKey,
		Command:              runtimeStrPtr(rec.Command),
		Cwd:                  runtimeStrPtr(rec.Cwd),
		URL:                  rec.URL,
		Provider:             rec.Provider,
		ProviderRef:          rec.ProviderRef,
		OwnerAgentID:         rec.OwnerAgentID,
		StartedByRunID:       rec.StartedByRunID,
		LastUsedAt:           lastUsedAt,
		StartedAt:            startedAt,
		StoppedAt:            stoppedAt,
		StopPolicy:           datatypes.JSON(stopPolicyJSON),
		HealthStatus:         rec.HealthStatus,
	}

	return m.db.WithContext(ctx).
		Where(models.WorkspaceRuntimeService{ID: rec.ID}).
		Assign(row).
		FirstOrCreate(&row).Error
}

func runtimeStrPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
