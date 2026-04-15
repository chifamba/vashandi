package opencodelocal

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"time"
)

// ProcessResult holds the result of running a child process.
type ProcessResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	TimedOut bool
}

// runProcess executes command with the given args, cwd, env overlay, and optional
// stdin. The provided context is used for cancellation; if the context deadline
// fires, TimedOut is set to true.
func runProcess(ctx context.Context, command string, args []string, cwd string, env map[string]string, stdin string) (*ProcessResult, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	if cwd != "" {
		cmd.Dir = cwd
	}

	// Build flat env slice from map.
	if env != nil {
		envSlice := make([]string, 0, len(env))
		for k, v := range env {
			envSlice = append(envSlice, k+"="+v)
		}
		cmd.Env = envSlice
	}

	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	startErr := cmd.Start()
	if startErr != nil {
		return nil, startErr
	}

	waitErr := cmd.Wait()

	result := &ProcessResult{
		Stdout: stdoutBuf.String(),
		Stderr: stderrBuf.String(),
	}

	if ctx.Err() == context.DeadlineExceeded || ctx.Err() == context.Canceled {
		result.TimedOut = true
		result.ExitCode = -1
		return result, nil
	}

	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
		}
	}
	return result, nil
}

// ExecutionConfig holds all parameters needed for a single OpenCode run.
type ExecutionConfig struct {
	// RunID is the Paperclip run identifier.
	RunID string
	// AgentID is the Paperclip agent identifier.
	AgentID string
	// Command is the opencode binary (defaults to "opencode").
	Command string
	// Model is the provider/model string (e.g. "openai/gpt-5.2-codex").
	Model string
	// Variant is an optional reasoning variant passed as --variant.
	Variant string
	// Cwd is the working directory for the opencode process.
	Cwd string
	// Prompt is written to stdin.
	Prompt string
	// SessionID is the OpenCode session to resume (empty = fresh session).
	SessionID string
	// ExtraArgs are appended to the CLI invocation.
	ExtraArgs []string
	// Env is the full environment for the child process (pre-merged).
	Env map[string]string
	// TimeoutSec is the hard run timeout (0 = no timeout).
	TimeoutSec int
}

// ExecutionResult is returned by ExecuteOpenCode.
type ExecutionResult struct {
	Parsed       ParsedOutput
	Proc         *ProcessResult
	ResolvedSID  *string // the session ID to persist, or nil to clear
	ClearSession bool    // true when the caller should delete the stored session
}

// ExecuteOpenCode runs `opencode run --format json` with the given config and
// returns the raw process result together with the parsed JSONL output.
func ExecuteOpenCode(ctx context.Context, cfg ExecutionConfig) (*ExecutionResult, error) {
	if cfg.Command == "" {
		cfg.Command = "opencode"
	}

	var runCtx context.Context
	var cancel context.CancelFunc
	if cfg.TimeoutSec > 0 {
		runCtx, cancel = context.WithTimeout(ctx, time.Duration(cfg.TimeoutSec)*time.Second)
	} else {
		runCtx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	args := buildArgs(cfg)
	proc, err := runProcess(runCtx, cfg.Command, args, cfg.Cwd, cfg.Env, cfg.Prompt)
	if err != nil {
		return nil, err
	}

	parsed := ParseOpenCodeJsonl(proc.Stdout)
	return &ExecutionResult{
		Parsed: parsed,
		Proc:   proc,
	}, nil
}

func buildArgs(cfg ExecutionConfig) []string {
	args := []string{"run", "--format", "json"}
	if cfg.SessionID != "" {
		args = append(args, "--session", cfg.SessionID)
	}
	if cfg.Model != "" {
		args = append(args, "--model", cfg.Model)
	}
	if cfg.Variant != "" {
		args = append(args, "--variant", cfg.Variant)
	}
	args = append(args, cfg.ExtraArgs...)
	return args
}
