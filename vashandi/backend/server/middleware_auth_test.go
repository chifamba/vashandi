package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestActorMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		authHeader     string
		expectedUserID string
		expectedSystem bool
		expectedAgent  bool
	}{
		{
			name:           "No Auth Header",
			authHeader:     "",
			expectedUserID: "",
			expectedSystem: false,
			expectedAgent:  false,
		},
		{
			name:           "Non-prefixed token is anonymous",
			authHeader:     "Bearer system",
			expectedUserID: "",
			expectedSystem: false,
			expectedAgent:  false,
		},
		{
			name:           "Non-prefixed user token is anonymous",
			authHeader:     "Bearer user123",
			expectedUserID: "",
			expectedSystem: false,
			expectedAgent:  false,
		},
		{
			name:           "Invalid Bearer Format",
			authHeader:     "Basic user:pass",
			expectedUserID: "",
			expectedSystem: false,
			expectedAgent:  false,
		},
		{
			name:           "Board token with nil DB stays anonymous",
			authHeader:     "Bearer pcp_board_sometoken",
			expectedUserID: "",
			expectedSystem: false,
			expectedAgent:  false,
		},
		{
			name:           "Agent token with nil DB stays anonymous",
			authHeader:     "Bearer pcp_agent_sometoken",
			expectedUserID: "",
			expectedSystem: false,
			expectedAgent:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			rr := httptest.NewRecorder()

			// Pass nil db — no DB lookups are performed in these unit tests.
			handler := ActorMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				actor, ok := r.Context().Value(ActorContextKey).(ActorInfo)
				if !ok {
					t.Fatalf("Expected ActorInfo in context")
				}

				if actor.UserID != tt.expectedUserID {
					t.Errorf("Expected UserID %q, got %q", tt.expectedUserID, actor.UserID)
				}

				if actor.IsSystem != tt.expectedSystem {
					t.Errorf("Expected IsSystem %v, got %v", tt.expectedSystem, actor.IsSystem)
				}

				if actor.IsAgent != tt.expectedAgent {
					t.Errorf("Expected IsAgent %v, got %v", tt.expectedAgent, actor.IsAgent)
				}
			}))

			handler.ServeHTTP(rr, req)
		})
	}
}
