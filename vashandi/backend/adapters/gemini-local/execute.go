package geminilocal

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
)

type ExecutionContext struct {
	RunID     string
	Agent     AgentInfo
	Config    map[string]interface{}
	Context   map[string]interface{}
	Runtime   map[string]interface{}
	AuthToken string

	OnLog   func(stream, chunk string) error
	OnMeta  func(meta map[string]interface{}) error
	OnSpawn func(pid int) error
}

type AgentInfo struct {
	ID        string
	Name      string
	CompanyID string
}

type ExecutionResult struct {
	ExitCode         int
	Signal           string
	TimedOut         bool
	ErrorMessage     string
	Usage            *UsageSummary
	SessionID        string
	SessionParams    map[string]interface{}
	SessionDisplayID string
	Provider         string
	Biller           string
	Model            string
	BillingType      string
	CostUsd          float64
	ResultJSON       map[string]interface{}
	Summary          string
	ClearSession     bool
}

func asString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func asBoolean(v interface{}) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

func Execute(ctx context.Context, ec ExecutionContext) (ExecutionResult, error) {
	runId := ec.RunID
	agent := ec.Agent
	config := ec.Config
	contextData := ec.Context

	command := asString(config["command"])
	if command == "" {
		command = "gemini"
	}

	model := asString(config["model"])
	if model == "" {
		model = ModelGemini20Flash
	}

	cwd := asString(config["cwd"])
	if cwd == "" {
		cwd, _ = os.Getwd()
	}

	// Prepare environment
	envMap := BuildPaperclipEnv(agent.ID, agent.CompanyID, runId)
	if cfgEnv, ok := config["env"].(map[string]interface{}); ok {
		for k, v := range cfgEnv {
			if s, ok := v.(string); ok {
				envMap[k] = s
			}
		}
	}
	envMap = EnsurePathInEnv(envMap)
	envSlice := MapToSlice(envMap)

	// Construct CLI Args
	args := []string{"--output-format", "stream-json"}

	sessionId := asString(ec.Runtime["sessionId"])
	if sessionId != "" {
		args = append(args, "--resume", sessionId)
	}

	if model != "" {
		args = append(args, "--model", model)
	}

	// Gemini args parsing rules check: "--sandbox" flag if configured
	bypass := true
	if val, ok := config["dangerouslyBypassApprovalsAndSandbox"].(bool); ok {
		bypass = val
	}
	if bypass {
		args = append(args, "--approval-mode", "yolo", "--sandbox=none")
	} else {
		args = append(args, "--sandbox")
	}

	// Any extra args from config
	if extras, ok := config["extraArgs"].([]interface{}); ok {
		for _, e := range extras {
			if s, ok := e.(string); ok {
				args = append(args, s)
			}
		}
	} else if argsObj, ok := config["args"].([]interface{}); ok {
		for _, e := range argsObj {
			if s, ok := e.(string); ok {
				args = append(args, s)
			}
		}
	}

	prompt := asString(contextData["prompt"])
	if prompt == "" {
		prompt = "Continue your work."
	}

	// Gemini wants prompt as positional argument or via --prompt flag.
	// As per instructions, "gemini-local adapter for Gemini CLI ... Google Gemini CLI expects prompts to be passed as positional arguments"
	args = append(args, prompt)

	if ec.OnMeta != nil {
		ec.OnMeta(map[string]interface{}{
			"adapterType": "gemini_local",
			"command":     command,
			"args":        args,
			"cwd":         cwd,
		})
	}

	proc := exec.CommandContext(ctx, command, args...)
	proc.Dir = cwd
	proc.Env = append(os.Environ(), envSlice...)

	stdoutPipe, _ := proc.StdoutPipe()
	stderrPipe, _ := proc.StderrPipe()

	if err := proc.Start(); err != nil {
		return ExecutionResult{}, fmt.Errorf("failed to start gemini: %w", err)
	}

	if ec.OnSpawn != nil {
		ec.OnSpawn(proc.Process.Pid)
	}

	var stdoutBuf strings.Builder
	var stderrBuf strings.Builder
	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()
		reader := io.TeeReader(stdoutPipe, &stdoutBuf)
		buf := make([]byte, 4096)
		for {
			n, err := reader.Read(buf)
			if n > 0 {
				chunk := string(buf[:n])
				if ec.OnLog != nil {
					ec.OnLog("stdout", chunk)
				}
			}
			if err != nil {
				break
			}
		}
	}()

	go func() {
		defer wg.Done()
		reader := io.TeeReader(stderrPipe, &stderrBuf)
		buf := make([]byte, 4096)
		for {
			n, err := reader.Read(buf)
			if n > 0 {
				chunk := string(buf[:n])
				if ec.OnLog != nil {
					ec.OnLog("stderr", chunk)
				}
			}
			if err != nil {
				break
			}
		}
	}()

	wg.Wait()
	err := proc.Wait()

	result := ExecutionResult{
		Provider: "google",
		Model:    model,
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				if status.Signaled() {
					result.Signal = status.Signal().String()
				}
			}
		} else {
			result.ExitCode = 1
		}
	}

	parsed := ParseGeminiJsonl(stdoutBuf.String())
	if parsed.SessionID != nil {
		result.SessionID = *parsed.SessionID
	}
	result.Summary = parsed.Summary
	result.Usage = parsed.Usage
	if parsed.CostUsd != nil {
		result.CostUsd = *parsed.CostUsd
	}

	if parsed.ErrorMessage != nil {
		result.ErrorMessage = *parsed.ErrorMessage
	} else if result.ExitCode != 0 {
		result.ErrorMessage = fmt.Sprintf("Gemini exited with code %d", result.ExitCode)
	}

	return result, nil
}
