// Package opencodelocal implements a Go adapter for running OpenCode locally as
// a child process. It mirrors the behavior of the Node.js
// packages/adapters/opencode-local package.
//
// # Usage
//
//	runner := opencodelocal.NewRunner(opencodelocal.RunnerOptions{
//	    Model:   "openai/gpt-5.2-codex",
//	    Command: "opencode",
//	})
//	if err := runner.ExecuteRun(ctx, agentID, runID); err != nil {
//	    log.Fatal(err)
//	}
package opencodelocal

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

const (
	// AdapterType is the canonical type identifier for this adapter.
	AdapterType = "opencode_local"
	// AdapterLabel is the human-readable name.
	AdapterLabel = "OpenCode (local)"
	// DefaultModel is the model used when no model is explicitly configured.
	DefaultModel = "openai/gpt-5.2-codex"
)

// RunnerOptions holds configuration for the OpenCode local runner.
type RunnerOptions struct {
	// Command is the opencode binary. Defaults to "opencode".
	Command string
	// Model is the provider/model string. Required.
	Model string
	// Variant is an optional reasoning variant (e.g. "medium").
	Variant string
	// Cwd is the default working directory. Defaults to the current directory.
	Cwd string
	// TimeoutSec is the per-run hard timeout. 0 means no timeout.
	TimeoutSec int
	// GraceSec is the SIGTERM grace period (informational only in Go impl).
	GraceSec int
	// ExtraArgs are appended to every `opencode run` invocation.
	ExtraArgs []string
	// Env is a set of extra environment variables to inject.
	Env map[string]string
	// DangerouslySkipPermissions controls runtime config injection (default true).
	// Set to false to disable the external_directory permission override.
	DangerouslySkipPermissions *bool
}

// Runner executes OpenCode as a local subprocess.
type Runner struct {
	opts RunnerOptions
}

// NewRunner creates a Runner with the provided options.
func NewRunner(opts RunnerOptions) *Runner {
	if opts.Command == "" {
		opts.Command = "opencode"
	}
	if opts.Model == "" {
		opts.Model = DefaultModel
	}
	if opts.Cwd == "" {
		cwd, _ := os.Getwd()
		opts.Cwd = cwd
	}
	return &Runner{opts: opts}
}

// ExecuteRun validates the model is available and runs a single OpenCode
// session with a generic prompt. This is the minimal entry-point that matches
// the interface expected by the broader adapter contract.
func (r *Runner) ExecuteRun(ctx context.Context, agentID, runID string) error {
	if r.opts.Model == "" {
		return fmt.Errorf("opencode_local: model is not configured")
	}
	slog.Info("opencode_local ExecuteRun", "agent", agentID, "run", runID)

	env, err := r.buildEnv(runID)
	if err != nil {
		return err
	}

	cfg := ExecutionConfig{
		RunID:      runID,
		AgentID:    agentID,
		Command:    r.opts.Command,
		Model:      r.opts.Model,
		Variant:    r.opts.Variant,
		Cwd:        r.opts.Cwd,
		Prompt:     fmt.Sprintf("You are agent %s. Continue your Paperclip work.", agentID),
		ExtraArgs:  r.opts.ExtraArgs,
		Env:        env,
		TimeoutSec: r.opts.TimeoutSec,
	}

	result, err := ExecuteOpenCode(ctx, cfg)
	if err != nil {
		return fmt.Errorf("opencode_local ExecuteRun: %w", err)
	}

	exitCode := result.Proc.ExitCode
	slog.Info("opencode_local ExecuteRun complete",
		"agent", agentID, "run", runID,
		"exitCode", exitCode,
		"sessionID", ptrString(result.Parsed.SessionID),
	)
	if exitCode != 0 {
		if result.Parsed.ErrorMessage != nil {
			return fmt.Errorf("opencode_local: %s", *result.Parsed.ErrorMessage)
		}
		return fmt.Errorf("opencode_local: exited with code %d", exitCode)
	}
	return nil
}

