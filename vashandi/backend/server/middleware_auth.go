package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"github.com/chifamba/vashandi/vashandi/backend/server/routes"
)

// runIDHeader is the header name for request correlation.
const runIDHeader = "x-paperclip-run-id"

// AuthMiddlewareOptions configures ActorMiddleware behaviour.
type AuthMiddlewareOptions struct {
	// DeploymentMode is either "local_trusted" or "authenticated".
	// Defaults to "local_trusted" when empty.
	DeploymentMode string

	// BetterAuthSecret verifies signed better-auth session cookies.
	BetterAuthSecret string
}

// ActorMiddleware resolves the caller identity and stores it in the request
// context. Auth is opt-in per route — the middleware always calls next
// regardless of whether a valid token was found.
//
// Modes:
//   - local_trusted: every request is treated as the board user with no token required.
//   - authenticated: bearer API keys are checked first; if absent, the
//     BetterAuth session cookie is resolved against the database.
//
// Token prefixes (bearer):
//   - pcp_board_  → look up in board_api_keys, set board actor (with IsInstanceAdmin)
//   - pcp_agent_  → look up in agent_api_keys, set agent actor
//   - JWT tokens  → verify signature, validate agent, set agent actor
//   - anything else → anonymous actor
//
// Headers:
//   - x-paperclip-run-id → extracted and stored in ActorInfo.RunID for request correlation
func ActorMiddleware(db *gorm.DB, opts AuthMiddlewareOptions) func(http.Handler) http.Handler {
	mode := opts.DeploymentMode
	if mode == "" {
		mode = "local_trusted"
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract x-paperclip-run-id header for request correlation
			runID := r.Header.Get(runIDHeader)

			// local_trusted: treat every request as the board user with full admin rights.
			if mode == "local_trusted" {
				actor := routes.ActorInfo{
					UserID:          "local-board",
					IsAgent:         false,
					IsInstanceAdmin: true,
					ActorType:       "board",
					ActorSource:     "local_implicit",
					RunID:           runID,
				}
				ctx := context.WithValue(r.Context(), routes.ActorKey, actor)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// authenticated mode: start anonymous and try to resolve an identity.
			actor := routes.ActorInfo{ActorType: "anonymous", RunID: runID}

			authHeader := r.Header.Get("Authorization")
			if !strings.HasPrefix(authHeader, "Bearer ") {
				// No bearer token — try a BetterAuth session cookie.
				if db != nil {
					if sessionActor, ok := resolveSessionCookieActor(r, db, opts.BetterAuthSecret); ok {
						actor = sessionActor
						actor.RunID = runID
					}
				}
				ctx := context.WithValue(r.Context(), routes.ActorKey, actor)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			token := strings.TrimPrefix(authHeader, "Bearer ")
			hash := sha256.Sum256([]byte(token))
			keyHash := hex.EncodeToString(hash[:])

			switch {
			case strings.HasPrefix(token, "pcp_board_") && db != nil:
				var key models.BoardAPIKey
				err := db.Where("key_hash = ? AND revoked_at IS NULL", keyHash).First(&key).Error
				if err == nil {
					isAdmin := isInstanceAdmin(db, key.UserID)
					now := time.Now()
					if updateErr := db.Model(&key).Update("last_used_at", now).Error; updateErr != nil {
						log.Printf("auth: board key touch error: %v", updateErr)
					}
					actor = routes.ActorInfo{
						UserID:          key.UserID,
						IsAgent:         false,
						IsInstanceAdmin: isAdmin,
						ActorType:       "board",
						ActorSource:     "board_key",
						RunID:           runID,
					}
				} else if !errors.Is(err, gorm.ErrRecordNotFound) {
					log.Printf("auth: board key lookup error: %v", err)
				}
			case strings.HasPrefix(token, "pcp_agent_") && db != nil:
				var key models.AgentAPIKey
				err := db.Where("key_hash = ? AND revoked_at IS NULL", keyHash).First(&key).Error
				if err == nil {
					// Validate that the agent exists and is not terminated or pending_approval
					var agent models.Agent
					agentErr := db.Where("id = ?", key.AgentID).First(&agent).Error
					if agentErr == nil && agent.Status != "terminated" && agent.Status != "pending_approval" {
						now := time.Now()
						if updateErr := db.Model(&key).Update("last_used_at", now).Error; updateErr != nil {
							log.Printf("auth: agent key touch error: %v", updateErr)
						}
						actor = routes.ActorInfo{
							AgentID:     key.AgentID,
							CompanyID:   key.CompanyID,
							IsAgent:     true,
							ActorType:   "agent",
							ActorSource: "agent_key",
							RunID:       runID,
						}
					}
				} else if !errors.Is(err, gorm.ErrRecordNotFound) {
					log.Printf("auth: agent key lookup error: %v", err)
				}
			default:
				// Try JWT verification for agent tokens
				if db != nil {
					if jwtActor, ok := tryJwtAgentAuth(r.Context(), db, token, runID); ok {
						actor = jwtActor
					}
				}
			}

			ctx := context.WithValue(r.Context(), routes.ActorKey, actor)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// tryJwtAgentAuth attempts to verify a JWT token and resolve an agent actor.
// Returns the actor and true if successful, or an empty actor and false otherwise.
// Note: JWT authentication is keyless (no API key involved), so there's no
// last_used_at to update. This matches the Node.js implementation behavior.
func tryJwtAgentAuth(ctx context.Context, db *gorm.DB, token, headerRunID string) (routes.ActorInfo, bool) {
	claims := VerifyLocalAgentJwt(token)
	if claims == nil {
		return routes.ActorInfo{}, false
	}

	// Look up the agent to validate it exists and belongs to the claimed company
	var agent models.Agent
	err := db.WithContext(ctx).Where("id = ?", claims.Sub).First(&agent).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("auth: agent lookup error for JWT: %v", err)
		}
		return routes.ActorInfo{}, false
	}

	// Verify the agent belongs to the claimed company
	if agent.CompanyID != claims.CompanyID {
		return routes.ActorInfo{}, false
	}

	// Check agent status - reject terminated or pending_approval agents
	if agent.Status == "terminated" || agent.Status == "pending_approval" {
		return routes.ActorInfo{}, false
	}

	// Determine the run ID - prefer header over JWT claim
	runID := headerRunID
	if runID == "" {
		runID = claims.RunID
	}

	return routes.ActorInfo{
		AgentID:     claims.Sub,
		CompanyID:   claims.CompanyID,
		IsAgent:     true,
		ActorType:   "agent",
		ActorSource: "agent_jwt",
		RunID:       runID,
	}, true
}

// resolveSessionCookieActor reads the BetterAuth session cookie from the
// request, looks up the session in the database, and returns a board ActorInfo
// if the session is valid and not expired.
func resolveSessionCookieActor(r *http.Request, db *gorm.DB, secret string) (routes.ActorInfo, bool) {
	token, ok := resolveBetterAuthSessionToken(r, secret)
	if !ok {
		return routes.ActorInfo{}, false
	}

	var session models.Session
	err := db.WithContext(r.Context()).
		Where("token = ? AND expires_at > ?", token, time.Now()).
		First(&session).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("auth: session lookup error: %v", err)
		}
		return routes.ActorInfo{}, false
	}

	isAdmin := isInstanceAdmin(db, session.UserID)
	return routes.ActorInfo{
		UserID:          session.UserID,
		IsAgent:         false,
		IsInstanceAdmin: isAdmin,
		ActorType:       "board",
		ActorSource:     "session",
	}, true
}

// isInstanceAdmin reports whether the given userID holds the "instance_admin" role.
func isInstanceAdmin(db *gorm.DB, userID string) bool {
	var count int64
	if err := db.Model(&models.InstanceUserRole{}).
		Where("user_id = ? AND role = ?", userID, "instance_admin").
		Count(&count).Error; err != nil {
		log.Printf("auth: instance admin check error for user %q: %v", userID, err)
		return false
	}
	return count > 0
}
