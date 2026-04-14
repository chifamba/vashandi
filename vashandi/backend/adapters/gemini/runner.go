package gemini

import (
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

type Part struct {
Text             string            `json:"text,omitempty"`
FunctionCall     *FunctionCall     `json:"functionCall,omitempty"`
FunctionResponse *FunctionResponse `json:"functionResponse,omitempty"`
}

type FunctionCall struct {
Name string          `json:"name"`
Args json.RawMessage `json:"args"`
}

type FunctionResponse struct {
Name     string          `json:"name"`
Response json.RawMessage `json:"response"`
}

type Content struct {
Role  string `json:"role"`
Parts []Part `json:"parts"`
}

type FunctionDeclaration struct {
Name        string          `json:"name"`
Description string          `json:"description,omitempty"`
Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type Tool struct {
FunctionDeclarations []FunctionDeclaration `json:"function_declarations"`
}

type ToolCall struct {
Name string
Args json.RawMessage
}

type Usage struct {
PromptTokenCount     int `json:"promptTokenCount"`
CandidatesTokenCount int `json:"candidatesTokenCount"`
}

type RunResult struct {
Content    string     `json:"content"`
ToolCalls  []ToolCall `json:"toolCalls,omitempty"`
StopReason string     `json:"stopReason"`
Usage      Usage      `json:"usage"`
}

type ToolExecutor func(ctx context.Context, call ToolCall) (string, error)

type Runner struct {
ApiKey  string
BaseURL string
Model   string
client  *http.Client
}

func NewRunner(apiKey string) *Runner {
baseURL := os.Getenv("GEMINI_API_BASE")
if baseURL == "" {
baseURL = "https://generativelanguage.googleapis.com"
}
model := os.Getenv("GEMINI_MODEL")
if model == "" {
model = "gemini-2.0-flash"
}
return &Runner{
ApiKey:  apiKey,
BaseURL: baseURL,
Model:   model,
client:  &http.Client{Timeout: 5 * time.Minute},
}
}

func (r *Runner) ExecuteRun(ctx context.Context, agentId, runContextId string) error {
if r.ApiKey == "" {
return fmt.Errorf("GEMINI_API_KEY is not set")
}
slog.Info("Gemini ExecuteRun", "agent", agentId, "run", runContextId)
result, err := r.SendMessages(ctx, "", []Content{{Role: "user", Parts: []Part{{Text: "Hello"}}}}, nil)
if err != nil {
return err
}
slog.Info("Gemini ExecuteRun complete", "stopReason", result.StopReason)
return nil
}

func (r *Runner) SendMessages(ctx context.Context, system string, contents []Content, tools []Tool) (*RunResult, error) {
body := map[string]interface{}{
"contents": contents,
"generationConfig": map[string]interface{}{
"maxOutputTokens": 4096,
},
}
if system != "" {
body["systemInstruction"] = map[string]interface{}{
"parts": []map[string]string{{"text": system}},
}
}
if len(tools) > 0 {
body["tools"] = tools
}

bodyBytes, _ := json.Marshal(body)
url := fmt.Sprintf("%s/v1beta/models/%s:streamGenerateContent?key=%s&alt=sse", r.BaseURL, r.Model, r.ApiKey)

req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
if err != nil {
return nil, err
}
req.Header.Set("Content-Type", "application/json")

resp, err := r.client.Do(req)
if err != nil {
return nil, fmt.Errorf("gemini request: %w", err)
}
defer resp.Body.Close()

if resp.StatusCode >= 400 {
b, _ := io.ReadAll(resp.Body)
return nil, fmt.Errorf("gemini error %d: %s", resp.StatusCode, b)
}

return r.parseResponse(resp.Body)
}

func (r *Runner) parseResponse(reader io.Reader) (*RunResult, error) {
result := &RunResult{}
var textParts []string

body, err := io.ReadAll(reader)
if err != nil {
return nil, err
}

for _, line := range strings.Split(string(body), "\n") {
line = strings.TrimSpace(line)
if !strings.HasPrefix(line, "data: ") {
continue
}
data := strings.TrimPrefix(line, "data: ")
if data == "" || data == "[DONE]" {
continue
}

var chunk struct {
Candidates []struct {
Content      Content `json:"content"`
FinishReason string  `json:"finishReason"`
} `json:"candidates"`
UsageMetadata Usage `json:"usageMetadata"`
}
if err := json.Unmarshal([]byte(data), &chunk); err != nil {
continue
}

result.Usage = chunk.UsageMetadata

for _, cand := range chunk.Candidates {
if cand.FinishReason != "" {
result.StopReason = cand.FinishReason
}
for _, part := range cand.Content.Parts {
if part.Text != "" {
textParts = append(textParts, part.Text)
}
if part.FunctionCall != nil {
result.ToolCalls = append(result.ToolCalls, ToolCall{
Name: part.FunctionCall.Name,
Args: part.FunctionCall.Args,
})
}
}
}
}

result.Content = strings.Join(textParts, "")
return result, nil
}

func (r *Runner) ConversationLoop(ctx context.Context, system string, contents []Content, tools []Tool, exec ToolExecutor, maxIter int) ([]Content, *RunResult, error) {
if maxIter <= 0 {
maxIter = 10
}
history := make([]Content, len(contents))
copy(history, contents)

var last *RunResult
for i := 0; i < maxIter; i++ {
res, err := r.SendMessages(ctx, system, history, tools)
if err != nil {
return history, nil, err
}
last = res

var modelParts []Part
if res.Content != "" {
modelParts = append(modelParts, Part{Text: res.Content})
}
for _, tc := range res.ToolCalls {
modelParts = append(modelParts, Part{FunctionCall: &FunctionCall{Name: tc.Name, Args: tc.Args}})
}
history = append(history, Content{Role: "model", Parts: modelParts})

if len(res.ToolCalls) == 0 || res.StopReason == "STOP" {
break
}

var userParts []Part
for _, tc := range res.ToolCalls {
out, err2 := exec(ctx, tc)
if err2 != nil {
out = `{"error":"` + err2.Error() + `"}`
}
userParts = append(userParts, Part{
FunctionResponse: &FunctionResponse{
Name:     tc.Name,
Response: json.RawMessage(`{"output":` + string(jsonString(out)) + `}`),
},
})
}
history = append(history, Content{Role: "user", Parts: userParts})
}
return history, last, nil
}

func jsonString(s string) json.RawMessage {
b, _ := json.Marshal(s)
return json.RawMessage(b)
}
