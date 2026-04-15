package services

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// JSON-RPC 2.0 constants
// ---------------------------------------------------------------------------

const jsonrpcVersion = "2.0"

// Standard JSON-RPC 2.0 error codes.
const (
	jsonrpcErrParseError     = -32700
	jsonrpcErrMethodNotFound = -32601
	jsonrpcErrInternalError  = -32603
)

// Plugin-specific error codes (mirror the TypeScript SDK).
const (
	pluginErrWorkerUnavailable    = -32001
	pluginErrTimeout              = -32002
	pluginErrMethodNotImplemented = -32004
)

// ---------------------------------------------------------------------------
// Timing constants (mirror TypeScript values)
// ---------------------------------------------------------------------------

const (
	rpcDefaultTimeout  = 30 * time.Second
	rpcMaxTimeout      = 5 * time.Minute
	rpcInitTimeout     = 15 * time.Second
	rpcShutdownDrain   = 10 * time.Second
	rpcSigtermGrace    = 5 * time.Second
	workerMinBackoff   = 1 * time.Second
	workerMaxBackoff   = 5 * time.Minute
	workerBackoffMult  = 2.0
	workerBackoffJitter = 0.25
	workerMaxCrashes   = 10
	workerCrashWindow  = 10 * time.Minute
	// stdoutScanBufSize is the per-line scanner buffer (4 MB).
	stdoutScanBufSize = 4 * 1024 * 1024
)

// ---------------------------------------------------------------------------
// JSON-RPC message types
// ---------------------------------------------------------------------------

// rpcMsg is a generic JSON-RPC 2.0 envelope. Fields are selectively populated
// depending on whether the message is a request, response, or notification.
type rpcMsg struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcErr         `json:"error,omitempty"`
}

// rpcErr is a JSON-RPC 2.0 error object.
type rpcErr struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *rpcErr) Error() string {
	return fmt.Sprintf("RPC error %d: %s", e.Code, e.Message)
}

// normaliseID converts a JSON-decoded ID (float64, string, int64) to int64.
// Returns -1 if the value cannot be normalised to a positive integer.
func normaliseID(raw interface{}) (int64, bool) {
	switch v := raw.(type) {
	case float64:
		return int64(v), true
	case int64:
		return v, true
	case json.Number:
		n, err := v.Int64()
		return n, err == nil
	}
	return -1, false
}

// pendingCall tracks an in-flight host→worker RPC call.
type pendingCall struct {
	id     int64
	method string
	ch     chan callResult
}

type callResult struct {
	result json.RawMessage
	err    error
}

// ---------------------------------------------------------------------------
// WorkerStatus
// ---------------------------------------------------------------------------

// WorkerStatus describes the current state of a plugin worker process.
type WorkerStatus string

const (
	WorkerStatusStopped  WorkerStatus = "stopped"
	WorkerStatusStarting WorkerStatus = "starting"
	WorkerStatusRunning  WorkerStatus = "running"
	WorkerStatusStopping WorkerStatus = "stopping"
	WorkerStatusCrashed  WorkerStatus = "crashed"
	WorkerStatusBackoff  WorkerStatus = "backoff"
)

// ---------------------------------------------------------------------------
// WorkerDiagnostics
// ---------------------------------------------------------------------------

// WorkerDiagnostics is a snapshot of worker state for monitoring.
type WorkerDiagnostics struct {
	PluginID           string       `json:"pluginId"`
	Status             WorkerStatus `json:"status"`
	PID                int          `json:"pid"`
	UptimeMs           *int64       `json:"uptimeMs,omitempty"`
	ConsecutiveCrashes int          `json:"consecutiveCrashes"`
	TotalCrashes       int          `json:"totalCrashes"`
	PendingRequests    int          `json:"pendingRequests"`
}

// ---------------------------------------------------------------------------
// WorkerHandle — manages a single plugin worker process
// ---------------------------------------------------------------------------

// WorkerHandle manages the lifecycle and RPC communication of one plugin worker
// Node.js process.
type WorkerHandle struct {
	manager  *PluginWorkerManager
	pluginID string

	mu             sync.Mutex
	status         WorkerStatus
	cmd            *exec.Cmd
	stdinMu        sync.Mutex
	stdinPipe      io.WriteCloser
	startedAt      time.Time
	intentionalStop bool

	nextID   atomic.Int64
	pendMu   sync.Mutex
	pending  map[int64]*pendingCall

	supportedMethods []string

	// openChannels maps channel → companyID for cleanup on crash.
	openChMu     sync.Mutex
	openChannels map[string]string

	// Crash recovery state.
	consecutiveCrashes int
	totalCrashes       int
	lastCrashAt        time.Time
	backoffTimerMu     sync.Mutex
	backoffTimer       *time.Timer
	nextRestartAt      time.Time
}

