package routes

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
)

const (
	betterAuthCookieBaseName = "better-auth.session_token"
	secureBetterAuthCookie   = "__Secure-" + betterAuthCookieBaseName
)

type GetSessionHandlerOptions struct {
	DeploymentMode   string
	BetterAuthSecret string
}

type sessionResponse struct {
	Session any `json:"session"`
	User    any `json:"user"`
}

type sessionInfo struct {
	ID        string     `json:"id"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	Token     *string    `json:"token,omitempty"`
	CreatedAt *time.Time `json:"createdAt,omitempty"`
	UpdatedAt *time.Time `json:"updatedAt,omitempty"`
	IPAddress *string    `json:"ipAddress,omitempty"`
	UserAgent *string    `json:"userAgent,omitempty"`
	UserID    string     `json:"userId"`
}

type sessionUser struct {
	ID            string     `json:"id"`
	Email         *string    `json:"email"`
	Name          *string    `json:"name"`
	EmailVerified bool       `json:"emailVerified"`
	Image         *string    `json:"image,omitempty"`
	CreatedAt     *time.Time `json:"createdAt,omitempty"`
	UpdatedAt     *time.Time `json:"updatedAt,omitempty"`
}

// GetSessionHandler returns session information for the current better-auth session.
func GetSessionHandler(db *gorm.DB, opts GetSessionHandlerOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.EqualFold(opts.DeploymentMode, "authenticated") {
			token, ok := resolveSessionToken(r, opts.BetterAuthSecret)
			if !ok {
				writeAuthJSON(w, http.StatusOK, nil)
				return
			}

			var session models.Session
			if err := db.Preload("User").
				Where("token = ? AND expires_at > ?", token, time.Now()).
				First(&session).Error; err != nil {
				writeAuthJSON(w, http.StatusOK, nil)
				return
			}

			resp := sessionResponse{
				Session: &sessionInfo{
					ID:        session.ID,
					ExpiresAt: &session.ExpiresAt,
					Token:     &session.Token,
					CreatedAt: &session.CreatedAt,
					UpdatedAt: &session.UpdatedAt,
					IPAddress: session.IPAddress,
					UserAgent: session.UserAgent,
					UserID:    session.UserID,
				},
				User: &sessionUser{
					ID:            session.User.ID,
					Email:         &session.User.Email,
					Name:          &session.User.Name,
					EmailVerified: session.User.EmailVerified,
					Image:         session.User.Image,
					CreatedAt:     &session.User.CreatedAt,
					UpdatedAt:     &session.User.UpdatedAt,
				},
			}
			writeAuthJSON(w, http.StatusOK, resp)
			return
		}

		actor := GetActorInfo(r)
		if actor.ActorType != "board" || actor.UserID == "" {
			writeAuthJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
			return
		}

		var user models.User
		if err := db.Where("id = ?", actor.UserID).First(&user).Error; err != nil {
			var namePtr *string
			if actor.UserID == "local-board" {
				s := "Local Board"
				namePtr = &s
			}
			resp := sessionResponse{
				Session: &sessionInfo{
					ID:     "paperclip:board:" + actor.UserID,
					UserID: actor.UserID,
				},
				User: &sessionUser{
					ID:            actor.UserID,
					Email:         nil,
					Name:          namePtr,
					EmailVerified: false,
				},
			}
			writeAuthJSON(w, http.StatusOK, resp)
			return
		}

		resp := sessionResponse{
			Session: &sessionInfo{
				ID:     "paperclip:board:" + actor.UserID,
				UserID: actor.UserID,
			},
			User: &sessionUser{
				ID:            user.ID,
				Email:         &user.Email,
				Name:          &user.Name,
				EmailVerified: user.EmailVerified,
				Image:         user.Image,
				CreatedAt:     &user.CreatedAt,
				UpdatedAt:     &user.UpdatedAt,
			},
		}
		writeAuthJSON(w, http.StatusOK, resp)
	}
}

func resolveSessionToken(r *http.Request, secret string) (string, bool) {
	for _, name := range []string{secureBetterAuthCookie, betterAuthCookieBaseName} {
		cookie, err := r.Cookie(name)
		if err != nil || cookie.Value == "" {
			continue
		}
		if secret != "" {
			if token, ok := verifySignedCookie(cookie.Value, secret); ok {
				return token, true
			}
			if decoded, decodeErr := url.QueryUnescape(cookie.Value); decodeErr == nil && strings.Contains(decoded, ".") {
				continue
			}
		}
		return cookie.Value, true
	}
	return "", false
}

func verifySignedCookie(rawValue, secret string) (string, bool) {
	decoded, err := url.QueryUnescape(rawValue)
	if err != nil {
		return "", false
	}
	signatureStart := strings.LastIndex(decoded, ".")
	if signatureStart < 1 {
		return "", false
	}
	value := decoded[:signatureStart]
	signature := decoded[signatureStart+1:]
	expected := signCookieSignature(value, secret)
	if subtle.ConstantTimeCompare([]byte(signature), []byte(expected)) != 1 {
		return "", false
	}
	return value, true
}

func signCookieSignature(value, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(value))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func writeAuthJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload) //nolint:errcheck
}
