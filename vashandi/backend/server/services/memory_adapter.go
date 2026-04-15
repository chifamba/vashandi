package services

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

	"github.com/chifamba/vashandi/vashandi/backend/shared/tls"
)

// MemoryAdapter defines the interface for interacting with OpenBrain
type MemoryAdapter interface {
	CreateNamespace(ctx context.Context, namespaceID, companyID string) error
	IngestMemory(ctx context.Context, namespaceID, text string, metadata map[string]string) error
	CreateMemory(ctx context.Context, namespaceID string, payload MemoryPayload) error
	QueryMemory(ctx context.Context, namespaceID, query string, limit int) ([]MemoryResult, error)
	CompileContext(ctx context.Context, req ContextRequest) (map[string]interface{}, error)
	RegisterAgent(ctx context.Context, namespaceID, agentID, name string) error
	DeregisterAgent(ctx context.Context, namespaceID, agentID string) error
	HandleTrigger(ctx context.Context, namespaceID, triggerType string, req TriggerRequest) (*TriggerResponse, error)
	ExportAudit(ctx context.Context, namespaceID, format string) ([]byte, string, error)
	ArchiveNamespace(ctx context.Context, namespaceID string) error
	DeleteNamespace(ctx context.Context, namespaceID string) error
	ListProposals(ctx context.Context, namespaceID string) ([]map[string]interface{}, error)
	ResolveProposal(ctx context.Context, namespaceID, proposalID, action string) error
}

type MemoryPayload struct {
	EntityType string                 `json:"entityType"`
	Text       string                 `json:"text"`
	Title      string                 `json:"title,omitempty"`
	Tier       int                    `json:"tier,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

type ContextRequest struct {
	NamespaceID string `json:"namespaceId"`
	AgentID     string `json:"agentId"`
	Intent      string `json:"intent"`
	Query       string `json:"query,omitempty"`
}

type TriggerRequest struct {
	AgentID     string         `json:"agentId,omitempty"`
	TaskQuery   string         `json:"taskQuery,omitempty"`
	Intent      string         `json:"intent,omitempty"`
	TokenBudget int            `json:"tokenBudget,omitempty"`
	Content     string         `json:"content,omitempty"`
	Summary     string         `json:"summary,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	ErrorText   string         `json:"errorText,omitempty"`
}

type TriggerResponse struct {
	Status      string   `json:"status"`
	PacketID    string   `json:"packetId,omitempty"`
	CreatedIDs  []string `json:"createdIds,omitempty"`
	ProposalIDs []string `json:"proposalIds,omitempty"`
}

type MemoryResult struct {
	ID       string            `json:"id"`
	Text     string            `json:"text"`
	Metadata map[string]string `json:"metadata"`
}

type OpenBrainAdapter struct {
	client     *http.Client
	baseURL    string
	authSecret string
}

func NewOpenBrainAdapter() *OpenBrainAdapter {
	baseURL := os.Getenv("OPENBRAIN_REST_URL")
	if baseURL == "" {
		baseURL = "https://openbrain:3101"
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Load mTLS config
	tlsCfg := tls.LoadConfigFromEnv()
	if tlsCfg.Enabled {
		cfg, err := tls.GetClientConfig(context.Background(), tlsCfg)
		if err == nil {
			client.Transport = &http.Transport{
				TLSClientConfig: cfg,
			}
			slog.Info("OpenBrain client configured with mTLS")
		} else {
			slog.Error("Failed to configure mTLS for OpenBrain client", "error", err)
		}
	}

	return &OpenBrainAdapter{
		client:  client,
		baseURL: baseURL,
	}
}

func (o *OpenBrainAdapter) CreateNamespace(ctx context.Context, namespaceID, companyID string) error {
	url := fmt.Sprintf("%s/internal/v1/namespaces", o.baseURL)
	payload := map[string]string{
		"namespaceId": namespaceID,
		"companyId":   companyID,
	}
	bodyBytes, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if err := o.do(req); err != nil {
		slog.ErrorContext(ctx, "Failed to register namespace in OpenBrain", "namespaceId", namespaceID, "error", err)
		return err
	}
	slog.InfoContext(ctx, "Namespace registered in OpenBrain", "namespaceId", namespaceID, "companyId", companyID)
	return nil
}

func (o *OpenBrainAdapter) IngestMemory(ctx context.Context, namespaceID, text string, metadata map[string]string) error {
	return o.CreateMemory(ctx, namespaceID, MemoryPayload{
		EntityType: metadata["type"],
		Text:       text,
		Metadata:   stringMapToAny(metadata),
	})
}

func (o *OpenBrainAdapter) CreateMemory(ctx context.Context, namespaceID string, payload MemoryPayload) error {
	url := fmt.Sprintf("%s/api/v1/memories?namespaceId=%s", o.baseURL, namespaceID)
	bodyBytes, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return o.do(req)
}

func (o *OpenBrainAdapter) RegisterAgent(ctx context.Context, namespaceID, agentID, name string) error {
	url := fmt.Sprintf("%s/internal/v1/namespaces/%s/agents", o.baseURL, namespaceID)
	payload := map[string]string{
		"agentId": agentID,
		"name":    name,
	}
	bodyBytes, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return o.do(req)
}

func (o *OpenBrainAdapter) DeregisterAgent(ctx context.Context, namespaceID, agentID string) error {
	url := fmt.Sprintf("%s/internal/v1/namespaces/%s/agents/%s", o.baseURL, namespaceID, agentID)
	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return err
	}
	return o.do(req)
}

