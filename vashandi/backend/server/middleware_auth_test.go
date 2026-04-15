package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chifamba/vashandi/vashandi/backend/server/routes"
)

func TestActorMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		authHeader     string
		expectedUserID string
		expectedSystem bool
		expectedAgent  bool
		expectedType   string
	}{
		{
			name:           "No Auth Header",
			authHeader:     "",
			expectedUserID: "",
			expectedSystem: false,
			expectedAgent:  false,
			expectedType:   "anonymous",
		},
		{
			name:           "Non-prefixed token is anonymous",
			authHeader:     "Bearer system",
			expectedUserID: "",
			expectedSystem: false,
			expectedAgent:  false,
			expectedType:   "anonymous",
		},
		{
			name:           "Non-prefixed user token is anonymous",
			authHeader:     "Bearer user123",
			expectedUserID: "",
			expectedSystem: false,
			expectedAgent:  false,
			expectedType:   "anonymous",
		},
		{
			name:           "Invalid Bearer Format",
			authHeader:     "Basic user:pass",
			expectedUserID: "",
			expectedSystem: false,
			expectedAgent:  false,
			expectedType:   "anonymous",
		},
		{
			name:           "Board token with nil DB stays anonymous",
			authHeader:     "Bearer pcp_board_sometoken",
			expectedUserID: "",
			expectedSystem: false,
			expectedAgent:  false,
			expectedType:   "anonymous",
		},
		{
			name:           "Agent token with nil DB stays anonymous",
			authHeader:     "Bearer pcp_agent_sometoken",
			expectedUserID: "",
			expectedSystem: false,
			expectedAgent:  false,
			expectedType:   "anonymous",
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
				actor, ok := r.Context().Value(routes.ActorKey).(routes.ActorInfo)
				if !ok {
					t.Fatalf("Expected routes.ActorInfo in context")
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

				if actor.ActorType != tt.expectedType {
					t.Errorf("Expected ActorType %q, got %q", tt.expectedType, actor.ActorType)
				}
			}))

			handler.ServeHTTP(rr, req)
		})
	}
}
