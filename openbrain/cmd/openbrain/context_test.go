package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chifamba/vashandi/openbrain/internal/brain"
	mcppkg "github.com/chifamba/vashandi/openbrain/internal/mcp"
)

func TestContextCompileAndTriggers(t *testing.T) {
	service := setupService(t)
	_, err := service.CreateMemory(t.Context(), brain.Actor{Kind: "service", NamespaceID: "ns1", TrustTier: 4}, brain.MemoryPayload{NamespaceID: "ns1", EntityType: "fact", Text: "build failures are often fixed by rerunning migrations", Tier: 1})
	require.NoError(t, err)
	app := &application{service: service, mcpServer: mcppkg.NewServer(service)}
	server := httptest.NewServer(app.routes())
	defer server.Close()

	body, _ := json.Marshal(brain.ContextRequest{NamespaceID: "ns1", AgentID: "agent-1", TaskQuery: "build failures", TokenBudget: 200})
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/context/compile", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer dev_secret_token")
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, http.StatusOK, res.StatusCode)
	var payload map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&payload))
	assert.Contains(t, payload, "snippets")

	triggerBody, _ := json.Marshal(brain.TriggerRequest{AgentID: "agent-1", TaskQuery: "build failures", TokenBudget: 200})
	req2, _ := http.NewRequest(http.MethodPost, server.URL+"/internal/v1/namespaces/ns1/triggers/run_start", bytes.NewReader(triggerBody))
	req2.Header.Set("Authorization", "Bearer dev_secret_token")
	res2, err := http.DefaultClient.Do(req2)
	require.NoError(t, err)
	defer res2.Body.Close()
	assert.Equal(t, http.StatusOK, res2.StatusCode)
}
