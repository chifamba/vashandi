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
	}{
		{
			name:           "No Auth Header",
			authHeader:     "",
			expectedUserID: "",
			expectedSystem: false,
		},
		{
			name:           "Bearer System - Vulnerability Removed",
			authHeader:     "Bearer system",
			expectedUserID: "system", // It should now just be treated as a normal user ID "system"
			expectedSystem: false,
		},
		{
			name:           "Normal User Token",
			authHeader:     "Bearer user123",
			expectedUserID: "user123",
			expectedSystem: false,
		},
		{
			name:           "Invalid Bearer Format",
			authHeader:     "Basic user:pass",
			expectedUserID: "",
			expectedSystem: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			rr := httptest.NewRecorder()

			handler := ActorMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			}))

			handler.ServeHTTP(rr, req)
		})
	}
}
