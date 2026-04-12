package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/chifamba/vashandi/openbrain/internal/brain"
)

type authContextKey string

const actorContextKey authContextKey = "openbrain-actor"

func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
		if authHeader == "" {
			http.Error(w, "Unauthorized: missing Authorization header", http.StatusUnauthorized)
			return
		}
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "Unauthorized: invalid Authorization header format", http.StatusUnauthorized)
			return
		}
		token := parts[1]
		actor, ok := actorFromToken(token)
		if !ok {
			http.Error(w, "Forbidden: invalid token", http.StatusForbidden)
			return
		}
		if actor.Kind == "" {
			actor.Kind = firstHeader(r, "X-OpenBrain-Actor-Kind", "X-Actor-Kind")
		}
		if actor.Kind == "" {
			actor.Kind = "service"
		}
		if actor.AgentID == "" {
			actor.AgentID = firstHeader(r, "X-OpenBrain-Agent-ID", "X-Agent-ID")
		}
		if actor.Name == "" {
			actor.Name = firstHeader(r, "X-OpenBrain-Actor-Name")
		}
		if actor.TrustTier == 0 {
			if value := firstHeader(r, "X-OpenBrain-Trust-Tier", "X-Trust-Tier"); value != "" {
				if parsed, err := strconv.Atoi(value); err == nil {
					actor.TrustTier = parsed
				}
			}
		}
		actor.RequestMeta = map[string]any{"remoteAddr": r.RemoteAddr}
		ctx := context.WithValue(r.Context(), actorContextKey, actor)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func actorFromToken(token string) (brain.Actor, bool) {
	if claims, ok := brain.ParseScopedToken(token); ok {
		return brain.Actor{Kind: claims.ActorKind, NamespaceID: claims.NamespaceID, AgentID: claims.AgentID, TrustTier: claims.TrustTier, Name: claims.Name}, true
	}
	if brain.LegacyTokenValid(token) {
		return brain.Actor{Kind: "service", TrustTier: 4}, true
	}
	return brain.Actor{}, false
}

func actorFromRequest(r *http.Request) brain.Actor {
	if actor, ok := r.Context().Value(actorContextKey).(brain.Actor); ok {
		return actor
	}
	return brain.Actor{Kind: "service", TrustTier: 4}
}

func maybeNamespaceAuthorized(w http.ResponseWriter, r *http.Request, namespaceID string) bool {
	actor := actorFromRequest(r)
	if actor.NamespaceID != "" && actor.NamespaceID != namespaceID {
		http.Error(w, "Forbidden: token namespace mismatch", http.StatusForbidden)
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(dst)
}

func firstHeader(r *http.Request, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(r.Header.Get(name)); value != "" {
			return value
		}
	}
	return ""
}