func newWorkerHandle(mgr *PluginWorkerManager, pluginID string) *WorkerHandle {
	return &WorkerHandle{
		manager:      mgr,
		pluginID:     pluginID,
		status:       WorkerStatusStopped,
		pending:      make(map[int64]*pendingCall),
		openChannels: make(map[string]string),
	}
}

func (h *WorkerHandle) setStatus(s WorkerStatus) {
	h.mu.Lock()
	h.status = s
	h.mu.Unlock()
}

func (h *WorkerHandle) getStatus() WorkerStatus {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.status
}

// send writes a JSON-RPC message as a single NDJSON line to the worker stdin.
func (h *WorkerHandle) send(msg rpcMsg) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal RPC message: %w", err)
	}
	data = append(data, '\n')

	h.stdinMu.Lock()
	defer h.stdinMu.Unlock()
	if h.stdinPipe == nil {
		return fmt.Errorf("worker stdin is closed")
	}
	_, err = h.stdinPipe.Write(data)
	return err
}

// Call sends a typed RPC request to the worker and blocks until the response
// arrives, the context is cancelled, or the call times out.
func (h *WorkerHandle) Call(ctx context.Context, method string, params interface{}, timeout time.Duration) (json.RawMessage, error) {
	h.mu.Lock()
	st := h.status
	h.mu.Unlock()
	if st != WorkerStatusRunning && st != WorkerStatusStarting {
		return nil, fmt.Errorf("worker %q is not running (status: %s)", h.pluginID, st)
	}

	if timeout <= 0 {
		timeout = rpcDefaultTimeout
	}
	if timeout > rpcMaxTimeout {
		timeout = rpcMaxTimeout
	}

	paramsRaw, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal params for %q: %w", method, err)
	}

	id := h.nextID.Add(1)
	ch := make(chan callResult, 1)
	call := &pendingCall{id: id, method: method, ch: ch}

	h.pendMu.Lock()
	h.pending[id] = call
	h.pendMu.Unlock()

	// Register timeout.
	timer := time.AfterFunc(timeout, func() {
		h.pendMu.Lock()
		if _, ok := h.pending[id]; ok {
			delete(h.pending, id)
			h.pendMu.Unlock()
			select {
			case ch <- callResult{err: &rpcErr{Code: pluginErrTimeout, Message: fmt.Sprintf("RPC %q timed out after %s", method, timeout)}}:
			default:
			}
		} else {
			h.pendMu.Unlock()
		}
	})

	if sendErr := h.send(rpcMsg{
		JSONRPC: jsonrpcVersion,
		ID:      id,
		Method:  method,
		Params:  paramsRaw,
	}); sendErr != nil {
		timer.Stop()
		h.pendMu.Lock()
		delete(h.pending, id)
		h.pendMu.Unlock()
		return nil, fmt.Errorf("failed to send %q to worker: %w", method, sendErr)
	}

	select {
	case <-ctx.Done():
		timer.Stop()
		h.pendMu.Lock()
		delete(h.pending, id)
		h.pendMu.Unlock()
		return nil, ctx.Err()
	case res := <-ch:
		timer.Stop()
		return res.result, res.err
	}
}

// notify sends a fire-and-forget notification (no response expected).
func (h *WorkerHandle) notify(method string, params interface{}) {
	paramsRaw, err := json.Marshal(params)
	if err != nil {
		return
	}
	_ = h.send(rpcMsg{
		JSONRPC: jsonrpcVersion,
		Method:  method,
		Params:  paramsRaw,
	})
}

// rejectAllPending cancels all in-flight RPC calls with the given error.
func (h *WorkerHandle) rejectAllPending(err error) {
	h.pendMu.Lock()
	pending := h.pending
	h.pending = make(map[int64]*pendingCall)
	h.pendMu.Unlock()

	for _, p := range pending {
		select {
		case p.ch <- callResult{err: err}:
		default:
		}
	}
}

// ---------------------------------------------------------------------------
// readLoop — runs in a goroutine, consumes worker stdout line-by-line
// ---------------------------------------------------------------------------

func (h *WorkerHandle) readLoop(stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	buf := make([]byte, stdoutScanBufSize)
	scanner.Buffer(buf, stdoutScanBufSize)

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		dec := json.NewDecoder(bytes.NewReader(line))
		dec.UseNumber()
		var msg rpcMsg
		if err := dec.Decode(&msg); err != nil {
			slog.Warn("[plugin-worker] unparseable message from worker", "pluginId", h.pluginID, "line", string(line[:min(len(line), 200)]))
			continue
		}

		hasID := msg.ID != nil
		hasMethod := msg.Method != ""

		switch {
		case hasID && !hasMethod:
			// Response to a host→worker request
			h.handleResponse(&msg)
		case hasMethod && hasID:
			// Worker→host request (expects a response)
			go h.handleWorkerRequest(&msg)
		case hasMethod && !hasID:
			// Worker→host notification (no response needed)
			go h.handleWorkerNotification(&msg)
		}
	}
}

