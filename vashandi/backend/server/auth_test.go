package server

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/scrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
)

func setupAuthTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+url.PathEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
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

func TestBetterAuthHandler_SignUpSignInSignOut(t *testing.T) {
	t.Setenv("BETTER_AUTH_SECRET", "test-secret")
	db := setupAuthTestDB(t)
	h := NewBetterAuthHandler(db, BetterAuthOptions{
		AllowedHostnames: []string{"app.example.com"},
		Secret:           "test-secret",
	})

	signUpBody, _ := json.Marshal(map[string]string{
		"email":    "Test@Example.com",
		"password": "password123",
		"name":     "Test User",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/sign-up/email", bytes.NewBuffer(signUpBody))
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("sign-up failed: %d %s", w.Code, w.Body.String())
	}

	var signUpResp struct {
		Token string `json:"token"`
		User  struct {
			Email         string `json:"email"`
			EmailVerified bool   `json:"emailVerified"`
		} `json:"user"`
	}
	if err := json.NewDecoder(w.Body).Decode(&signUpResp); err != nil {
		t.Fatalf("decode sign-up response: %v", err)
	}
	if signUpResp.User.Email != "test@example.com" {
		t.Fatalf("expected normalized email, got %q", signUpResp.User.Email)
	}
	if signUpResp.Token == "" {
		t.Fatal("expected session token in sign-up response")
	}
	if signUpResp.User.EmailVerified {
		t.Fatal("new users should start unverified")
	}

	sessionCookie := findCookie(w.Result().Cookies(), betterAuthCookieBaseName)
	if sessionCookie == nil {
		t.Fatal("expected session cookie")
	}
	if _, ok := verifyBetterAuthCookieValue(sessionCookie.Value, "test-secret"); !ok {
		t.Fatal("expected signed better-auth cookie")
	}

	signInBody, _ := json.Marshal(map[string]string{
		"email":    "test@example.com",
		"password": "password123",
	})
	req = httptest.NewRequest(http.MethodPost, "/api/auth/sign-in/email", bytes.NewBuffer(signInBody))
	req.Header.Set("Origin", "https://app.example.com")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("sign-in failed: %d %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/auth/sign-out", nil)
	req.Header.Set("Origin", "https://app.example.com")
	req.AddCookie(sessionCookie)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("sign-out failed: %d %s", w.Code, w.Body.String())
	}

	var count int64
	db.Model(&models.Session{}).Where("token = ?", signUpResp.Token).Count(&count)
	if count != 0 {
		t.Fatalf("expected session to be deleted, got %d", count)
	}
}

func TestBetterAuthHandler_UpgradesLegacyPasswordHash(t *testing.T) {
	db := setupAuthTestDB(t)
	h := NewBetterAuthHandler(db, BetterAuthOptions{
		AllowedHostnames: []string{"app.example.com"},
		Secret:           "test-secret",
	})

	legacyHash, err := hashLegacyPassword("password123")
	if err != nil {
		t.Fatalf("hash legacy password: %v", err)
	}
	user := models.User{
		ID:            "user-1",
		Name:          "Test User",
		Email:         "test@example.com",
		EmailVerified: true,
	}
	account := models.Account{
		ID:         "acct-1",
		AccountID:  user.ID,
		ProviderID: "credential",
		UserID:     user.ID,
		Password:   &legacyHash,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Create(&account).Error; err != nil {
		t.Fatalf("create account: %v", err)
	}

	signInBody, _ := json.Marshal(map[string]string{
		"email":    "test@example.com",
		"password": "password123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/sign-in/email", bytes.NewBuffer(signInBody))
	req.Header.Set("Origin", "https://app.example.com")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("sign-in failed: %d %s", w.Code, w.Body.String())
	}

	var updated models.Account
	if err := db.First(&updated, "id = ?", account.ID).Error; err != nil {
		t.Fatalf("reload account: %v", err)
	}
	if updated.Password == nil || *updated.Password == legacyHash {
		t.Fatal("expected password hash to be upgraded")
	}
	if !verifyBetterAuthPassword("password123", *updated.Password) {
		t.Fatal("expected upgraded better-auth password hash")
	}
}

func TestBetterAuthHandler_DisableSignUpAndTrustedOrigins(t *testing.T) {
	db := setupAuthTestDB(t)
	h := NewBetterAuthHandler(db, BetterAuthOptions{
		DisableSignUp:    true,
		AllowedHostnames: []string{"app.example.com"},
		Secret:           "test-secret",
	})

	signUpBody := `{"email":"test@example.com","password":"password123","name":"Test User"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/sign-up/email", strings.NewReader(signUpBody))
	req.Header.Set("Origin", "https://evil.example.com")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected origin rejection, got %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/auth/sign-up/email", strings.NewReader(signUpBody))
	req.Header.Set("Origin", "https://app.example.com")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected disable sign-up rejection, got %d", w.Code)
	}
}

func TestBetterAuthHandler_EmailVerificationFlow(t *testing.T) {
	db := setupAuthTestDB(t)
	h := NewBetterAuthHandler(db, BetterAuthOptions{
		RequireEmailVerification: true,
		AllowedHostnames:         []string{"app.example.com"},
		Secret:                   "test-secret",
	})

	signUpBody := `{"email":"test@example.com","password":"password123","name":"Test User","callbackURL":"/welcome"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/sign-up/email", strings.NewReader(signUpBody))
	req.Header.Set("Origin", "https://app.example.com")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("sign-up failed: %d %s", w.Code, w.Body.String())
	}
	if findCookie(w.Result().Cookies(), betterAuthCookieBaseName) != nil {
		t.Fatal("did not expect session cookie before verification")
	}

	token, err := createEmailVerificationToken("test-secret", "test@example.com", time.Hour)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/auth/verify-email?token="+token+"&callbackURL=%2Fwelcome", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusFound {
		t.Fatalf("verify-email failed: %d %s", w.Code, w.Body.String())
	}

	var user models.User
	if err := db.First(&user, "email = ?", "test@example.com").Error; err != nil {
		t.Fatalf("load user: %v", err)
	}
	if !user.EmailVerified {
		t.Fatal("expected user to be verified")
	}

	signInBody := `{"email":"test@example.com","password":"password123"}`
	req = httptest.NewRequest(http.MethodPost, "/api/auth/sign-in/email", strings.NewReader(signInBody))
	req.Header.Set("Origin", "https://app.example.com")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("sign-in after verification failed: %d %s", w.Code, w.Body.String())
	}
}

func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

func hashLegacyPassword(password string) (string, error) {
	salt := generateRandomBytes(passwordSaltLen)
	hash, err := scrypt.Key([]byte(password), salt, legacyPasswordN, legacyPasswordR, legacyPasswordP, legacyPasswordKeyLen)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(salt) + ":" + hex.EncodeToString(hash), nil
}
