package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"strings"

	"gorm.io/gorm"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
)

type contextKey string

const ActorContextKey contextKey = "actor"

type ActorInfo struct {
	// Board actor fields
	UserID string
	// Agent actor fields
	AgentID   string
	CompanyID string
	// Common
	IsSystem bool
	IsAgent  bool
}

// ActorMiddleware resolves the caller identity from the Authorization header and
// stores it in the request context. Auth is opt-in per route — the middleware
// always calls next regardless of whether a valid token was found.
//
// Token prefixes:
//   - pcp_board_  → look up in board_api_keys, set board actor
//   - pcp_agent_  → look up in agent_api_keys, set agent actor
//   - anything else → anonymous actor
func ActorMiddleware(db *gorm.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var actor ActorInfo

			authHeader := r.Header.Get("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				token := strings.TrimPrefix(authHeader, "Bearer ")
				hash := sha256.Sum256([]byte(token))
				keyHash := hex.EncodeToString(hash[:])

				switch {
				case strings.HasPrefix(token, "pcp_board_") && db != nil:
					var key models.BoardAPIKey
					err := db.Where("key_hash = ? AND revoked_at IS NULL", keyHash).First(&key).Error
					if err == nil {
						actor = ActorInfo{UserID: key.UserID, IsAgent: false}
					} else if !errors.Is(err, gorm.ErrRecordNotFound) {
						log.Printf("auth: board key lookup error: %v", err)
					}
				case strings.HasPrefix(token, "pcp_agent_") && db != nil:
					var key models.AgentAPIKey
					err := db.Where("key_hash = ? AND revoked_at IS NULL", keyHash).First(&key).Error
					if err == nil {
						actor = ActorInfo{AgentID: key.AgentID, CompanyID: key.CompanyID, IsAgent: true}
					} else if !errors.Is(err, gorm.ErrRecordNotFound) {
						log.Printf("auth: agent key lookup error: %v", err)
					}
				}
			}

			ctx := context.WithValue(r.Context(), ActorContextKey, actor)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