func (h *WorkerHandle) handleResponse(msg *rpcMsg) {
	id, ok := normaliseID(msg.ID)
	if !ok {
		slog.Warn("[plugin-worker] received response with non-integer id", "pluginId", h.pluginID)
		return
	}

	h.pendMu.Lock()
	call, exists := h.pending[id]
	if exists {
		delete(h.pending, id)
	}
	h.pendMu.Unlock()

	if !exists {
		return
	}

	var res callResult
	if msg.Error != nil {
		res.err = msg.Error
	} else {
		res.result = msg.Result
	}
	select {
	case call.ch <- res:
	default:
	}
}

func (h *WorkerHandle) handleWorkerRequest(msg *rpcMsg) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, rpcError := h.manager.dispatchHostMethod(ctx, h.pluginID, msg.Method, msg.Params)

	resp := rpcMsg{JSONRPC: jsonrpcVersion, ID: msg.ID}
	if rpcError != nil {
		resp.Error = rpcError
	} else {
		raw, _ := json.Marshal(result)
		resp.Result = raw
	}
	if err := h.send(resp); err != nil {
		slog.Warn("[plugin-worker] failed to send host response", "pluginId", h.pluginID, "method", msg.Method, "err", err)
	}
}

func (h *WorkerHandle) handleWorkerNotification(msg *rpcMsg) {
	switch msg.Method {
	case "log":
		var params struct {
			Level   string                 `json:"level"`
			Message string                 `json:"message"`
			Meta    map[string]interface{} `json:"meta"`
		}
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return
		}
		level := params.Level
		text := fmt.Sprintf("[plugin:%s] %s", h.pluginID, params.Message)
		switch level {
		case "error":
			slog.Error(text, "pluginId", h.pluginID)
		case "warn":
			slog.Warn(text, "pluginId", h.pluginID)
		case "debug":
			slog.Debug(text, "pluginId", h.pluginID)
		default:
			slog.Info(text, "pluginId", h.pluginID)
		}
		// Persist log to DB
		h.manager.persistWorkerLog(h.pluginID, level, params.Message, params.Meta)

	case "streams.open", "streams.emit", "streams.close":
		var params map[string]interface{}
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return
		}
		channel, _ := params["channel"].(string)
		companyID, _ := params["companyId"].(string)

		switch msg.Method {
		case "streams.open":
			if channel != "" {
				h.openChMu.Lock()
				h.openChannels[channel] = companyID
				h.openChMu.Unlock()
			}
			if h.manager.streamBus != nil && channel != "" {
				h.manager.streamBus.Publish(h.pluginID, channel, companyID, params, StreamEventOpen)
			}
		case "streams.emit":
			if h.manager.streamBus != nil && channel != "" {
				h.manager.streamBus.Publish(h.pluginID, channel, companyID, params, StreamEventMessage)
			}
		case "streams.close":
			h.openChMu.Lock()
			delete(h.openChannels, channel)
			h.openChMu.Unlock()
			if h.manager.streamBus != nil && channel != "" {
				h.manager.streamBus.Publish(h.pluginID, channel, companyID, params, StreamEventClose)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Process lifecycle — start / stop
// ---------------------------------------------------------------------------

func (h *WorkerHandle) start(ctx context.Context, entrypoint string) error {
	h.mu.Lock()
	if h.status == WorkerStatusRunning || h.status == WorkerStatusStarting {
		h.mu.Unlock()
		return fmt.Errorf("worker for %q is already %s", h.pluginID, h.status)
	}
	h.intentionalStop = false
	h.status = WorkerStatusStarting
	h.mu.Unlock()

	// Build minimal worker environment (security: don't leak host env).
	workerEnv := []string{
		"PAPERCLIP_PLUGIN_ID=" + h.pluginID,
		"NODE_ENV=" + getEnvOr("NODE_ENV", "production"),
		"TZ=" + getEnvOr("TZ", "UTC"),
	}
	if path := os.Getenv("PATH"); path != "" {
		workerEnv = append(workerEnv, "PATH="+path)
	}
	if nodePath := os.Getenv("NODE_PATH"); nodePath != "" {
		workerEnv = append(workerEnv, "NODE_PATH="+nodePath)
	}

	cmd := exec.CommandContext(ctx, "node", entrypoint)
	cmd.Env = workerEnv

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		h.setStatus(WorkerStatusCrashed)
		return fmt.Errorf("failed to create stdin pipe for plugin %q: %w", h.pluginID, err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		stdinPipe.Close()
		h.setStatus(WorkerStatusCrashed)
		return fmt.Errorf("failed to create stdout pipe for plugin %q: %w", h.pluginID, err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		stdinPipe.Close()
		stdoutPipe.Close()
		h.setStatus(WorkerStatusCrashed)
		return fmt.Errorf("failed to create stderr pipe for plugin %q: %w", h.pluginID, err)
	}

	if err := cmd.Start(); err != nil {
		stdinPipe.Close()
		h.setStatus(WorkerStatusCrashed)
		return fmt.Errorf("failed to start plugin worker %q: %w", h.pluginID, err)
	}

	h.mu.Lock()
	h.cmd = cmd
	h.stdinPipe = stdinPipe
	h.startedAt = time.Now()
	h.mu.Unlock()

	// Start stderr logger goroutine.
	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			slog.Warn("[plugin-worker stderr]", "pluginId", h.pluginID, "line", scanner.Text())
		}
	}()

	// Start stdout read loop goroutine.
	go func() {
		h.readLoop(stdoutPipe)
		// stdout closed — process exited.
		waitErr := cmd.Wait()
		h.onProcessExit(cmd.ProcessState.ExitCode(), waitErr)
	}()

	// Send initialize RPC.
	config, manifest := h.manager.loadPluginConfigAndManifest(ctx, h.pluginID)

	instanceID := getEnvOr("PAPERCLIP_INSTANCE_ID", "local")
	hostVersion := getEnvOr("PAPERCLIP_VERSION", "0.0.0")

	initParams := map[string]interface{}{
		"manifest": manifest,
		"config":   config,
		"instanceInfo": map[string]interface{}{
			"instanceId":  instanceID,
			"hostVersion": hostVersion,
		},
		"apiVersion": 1,
	}

	raw, err := h.Call(ctx, "initialize", initParams, rpcInitTimeout)
	if err != nil {
		slog.Error("[plugin-worker] initialize failed", "pluginId", h.pluginID, "err", err)
		h.killProcess()
		h.setStatus(WorkerStatusCrashed)
		return fmt.Errorf("plugin worker initialize failed for %q: %w", h.pluginID, err)
	}

	var initResult struct {
		OK               bool     `json:"ok"`
		SupportedMethods []string `json:"supportedMethods"`
	}
	if err := json.Unmarshal(raw, &initResult); err == nil {
		h.mu.Lock()
		h.supportedMethods = initResult.SupportedMethods
		h.mu.Unlock()
	}

	h.mu.Lock()
	h.consecutiveCrashes = 0
	h.status = WorkerStatusRunning
	h.mu.Unlock()

	slog.Info("[plugin-worker] started and initialized", "pluginId", h.pluginID, "pid", cmd.Process.Pid)
	return nil
}

func (h *WorkerHandle) onProcessExit(exitCode int, waitErr error) {
	h.mu.Lock()
	wasIntentional := h.intentionalStop
	h.stdinPipe = nil
	h.cmd = nil
	h.mu.Unlock()

	rejectErr := fmt.Errorf("worker process for plugin %q exited (code=%d)", h.pluginID, exitCode)
	h.rejectAllPending(rejectErr)

	// Emit synthetic close for any open stream channels.
	if h.manager.streamBus != nil {
		h.openChMu.Lock()
		channels := make(map[string]string, len(h.openChannels))
		for ch, co := range h.openChannels {
			channels[ch] = co
		}
		h.openChannels = make(map[string]string)
		h.openChMu.Unlock()
		for channel, companyID := range channels {
			h.manager.streamBus.Publish(h.pluginID, channel, companyID, map[string]interface{}{
				"channel":   channel,
				"companyId": companyID,
			}, StreamEventClose)
		}
	}

	if wasIntentional {
		h.setStatus(WorkerStatusStopped)
		slog.Info("[plugin-worker] stopped", "pluginId", h.pluginID, "exitCode", exitCode)
		return
	}

	// Unexpected exit — crash recovery.
	now := time.Now()
	h.mu.Lock()
	h.totalCrashes++
	if !h.lastCrashAt.IsZero() && now.Sub(h.lastCrashAt) > workerCrashWindow {
		h.consecutiveCrashes = 0
	}
	h.consecutiveCrashes++
	h.lastCrashAt = now
	consecutive := h.consecutiveCrashes
	h.mu.Unlock()

	slog.Error("[plugin-worker] process crashed", "pluginId", h.pluginID, "exitCode", exitCode, "consecutiveCrashes", consecutive)
	h.setStatus(WorkerStatusCrashed)

	if consecutive <= workerMaxCrashes {
		h.scheduleRestart()
	} else {
		slog.Error("[plugin-worker] max consecutive crashes reached, not restarting", "pluginId", h.pluginID, "maxCrashes", workerMaxCrashes)
	}
}

func (h *WorkerHandle) scheduleRestart() {
	h.mu.Lock()
	consecutive := h.consecutiveCrashes
	h.mu.Unlock()

	delay := computeBackoff(consecutive)

	h.setStatus(WorkerStatusBackoff)
	slog.Info("[plugin-worker] scheduling restart with backoff", "pluginId", h.pluginID, "delayMs", delay.Milliseconds(), "consecutiveCrashes", consecutive)

	h.backoffTimerMu.Lock()
	h.backoffTimer = time.AfterFunc(delay, func() {
		h.backoffTimerMu.Lock()
		h.backoffTimer = nil
		h.backoffTimerMu.Unlock()

		entrypoint, err := h.manager.resolveEntrypoint(context.Background(), h.pluginID)
		if err != nil {
			slog.Error("[plugin-worker] restart failed: cannot resolve entrypoint", "pluginId", h.pluginID, "err", err)
			return
		}
		if err := h.start(context.Background(), entrypoint); err != nil {
			slog.Error("[plugin-worker] restart after backoff failed", "pluginId", h.pluginID, "err", err)
		}
	})
	h.backoffTimerMu.Unlock()
}

func (h *WorkerHandle) cancelPendingRestart() {
	h.backoffTimerMu.Lock()
	if h.backoffTimer != nil {
		h.backoffTimer.Stop()
		h.backoffTimer = nil
	}
	h.backoffTimerMu.Unlock()
}

// stop performs a graceful shutdown: shutdown RPC → SIGTERM → SIGKILL.
func (h *WorkerHandle) stop(ctx context.Context) error {
	h.cancelPendingRestart()

	h.mu.Lock()
	st := h.status
	h.mu.Unlock()

	if st == WorkerStatusStopped || st == WorkerStatusStopping {
		return nil
	}

	h.mu.Lock()
	h.intentionalStop = true
	h.status = WorkerStatusStopping
	cmd := h.cmd
	h.mu.Unlock()

	if cmd == nil || cmd.Process == nil {
		h.setStatus(WorkerStatusStopped)
		return nil
	}

	// Attempt graceful shutdown via RPC.
	shutCtx, shutCancel := context.WithTimeout(ctx, rpcShutdownDrain)
	defer shutCancel()
	_, _ = h.Call(shutCtx, "shutdown", map[string]interface{}{}, rpcShutdownDrain)

	// Check if still alive.
	h.mu.Lock()
	stillRunning := h.cmd != nil
	h.mu.Unlock()

	if !stillRunning {
		return nil
	}

	// SIGTERM.
	_ = cmd.Process.Signal(os.Interrupt)
	sigtermCtx, sigtermCancel := context.WithTimeout(ctx, rpcSigtermGrace)
	defer sigtermCancel()
	<-sigtermCtx.Done()

	h.mu.Lock()
	stillRunning = h.cmd != nil
	h.mu.Unlock()

	if !stillRunning {
		return nil
	}

	// SIGKILL.
	h.killProcess()
	return nil
}

func (h *WorkerHandle) killProcess() {
	h.mu.Lock()
	cmd := h.cmd
	h.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

// SupportsMethods returns true if the worker reported the given method during
// initialization.
func (h *WorkerHandle) SupportsMethod(method string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, m := range h.supportedMethods {
		if m == method {
			return true
		}
	}
	return false
}

// Diagnostics returns a snapshot of the worker's current state.
func (h *WorkerHandle) Diagnostics() WorkerDiagnostics {
	h.mu.Lock()
	defer h.mu.Unlock()

	diag := WorkerDiagnostics{
		PluginID:           h.pluginID,
		Status:             h.status,
		ConsecutiveCrashes: h.consecutiveCrashes,
		TotalCrashes:       h.totalCrashes,
	}
	if h.cmd != nil && h.cmd.Process != nil {
		diag.PID = h.cmd.Process.Pid
	}
	if h.status == WorkerStatusRunning && !h.startedAt.IsZero() {
		ms := time.Since(h.startedAt).Milliseconds()
		diag.UptimeMs = &ms
	}
	h.pendMu.Lock()
	diag.PendingRequests = len(h.pending)
	h.pendMu.Unlock()
	return diag
}

// ---------------------------------------------------------------------------
// PluginWorkerManager — manages all plugin workers
// ---------------------------------------------------------------------------

// PluginWorkerManagerOptions configures a PluginWorkerManager.
type PluginWorkerManagerOptions struct {
	// DB is required — used for config lookups, state, and log persistence.
	DB *gorm.DB
	// StreamBus receives stream notifications from workers for SSE fan-out.
	StreamBus *PluginStreamBus
	// PluginDir is the root directory where plugins are installed.
	// Defaults to ~/.paperclip/plugins when empty.
	PluginDir string
	// SecretsResolve optionally resolves secret references.
	SecretsResolve func(ctx context.Context, secretRef string) (string, error)
	// ActivityLog optionally records plugin activity entries.
	ActivityLog func(ctx context.Context, entry LogEntry) (string, error)
	// EventsEmit optionally publishes live events to the realtime hub.
	EventsEmit func(companyID, eventType string, payload interface{})
}

// PluginWorkerManager manages Node.js plugin worker processes. One worker
// process is spawned per installed plugin. It is safe for concurrent use.
type PluginWorkerManager struct {
	db        *gorm.DB
	streamBus *PluginStreamBus
	pluginDir string
	opts      PluginWorkerManagerOptions

	mu      sync.RWMutex
	workers map[string]*WorkerHandle

	// startupLocks prevents concurrent spawns for the same plugin.
	startupLocksMu sync.Mutex
	startupLocks   map[string]chan struct{}
}

// NewPluginWorkerManager creates a PluginWorkerManager.
func NewPluginWorkerManager(opts PluginWorkerManagerOptions) *PluginWorkerManager {
	pluginDir := opts.PluginDir
	if pluginDir == "" {
		home, _ := os.UserHomeDir()
		pluginDir = filepath.Join(home, ".paperclip", "plugins")
	}
	return &PluginWorkerManager{
		db:           opts.DB,
		streamBus:    opts.StreamBus,
		pluginDir:    pluginDir,
		opts:         opts,
		workers:      make(map[string]*WorkerHandle),
		startupLocks: make(map[string]chan struct{}),
	}
}

// StartReadyPlugins queries the database for all ready plugins and starts a
// worker for each one. Errors are logged but do not stop iteration.
func (m *PluginWorkerManager) StartReadyPlugins(ctx context.Context) {
	var plugins []models.Plugin
	if err := m.db.WithContext(ctx).Where("status = ?", "ready").Find(&plugins).Error; err != nil {
		slog.Error("[plugin-worker-manager] failed to query ready plugins", "err", err)
		return
	}
	slog.Info("[plugin-worker-manager] starting workers for ready plugins", "count", len(plugins))
	for _, p := range plugins {
		p := p
		go func() {
			if err := m.StartWorker(ctx, p.ID); err != nil {
				slog.Warn("[plugin-worker-manager] failed to start worker at startup", "pluginId", p.ID, "pluginKey", p.PluginKey, "err", err)
			}
		}()
	}
}

// StartWorker resolves the plugin's entrypoint from DB and starts a worker.
func (m *PluginWorkerManager) StartWorker(ctx context.Context, pluginID string) error {
	// Concurrency guard: if a start is in-flight, wait for it.
	m.startupLocksMu.Lock()
	if existing, ok := m.startupLocks[pluginID]; ok {
		m.startupLocksMu.Unlock()
		<-existing
		return nil
	}
	done := make(chan struct{})
	m.startupLocks[pluginID] = done
	m.startupLocksMu.Unlock()

	defer func() {
		m.startupLocksMu.Lock()
		delete(m.startupLocks, pluginID)
		m.startupLocksMu.Unlock()
		close(done)
	}()

	// Check if a running worker already exists.
	m.mu.RLock()
	existing, ok := m.workers[pluginID]
	m.mu.RUnlock()
	if ok {
		st := existing.getStatus()
		if st != WorkerStatusStopped && st != WorkerStatusCrashed {
			return nil // already running
		}
	}

	entrypoint, err := m.resolveEntrypoint(ctx, pluginID)
	if err != nil {
		return fmt.Errorf("cannot resolve entrypoint for plugin %s: %w", pluginID, err)
	}

	handle := newWorkerHandle(m, pluginID)
	m.mu.Lock()
	m.workers[pluginID] = handle
	m.mu.Unlock()

	if err := handle.start(ctx, entrypoint); err != nil {
		return err
	}
	return nil
}

// StopWorker gracefully stops and removes the worker for the given plugin.
func (m *PluginWorkerManager) StopWorker(ctx context.Context, pluginID string) error {
	m.mu.RLock()
	handle, ok := m.workers[pluginID]
	m.mu.RUnlock()
	if !ok {
		return nil
	}

	err := handle.stop(ctx)

	m.mu.Lock()
	delete(m.workers, pluginID)
	m.mu.Unlock()

	return err
}

// StopAll gracefully stops all managed workers. Called during server shutdown.
func (m *PluginWorkerManager) StopAll(ctx context.Context) {
	m.mu.RLock()
	handles := make([]*WorkerHandle, 0, len(m.workers))
	for _, h := range m.workers {
		handles = append(handles, h)
	}
	m.mu.RUnlock()

	var wg sync.WaitGroup
	for _, h := range handles {
		wg.Add(1)
		h := h
		go func() {
			defer wg.Done()
			if err := h.stop(ctx); err != nil {
				slog.Error("[plugin-worker-manager] error stopping worker", "pluginId", h.pluginID, "err", err)
			}
		}()
	}
	wg.Wait()

	m.mu.Lock()
	m.workers = make(map[string]*WorkerHandle)
	m.mu.Unlock()
}

// GetWorker returns the worker handle for a plugin, or nil if not registered.
func (m *PluginWorkerManager) GetWorker(pluginID string) *WorkerHandle {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.workers[pluginID]
}

// IsRunning returns true if a worker for the plugin is in the running state.
func (m *PluginWorkerManager) IsRunning(pluginID string) bool {
	h := m.GetWorker(pluginID)
	if h == nil {
		return false
	}
	return h.getStatus() == WorkerStatusRunning
}

// Call sends an RPC to the specified plugin's worker.
func (m *PluginWorkerManager) Call(ctx context.Context, pluginID, method string, params interface{}, timeout time.Duration) (json.RawMessage, error) {
	h := m.GetWorker(pluginID)
	if h == nil {
		return nil, &rpcErr{Code: pluginErrWorkerUnavailable, Message: fmt.Sprintf("no worker registered for plugin %q", pluginID)}
	}
	return h.Call(ctx, method, params, timeout)
}

// Diagnostics returns a snapshot of all workers.
func (m *PluginWorkerManager) Diagnostics() []WorkerDiagnostics {
	m.mu.RLock()
	diags := make([]WorkerDiagnostics, 0, len(m.workers))
	for _, h := range m.workers {
		diags = append(diags, h.Diagnostics())
	}
	m.mu.RUnlock()
	return diags
}

// ---------------------------------------------------------------------------
// Host-side RPC dispatch (worker → host calls)
// ---------------------------------------------------------------------------

// dispatchHostMethod handles a worker→host RPC call and returns the result or
// an *rpcErr.
func (m *PluginWorkerManager) dispatchHostMethod(ctx context.Context, pluginID, method string, rawParams json.RawMessage) (interface{}, *rpcErr) {
	switch method {
	case "config.get":
		cfg, err := m.loadPluginConfig(ctx, pluginID)
		if err != nil {
			return nil, &rpcErr{Code: jsonrpcErrInternalError, Message: err.Error()}
		}
		return cfg, nil

	case "http.fetch":
		var params struct {
			URL  string                 `json:"url"`
			Init map[string]interface{} `json:"init"`
		}
		if err := json.Unmarshal(rawParams, &params); err != nil {
			return nil, &rpcErr{Code: jsonrpcErrInternalError, Message: "invalid http.fetch params"}
		}
		method := "GET"
		if m, ok := params.Init["method"].(string); ok {
			method = m
		}
		headers := map[string]string{}
		if h, ok := params.Init["headers"].(map[string]interface{}); ok {
			for k, v := range h {
				if sv, ok := v.(string); ok {
					headers[k] = sv
				}
			}
		}
		resp, err := SafeFetch(ctx, method, params.URL, headers)
		if err != nil {
			return nil, &rpcErr{Code: jsonrpcErrInternalError, Message: err.Error()}
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
		respHeaders := make(map[string]string, len(resp.Header))
		for k, vs := range resp.Header {
			if len(vs) > 0 {
				respHeaders[k] = vs[0]
			}
		}
		return map[string]interface{}{
			"status":     resp.StatusCode,
			"statusText": resp.Status,
			"headers":    respHeaders,
			"body":       string(body),
		}, nil

	case "secrets.resolve":
		if m.opts.SecretsResolve == nil {
			return nil, &rpcErr{Code: jsonrpcErrMethodNotFound, Message: "secrets.resolve not configured"}
		}
		var params struct {
			SecretRef string `json:"secretRef"`
		}
		if err := json.Unmarshal(rawParams, &params); err != nil {
			return nil, &rpcErr{Code: jsonrpcErrInternalError, Message: "invalid params"}
		}
		val, err := m.opts.SecretsResolve(ctx, params.SecretRef)
		if err != nil {
			return nil, &rpcErr{Code: jsonrpcErrInternalError, Message: err.Error()}
		}
		return val, nil

	case "events.emit":
		if m.opts.EventsEmit == nil {
			return nil, nil
		}
		var params struct {
			Name      string      `json:"name"`
			CompanyID string      `json:"companyId"`
			Payload   interface{} `json:"payload"`
		}
		if err := json.Unmarshal(rawParams, &params); err != nil {
			return nil, &rpcErr{Code: jsonrpcErrInternalError, Message: "invalid params"}
		}
		m.opts.EventsEmit(params.CompanyID, params.Name, params.Payload)
		return nil, nil

	case "activity.log":
		if m.opts.ActivityLog == nil {
			return nil, nil
		}
		var params map[string]interface{}
		if err := json.Unmarshal(rawParams, &params); err != nil {
			return nil, &rpcErr{Code: jsonrpcErrInternalError, Message: "invalid params"}
		}
		cid, _ := params["companyId"].(string)
		msg, _ := params["message"].(string)
		_, _ = m.opts.ActivityLog(ctx, LogEntry{
			CompanyID:  cid,
			ActorType:  "plugin",
			ActorID:    pluginID,
			Action:     "plugin.activity",
			EntityType: "plugin",
			EntityID:   pluginID,
			Details:    map[string]interface{}{"message": msg, "params": params},
		})
		return nil, nil

	case "state.get", "state.set", "state.delete":
		// Plugin state persistence via plugin_data-style table is not yet ported.
		// Return empty/nil results to keep the worker functional.
		switch method {
		case "state.get":
			return nil, nil
		default:
			return nil, nil
		}

	case "entities.upsert", "entities.list":
		// Entity management not yet ported to Go backend.
		switch method {
		case "entities.list":
			return []interface{}{}, nil
		default:
			return nil, nil
		}

	case "events.subscribe":
		// Event subscriptions managed by the Node.js worker runtime; host ack.
		return nil, nil

	default:
		return nil, &rpcErr{Code: jsonrpcErrMethodNotFound, Message: fmt.Sprintf("host does not handle method %q", method)}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// resolveEntrypoint looks up the plugin in the DB and resolves its worker
// entrypoint path from the manifest.
func (m *PluginWorkerManager) resolveEntrypoint(ctx context.Context, pluginID string) (string, error) {
	var plugin models.Plugin
	if err := m.db.WithContext(ctx).First(&plugin, "id = ?", pluginID).Error; err != nil {
		return "", fmt.Errorf("plugin %s not found: %w", pluginID, err)
	}
	loader := NewPluginLoaderService()
	return loader.ResolveWorkerEntrypoint(&plugin, m.pluginDir, func(p string) bool {
		_, err := os.Stat(p)
		return err == nil
	})
}

// loadPluginConfig retrieves the plugin's current config from the DB.
func (m *PluginWorkerManager) loadPluginConfig(ctx context.Context, pluginID string) (map[string]interface{}, error) {
	var cfg models.PluginConfig
	err := m.db.WithContext(ctx).First(&cfg, "plugin_id = ?", pluginID).Error
	if err == gorm.ErrRecordNotFound {
		return map[string]interface{}{}, nil
	}
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(cfg.ConfigJSON, &result); err != nil {
		return map[string]interface{}{}, nil
	}
	return result, nil
}

// loadPluginConfigAndManifest returns both config and manifest for initialize.
func (m *PluginWorkerManager) loadPluginConfigAndManifest(ctx context.Context, pluginID string) (map[string]interface{}, map[string]interface{}) {
	cfg, _ := m.loadPluginConfig(ctx, pluginID)

	var plugin models.Plugin
	manifest := map[string]interface{}{}
	if err := m.db.WithContext(ctx).First(&plugin, "id = ?", pluginID).Error; err == nil {
		if plugin.ManifestJSON != nil {
			_ = json.Unmarshal(plugin.ManifestJSON, &manifest)
		}
	}
	return cfg, manifest
}

// persistWorkerLog writes a worker log notification to the plugin_logs table.
func (m *PluginWorkerManager) persistWorkerLog(pluginID, level, message string, meta map[string]interface{}) {
	if m.db == nil {
		return
	}
	metaJSON, _ := json.Marshal(meta)
	entry := models.PluginLog{
		PluginID: pluginID,
		Level:    level,
		Message:  message,
		Meta:     datatypes.JSON(metaJSON),
	}
	_ = m.db.Create(&entry).Error
}

// computeBackoff returns an exponentially increasing delay with ±25% jitter.
func computeBackoff(consecutiveCrashes int) time.Duration {
	exp := math.Pow(workerBackoffMult, float64(consecutiveCrashes-1))
	base := float64(workerMinBackoff) * exp
	jitter := base * workerBackoffJitter * (rand.Float64()*2 - 1) //nolint:gosec
	delay := time.Duration(math.Min(base+jitter, float64(workerMaxBackoff)))
	if delay < workerMinBackoff {
		delay = workerMinBackoff
	}
	return delay
}

func getEnvOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
