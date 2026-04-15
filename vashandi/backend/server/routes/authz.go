package routes

import (
	"context"
	"fmt"
	"net/http"
)

// actorContextKeyType is an unexported struct used as the context key type for
// ActorInfo. Using a package-specific struct prevents collisions with keys from
// other packages that happen to use the same underlying string or int value.
type actorContextKeyType struct{}

// ActorKey is the context key under which ActorInfo is stored.
// It is exported so that the server middleware (which imports this package)
// can store the actor under the same key, avoiding the type-mismatch bug that
// caused GetActorInfo to always fall back to header parsing.
var ActorKey = actorContextKeyType{}

// ActorInfo carries identity information set by the auth middleware.
type ActorInfo struct {
	UserID          string
	AgentID         string
	CompanyID       string
	IsSystem        bool
	IsAgent         bool
	IsInstanceAdmin bool
	// ActorType is one of "user", "agent", "system", "board", "anonymous"
	ActorType string
	// ActorSource identifies how the actor was authenticated.
	// Board actors: "local_implicit" (local_trusted mode), "session" (cookie session),
	// or "board_key" (bearer board API key).
	// Agent actors: "agent_key" (bearer agent API key).
	// Anonymous actors: "".
	ActorSource string
}

// GetActorInfo extracts the ActorInfo stored by the server's ActorMiddleware.
// It reads directly from the context using the exported ActorKey; the
// middleware is responsible for always setting a value so there is no fallback.
func GetActorInfo(r *http.Request) ActorInfo {
	if v := r.Context().Value(ActorKey); v != nil {
		if ai, ok := v.(ActorInfo); ok {
			return ai
		}
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
func AssertInstanceAdmin(r *http.Request) error {
	actor := GetActorInfo(r)
	if actor.IsSystem {
		return nil
	}
	if actor.IsAgent {
		return fmt.Errorf("instance admin access required: got agent actor")
	}
	if actor.UserID == "" {
		return fmt.Errorf("unauthenticated: no actor identity found")
	}
	if !actor.IsInstanceAdmin {
		return fmt.Errorf("instance admin access required")
	}
	return nil
}

// AssertCompanyAccess returns an error unless the actor has access to the given company.
// Agents may only access their own company; board and system actors are allowed everywhere.
func AssertCompanyAccess(r *http.Request, companyID string) error {
	actor := GetActorInfo(r)
	if actor.IsSystem {
		return nil
	}
	if !actor.IsAgent {
		// Board actors have cross-company access.
		if actor.UserID == "" {
			return fmt.Errorf("unauthenticated: no actor identity found")
		}
		return nil
	}
	// Agent actors are scoped to a single company.
	if actor.AgentID == "" {
		return fmt.Errorf("agent actor has no AgentID set")
	}
	if actor.CompanyID != companyID {
		return fmt.Errorf("agent not authorized for company %q", companyID)
	}
	return nil
}

// WithActor stores an ActorInfo in the context (useful for tests).
func WithActor(ctx context.Context, actor ActorInfo) context.Context {
	return context.WithValue(ctx, ActorKey, actor)
}
