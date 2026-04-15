package server

import (
	"net/http"

	"github.com/chifamba/vashandi/vashandi/backend/server/routes"
)

// BoardGuard returns 403 if the request is authenticated as an agent.
// Use this on board-only endpoints that agents must not access.
func BoardGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actor, _ := r.Context().Value(routes.ActorKey).(routes.ActorInfo)
		if actor.IsAgent {
			WriteError(w, http.StatusForbidden, "board access required", "BOARD_ONLY")
			return
		}
		next.ServeHTTP(w, r)
	})
}
