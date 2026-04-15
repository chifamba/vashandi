package services

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
)

type WorkerStatus string

const (
	WorkerStatusStopped  WorkerStatus = "stopped"
	WorkerStatusStarting WorkerStatus = "starting"
	WorkerStatusRunning  WorkerStatus = "running"
	WorkerStatusStopping WorkerStatus = "stopping"
	WorkerStatusCrashed  WorkerStatus = "crashed"
	WorkerStatusBackoff  WorkerStatus = "backoff"
)

type PluginWorkerHandle struct {
	PluginID   string
	Status     WorkerStatus
	Cmd        *exec.Cmd
	CancelFunc context.CancelFunc

	Stdin  io.WriteCloser
	Stdout io.ReadCloser
	Stderr io.ReadCloser

	SandboxOpts PluginSandboxOptions
	Sandbox     *PluginRuntimeSandbox

	mu sync.Mutex
}

func (h *PluginWorkerHandle) Start() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.Status == WorkerStatusRunning || h.Status == WorkerStatusStarting {
		return fmt.Errorf("worker already running or starting")
	}

	h.Status = WorkerStatusStarting

	// Create command using Sandbox
	cmd, err := h.Sandbox.SpawnWorker(h.SandboxOpts, []string{})
	if err != nil {
		h.Status = WorkerStatusCrashed
		return fmt.Errorf("failed to prepare worker command: %w", err)
	}

	h.Cmd = cmd

	stdin, err := h.Cmd.StdinPipe()
	if err != nil {
		return err
	}
	h.Stdin = stdin

	stdout, err := h.Cmd.StdoutPipe()
	if err != nil {
		return err
	}
	h.Stdout = stdout

	stderr, err := h.Cmd.StderrPipe()
	if err != nil {
		return err
	}
	h.Stderr = stderr

	if err := h.Cmd.Start(); err != nil {
		h.Status = WorkerStatusCrashed
		return fmt.Errorf("failed to start worker: %w", err)
	}

	h.Status = WorkerStatusRunning

	// Start monitor goroutine
	go func() {
		h.Cmd.Wait()
		h.mu.Lock()
		if h.Status != WorkerStatusStopping && h.Status != WorkerStatusStopped {
			h.Status = WorkerStatusCrashed
		} else {
			h.Status = WorkerStatusStopped
		}
		h.mu.Unlock()
	}()

	return nil
}

func (h *PluginWorkerHandle) Stop() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.Status == WorkerStatusStopped || h.Status == WorkerStatusCrashed {
		return nil
	}

	h.Status = WorkerStatusStopping

	if h.Cmd != nil && h.Cmd.Process != nil {
		// Try graceful shutdown, then kill
		h.Cmd.Process.Signal(syscall.SIGTERM)

		go func(p *os.Process) {
			time.Sleep(5 * time.Second)
			p.Kill()
		}(h.Cmd.Process)
	}

	if h.CancelFunc != nil {
		h.CancelFunc()
	}

	return nil
}

// IPC call enforcing timeout
func (h *PluginWorkerHandle) Call(method string, params interface{}, timeoutMs int) (interface{}, error) {
	if h.Status != WorkerStatusRunning {
		return nil, fmt.Errorf("worker is not running")
	}

	if timeoutMs == 0 {
		timeoutMs = h.SandboxOpts.TimeoutMs
		if timeoutMs == 0 {
			timeoutMs = DefaultPluginSandboxTimeoutMs
		}
	}

	// This is a stub for IPC. In real implementation, it serializes JSON-RPC, sends to h.Stdin, reads h.Stdout, and enforces timeout.
	// For now, simulating timeout enforcement.

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	resCh := make(chan interface{}, 1)
	errCh := make(chan error, 1)

	go func() {
		// Mock waiting for response
		time.Sleep(10 * time.Millisecond)
		resCh <- "success"
	}()

	select {
	case res := <-resCh:
		return res, nil
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, fmt.Errorf("rpc call timeout after %d ms", timeoutMs)
	}
}

type PluginWorkerManager struct {
	Workers map[string]*PluginWorkerHandle
	Sandbox *PluginRuntimeSandbox
	mu      sync.RWMutex
}

func NewPluginWorkerManager() *PluginWorkerManager {
	return &PluginWorkerManager{
		Workers: make(map[string]*PluginWorkerHandle),
		Sandbox: NewPluginRuntimeSandbox(),
	}
}

func (m *PluginWorkerManager) StartWorker(plugin *models.Plugin, opts PluginSandboxOptions) (*PluginWorkerHandle, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.Workers[plugin.ID]; ok {
		if existing.Status == WorkerStatusRunning {
			return nil, fmt.Errorf("worker already running for plugin %s", plugin.ID)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	_ = ctx

	handle := &PluginWorkerHandle{
		PluginID:    plugin.ID,
		Status:      WorkerStatusStopped,
		SandboxOpts: opts,
		Sandbox:     m.Sandbox,
		CancelFunc:  cancel,
	}

	m.Workers[plugin.ID] = handle

	if err := handle.Start(); err != nil {
		cancel()
		return nil, err
	}

	return handle, nil
}

func (m *PluginWorkerManager) StopWorker(pluginID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	handle, ok := m.Workers[pluginID]
	if !ok {
		return fmt.Errorf("worker not found for plugin %s", pluginID)
	}

	err := handle.Stop()
	delete(m.Workers, pluginID)
	return err
}

func (m *PluginWorkerManager) StopAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var firstErr error
	for id, handle := range m.Workers {
		if err := handle.Stop(); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(m.Workers, id)
	}
	return firstErr
}
