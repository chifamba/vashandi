package opencodelocal

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
		command = "opencode"
	}

	model := asString(config["model"])
	variant := asString(config["variant"])

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
	
	// OpenCode specific guard against dropping opencode.json in workspace CWD
	envMap["OPENCODE_DISABLE_PROJECT_CONFIG"] = "true"

	rConf, err := PrepareOpenCodeRuntimeConfig(envMap)
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("failed to prepare runtime config: %w", err)
	}
	defer rConf.Cleanup()

	envMap = EnsurePathInEnv(rConf.Env)
	envSlice := MapToSlice(envMap)

	// Inject skills
	skillsRoot, _ := ResolveRepoSkillsDir()
	var desiredSkills []string
	if ds, ok := config["desiredSkills"].([]interface{}); ok {
		for _, s := range ds {
			if str, ok := s.(string); ok {
				desiredSkills = append(desiredSkills, str)
			}
		}
	}
	EnsureOpenCodeSkillsInjected(skillsRoot, desiredSkills)

	// Args matching `execute.ts`
	args := []string{"run", "--format", "json"}

	sessionId := asString(ec.Runtime["sessionId"])
	if sessionId != "" {
		args = append(args, "--session", sessionId)
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	if variant != "" {
		args = append(args, "--variant", variant)
	}

	prompt := asString(contextData["prompt"])
	if prompt == "" {
		prompt = "Continue your work."
	}

	if ec.OnMeta != nil {
		ec.OnMeta(map[string]interface{}{
			"adapterType": "opencode_local",
			"command":     command,
			"args":        args,
			"cwd":         cwd,
		})
	}

	proc := exec.CommandContext(ctx, command, args...)
	proc.Dir = cwd
	proc.Env = append(os.Environ(), envSlice...)
	proc.Stdin = strings.NewReader(prompt)
	
	stdoutPipe, _ := proc.StdoutPipe()
	stderrPipe, _ := proc.StderrPipe()

	if err := proc.Start(); err != nil {
		return ExecutionResult{}, fmt.Errorf("failed to start opencode: %w", err)
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
	err = proc.Wait()

	result := ExecutionResult{
		Provider: "anthropic", // Typical OpenCode fallback default
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

	parsed := ParseOpenCodeJsonl(stdoutBuf.String())
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
		if result.ExitCode == 0 {
			result.ExitCode = 1
		}
	} else if result.ExitCode != 0 {
		result.ErrorMessage = fmt.Sprintf("OpenCode exited with code %d", result.ExitCode)
	}

	return result, nil
}
