package realtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"github.com/chifamba/vashandi/vashandi/backend/server/routes"
)

// LiveEventsHandler returns an http.HandlerFunc that upgrades the request to a
// WebSocket connection and forwards company-scoped live events to the client.
// If companyId is "*", the client subscribes to global events instead.
//
// Authentication order:
//  1. Actor already resolved by ActorMiddleware (board or agent bearer token in header).
//  2. `?token=` query-string parameter (for agents/browsers that cannot easily set the
//     Authorization header before the upgrade).
//  3. local_trusted deployment mode — anonymous caller is granted board access.
//
// Agents may only subscribe to events for their own company (not global).
func (h *Hub) LiveEventsHandler(db *gorm.DB, deploymentMode string, resolveSessionCookieActor func(r *http.Request, db *gorm.DB) (routes.ActorInfo, bool)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")
		if companyID == "" {
			http.Error(w, "missing companyId", http.StatusBadRequest)
			return
		}

		actor := routes.GetActorInfo(r)

		// Fallback: try the `?token=` query param when the actor is still anonymous
		// (e.g. a browser WebSocket that cannot set an Authorization header).
		if actor.ActorType == "anonymous" {
			if queryToken := strings.TrimSpace(r.URL.Query().Get("token")); queryToken != "" {
				actor = resolveTokenActor(db, queryToken)
			}
		}

		// Fallback: try BetterAuth session cookie for authenticated deployments
		if actor.ActorType == "anonymous" && deploymentMode == "authenticated" && resolveSessionCookieActor != nil {
			if sessionActor, ok := resolveSessionCookieActor(r, db); ok {
				actor = sessionActor
			}
		}

		// Further fallback: local_trusted mode allows unauthenticated board access.
		if actor.ActorType == "anonymous" && deploymentMode == "local_trusted" {
			actor = routes.ActorInfo{ActorType: "board", UserID: "board"}
		}

		if actor.ActorType == "anonymous" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		// Global channel ("*") is only available to board users, not agents.
		if companyID == "*" && actor.IsAgent {
			http.Error(w, "forbidden: agents cannot subscribe to global events", http.StatusForbidden)
			return
		}

		// Agents can only subscribe to events for their own company.
		if actor.IsAgent && actor.CompanyID != companyID {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		conn, err := Upgrader.Upgrade(w, r, nil)
		if err != nil {
			// Upgrader already wrote an error response.
			slog.Warn("ws: upgrade failed", "company", companyID, "error", err)
			return
		}

		client := &Client{
			hub:       h,
			companyID: companyID,
			conn:      conn,
			send:      make(chan []byte, 256),
			done:      make(chan struct{}),
		}
		h.register(client)

		go client.writePump()
		go client.readPump()
	}
}

// resolveTokenActor hashes the raw token, looks it up in agent_api_keys, and
// returns the matching ActorInfo. Returns an anonymous actor on any failure.
func resolveTokenActor(db *gorm.DB, token string) routes.ActorInfo {
	if db == nil {
		return routes.ActorInfo{ActorType: "anonymous"}
	}

	hash := sha256.Sum256([]byte(token))
	keyHash := hex.EncodeToString(hash[:])

	var key models.AgentAPIKey
	err := db.Where("key_hash = ? AND revoked_at IS NULL", keyHash).First(&key).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Warn("ws: agent key lookup error", "error", err)
		}
		return routes.ActorInfo{ActorType: "anonymous"}
	}

	// Update last-used timestamp asynchronously so the upgrade is not delayed.
	// A short timeout prevents the goroutine from leaking if the DB is slow.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		now := time.Now()
		if updateErr := db.WithContext(ctx).Model(&models.AgentAPIKey{}).
			Where("id = ?", key.ID).
			Update("last_used_at", now).Error; updateErr != nil {
			slog.Warn("ws: failed to update agent key last_used_at", "error", updateErr)
		}
	}()

	return routes.ActorInfo{
		AgentID:   key.AgentID,
		CompanyID: key.CompanyID,
		IsAgent:   true,
		ActorType: "agent",
	}
}
