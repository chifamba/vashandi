package main

import (
	"net/http"
	"os"
	"strings"
)

// AuthMiddleware intercepts requests and validates the Authorization header
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Define the expected token (fallback for dev)
		expectedToken := os.Getenv("OPENBRAIN_API_KEY")
		if expectedToken == "" {
			expectedToken = "dev_secret_token"
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Unauthorized: missing Authorization header", http.StatusUnauthorized)
			return
		}

		// Check for Bearer token format
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "Unauthorized: invalid Authorization header format", http.StatusUnauthorized)
			return
		}

		// Validate the token
		token := parts[1]
		if token != expectedToken {
			http.Error(w, "Forbidden: invalid token", http.StatusForbidden)
			return
		}

		// Proceed to the next handler if authorized
		next.ServeHTTP(w, r)
	})
}