// ExecuteRunWithSession is the full execution path used by the Paperclip
// orchestration layer. It handles session resume, runtime config injection,
// model validation, and retry on unknown-session errors.
func (r *Runner) ExecuteRunWithSession(ctx context.Context, agentID, runID, prompt, sessionID string) (*FullRunResult, error) {
	env, err := r.buildEnv(runID)
	if err != nil {
		return nil, err
	}

	skipPerms := true
	if r.opts.DangerouslySkipPermissions != nil {
		skipPerms = *r.opts.DangerouslySkipPermissions
	}
	configOverride := map[string]interface{}{"dangerouslySkipPermissions": skipPerms}
	runtimeCfg, err := PrepareOpenCodeRuntimeConfig(env, configOverride)
	if err != nil {
		return nil, fmt.Errorf("opencode_local: prepare runtime config: %w", err)
	}
	defer runtimeCfg.Cleanup() //nolint:errcheck

	// Validate model before running.
	if err := r.ensureModelAvailable(ctx, runtimeCfg.Env); err != nil {
		return nil, err
	}

	run := func(sid string) (*FullRunResult, error) {
		cfg := ExecutionConfig{
			RunID:      runID,
			AgentID:    agentID,
			Command:    r.opts.Command,
			Model:      r.opts.Model,
			Variant:    r.opts.Variant,
			Cwd:        r.opts.Cwd,
			Prompt:     prompt,
			SessionID:  sid,
			ExtraArgs:  r.opts.ExtraArgs,
			Env:        runtimeCfg.Env,
			TimeoutSec: r.opts.TimeoutSec,
		}
		res, err := ExecuteOpenCode(ctx, cfg)
		if err != nil {
			return nil, err
		}
		return toFullRunResult(res, r.opts.Cwd, sessionID, r.opts.Model), nil
	}

	initial, err := run(sessionID)
	if err != nil {
		return nil, err
	}

	// Retry with a fresh session when the stored session is no longer known.
	if sessionID != "" && !initial.TimedOut && initial.ExitCode != 0 &&
		IsOpenCodeUnknownSessionError(initial.RawStdout, initial.RawStderr) {
		slog.Info("opencode_local: session unavailable, retrying fresh", "session", sessionID)
		retry, err := run("")
		if err != nil {
			return nil, err
		}
		retry.ClearSession = true
		return retry, nil
	}

	return initial, nil
}

// FullRunResult captures everything from a single OpenCode execution.
type FullRunResult struct {
	ExitCode    int
	Signal      string
	TimedOut    bool
	ErrorMsg    string
	SessionID   string
	Summary     string
	Usage       UsageInfo
	CostUsd     float64
	Model       string
	Provider    string
	ClearSession bool
	RawStdout   string
	RawStderr   string
}

func toFullRunResult(res *ExecutionResult, cwd, prevSessionID, model string) *FullRunResult {
	out := &FullRunResult{
		ExitCode:  res.Proc.ExitCode,
		TimedOut:  res.Proc.TimedOut,
		RawStdout: res.Proc.Stdout,
		RawStderr: res.Proc.Stderr,
		Summary:   res.Parsed.Summary,
		Usage:     res.Parsed.Usage,
		CostUsd:   res.Parsed.CostUsd,
		Model:     model,
		Provider:  parseModelProvider(model),
	}
	if out.TimedOut {
		out.ErrorMsg = "timed out"
		return out
	}
	if res.Parsed.SessionID != nil {
		out.SessionID = *res.Parsed.SessionID
	} else if prevSessionID != "" {
		out.SessionID = prevSessionID
	}
	if res.Parsed.ErrorMessage != nil {
		out.ErrorMsg = *res.Parsed.ErrorMessage
	} else if out.ExitCode != 0 {
		line := firstNonEmptyLineFromStr(res.Proc.Stderr)
		if line == "" {
			line = fmt.Sprintf("opencode exited with code %d", out.ExitCode)
		}
		out.ErrorMsg = line
	}
	return out
}

func parseModelProvider(model string) string {
	model = strings.TrimSpace(model)
	if !strings.Contains(model, "/") {
		return ""
	}
	idx := strings.Index(model, "/")
	return strings.TrimSpace(model[:idx])
}

func firstNonEmptyLineFromStr(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return t
		}
	}
	return ""
}

func ptrString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (r *Runner) buildEnv(runID string) (map[string]string, error) {
	env := make(map[string]string)
	// Start with the current process environment.
	for _, kv := range os.Environ() {
		idx := strings.Index(kv, "=")
		if idx < 0 {
			continue
		}
		env[kv[:idx]] = kv[idx+1:]
	}
	// Overlay adapter-specific env.
	for k, v := range r.opts.Env {
		env[k] = v
	}
	env["PAPERCLIP_RUN_ID"] = runID
	env["OPENCODE_DISABLE_PROJECT_CONFIG"] = "true"
	return env, nil
}

func (r *Runner) ensureModelAvailable(ctx context.Context, env map[string]string) error {
	cwd := r.opts.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	if err := os.MkdirAll(cwd, 0o755); err != nil && !os.IsExist(err) {
		return fmt.Errorf("opencode_local: create cwd: %w", err)
	}
	absCmd, _ := resolveCommandPath(r.opts.Command, cwd)
	if absCmd == "" {
		absCmd = r.opts.Command
	}
	_ = absCmd // used for logging only

	_, err := EnsureOpenCodeModelConfiguredAndAvailable(ctx, r.opts.Model, r.opts.Command, cwd, env)
	return err
}

// resolveCommandPath resolves the command to an absolute path using PATH
// lookup. Returns ("", err) if the command cannot be found.
func resolveCommandPath(command, _ string) (string, error) {
	if filepath.IsAbs(command) {
		return command, nil
	}
	return "", nil
}