func (o *OpenBrainAdapter) CompileContext(ctx context.Context, reqData ContextRequest) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/v1/context/compile", o.baseURL)
	bodyBytes, _ := json.Marshal(reqData)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	
	resp, err := o.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("openbrain error: %d", resp.StatusCode)
	}
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

func (o *OpenBrainAdapter) do(req *http.Request) error {
	resp, err := o.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("openbrain error: %d", resp.StatusCode)
	}
	return nil
}

func stringMapToAny(m map[string]string) map[string]interface{} {
	out := make(map[string]interface{})
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (o *OpenBrainAdapter) QueryMemory(ctx context.Context, namespaceID, query string, limit int) ([]MemoryResult, error) {
	url := fmt.Sprintf("%s/api/v1/namespaces/%s/memories/query", o.baseURL, namespaceID)

	payload := map[string]interface{}{
		"query": query,
		"limit": limit,
	}
	bodyBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		slog.Error("Failed to query memory from OpenBrain (fallback active)", "error", err)
		return []MemoryResult{}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("openbrain error: %d", resp.StatusCode)
	}

	var result struct {
		Records []MemoryResult `json:"records"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Records, nil
}

func (o *OpenBrainAdapter) HandleTrigger(ctx context.Context, namespaceID, triggerType string, triggerReq TriggerRequest) (*TriggerResponse, error) {
	url := fmt.Sprintf("%s/internal/v1/namespaces/%s/triggers/%s", o.baseURL, namespaceID, triggerType)

	bodyBytes, _ := json.Marshal(triggerReq)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("openbrain error: %d", resp.StatusCode)
	}

	var result TriggerResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (o *OpenBrainAdapter) ExportAudit(ctx context.Context, namespaceID, format string) ([]byte, string, error) {
	url := fmt.Sprintf("%s/internal/v1/namespaces/%s/audit/export?format=%s", o.baseURL, namespaceID, format)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, "", err
	}

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, "", fmt.Errorf("openbrain error: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	return body, resp.Header.Get("Content-Type"), nil
}

func (o *OpenBrainAdapter) ArchiveNamespace(ctx context.Context, namespaceID string) error {
	// OpenBrain uses DELETE for both archiving and deletion logic depending on internal status
	url := fmt.Sprintf("%s/internal/v1/namespaces/%s", o.baseURL, namespaceID)
	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return err
	}
	return o.do(req)
}

func (o *OpenBrainAdapter) DeleteNamespace(ctx context.Context, namespaceID string) error {
	url := fmt.Sprintf("%s/internal/v1/namespaces/%s", o.baseURL, namespaceID)
	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return err
	}
	return o.do(req)
}

func (o *OpenBrainAdapter) ListProposals(ctx context.Context, namespaceID string) ([]map[string]interface{}, error) {
	url := fmt.Sprintf("%s/internal/v1/namespaces/%s/proposals", o.baseURL, namespaceID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("openbrain error: %d", resp.StatusCode)
	}
	var result []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

func (o *OpenBrainAdapter) ResolveProposal(ctx context.Context, namespaceID, proposalID, action string) error {
	url := fmt.Sprintf("%s/internal/v1/namespaces/%s/proposals/%s", o.baseURL, namespaceID, proposalID)
	payload := map[string]string{"action": action}
	bodyBytes, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return o.do(req)
}

// CompileContextXML fetches semantic memory and formats it as XML for LLM system prompt injection.
func (o *OpenBrainAdapter) CompileContextXML(ctx context.Context, companyID, agentID, taskID string) (string, error) {
data, err := o.CompileContext(ctx, ContextRequest{
NamespaceID: companyID,
AgentID:     agentID,
Intent:      "heartbeat_invocation",
Query:       taskID,
})
if err != nil || data == nil {
return "", err
}

var sb strings.Builder
sb.WriteString("<memory_context>\n")
sb.WriteString(fmt.Sprintf("  <meta agent_id=%q task_id=%q timestamp=%q/>\n",
agentID, taskID, time.Now().UTC().Format(time.RFC3339)))

if memories, ok := data["memories"]; ok {
if memList, ok := memories.([]interface{}); ok {
sb.WriteString(fmt.Sprintf("  <memories count=\"%d\">\n", len(memList)))
for _, m := range memList {
if mMap, ok := m.(map[string]interface{}); ok {
title, _ := mMap["title"].(string)
text, _ := mMap["text"].(string)
tier, _ := mMap["tier"].(float64)
relevance, _ := mMap["relevance"].(float64)
sb.WriteString(fmt.Sprintf("    <memory title=%q tier=\"%.0f\" relevance=\"%.4f\">\n",
xmlEscape(title), tier, relevance))
sb.WriteString("      " + xmlEscape(text) + "\n")
sb.WriteString("    </memory>\n")
}
}
sb.WriteString("  </memories>\n")
}
}

sb.WriteString("</memory_context>")
return sb.String(), nil
}

// InjectContextIntoPrompt prepends an XML memory block to a system prompt.
func InjectContextIntoPrompt(systemPrompt, memoryXML string) string {
if memoryXML == "" {
return systemPrompt
}
return "<agent_memory>\n" + memoryXML + "\n</agent_memory>\n\n" + systemPrompt
}

var xmlEscaper = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	`"`, "&quot;",
	"'", "&#39;",
)

func xmlEscape(s string) string {
	return xmlEscaper.Replace(s)
}
