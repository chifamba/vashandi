package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
)

func setupAuthTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&auth_test=1"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	// Create necessary tables
	db.Exec(`CREATE TABLE "user" (
		id text PRIMARY KEY,
		name text NOT NULL,
		email text NOT NULL,
		email_verified boolean NOT NULL DEFAULT 0,
		image text,
		created_at datetime NOT NULL,
		updated_at datetime NOT NULL
	)`)

	db.Exec(`CREATE TABLE "account" (
		id text PRIMARY KEY,
		account_id text NOT NULL,
		provider_id text NOT NULL,
		user_id text NOT NULL,
		access_token text,
		refresh_token text,
		id_token text,
		access_token_expires_at datetime,
		refresh_token_expires_at datetime,
		scope text,
		password text,
		created_at datetime NOT NULL,
		updated_at datetime NOT NULL,
		FOREIGN KEY (user_id) REFERENCES "user"(id) ON DELETE CASCADE
	)`)

	db.Exec(`CREATE TABLE "session" (
		id text PRIMARY KEY,
		expires_at datetime NOT NULL,
		token text NOT NULL,
		created_at datetime NOT NULL,
		updated_at datetime NOT NULL,
		ip_address text,
		user_agent text,
		user_id text NOT NULL,
		FOREIGN KEY (user_id) REFERENCES "user"(id) ON DELETE CASCADE
	)`)

	return db
}

func TestBetterAuthHandler_SignUpSignIn(t *testing.T) {
	db := setupAuthTestDB(t)
	h := NewBetterAuthHandler(db)

	// 1. Sign Up
	signUpBody, _ := json.Marshal(map[string]string{
		"email":    "test@example.com",
		"password": "password123",
		"name":     "Test User",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/sign-up/email", bytes.NewBuffer(signUpBody))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("sign-up failed: %d %s", w.Code, w.Body.String())
	}

	var signUpResp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&signUpResp)
	userResp := signUpResp["user"].(map[string]interface{})
	if userResp["email"] != "test@example.com" {
		t.Errorf("expected email test@example.com, got %v", userResp["email"])
	}

	// Verify cookie
	cookies := w.Result().Cookies()
	var sessionToken string
	for _, c := range cookies {
		if c.Name == cookieName {
			sessionToken = c.Value
		}
	}
	if sessionToken == "" {
		t.Fatal("expected session token cookie")
	}

	// 2. Sign In
	signInBody, _ := json.Marshal(map[string]string{
		"email":    "test@example.com",
		"password": "password123",
	})
	req = httptest.NewRequest(http.MethodPost, "/api/auth/sign-in/email", bytes.NewBuffer(signInBody))
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("sign-in failed: %d %s", w.Code, w.Body.String())
	}

	// 3. Sign In with wrong password
	signInBodyWrong, _ := json.Marshal(map[string]string{
		"email":    "test@example.com",
		"password": "wrongpassword",
	})
	req = httptest.NewRequest(http.MethodPost, "/api/auth/sign-in/email", bytes.NewBuffer(signInBodyWrong))
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong password, got %d", w.Code)
	}

	// 4. Sign Out
	req = httptest.NewRequest(http.MethodPost, "/api/auth/sign-out", nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: sessionToken})
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("sign-out failed: %d", w.Code)
	}

	// Verify session is deleted
	var count int64
	db.Model(&models.Session{}).Where("token = ?", sessionToken).Count(&count)
	if count != 0 {
		t.Errorf("expected session to be deleted, got %d", count)
	}
}
