package server

import (
	"context"
	"net/http"
	"strings"
)

type contextKey string

const ActorContextKey contextKey = "actor"

type ActorInfo struct {
	UserID     string
	IsSystem   bool
	IsAgent    bool
}

// ActorMiddleware is a stub for the auth middleware that will parse JWTs and session tokens.
func ActorMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")

		var actor ActorInfo

		if strings.HasPrefix(authHeader, "Bearer ") {
			token := strings.TrimPrefix(authHeader, "Bearer ")
			// In a real implementation, we would verify the JWT or session token here.
			// For now, we just pass the token as the user ID for testing.
			if token == "system" {
				actor = ActorInfo{IsSystem: true}
			} else {
				actor = ActorInfo{UserID: token, IsAgent: false}
			}
		}

		ctx := context.WithValue(r.Context(), ActorContextKey, actor)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
