package routes

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupSessionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.Exec(`CREATE TABLE "user" (
		id text PRIMARY KEY,
		name text NOT NULL,
		email text NOT NULL,
		email_verified boolean NOT NULL DEFAULT 0,
		image text,
		created_at datetime NOT NULL,
		updated_at datetime NOT NULL
	)`)
	db.Exec(`CREATE TABLE "session" (
		id text PRIMARY KEY,
		expires_at datetime NOT NULL,
		token text NOT NULL,
		created_at datetime NOT NULL,
		updated_at datetime NOT NULL,
		ip_address text,
		user_agent text,
		user_id text NOT NULL
	)`)
	return db
}

func TestGetSessionHandlerAuthenticatedMode(t *testing.T) {
	db := setupSessionTestDB(t)
	now := time.Now().UTC()
	db.Exec(`INSERT INTO "user"(id, name, email, email_verified, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"user-123", "Test User", "test@example.com", true, now, now)
	db.Exec(`INSERT INTO "session"(id, expires_at, token, created_at, updated_at, user_id) VALUES (?, ?, ?, ?, ?, ?)`,
		"sess-123", now.Add(time.Hour), "session-token", now, now, "user-123")

	req := httptest.NewRequest(http.MethodGet, "/api/auth/get-session", nil)
	req.AddCookie(&http.Cookie{
		Name:  betterAuthCookieBaseName,
		Value: signCookie("session-token", "test-secret"),
	})
	rr := httptest.NewRecorder()

	GetSessionHandler(db, GetSessionHandlerOptions{
		DeploymentMode:   "authenticated",
		BetterAuthSecret: "test-secret",
	})(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want status 200, got %d", rr.Code)
	}

	var resp struct {
		Session *struct {
			ID     string `json:"id"`
			Token  string `json:"token"`
			UserID string `json:"userId"`
		} `json:"session"`
		User *struct {
			ID            string `json:"id"`
			Email         string `json:"email"`
			EmailVerified bool   `json:"emailVerified"`
		} `json:"user"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Session == nil || resp.Session.ID != "sess-123" || resp.Session.Token != "session-token" {
		t.Fatalf("unexpected session payload: %#v", resp.Session)
	}
	if resp.User == nil || !resp.User.EmailVerified || resp.User.Email != "test@example.com" {
		t.Fatalf("unexpected user payload: %#v", resp.User)
	}
}

func TestGetSessionHandlerAuthenticatedModeReturnsNullWithoutSession(t *testing.T) {
	db := setupSessionTestDB(t)
	req := httptest.NewRequest(http.MethodGet, "/api/auth/get-session", nil)
	rr := httptest.NewRecorder()

	GetSessionHandler(db, GetSessionHandlerOptions{
		DeploymentMode:   "authenticated",
		BetterAuthSecret: "test-secret",
	})(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want status 200, got %d", rr.Code)
	}
	if body := rr.Body.String(); body != "null\n" {
		t.Fatalf("expected null response, got %q", body)
	}
}

func TestGetSessionHandlerLocalTrustedFallback(t *testing.T) {
	db := setupSessionTestDB(t)
	req := httptest.NewRequest(http.MethodGet, "/api/auth/get-session", nil)
	req = req.WithContext(WithActor(req.Context(), ActorInfo{ActorType: "board", UserID: "local-board", IsInstanceAdmin: true}))
	rr := httptest.NewRecorder()

	GetSessionHandler(db, GetSessionHandlerOptions{DeploymentMode: "local_trusted"})(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want status 200, got %d", rr.Code)
	}
	var resp struct {
		User *struct {
			Name *string `json:"name"`
		} `json:"user"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.User == nil || resp.User.Name == nil || *resp.User.Name != "Local Board" {
		t.Fatalf("unexpected local trusted response: %#v", resp.User)
	}
}

func signCookie(value, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(value))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return url.QueryEscape(value + "." + signature)
}
