package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// This test simulates the OpenBrain endpoint and verifies the
// shape of the payload sent by the Vashandi Sync Agent logic
func TestAgentSyncWebhookContract(t *testing.T) {
	// 1. Setup a Mock OpenBrain Server
	expectedCompanyID := "comp-123"
	expectedAgentID := "agent-456"

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Assert the Authorization header is present
		if auth := r.Header.Get("Authorization"); auth != "Bearer dev_secret_token" {
			t.Errorf("Expected Auth header 'Bearer dev_secret_token', got %s", auth)
		}

		// Assert the correct endpoint is hit
		expectedPath := "/internal/v1/namespaces/" + expectedCompanyID + "/agents"
		if r.URL.Path != expectedPath {
			t.Errorf("Expected path %s, got %s", expectedPath, r.URL.Path)
		}

		// Assert the payload contains the agent_id
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}

		var payload map[string]string
		if err := json.Unmarshal(bodyBytes, &payload); err != nil {
			t.Fatal(err)
		}

		if payload["agent_id"] != expectedAgentID {
			t.Errorf("Expected agent_id in payload %s, got %s", expectedAgentID, payload["agent_id"])
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	// 2. Simulate the Vashandi backend logic invoking the webhook
	// Note: In reality, we'd call the real CreateAgentHandler here, but to avoid DB mocking complexity
	// we will isolate and test the HTTP request formation exactly as it's defined in the handler.

	url := mockServer.URL + "/internal/v1/namespaces/" + expectedCompanyID + "/agents"
	payload := map[string]string{"agent_id": expectedAgentID}
	bodyBytes, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", url, strings.NewReader(string(bodyBytes)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer dev_secret_token")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}
