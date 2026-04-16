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
		deploymentMode string
		authHeader     string
		expectedUserID string
		expectedSystem bool
		expectedAgent  bool
		expectedType   string
		expectedAdmin  bool
	}{
		// ── local_trusted mode ─────────────────────────────────────────────────
		{
			name:           "local_trusted: no header → board actor",
			deploymentMode: "local_trusted",
			authHeader:     "",
			expectedUserID: "local-board",
			expectedAgent:  false,
			expectedType:   "board",
			expectedAdmin:  true,
		},
		{
			name:           "local_trusted: any bearer token still board actor",
			deploymentMode: "local_trusted",
			authHeader:     "Bearer pcp_board_sometoken",
			expectedUserID: "local-board",
			expectedAgent:  false,
			expectedType:   "board",
			expectedAdmin:  true,
		},

		// ── authenticated mode ─────────────────────────────────────────────────
		{
			name:           "authenticated: No Auth Header → anonymous",
			deploymentMode: "authenticated",
			authHeader:     "",
			expectedUserID: "",
			expectedSystem: false,
			expectedAgent:  false,
			expectedType:   "anonymous",
		},
		{
			name:           "authenticated: Non-prefixed token is anonymous",
			deploymentMode: "authenticated",
			authHeader:     "Bearer system",
			expectedUserID: "",
			expectedSystem: false,
			expectedAgent:  false,
			expectedType:   "anonymous",
		},
		{
			name:           "authenticated: Non-prefixed user token is anonymous",
			deploymentMode: "authenticated",
			authHeader:     "Bearer user123",
			expectedUserID: "",
			expectedSystem: false,
			expectedAgent:  false,
			expectedType:   "anonymous",
		},
		{
			name:           "authenticated: Invalid Bearer Format",
			deploymentMode: "authenticated",
			authHeader:     "Basic user:pass",
			expectedUserID: "",
			expectedSystem: false,
			expectedAgent:  false,
			expectedType:   "anonymous",
		},
		{
			name:           "authenticated: Board token with nil DB stays anonymous",
			deploymentMode: "authenticated",
			authHeader:     "Bearer pcp_board_sometoken",
			expectedUserID: "",
			expectedSystem: false,
			expectedAgent:  false,
			expectedType:   "anonymous",
		},
		{
			name:           "authenticated: Agent token with nil DB stays anonymous",
			deploymentMode: "authenticated",
			authHeader:     "Bearer pcp_agent_sometoken",
			expectedUserID: "",
			expectedSystem: false,
			expectedAgent:  false,
			expectedType:   "anonymous",
		},

		// ── default / empty mode (treated as local_trusted) ───────────────────
		{
			name:           "empty mode defaults to local_trusted",
			deploymentMode: "",
			authHeader:     "",
			expectedUserID: "local-board",
			expectedAgent:  false,
			expectedType:   "board",
			expectedAdmin:  true,
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
			handler := ActorMiddleware(nil, AuthMiddlewareOptions{DeploymentMode: tt.deploymentMode})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

				if actor.IsInstanceAdmin != tt.expectedAdmin {
					t.Errorf("Expected IsInstanceAdmin %v, got %v", tt.expectedAdmin, actor.IsInstanceAdmin)
				}
			}))

			handler.ServeHTTP(rr, req)
		})
	}
}

func TestActorMiddleware_RunIDHeader(t *testing.T) {
	tests := []struct {
		name           string
		deploymentMode string
		runIDHeader    string
		expectedRunID  string
	}{
		{
			name:           "local_trusted: extracts x-paperclip-run-id header",
			deploymentMode: "local_trusted",
			runIDHeader:    "run-12345",
			expectedRunID:  "run-12345",
		},
		{
			name:           "local_trusted: empty header results in empty RunID",
			deploymentMode: "local_trusted",
			runIDHeader:    "",
			expectedRunID:  "",
		},
		{
			name:           "authenticated: extracts x-paperclip-run-id header",
			deploymentMode: "authenticated",
			runIDHeader:    "run-67890",
			expectedRunID:  "run-67890",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if tt.runIDHeader != "" {
				req.Header.Set("x-paperclip-run-id", tt.runIDHeader)
			}

			rr := httptest.NewRecorder()

			handler := ActorMiddleware(nil, AuthMiddlewareOptions{DeploymentMode: tt.deploymentMode})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				actor, ok := r.Context().Value(routes.ActorKey).(routes.ActorInfo)
				if !ok {
					t.Fatal("Expected routes.ActorInfo in context")
				}

				if actor.RunID != tt.expectedRunID {
					t.Errorf("Expected RunID %q, got %q", tt.expectedRunID, actor.RunID)
				}
			}))

			handler.ServeHTTP(rr, req)
		})
	}
}

