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

// betterAuthCookieName is the default session cookie name used by BetterAuth.
const betterAuthCookieName = "better-auth.session_token"

// AuthMiddlewareOptions configures ActorMiddleware behaviour.
type AuthMiddlewareOptions struct {
	// DeploymentMode is either "local_trusted" or "authenticated".
	// Defaults to "local_trusted" when empty.
	DeploymentMode string
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
//   - anything else → anonymous actor
func ActorMiddleware(db *gorm.DB, opts AuthMiddlewareOptions) func(http.Handler) http.Handler {
	mode := opts.DeploymentMode
	if mode == "" {
		mode = "local_trusted"
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// local_trusted: treat every request as the board user with full admin rights.
			if mode == "local_trusted" {
				actor := routes.ActorInfo{
					UserID:          "local-board",
					IsAgent:         false,
					IsInstanceAdmin: true,
					ActorType:       "board",
				}
				ctx := context.WithValue(r.Context(), routes.ActorKey, actor)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// authenticated mode: start anonymous and try to resolve an identity.
			actor := routes.ActorInfo{ActorType: "anonymous"}

			authHeader := r.Header.Get("Authorization")
			if !strings.HasPrefix(authHeader, "Bearer ") {
				// No bearer token — try a BetterAuth session cookie.
				if db != nil {
					if sessionActor, ok := resolveSessionCookieActor(r, db); ok {
						actor = sessionActor
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
					db.Model(&key).Update("last_used_at", now) //nolint:errcheck
					actor = routes.ActorInfo{
						UserID:          key.UserID,
						IsAgent:         false,
						IsInstanceAdmin: isAdmin,
						ActorType:       "board",
					}
				} else if !errors.Is(err, gorm.ErrRecordNotFound) {
					log.Printf("auth: board key lookup error: %v", err)
				}
			case strings.HasPrefix(token, "pcp_agent_") && db != nil:
				var key models.AgentAPIKey
				err := db.Where("key_hash = ? AND revoked_at IS NULL", keyHash).First(&key).Error
				if err == nil {
					actor = routes.ActorInfo{
						AgentID:   key.AgentID,
						CompanyID: key.CompanyID,
						IsAgent:   true,
						ActorType: "agent",
					}
				} else if !errors.Is(err, gorm.ErrRecordNotFound) {
					log.Printf("auth: agent key lookup error: %v", err)
				}
			}

			ctx := context.WithValue(r.Context(), routes.ActorKey, actor)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// resolveSessionCookieActor reads the BetterAuth session cookie from the
// request, looks up the session in the database, and returns a board ActorInfo
// if the session is valid and not expired.
func resolveSessionCookieActor(r *http.Request, db *gorm.DB) (routes.ActorInfo, bool) {
	cookie, err := r.Cookie(betterAuthCookieName)
	if err != nil || cookie.Value == "" {
		return routes.ActorInfo{}, false
	}

	var session models.Session
	err = db.WithContext(r.Context()).
		Where("token = ? AND expires_at > ?", cookie.Value, time.Now()).
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
	}, true
}

// isInstanceAdmin reports whether the given userID holds the "instance_admin" role.
func isInstanceAdmin(db *gorm.DB, userID string) bool {
	var count int64
	db.Model(&models.InstanceUserRole{}).
		Where("user_id = ? AND role = ?", userID, "instance_admin").
		Count(&count)
	return count > 0
}
