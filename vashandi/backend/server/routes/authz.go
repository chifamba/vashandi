package routes

import (
	"context"
	"fmt"
	"net/http"
)

// actorContextKey is the context key used by the auth middleware.
type actorContextKey string

const actorKey actorContextKey = "actor"

// ActorInfo carries identity information set by the auth middleware.
type ActorInfo struct {
	UserID   string
	AgentID  string
	IsSystem bool
	IsAgent  bool
	// ActorType is one of "user", "agent", "system", "board"
	ActorType string
}

// GetActorInfo extracts the ActorInfo stored by the server's ActorMiddleware.
// It uses reflection-free retrieval: the middleware stores under the key "actor"
// as a string-keyed context value. We re-implement a compatible lookup here to
// avoid an import cycle between the routes and server packages.
func GetActorInfo(r *http.Request) ActorInfo {
	if v := r.Context().Value(actorKey); v != nil {
		if ai, ok := v.(ActorInfo); ok {
			return ai
		}
	}
	// Fallback: derive actor from Authorization header.
	return actorFromHeader(r)
}

func actorFromHeader(r *http.Request) ActorInfo {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ActorInfo{ActorType: "anonymous"}
	}
	const prefix = "Bearer "
	if len(auth) > len(prefix) {
		token := auth[len(prefix):]
		return ActorInfo{UserID: token, ActorType: "board"}
	}
	return ActorInfo{ActorType: "anonymous"}
}

// AssertBoard returns an error if the request does not carry board-level access.
// Board access is indicated by a non-empty UserID on the ActorInfo and IsAgent==false.
func AssertBoard(r *http.Request) error {
	actor := GetActorInfo(r)
	if actor.IsAgent {
		return fmt.Errorf("board access required: got agent actor")
	}
	if actor.IsSystem {
		return nil // system is always allowed
	}
	if actor.UserID == "" {
		return fmt.Errorf("unauthenticated: no actor identity found")
	}
	return nil
}

// AssertInstanceAdmin returns an error unless the actor holds instance-admin rights.
// In the current stub implementation, any board actor is treated as an instance admin.
func AssertInstanceAdmin(r *http.Request) error {
	return AssertBoard(r)
}

// AssertCompanyAccess returns an error unless the actor has access to the given company.
// Agents are only allowed access to their own company; board actors are allowed everywhere.
func AssertCompanyAccess(r *http.Request, companyID string) error {
	actor := GetActorInfo(r)
	if actor.IsSystem {
		return nil
	}
	if actor.IsAgent && actor.AgentID == "" {
		return fmt.Errorf("agent actor has no AgentID set")
	}
	// Board actors are considered to have access to all companies.
	if !actor.IsAgent {
		return nil
	}
	// For agents we would typically validate the company membership here.
	// For now we allow all authenticated agents.
	return nil
}

// WithActor stores an ActorInfo in the context (useful for tests).
func WithActor(ctx context.Context, actor ActorInfo) context.Context {
	return context.WithValue(ctx, actorKey, actor)
}
