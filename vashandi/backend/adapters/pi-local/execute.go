package pilocal

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"
)

// ─── Types ───────────────────────────────────────────────────────────────────

// AgentInfo holds the minimal agent fields needed to build the execution env.
type AgentInfo struct {
	ID        string
	Name      string
	CompanyID string
}

// ExecutionContext mirrors AdapterExecutionContext from adapter-utils.
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

// ExecutionResult mirrors AdapterExecutionResult.
type ExecutionResult struct {
	ExitCode     *int
	Signal       *string
	TimedOut     bool
	ErrorMessage *string
	Usage        *UsageSummary
	SessionID    *string
	SessionParams map[string]interface{}
	SessionDisplayID *string
	Provider     *string
	Biller       string
	Model        string
	BillingType  string
	CostUsd      float64
	ResultJSON   map[string]interface{}
	Summary      string
	ClearSession bool
}

// ─── Constants ────────────────────────────────────────────────────────────────

const (
	defaultPiCommand = "pi"
	defaultTimeoutSec = 0
	defaultGraceSec   = 20
)

var (
	paperclipSessionsDir = filepath.Join(homeDir(), ".pi", "paperclips")
	piAgentSkillsDir     = filepath.Join(homeDir(), ".pi", "agent", "skills")
)

func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return os.TempDir()
	}
	return h
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func configString(config map[string]interface{}, key, fallback string) string {
	if v, ok := config[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return fallback
}

func configInt(config map[string]interface{}, key string, fallback int) int {
	if v, ok := config[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		case int64:
			return int(n)
		}
	}
	return fallback
}

func configStringSlice(config map[string]interface{}, key string) []string {
	if v, ok := config[key]; ok {
		if arr, ok := v.([]interface{}); ok {
			out := make([]string, 0, len(arr))
			for _, item := range arr {
				if s, ok := item.(string); ok {
					out = append(out, s)
				}
			}
			return out
		}
	}
	return nil
}

func parseModelProvider(model string) string {
	model = strings.TrimSpace(model)
	if !strings.Contains(model, "/") {
		return ""
	}
	return strings.TrimSpace(model[:strings.Index(model, "/")])
}

func parseModelID(model string) string {
	model = strings.TrimSpace(model)
	if !strings.Contains(model, "/") {
		return model
	}
	return strings.TrimSpace(model[strings.Index(model, "/")+1:])
}

func buildSessionPath(agentID, timestamp string) string {
	// Replace : and . with - to make the timestamp filesystem-safe.
	re := regexp.MustCompile(`[:\.]`)
	safeTS := re.ReplaceAllString(timestamp, "-")
	return filepath.Join(paperclipSessionsDir, safeTS+"-"+agentID+".jsonl")
}

// buildPaperclipEnv builds the base PAPERCLIP_* environment from an agent.
func buildPaperclipEnv(agent AgentInfo) map[string]string {
	return map[string]string{
		"PAPERCLIP_AGENT_ID":      agent.ID,
		"PAPERCLIP_AGENT_NAME":    agent.Name,
		"PAPERCLIP_COMPANY_ID":    agent.CompanyID,
	}
}

// ensurePathInEnv ensures common binary directories are in PATH.
func ensurePathInEnv(env map[string]string) map[string]string {
	result := make(map[string]string, len(env))
	for k, v := range env {
		result[k] = v
	}
	path := result["PATH"]
	extras := []string{
		filepath.Join(homeDir(), ".local", "bin"),
		"/usr/local/bin",
		"/usr/bin",
		"/bin",
	}
	existing := strings.Split(path, string(os.PathListSeparator))
	existingSet := make(map[string]bool, len(existing))
	for _, p := range existing {
		existingSet[p] = true
	}
	for _, e := range extras {
		if !existingSet[e] {
			if path != "" {
				path = e + string(os.PathListSeparator) + path
			} else {
				path = e
			}
		}
	}
	result["PATH"] = path
	return result
}

// resolvePiBiller infers the billing provider from the environment, falling back
// to the explicit provider string.
func resolvePiBiller(env map[string]string, provider string) string {
	// Mirrors inferOpenAiCompatibleBiller: check common provider env keys.
	type providerKey struct {
		envKey   string
		provider string
	}
	keys := []providerKey{
		{"OPENAI_API_KEY", "openai"},
		{"ANTHROPIC_API_KEY", "anthropic"},
		{"XAI_API_KEY", "xai"},
		{"GROQ_API_KEY", "groq"},
		{"MISTRAL_API_KEY", "mistral"},
		{"TOGETHER_API_KEY", "together"},
	}
	for _, k := range keys {
		if v, ok := env[k.envKey]; ok && strings.TrimSpace(v) != "" {
			return k.provider
		}
	}
	if provider != "" {
		return provider
	}
	return "unknown"
}

