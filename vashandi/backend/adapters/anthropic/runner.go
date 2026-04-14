package anthropic

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

type Message struct {
Role    string      `json:"role"`
Content interface{} `json:"content"`
}

type ContentBlock struct {
Type      string          `json:"type"`
Text      string          `json:"text,omitempty"`
ID        string          `json:"id,omitempty"`
Name      string          `json:"name,omitempty"`
Input     json.RawMessage `json:"input,omitempty"`
ToolUseID string          `json:"tool_use_id,omitempty"`
Content   string          `json:"content,omitempty"`
}

type Tool struct {
Name        string          `json:"name"`
Description string          `json:"description,omitempty"`
InputSchema json.RawMessage `json:"input_schema"`
}

type ToolCall struct {
ID    string          `json:"id"`
Name  string          `json:"name"`
Input json.RawMessage `json:"input"`
}

type Usage struct {
InputTokens  int `json:"input_tokens"`
OutputTokens int `json:"output_tokens"`
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
baseURL := os.Getenv("ANTHROPIC_API_BASE")
if baseURL == "" {
baseURL = "https://api.anthropic.com"
}
model := os.Getenv("ANTHROPIC_MODEL")
if model == "" {
model = "claude-sonnet-4-20250514"
}
return &Runner{
ApiKey:    apiKey,
BaseURL:   baseURL,
Model:     model,
MaxTokens: 4096,
client:    &http.Client{Timeout: 5 * time.Minute},
}
}

func (r *Runner) ExecuteRun(ctx context.Context, agentId, runContextId string) error {
if r.ApiKey == "" {
return fmt.Errorf("ANTHROPIC_API_KEY is not set")
}
slog.Info("Anthropic ExecuteRun", "agent", agentId, "run", runContextId)
result, err := r.SendMessages(ctx, "", []Message{{Role: "user", Content: "Hello"}}, nil)
if err != nil {
return err
}
slog.Info("Anthropic ExecuteRun complete", "stopReason", result.StopReason)
return nil
}

func (r *Runner) SendMessages(ctx context.Context, system string, messages []Message, tools []Tool) (*RunResult, error) {
body := map[string]interface{}{
"model":      r.Model,
"max_tokens": r.MaxTokens,
"messages":   messages,
"stream":     true,
}
if system != "" {
body["system"] = system
}
if len(tools) > 0 {
body["tools"] = tools
}

bodyBytes, _ := json.Marshal(body)
url := r.BaseURL + "/v1/messages"
req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
if err != nil {
return nil, err
}
req.Header.Set("Content-Type", "application/json")
req.Header.Set("x-api-key", r.ApiKey)
req.Header.Set("anthropic-version", "2023-06-01")

resp, err := r.client.Do(req)
if err != nil {
return nil, fmt.Errorf("anthropic request: %w", err)
}
defer resp.Body.Close()

if resp.StatusCode >= 400 {
b, _ := io.ReadAll(resp.Body)
return nil, fmt.Errorf("anthropic error %d: %s", resp.StatusCode, b)
}

return r.parseSSE(resp.Body)
}

func (r *Runner) parseSSE(reader io.Reader) (*RunResult, error) {
result := &RunResult{}
var textParts []string
var currentTC *ToolCall
var inputBuf bytes.Buffer

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

var ev map[string]json.RawMessage
if err := json.Unmarshal([]byte(data), &ev); err != nil {
continue
}
var evType string
json.Unmarshal(ev["type"], &evType)

switch evType {
case "content_block_start":
var cb struct {
ContentBlock struct {
Type string `json:"type"`
ID   string `json:"id"`
Name string `json:"name"`
} `json:"content_block"`
}
json.Unmarshal([]byte(data), &cb)
if cb.ContentBlock.Type == "tool_use" {
currentTC = &ToolCall{ID: cb.ContentBlock.ID, Name: cb.ContentBlock.Name}
inputBuf.Reset()
}
case "content_block_delta":
var delta struct {
Delta struct {
Type        string `json:"type"`
Text        string `json:"text"`
PartialJSON string `json:"partial_json"`
} `json:"delta"`
}
json.Unmarshal([]byte(data), &delta)
if delta.Delta.Type == "text_delta" {
textParts = append(textParts, delta.Delta.Text)
} else if delta.Delta.Type == "input_json_delta" {
inputBuf.WriteString(delta.Delta.PartialJSON)
}
case "content_block_stop":
if currentTC != nil {
currentTC.Input = json.RawMessage(inputBuf.Bytes())
result.ToolCalls = append(result.ToolCalls, *currentTC)
currentTC = nil
}
case "message_delta":
var md struct {
Delta struct {
StopReason string `json:"stop_reason"`
} `json:"delta"`
Usage struct {
OutputTokens int `json:"output_tokens"`
} `json:"usage"`
}
json.Unmarshal([]byte(data), &md)
if md.Delta.StopReason != "" {
result.StopReason = md.Delta.StopReason
}
if md.Usage.OutputTokens > 0 {
result.Usage.OutputTokens = md.Usage.OutputTokens
}
case "message_start":
var ms struct {
Message struct {
Usage Usage `json:"usage"`
} `json:"message"`
}
json.Unmarshal([]byte(data), &ms)
result.Usage = ms.Message.Usage
}
}
result.Content = strings.Join(textParts, "")
return result, scanner.Err()
}

func (r *Runner) ConversationLoop(ctx context.Context, system string, msgs []Message, tools []Tool, exec ToolExecutor, maxIter int) ([]Message, *RunResult, error) {
if maxIter <= 0 {
maxIter = 10
}
history := make([]Message, len(msgs))
copy(history, msgs)

var last *RunResult
for i := 0; i < maxIter; i++ {
res, err := r.SendMessages(ctx, system, history, tools)
if err != nil {
return history, nil, err
}
last = res

var blocks []ContentBlock
if res.Content != "" {
blocks = append(blocks, ContentBlock{Type: "text", Text: res.Content})
}
for _, tc := range res.ToolCalls {
blocks = append(blocks, ContentBlock{Type: "tool_use", ID: tc.ID, Name: tc.Name, Input: tc.Input})
}
history = append(history, Message{Role: "assistant", Content: blocks})

if len(res.ToolCalls) == 0 || res.StopReason == "end_turn" {
break
}

var results []ContentBlock
for _, tc := range res.ToolCalls {
out, err2 := exec(ctx, tc)
if err2 != nil {
out = "Error: " + err2.Error()
}
results = append(results, ContentBlock{Type: "tool_result", ToolUseID: tc.ID, Content: out})
}
history = append(history, Message{Role: "user", Content: results})
}
return history, last, nil
}
