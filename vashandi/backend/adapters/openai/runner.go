package openai

import (
"bufio"
"bytes"
"context"
"encoding/json"
"fmt"
"io"
"log/slog"
"net/http"
"os"
"strings"
"time"
)

type ChatMessage struct {
Role       string     `json:"role"`
Content    string     `json:"content,omitempty"`
ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
ToolCallID string     `json:"tool_call_id,omitempty"`
}

type ToolCall struct {
ID       string       `json:"id"`
Type     string       `json:"type"`
Function FunctionCall `json:"function"`
}

type FunctionCall struct {
Name      string `json:"name"`
Arguments string `json:"arguments"`
}

type Tool struct {
Type     string       `json:"type"`
Function ToolFunction `json:"function"`
}

type ToolFunction struct {
Name        string          `json:"name"`
Description string          `json:"description,omitempty"`
Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type Usage struct {
PromptTokens     int `json:"prompt_tokens"`
CompletionTokens int `json:"completion_tokens"`
}

type RunResult struct {
Content    string     `json:"content"`
ToolCalls  []ToolCall `json:"toolCalls,omitempty"`
StopReason string     `json:"stopReason"`
Usage      Usage      `json:"usage"`
}

type ToolExecutor func(ctx context.Context, call ToolCall) (string, error)

type Runner struct {
ApiKey    string
BaseURL   string
Model     string
MaxTokens int
client    *http.Client
}

func NewRunner(apiKey string) *Runner {
baseURL := os.Getenv("OPENAI_API_BASE")
if baseURL == "" {
baseURL = "https://api.openai.com"
}
model := os.Getenv("OPENAI_MODEL")
if model == "" {
model = "gpt-4o"
}
return &Runner{
ApiKey: apiKey, BaseURL: baseURL, Model: model, MaxTokens: 4096,
client: &http.Client{Timeout: 5 * time.Minute},
}
}

func (r *Runner) ExecuteRun(ctx context.Context, agentId, runContextId string) error {
if r.ApiKey == "" {
return fmt.Errorf("OPENAI_API_KEY is not set")
}
slog.Info("OpenAI ExecuteRun", "agent", agentId, "run", runContextId)
result, err := r.SendMessages(ctx, "", []ChatMessage{{Role: "user", Content: "Hello"}}, nil)
if err != nil {
return err
}
slog.Info("OpenAI ExecuteRun complete", "stopReason", result.StopReason)
return nil
}

func (r *Runner) SendMessages(ctx context.Context, system string, messages []ChatMessage, tools []Tool) (*RunResult, error) {
var msgs []ChatMessage
if system != "" {
msgs = append(msgs, ChatMessage{Role: "system", Content: system})
}
msgs = append(msgs, messages...)

body := map[string]interface{}{
"model":      r.Model,
"max_tokens": r.MaxTokens,
"messages":   msgs,
"stream":     true,
}
if len(tools) > 0 {
body["tools"] = tools
}

bodyBytes, _ := json.Marshal(body)
req, err := http.NewRequestWithContext(ctx, "POST", r.BaseURL+"/v1/chat/completions", bytes.NewReader(bodyBytes))
if err != nil {
return nil, err
}
req.Header.Set("Content-Type", "application/json")
req.Header.Set("Authorization", "Bearer "+r.ApiKey)

resp, err := r.client.Do(req)
if err != nil {
return nil, fmt.Errorf("openai request: %w", err)
}
defer resp.Body.Close()

if resp.StatusCode >= 400 {
b, _ := io.ReadAll(resp.Body)
return nil, fmt.Errorf("openai error %d: %s", resp.StatusCode, b)
}
return r.parseSSE(resp.Body)
}

func (r *Runner) parseSSE(reader io.Reader) (*RunResult, error) {
result := &RunResult{}
var textParts []string
tcMap := make(map[int]*ToolCall)

scanner := bufio.NewScanner(reader)
for scanner.Scan() {
line := scanner.Text()
if !strings.HasPrefix(line, "data: ") {
continue
}
data := strings.TrimPrefix(line, "data: ")
if data == "[DONE]" {
break
}

var chunk struct {
Choices []struct {
Delta struct {
Content   string `json:"content"`
ToolCalls []struct {
Index    int    `json:"index"`
ID       string `json:"id"`
Type     string `json:"type"`
Function struct {
Name      string `json:"name"`
Arguments string `json:"arguments"`
} `json:"function"`
} `json:"tool_calls"`
} `json:"delta"`
FinishReason string `json:"finish_reason"`
} `json:"choices"`
Usage *Usage `json:"usage"`
}
if err := json.Unmarshal([]byte(data), &chunk); err != nil {
continue
}
if chunk.Usage != nil {
result.Usage = *chunk.Usage
}
for _, choice := range chunk.Choices {
if choice.Delta.Content != "" {
textParts = append(textParts, choice.Delta.Content)
}
for _, tc := range choice.Delta.ToolCalls {
ex, ok := tcMap[tc.Index]
if !ok {
ex = &ToolCall{Type: "function"}
tcMap[tc.Index] = ex
}
if tc.ID != "" {
ex.ID = tc.ID
}
ex.Function.Name += tc.Function.Name
ex.Function.Arguments += tc.Function.Arguments
}
if choice.FinishReason != "" {
result.StopReason = choice.FinishReason
}
}
}

result.Content = strings.Join(textParts, "")
for i := 0; i < len(tcMap); i++ {
if tc, ok := tcMap[i]; ok {
result.ToolCalls = append(result.ToolCalls, *tc)
}
}
return result, scanner.Err()
}

func (r *Runner) ConversationLoop(ctx context.Context, system string, msgs []ChatMessage, tools []Tool, exec ToolExecutor, maxIter int) ([]ChatMessage, *RunResult, error) {
if maxIter <= 0 {
maxIter = 10
}
history := make([]ChatMessage, len(msgs))
copy(history, msgs)

var last *RunResult
for i := 0; i < maxIter; i++ {
res, err := r.SendMessages(ctx, system, history, tools)
if err != nil {
return history, nil, err
}
last = res
history = append(history, ChatMessage{Role: "assistant", Content: res.Content, ToolCalls: res.ToolCalls})
if len(res.ToolCalls) == 0 || res.StopReason == "stop" {
break
}
for _, tc := range res.ToolCalls {
out, err2 := exec(ctx, tc)
if err2 != nil {
out = "Error: " + err2.Error()
}
history = append(history, ChatMessage{Role: "tool", ToolCallID: tc.ID, Content: out})
}
}
return history, last, nil
}