// renderTemplate replaces {{agent.id}}, {{agent.name}}, {{agent.companyId}},
// {{run.id}} etc. in a template string.
func renderTemplate(tmpl string, agent AgentInfo, runID string) string {
	r := strings.NewReplacer(
		"{{agent.id}}", agent.ID,
		"{{agent.name}}", agent.Name,
		"{{agent.companyId}}", agent.CompanyID,
		"{{agentId}}", agent.ID,
		"{{companyId}}", agent.CompanyID,
		"{{runId}}", runID,
		"{{run.id}}", runID,
	)
	return r.Replace(tmpl)
}

// joinPromptSections joins non-empty sections with double newlines.
func joinPromptSections(sections []string) string {
	var parts []string
	for _, s := range sections {
		if t := strings.TrimSpace(s); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, "\n\n")
}

// ─── Process Runner ───────────────────────────────────────────────────────────

// procResult holds the output of a subprocess run.
type procResult struct {
	Stdout   string
	Stderr   string
	ExitCode *int
	Signal   *string
	TimedOut bool
}

// runProcess executes command with args, streaming output via onLog.
// timeoutSec=0 means no timeout; graceSec controls SIGTERM→SIGKILL interval.
func runProcess(
	ctx context.Context,
	command string,
	args []string,
	cwd string,
	env []string,
	timeoutSec, graceSec int,
	onLog func(stream, chunk string) error,
	onSpawn func(pid int) error,
) procResult {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = cwd
	cmd.Env = env
	// Process group so we can kill the whole tree on timeout.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdoutPipe, _ := cmd.StdoutPipe()
	stderrPipe, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		msg := fmt.Sprintf("[error] could not start %s: %s\n", command, err.Error())
		if onLog != nil {
			_ = onLog("stderr", msg)
		}
		return procResult{Stderr: msg}
	}

	if onSpawn != nil {
		_ = onSpawn(cmd.Process.Pid)
	}

	var (
		stdoutBuf strings.Builder
		stderrBuf strings.Builder
		wg        sync.WaitGroup
	)

	// Stream stdout (buffered by lines to handle partial JSON chunks)
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		var pending strings.Builder
		for {
			n, err := stdoutPipe.Read(buf)
			if n > 0 {
				chunk := string(buf[:n])
				stdoutBuf.WriteString(chunk)
				pending.WriteString(chunk)
				// emit complete lines
				s := pending.String()
				for {
					idx := strings.IndexByte(s, '\n')
					if idx < 0 {
						break
					}
					line := s[:idx+1]
					s = s[idx+1:]
					if onLog != nil && strings.TrimFunc(line, unicode.IsSpace) != "" {
						_ = onLog("stdout", line)
					}
				}
				pending.Reset()
				pending.WriteString(s)
			}
			if err != nil {
				// flush remaining
				if rem := pending.String(); rem != "" && onLog != nil {
					_ = onLog("stdout", rem)
				}
				break
			}
		}
	}()

	// Stream stderr immediately (not JSONL)
	wg.Add(1)
	go func() {
		defer wg.Done()
		data, _ := io.ReadAll(stderrPipe)
		s := string(data)
		stderrBuf.WriteString(s)
		if s != "" && onLog != nil {
			_ = onLog("stderr", s)
		}
	}()

	// Optional wall-clock timeout (separate from ctx deadline).
	timedOut := false
	var killTimer *time.Timer
	if timeoutSec > 0 {
		killTimer = time.AfterFunc(time.Duration(timeoutSec)*time.Second, func() {
			timedOut = true
			// SIGTERM first
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
			// SIGKILL after grace period
			time.AfterFunc(time.Duration(graceSec)*time.Second, func() {
				_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			})
		})
	}

	wg.Wait()
	err := cmd.Wait()
	if killTimer != nil {
		killTimer.Stop()
	}

	res := procResult{
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
		TimedOut: timedOut,
	}
	if cmd.ProcessState != nil {
		code := cmd.ProcessState.ExitCode()
		res.ExitCode = &code
		if sig, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok {
			if sig.Signaled() {
				s := sig.Signal().String()
				res.Signal = &s
			}
		}
	}
	_ = err
	return res
}

