package services

import (
	"context"
	"testing"
)

func TestPluginRuntimeSandbox_PrepareSandboxCommand(t *testing.T) {
	sandbox := NewPluginRuntimeSandbox()

	opts := PluginSandboxOptions{
		EntrypointPath: "/tmp/fake-plugin.js",
		TimeoutMs:      1000,
		Env:            []string{"CUSTOM_VAR=1"},
	}

	cmd := sandbox.PrepareSandboxCommand(context.Background(), "node", []string{opts.EntrypointPath}, opts)

	if cmd == nil {
		t.Fatalf("Expected cmd to be non-nil")
	}

	if cmd.Path != "node" && cmd.Args[0] != "node" {
		t.Errorf("Expected command to be node, got %v", cmd.Path)
	}

	if len(cmd.Env) != 1 || cmd.Env[0] != "CUSTOM_VAR=1" {
		t.Errorf("Expected custom environment, got %v", cmd.Env)
	}

	if cmd.Args[1] != "/tmp/fake-plugin.js" {
		t.Errorf("Expected entrypoint argument, got %v", cmd.Args)
	}
}

func TestPluginWorkerHandle_CallTimeout(t *testing.T) {
	// We test the timeout behavior logic
	handle := &PluginWorkerHandle{
		Status: WorkerStatusRunning,
		SandboxOpts: PluginSandboxOptions{
			TimeoutMs: 50, // very short timeout
		},
	}

	// Wait more than the timeout duration
	// The stub implementation sleeps for 10ms, let's make the timeout 1ms to ensure failure

	_, err := handle.Call("fakeMethod", nil, 1) // 1ms timeout

	if err == nil {
		t.Errorf("Expected timeout error, got nil")
	} else if err.Error() != "rpc call timeout after 1 ms" {
		t.Errorf("Expected specific timeout error, got %v", err)
	}
}

func TestPluginCapabilityValidator(t *testing.T) {
	validator := NewPluginCapabilityValidator()
	err := validator.AssertOperation(nil, "someOperation")
	if err != nil {
		t.Errorf("Expected nil error for stub, got %v", err)
	}
}
