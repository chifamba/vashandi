package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
)

// MemoryAdapter defines the interface for interacting with OpenBrain
type MemoryAdapter interface {
	IngestMemory(ctx context.Context, namespaceID, text string, metadata map[string]string) error
	QueryMemory(ctx context.Context, namespaceID, query string, limit int) ([]MemoryResult, error)
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
		baseURL = "http://openbrain:3101"
	}
	secret := os.Getenv("OPENBRAIN_AUTH_SECRET")
	if secret == "" {
		secret = "dev_secret_token"
	}

	return &OpenBrainAdapter{
		client:     &http.Client{},
		baseURL:    baseURL,
		authSecret: secret,
	}
}

func (o *OpenBrainAdapter) IngestMemory(ctx context.Context, namespaceID, text string, metadata map[string]string) error {
	url := fmt.Sprintf("%s/v1/namespaces/%s/memories", o.baseURL, namespaceID)

	payload := map[string]interface{}{
		"records": []map[string]interface{}{
			{
				"text":     text,
				"metadata": metadata,
			},
		},
	}
	bodyBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.authSecret)

	resp, err := o.client.Do(req)
	if err != nil {
		slog.Error("Failed to ingest memory to OpenBrain", "error", err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("openbrain returned error status: %d", resp.StatusCode)
	}

	return nil
}

func (o *OpenBrainAdapter) QueryMemory(ctx context.Context, namespaceID, query string, limit int) ([]MemoryResult, error) {
	url := fmt.Sprintf("%s/v1/namespaces/%s/memories/query", o.baseURL, namespaceID)

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
	req.Header.Set("Authorization", "Bearer "+o.authSecret)

	resp, err := o.client.Do(req)
	if err != nil {
		slog.Error("Failed to query memory from OpenBrain (fallback active)", "error", err)
		// Fallback Strategy (Task 2.4): Degrade gracefully
		return []MemoryResult{}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("openbrain returned error status: %d", resp.StatusCode)
	}

	var result struct {
		Records []MemoryResult `json:"records"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Records, nil
}

type Proposal struct {
	ID            string   `json:"id"`
	NamespaceID   string   `json:"namespace_id"`
	MemoryIDs     []string `json:"memory_ids"`
	SuggestedText string   `json:"suggested_text"`
	Status        string   `json:"status"`
}

func (o *OpenBrainAdapter) ListProposals(ctx context.Context, namespaceID string) ([]Proposal, error) {
	url := fmt.Sprintf("%s/v1/namespaces/%s/proposals", o.baseURL, namespaceID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+o.authSecret)

	resp, err := o.client.Do(req)
	if err != nil {
		slog.Error("Failed to list proposals from OpenBrain", "error", err)
		return []Proposal{}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("openbrain returned error status: %d", resp.StatusCode)
	}

	var results []Proposal
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, err
	}

	return results, nil
}

func (o *OpenBrainAdapter) ResolveProposal(ctx context.Context, namespaceID, proposalID, action string) error {
	url := fmt.Sprintf("%s/v1/namespaces/%s/proposals/%s/resolve", o.baseURL, namespaceID, proposalID)

	payload := map[string]string{"action": action}
	bodyBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.authSecret)

	resp, err := o.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("openbrain returned error status: %d", resp.StatusCode)
	}

	return nil
}