// ─── Execute ─────────────────────────────────────────────────────────────────

// Execute runs the Pi CLI agent and returns an ExecutionResult.
// It mirrors the Node.js execute() function in execute.ts.
func Execute(ctx context.Context, ec ExecutionContext) (ExecutionResult, error) {
	config := ec.Config
	if config == nil {
		config = map[string]interface{}{}
	}
	context_ := ec.Context
	if context_ == nil {
		context_ = map[string]interface{}{}
	}
	runtime := ec.Runtime
	if runtime == nil {
		runtime = map[string]interface{}{}
	}

	promptTemplate := configString(config, "promptTemplate",
		"You are agent {{agent.id}} ({{agent.name}}). Continue your Paperclip work.")
	command := configString(config, "command", defaultPiCommand)
	model := strings.TrimSpace(configString(config, "model", ""))
	thinking := strings.TrimSpace(configString(config, "thinking", ""))

	provider := parseModelProvider(model)
	modelID := parseModelID(model)

	// Workspace context
	wsCwd := asString(getNestedString(context_, "paperclipWorkspace", "cwd"), "")
	wsSource := asString(getNestedString(context_, "paperclipWorkspace", "source"), "")
	wsID := asString(getNestedString(context_, "paperclipWorkspace", "workspaceId"), "")
	wsRepoURL := asString(getNestedString(context_, "paperclipWorkspace", "repoUrl"), "")
	wsRepoRef := asString(getNestedString(context_, "paperclipWorkspace", "repoRef"), "")
	agentHome := asString(getNestedString(context_, "paperclipWorkspace", "agentHome"), "")

	configuredCwd := strings.TrimSpace(configString(config, "cwd", ""))
	useConfiguredInsteadOfAgentHome := wsSource == "agent_home" && configuredCwd != ""
	effectiveWsCwd := wsCwd
	if useConfiguredInsteadOfAgentHome {
		effectiveWsCwd = ""
	}
	cwd := effectiveWsCwd
	if cwd == "" {
		cwd = configuredCwd
	}
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			cwd = "."
		}
	}

	// Ensure cwd is absolute and exists.
	if !filepath.IsAbs(cwd) {
		abs, err := filepath.Abs(cwd)
		if err == nil {
			cwd = abs
		}
	}
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		return ExecutionResult{}, fmt.Errorf("could not create working directory %q: %w", cwd, err)
	}

	// Ensure sessions directory.
	if err := os.MkdirAll(paperclipSessionsDir, 0o755); err != nil {
		return ExecutionResult{}, fmt.Errorf("could not create Pi sessions dir: %w", err)
	}

	// Build environment.
	envConfig, _ := config["env"].(map[string]interface{})
	hasExplicitAPIKey := false
	if v, ok := envConfig["PAPERCLIP_API_KEY"]; ok {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			hasExplicitAPIKey = true
		}
	}

	env := buildPaperclipEnv(ec.Agent)
	env["PAPERCLIP_RUN_ID"] = ec.RunID

	// Context-derived vars
	wakeTaskID := coalesceContextString(context_, "taskId", "issueId")
	wakeReason := trimmedContextString(context_, "wakeReason")
	wakeCommentID := coalesceContextString(context_, "wakeCommentId", "commentId")
	approvalID := trimmedContextString(context_, "approvalId")
	approvalStatus := trimmedContextString(context_, "approvalStatus")

	if wakeTaskID != "" {
		env["PAPERCLIP_TASK_ID"] = wakeTaskID
	}
	if wakeReason != "" {
		env["PAPERCLIP_WAKE_REASON"] = wakeReason
	}
	if wakeCommentID != "" {
		env["PAPERCLIP_WAKE_COMMENT_ID"] = wakeCommentID
	}
	if approvalID != "" {
		env["PAPERCLIP_APPROVAL_ID"] = approvalID
	}
	if approvalStatus != "" {
		env["PAPERCLIP_APPROVAL_STATUS"] = approvalStatus
	}
	if wsCwd != "" {
		env["PAPERCLIP_WORKSPACE_CWD"] = wsCwd
	}
	if wsSource != "" {
		env["PAPERCLIP_WORKSPACE_SOURCE"] = wsSource
	}
	if wsID != "" {
		env["PAPERCLIP_WORKSPACE_ID"] = wsID
	}
	if wsRepoURL != "" {
		env["PAPERCLIP_WORKSPACE_REPO_URL"] = wsRepoURL
	}
	if wsRepoRef != "" {
		env["PAPERCLIP_WORKSPACE_REPO_REF"] = wsRepoRef
	}
	if agentHome != "" {
		env["AGENT_HOME"] = agentHome
	}

	// Merge explicit env config over computed env.
	for k, v := range envConfig {
		if s, ok := v.(string); ok {
			env[k] = s
		}
	}
	if !hasExplicitAPIKey && ec.AuthToken != "" {
		env["PAPERCLIP_API_KEY"] = ec.AuthToken
	}

	runtimeEnv := ensurePathInEnv(env)

	// Validate model is available before execution.
	if _, err := EnsurePiModelConfiguredAndAvailable(model, DiscoverInput{
		Command: command,
		Cwd:     cwd,
		Env:     runtimeEnv,
	}); err != nil {
		return ExecutionResult{}, err
	}

	timeoutSec := configInt(config, "timeoutSec", defaultTimeoutSec)
	graceSec := configInt(config, "graceSec", defaultGraceSec)

	// Extra args from config.
	extraArgs := configStringSlice(config, "extraArgs")
	if len(extraArgs) == 0 {
		extraArgs = configStringSlice(config, "args")
	}

	// Session handling.
	runtimeSessionParams, _ := runtime["sessionParams"].(map[string]interface{})
	runtimeSessionID := asString(runtimeSessionParams["sessionId"], "")
	if runtimeSessionID == "" {
		runtimeSessionID = asString(runtime["sessionId"], "")
	}
	runtimeSessionCwd := asString(runtimeSessionParams["cwd"], "")

	absRuntimeSessionCwd := ""
	absCwd, _ := filepath.Abs(cwd)
	if runtimeSessionCwd != "" {
		absRuntimeSessionCwd, _ = filepath.Abs(runtimeSessionCwd)
	}
	canResumeSession := runtimeSessionID != "" &&
		(absRuntimeSessionCwd == "" || absRuntimeSessionCwd == absCwd)

	now := time.Now().UTC()
	sessionPath := runtimeSessionID
	if !canResumeSession {
		sessionPath = buildSessionPath(ec.Agent.ID, now.Format(time.RFC3339Nano))
	}

	if runtimeSessionID != "" && !canResumeSession {
		if ec.OnLog != nil {
			_ = ec.OnLog("stdout", fmt.Sprintf(
				"[paperclip] Pi session %q was saved for cwd %q and will not be resumed in %q.\n",
				runtimeSessionID, runtimeSessionCwd, cwd))
		}
	}

	// Ensure session file exists on first run.
	if !canResumeSession {
		f, err := os.OpenFile(sessionPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil && !os.IsExist(err) {
			return ExecutionResult{}, fmt.Errorf("could not create Pi session file: %w", err)
		}
		if f != nil {
			f.Close()
		}
	}

	// Instructions file / system prompt extension.
	instructionsFilePath := strings.TrimSpace(configString(config, "instructionsFilePath", ""))
	resolvedInstructionsFilePath := ""
	instructionsFileDir := ""
	if instructionsFilePath != "" {
		resolvedInstructionsFilePath = filepath.Join(cwd, instructionsFilePath)
		if !filepath.IsAbs(instructionsFilePath) {
			resolvedInstructionsFilePath = filepath.Join(cwd, instructionsFilePath)
		} else {
			resolvedInstructionsFilePath = instructionsFilePath
		}
		instructionsFileDir = filepath.Dir(instructionsFilePath) + "/"
	}

	systemPromptExtension := ""
	instructionsReadFailed := false
	if resolvedInstructionsFilePath != "" {
		data, err := os.ReadFile(resolvedInstructionsFilePath)
		if err != nil {
			instructionsReadFailed = true
			if ec.OnLog != nil {
				_ = ec.OnLog("stdout", fmt.Sprintf(
					"[paperclip] Warning: could not read agent instructions file %q: %s\n",
					resolvedInstructionsFilePath, err.Error()))
			}
			systemPromptExtension = promptTemplate
		} else {
			systemPromptExtension = string(data) + "\n\n" +
				"The above agent instructions were loaded from " + resolvedInstructionsFilePath + ". " +
				"Resolve any relative file references from " + instructionsFileDir + ".\n\n" +
				"You are agent {{agent.id}} ({{agent.name}}). Continue your Paperclip work."
		}
	} else {
		systemPromptExtension = promptTemplate
	}

	bootstrapPromptTemplate := configString(config, "bootstrapPromptTemplate", "")

	renderedSystemPromptExtension := renderTemplate(systemPromptExtension, ec.Agent, ec.RunID)
	renderedBootstrapPrompt := ""
	if !canResumeSession && strings.TrimSpace(bootstrapPromptTemplate) != "" {
		renderedBootstrapPrompt = strings.TrimSpace(renderTemplate(bootstrapPromptTemplate, ec.Agent, ec.RunID))
	}

	// Wake prompt
	wakePrompt := buildWakePrompt(context_, canResumeSession)
	shouldUseResumeDeltaPrompt := canResumeSession && wakePrompt != ""
	renderedHeartbeatPrompt := ""
	if !shouldUseResumeDeltaPrompt {
		renderedHeartbeatPrompt = renderTemplate(promptTemplate, ec.Agent, ec.RunID)
	}

	sessionHandoffNote := strings.TrimSpace(asString(context_["paperclipSessionHandoffMarkdown"], ""))

	userPrompt := joinPromptSections([]string{
		renderedBootstrapPrompt,
		wakePrompt,
		sessionHandoffNote,
		renderedHeartbeatPrompt,
	})

	// Build command args.
	buildArgs := func(sessionFile string) []string {
		args := []string{}
		args = append(args, "--mode", "json")
		args = append(args, "-p")
		args = append(args, "--append-system-prompt", renderedSystemPromptExtension)
		if provider != "" {
			args = append(args, "--provider", provider)
		}
		if modelID != "" {
			args = append(args, "--model", modelID)
		}
		if thinking != "" {
			args = append(args, "--thinking", thinking)
		}
		args = append(args, "--tools", "read,bash,edit,write,grep,find,ls")
		args = append(args, "--session", sessionFile)
		args = append(args, "--skill", piAgentSkillsDir)
		args = append(args, extraArgs...)
		args = append(args, userPrompt)
		return args
	}

	// Command notes for onMeta.
	commandNotes := []string{}
	if resolvedInstructionsFilePath != "" {
		if instructionsReadFailed {
			commandNotes = append(commandNotes,
				fmt.Sprintf("Configured instructionsFilePath %s, but file could not be read; continuing without injected instructions.", resolvedInstructionsFilePath))
		} else {
			commandNotes = append(commandNotes,
				fmt.Sprintf("Loaded agent instructions from %s", resolvedInstructionsFilePath),
				fmt.Sprintf("Appended instructions + path directive to system prompt (relative references from %s).", instructionsFileDir),
			)
		}
	}

	// Runtime env slice.
	runtimeEnvSlice := mergeEnv(os.Environ(), runtimeEnv)

	runAttempt := func(sessionFile string) procResult {
		args := buildArgs(sessionFile)

		if ec.OnMeta != nil {
			_ = ec.OnMeta(map[string]interface{}{
				"adapterType":  "pi_local",
				"command":      command,
				"cwd":          cwd,
				"commandNotes": commandNotes,
				"commandArgs":  args,
				"prompt":       userPrompt,
			})
		}

		return runProcess(ctx, command, args, cwd, runtimeEnvSlice, timeoutSec, graceSec, ec.OnLog, ec.OnSpawn)
	}

	toResult := func(proc procResult, clearSessionOnMissingSession bool) ExecutionResult {
		if proc.TimedOut {
			return ExecutionResult{
				ExitCode:     proc.ExitCode,
				Signal:       proc.Signal,
				TimedOut:     true,
				ErrorMessage: strPtr(fmt.Sprintf("Timed out after %ds", timeoutSec)),
				ClearSession: clearSessionOnMissingSession,
			}
		}

		var resolvedSessionID *string
		var resolvedSessionParams map[string]interface{}
		if !clearSessionOnMissingSession {
			resolvedSessionID = strPtr(sessionPath)
			resolvedSessionParams = map[string]interface{}{
				"sessionId": sessionPath,
				"cwd":       cwd,
			}
		}

		parsed := ParsePiJsonl(proc.Stdout)

		stderrLine := firstNonEmptyLine(proc.Stderr)
		rawExitCode := 0
		if proc.ExitCode != nil {
			rawExitCode = *proc.ExitCode
		}
		parsedError := ""
		for _, e := range parsed.Errors {
			if strings.TrimSpace(e) != "" {
				parsedError = e
				break
			}
		}
		effectiveExitCode := rawExitCode
		if rawExitCode == 0 && parsedError != "" {
			effectiveExitCode = 1
		}

		var errorMessage *string
		if effectiveExitCode != 0 {
			msg := parsedError
			if msg == "" {
				msg = stderrLine
			}
			if msg == "" {
				msg = fmt.Sprintf("Pi exited with code %d", rawExitCode)
			}
			errorMessage = &msg
		}

		var summary string
		if parsed.FinalMessage != nil && *parsed.FinalMessage != "" {
			summary = *parsed.FinalMessage
		} else {
			summary = strings.TrimSpace(strings.Join(parsed.Messages, "\n\n"))
		}

		providerPtr := (*string)(nil)
		if provider != "" {
			providerPtr = &provider
		}

		return ExecutionResult{
			ExitCode:         intPtr(effectiveExitCode),
			Signal:           proc.Signal,
			TimedOut:         false,
			ErrorMessage:     errorMessage,
			Usage:            &parsed.Usage,
			SessionID:        resolvedSessionID,
			SessionParams:    resolvedSessionParams,
			SessionDisplayID: resolvedSessionID,
			Provider:         providerPtr,
			Biller:           resolvePiBiller(runtimeEnv, provider),
			Model:            model,
			BillingType:      "unknown",
			CostUsd:          parsed.Usage.CostUsd,
			ResultJSON: map[string]interface{}{
				"stdout": proc.Stdout,
				"stderr": proc.Stderr,
			},
			Summary:      summary,
			ClearSession: clearSessionOnMissingSession,
		}
	}

	initial := runAttempt(sessionPath)
	initialExitCode := 0
	if initial.ExitCode != nil {
		initialExitCode = *initial.ExitCode
	}
	initialParsed := ParsePiJsonl(initial.Stdout)
	initialFailed := !initial.TimedOut && (initialExitCode != 0 || len(initialParsed.Errors) > 0)

	if canResumeSession && initialFailed &&
		IsPiUnknownSessionError(initial.Stdout, initial.Stderr) {
		if ec.OnLog != nil {
			_ = ec.OnLog("stdout", fmt.Sprintf(
				"[paperclip] Pi session %q is unavailable; retrying with a fresh session.\n",
				runtimeSessionID))
		}
		newSessionPath := buildSessionPath(ec.Agent.ID, time.Now().UTC().Format(time.RFC3339Nano))
		f, err := os.OpenFile(newSessionPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil && !os.IsExist(err) {
			return ExecutionResult{}, fmt.Errorf("could not create fallback Pi session file: %w", err)
		}
		if f != nil {
			f.Close()
		}
		retry := runAttempt(newSessionPath)
		return toResult(retry, true), nil
	}

	return toResult(initial, false), nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func strPtr(s string) *string { return &s }
func intPtr(n int) *int       { return &n }

// getNestedString navigates a context map through the given key path and
// returns the final string value or nil.
func getNestedString(m map[string]interface{}, keys ...string) interface{} {
	var current interface{} = m
	for _, k := range keys {
		if cm, ok := current.(map[string]interface{}); ok {
			current = cm[k]
		} else {
			return nil
		}
	}
	return current
}

func trimmedContextString(ctx map[string]interface{}, key string) string {
	if s, ok := ctx[key].(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

// coalesceContextString returns the first non-empty trimmed string from the
// given context keys.
func coalesceContextString(ctx map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if s := trimmedContextString(ctx, k); s != "" {
			return s
		}
	}
	return ""
}

// buildWakePrompt builds the wake reason prompt string (simplified port of
// renderPaperclipWakePrompt from adapter-utils).
func buildWakePrompt(ctx map[string]interface{}, resumedSession bool) string {
	wakePayload, _ := ctx["paperclipWake"].(map[string]interface{})
	if wakePayload == nil {
		return ""
	}

	reason := asString(wakePayload["reason"], "")
	if reason == "" {
		return ""
	}

	parts := []string{"Wake reason: " + reason}

	if taskID := asString(wakePayload["taskId"], ""); taskID != "" {
		parts = append(parts, "Task ID: "+taskID)
	}
	if commentID := asString(wakePayload["commentId"], ""); commentID != "" {
		parts = append(parts, "Comment ID: "+commentID)
	}
	if resumedSession {
		parts = append(parts, "(Resuming existing session)")
	}

	return strings.Join(parts, "\n")
}
