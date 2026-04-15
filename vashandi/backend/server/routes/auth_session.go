package routes

import (
	"encoding/json"
	"net/http"
)

// sessionResponse mirrors the shape returned by Node.js /api/auth/get-session
// so that the UI client can be used unchanged.
type sessionResponse struct {
	Session *sessionInfo `json:"session"`
	User    *sessionUser `json:"user"`
}

type sessionInfo struct {
	ID     string `json:"id"`
	UserID string `json:"userId"`
}

type sessionUser struct {
	ID    string  `json:"id"`
	Email *string `json:"email"`
	Name  *string `json:"name"`
}

// GetSessionHandler returns session information for the authenticated actor.
// It mirrors the /api/auth/get-session endpoint in the Node.js server so that
// the UI can determine whether a board session is active.
func GetSessionHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor := GetActorInfo(r)
		if actor.ActorType != "board" || actor.UserID == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "Unauthorized"}) //nolint:errcheck
			return
		}

		var name *string
		if actor.UserID == "local-board" {
			s := "Local Board"
			name = &s
		}

		resp := sessionResponse{
			Session: &sessionInfo{
				ID:     "paperclip:board:" + actor.UserID,
				UserID: actor.UserID,
			},
			User: &sessionUser{
				ID:    actor.UserID,
				Email: nil,
				Name:  name,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	}
}
