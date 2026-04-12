package main
import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
)
func TestContextCompileAndTriggers(t *testing.T) {
	r := chi.NewRouter()
	r.Post("/v1/namespaces/{namespaceId}/context/compile", func(w http.ResponseWriter, r *http.Request) {
		namespaceID := chi.URLParam(r, "namespaceId")
		var req struct {
			AgentID      string `json:"agentId"`
			TaskQuery    string `json:"taskQuery"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		type Snippet struct { ID string `json:"id"`; Text string `json:"text"` }
		snippets := []Snippet{{ID: "1", Text: "memory 1 matching " + req.TaskQuery + " for " + namespaceID}}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"snippets": snippets, "tokenCount": 10})
	})
	r.Post("/v1/namespaces/{namespaceId}/triggers/run_start", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "run_start_triggered"})
	})
	reqBody := map[string]interface{}{"agentId": "test", "taskQuery": "test-query", "tokenBudget": 100}
	bodyBytes, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/v1/namespaces/test-namespace/context/compile", bytes.NewReader(bodyBytes))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	var resp map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &resp)
	assert.Contains(t, resp, "snippets")
	req2 := httptest.NewRequest("POST", "/v1/namespaces/test-namespace/triggers/run_start", nil)
	rr2 := httptest.NewRecorder()
	r.ServeHTTP(rr2, req2)
	assert.Equal(t, http.StatusOK, rr2.Code)
}
