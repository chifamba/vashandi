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

// ============================================================================
// OAuth Tests
// ============================================================================

func TestBetterAuthHandler_OAuthSignIn_RedirectsToProvider(t *testing.T) {
	db := setupAuthTestDB(t)
	h := NewBetterAuthHandler(db, BetterAuthOptions{
		AllowedHostnames: []string{"app.example.com"},
		PublicBaseURL:    "https://app.example.com",
		Secret:           "test-secret",
		OAuthProviders: map[string]OAuthProviderConfig{
			"google": {
				ClientID:       "test-client-id",
				ClientSecret:   "test-client-secret",
				AuthURL:        "https://accounts.google.com/o/oauth2/v2/auth",
				TokenURL:       "https://oauth2.googleapis.com/token",
				UserInfoURL:    "https://www.googleapis.com/oauth2/v3/userinfo",
				Scopes:         []string{"openid", "email", "profile"},
				AccountIDClaim: "sub",
			},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/auth/signin/google?callbackURL=/dashboard", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected redirect, got %d: %s", w.Code, w.Body.String())
	}

	location := w.Header().Get("Location")
	if !strings.HasPrefix(location, "https://accounts.google.com/o/oauth2/v2/auth") {
		t.Fatalf("expected Google auth URL, got %s", location)
	}
	if !strings.Contains(location, "client_id=test-client-id") {
		t.Fatalf("expected client_id in URL: %s", location)
	}
	if !strings.Contains(location, "redirect_uri=") {
		t.Fatalf("expected redirect_uri in URL: %s", location)
	}
	if !strings.Contains(location, "state=") {
		t.Fatalf("expected state in URL: %s", location)
	}

	stateCookie := findCookie(w.Result().Cookies(), oauthStateCookieName)
	if stateCookie == nil {
		t.Fatal("expected OAuth state cookie")
	}
}

func TestBetterAuthHandler_OAuthSignIn_UnconfiguredProvider(t *testing.T) {
	db := setupAuthTestDB(t)
	h := NewBetterAuthHandler(db, BetterAuthOptions{
		AllowedHostnames: []string{"app.example.com"},
		Secret:           "test-secret",
		OAuthProviders:   map[string]OAuthProviderConfig{},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/auth/signin/google", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if !strings.Contains(resp["error"], "not configured") {
		t.Fatalf("expected 'not configured' error, got: %s", resp["error"])
	}
}

func TestBetterAuthHandler_OAuthCallback_InvalidState(t *testing.T) {
	db := setupAuthTestDB(t)
	h := NewBetterAuthHandler(db, BetterAuthOptions{
		AllowedHostnames: []string{"app.example.com"},
		Secret:           "test-secret",
		OAuthProviders: map[string]OAuthProviderConfig{
			"google": {
				ClientID:       "test-client-id",
				ClientSecret:   "test-client-secret",
				AuthURL:        "https://accounts.google.com/o/oauth2/v2/auth",
				TokenURL:       "https://oauth2.googleapis.com/token",
				UserInfoURL:    "https://www.googleapis.com/oauth2/v3/userinfo",
				Scopes:         []string{"openid", "email", "profile"},
				AccountIDClaim: "sub",
			},
		},
	})

	// Callback without state cookie
	req := httptest.NewRequest(http.MethodGet, "/api/auth/callback/google?code=test-code&state=test-state", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected redirect, got %d", w.Code)
	}
	location := w.Header().Get("Location")
	if !strings.Contains(location, "error=") {
		t.Fatalf("expected error in redirect URL: %s", location)
	}
}

func TestBetterAuthHandler_ListAccounts_Unauthorized(t *testing.T) {
	db := setupAuthTestDB(t)
	h := NewBetterAuthHandler(db, BetterAuthOptions{
		AllowedHostnames: []string{"app.example.com"},
		Secret:           "test-secret",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/auth/accounts", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestBetterAuthHandler_ListAccounts_WithSession(t *testing.T) {
	db := setupAuthTestDB(t)
	h := NewBetterAuthHandler(db, BetterAuthOptions{
		AllowedHostnames: []string{"app.example.com"},
		Secret:           "test-secret",
	})

	// Create user and accounts
	now := time.Now()
	user := models.User{
		ID:            "user-1",
		Name:          "Test User",
		Email:         "test@example.com",
		EmailVerified: true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	db.Create(&user)

	credentialAccount := models.Account{
		ID:         "acct-1",
		AccountID:  user.ID,
		ProviderID: "credential",
		UserID:     user.ID,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	googleAccount := models.Account{
		ID:         "acct-2",
		AccountID:  "google-123",
		ProviderID: "google",
		UserID:     user.ID,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	db.Create(&credentialAccount)
	db.Create(&googleAccount)

	// Create session
	sessionToken := "test-session-token"
	session := models.Session{
		ID:        "sess-1",
		Token:     sessionToken,
		UserID:    user.ID,
		ExpiresAt: now.Add(24 * time.Hour),
		CreatedAt: now,
		UpdatedAt: now,
	}
	db.Create(&session)

	// Request with session cookie
	cookie := &http.Cookie{
		Name:  betterAuthCookieBaseName,
		Value: sessionToken,
	}
	req := httptest.NewRequest(http.MethodGet, "/api/auth/accounts", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var accounts []struct {
		ID         string `json:"id"`
		ProviderID string `json:"providerId"`
		AccountID  string `json:"accountId"`
	}
	if err := json.NewDecoder(w.Body).Decode(&accounts); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(accounts))
	}
}

func TestBetterAuthHandler_UnlinkAccount_PreventLastMethod(t *testing.T) {
	db := setupAuthTestDB(t)
	h := NewBetterAuthHandler(db, BetterAuthOptions{
		AllowedHostnames: []string{"app.example.com"},
		Secret:           "test-secret",
	})

	// Create user with single account
	now := time.Now()
	user := models.User{
		ID:            "user-1",
		Name:          "Test User",
		Email:         "test@example.com",
		EmailVerified: true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	db.Create(&user)

	googleAccount := models.Account{
		ID:         "acct-1",
		AccountID:  "google-123",
		ProviderID: "google",
		UserID:     user.ID,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	db.Create(&googleAccount)

	// Create session
	sessionToken := "test-session-token"
	session := models.Session{
		ID:        "sess-1",
		Token:     sessionToken,
		UserID:    user.ID,
		ExpiresAt: now.Add(24 * time.Hour),
		CreatedAt: now,
		UpdatedAt: now,
	}
	db.Create(&session)

	// Try to unlink the only account
	cookie := &http.Cookie{
		Name:  betterAuthCookieBaseName,
		Value: sessionToken,
	}
	req := httptest.NewRequest(http.MethodDelete, "/api/auth/unlink/google", nil)
	req.Header.Set("Origin", "https://app.example.com")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if !strings.Contains(resp["error"], "Cannot unlink last") {
		t.Fatalf("expected 'Cannot unlink last' error, got: %s", resp["error"])
	}
}

func TestBetterAuthHandler_UnlinkAccount_Success(t *testing.T) {
	db := setupAuthTestDB(t)
	h := NewBetterAuthHandler(db, BetterAuthOptions{
		AllowedHostnames: []string{"app.example.com"},
		Secret:           "test-secret",
	})

	// Create user with two accounts
	now := time.Now()
	user := models.User{
		ID:            "user-1",
		Name:          "Test User",
		Email:         "test@example.com",
		EmailVerified: true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	db.Create(&user)

	credentialAccount := models.Account{
		ID:         "acct-1",
		AccountID:  user.ID,
		ProviderID: "credential",
		UserID:     user.ID,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	googleAccount := models.Account{
		ID:         "acct-2",
		AccountID:  "google-123",
		ProviderID: "google",
		UserID:     user.ID,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	db.Create(&credentialAccount)
	db.Create(&googleAccount)

	// Create session
	sessionToken := "test-session-token"
	session := models.Session{
		ID:        "sess-1",
		Token:     sessionToken,
		UserID:    user.ID,
		ExpiresAt: now.Add(24 * time.Hour),
		CreatedAt: now,
		UpdatedAt: now,
	}
	db.Create(&session)

	// Unlink Google account (credential remains)
	cookie := &http.Cookie{
		Name:  betterAuthCookieBaseName,
		Value: sessionToken,
	}
	req := httptest.NewRequest(http.MethodDelete, "/api/auth/unlink/google", nil)
	req.Header.Set("Origin", "https://app.example.com")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify account was deleted
	var count int64
	db.Model(&models.Account{}).Where("user_id = ? AND provider_id = ?", user.ID, "google").Count(&count)
	if count != 0 {
		t.Fatal("expected Google account to be deleted")
	}
}

func TestBetterAuthHandler_LinkAccount_AlreadyLinked(t *testing.T) {
	db := setupAuthTestDB(t)
	h := NewBetterAuthHandler(db, BetterAuthOptions{
		AllowedHostnames: []string{"app.example.com"},
		Secret:           "test-secret",
		OAuthProviders: map[string]OAuthProviderConfig{
			"google": {
				ClientID:       "test-client-id",
				ClientSecret:   "test-client-secret",
				AuthURL:        "https://accounts.google.com/o/oauth2/v2/auth",
				TokenURL:       "https://oauth2.googleapis.com/token",
				UserInfoURL:    "https://www.googleapis.com/oauth2/v3/userinfo",
				Scopes:         []string{"openid", "email", "profile"},
				AccountIDClaim: "sub",
			},
		},
	})

	// Create user with Google already linked
	now := time.Now()
	user := models.User{
		ID:            "user-1",
		Name:          "Test User",
		Email:         "test@example.com",
		EmailVerified: true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	db.Create(&user)

	googleAccount := models.Account{
		ID:         "acct-1",
		AccountID:  "google-123",
		ProviderID: "google",
		UserID:     user.ID,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	db.Create(&googleAccount)

	// Create session
	sessionToken := "test-session-token"
	session := models.Session{
		ID:        "sess-1",
		Token:     sessionToken,
		UserID:    user.ID,
		ExpiresAt: now.Add(24 * time.Hour),
		CreatedAt: now,
		UpdatedAt: now,
	}
	db.Create(&session)

	// Try to link Google again
	cookie := &http.Cookie{
		Name:  betterAuthCookieBaseName,
		Value: sessionToken,
	}
	req := httptest.NewRequest(http.MethodPost, "/api/auth/link/google", nil)
	req.Header.Set("Origin", "https://app.example.com")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict, got %d: %s", w.Code, w.Body.String())
	}
}

func TestExtractProviderFromPath(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/api/auth/signin/google", "google"},
		{"/api/auth/signin/github", "github"},
		{"/api/auth/callback/google", "google"},
		{"/api/auth/callback/github/", "github"},
		{"/api/auth/link/google", "google"},
		{"/api/auth/unlink/github", "github"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := extractProviderFromPath(tt.path)
			if result != tt.expected {
				t.Errorf("extractProviderFromPath(%q) = %q, want %q", tt.path, result, tt.expected)
			}
		})
	}
}

func TestBuildOAuthAuthorizationURL(t *testing.T) {
	h := &BetterAuthHandler{}
	config := OAuthProviderConfig{
		AuthURL:   "https://accounts.google.com/o/oauth2/v2/auth",
		Scopes:    []string{"openid", "email", "profile"},
		ClientID:  "test-client-id",
	}

	url := h.buildOAuthAuthorizationURL(config, "test-state", "https://example.com/callback")

	if !strings.Contains(url, "client_id=test-client-id") {
		t.Errorf("expected client_id in URL: %s", url)
	}
	if !strings.Contains(url, "state=test-state") {
		t.Errorf("expected state in URL: %s", url)
	}
	if !strings.Contains(url, "response_type=code") {
		t.Errorf("expected response_type=code in URL: %s", url)
	}
	if !strings.Contains(url, "scope=openid+email+profile") && !strings.Contains(url, "scope=openid%20email%20profile") {
		t.Errorf("expected scope in URL: %s", url)
	}
}

func TestExtractStringClaim(t *testing.T) {
	data := map[string]any{
		"email":      "test@example.com",
		"name":       "Test User",
		"id":         12345.0, // JSON numbers are float64
		"int_id":     42,
		"avatar_url": "https://example.com/avatar.jpg",
	}

	tests := []struct {
		keys     []string
		expected string
	}{
		{[]string{"email"}, "test@example.com"},
		{[]string{"name"}, "Test User"},
		{[]string{"id"}, "12345"},
		{[]string{"missing", "email"}, "test@example.com"},
		{[]string{"missing"}, ""},
		{[]string{""}, ""},
		{[]string{"avatar_url", "picture"}, "https://example.com/avatar.jpg"},
	}

	for _, tt := range tests {
		result := extractStringClaim(data, tt.keys...)
		if result != tt.expected {
			t.Errorf("extractStringClaim(%v) = %q, want %q", tt.keys, result, tt.expected)
		}
	}
}
