package server

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/scrypt"
	"golang.org/x/text/unicode/norm"
	"gorm.io/gorm"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
)

const (
	betterAuthCookieBaseName = "better-auth.session_token"
	betterAuthSessionDays    = 7
	betterAuthPasswordN      = 16384
	betterAuthPasswordR      = 16
	betterAuthPasswordP      = 1
	betterAuthPasswordKeyLen = 64
	legacyPasswordN          = 16384
	legacyPasswordR          = 8
	legacyPasswordP          = 1
	legacyPasswordKeyLen     = 64
	passwordSaltLen          = 16
	minPasswordLength        = 8
	maxPasswordLength        = 128
	emailVerificationTTL     = time.Hour
	signedCookieSigLength    = 44 // base64-encoded HMAC-SHA256 digest length, including trailing "="
)

// OAuthProviderConfig holds configuration for an OAuth 2.0 provider.
type OAuthProviderConfig struct {
	ClientID     string
	ClientSecret string
	AuthURL      string
	TokenURL     string
	UserInfoURL  string
	Scopes       []string
	// EmailClaim is the JSON path to extract email from userinfo response (default: "email")
	EmailClaim string
	// NameClaim is the JSON path to extract name from userinfo response (default: "name")
	NameClaim string
	// ImageClaim is the JSON path to extract image URL from userinfo response (default: "picture" or "avatar_url")
	ImageClaim string
	// AccountIDClaim is the JSON path to extract unique account ID (default: "sub" or "id")
	AccountIDClaim string
}

// Predefined OAuth provider configurations
var (
	GoogleOAuthConfig = OAuthProviderConfig{
		AuthURL:        "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL:       "https://oauth2.googleapis.com/token",
		UserInfoURL:    "https://www.googleapis.com/oauth2/v3/userinfo",
		Scopes:         []string{"openid", "email", "profile"},
		EmailClaim:     "email",
		NameClaim:      "name",
		ImageClaim:     "picture",
		AccountIDClaim: "sub",
	}

	GitHubOAuthConfig = OAuthProviderConfig{
		AuthURL:        "https://github.com/login/oauth/authorize",
		TokenURL:       "https://github.com/login/oauth/access_token",
		UserInfoURL:    "https://api.github.com/user",
		Scopes:         []string{"user:email", "read:user"},
		EmailClaim:     "email",
		NameClaim:      "name",
		ImageClaim:     "avatar_url",
		AccountIDClaim: "id",
	}
)

type BetterAuthOptions struct {
	DisableSignUp            bool
	RequireEmailVerification bool
	AllowedHostnames         []string
	PublicBaseURL            string
	Secret                   string
	// OAuthProviders maps provider names (e.g., "google", "github") to their configurations.
	// Client ID and Client Secret should be set via environment variables.
	OAuthProviders map[string]OAuthProviderConfig
}

type BetterAuthHandler struct {
	db                       *gorm.DB
	disableSignUp            bool
	requireEmailVerification bool
	trustedOrigins           map[string]struct{}
	publicBaseURL            string
	secret                   string
	useSecureCookies         bool
	oauthProviders           map[string]OAuthProviderConfig
}

type betterAuthUserPayload struct {
	ID            string     `json:"id"`
	Email         string     `json:"email"`
	Name          string     `json:"name"`
	EmailVerified bool       `json:"emailVerified"`
	Image         *string    `json:"image"`
	CreatedAt     *time.Time `json:"createdAt,omitempty"`
	UpdatedAt     *time.Time `json:"updatedAt,omitempty"`
}

type betterAuthSessionPayload struct {
	ID        string    `json:"id"`
	ExpiresAt time.Time `json:"expiresAt"`
	Token     string    `json:"token"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	IPAddress *string   `json:"ipAddress"`
	UserAgent *string   `json:"userAgent"`
	UserID    string    `json:"userId"`
}

func NewBetterAuthHandler(db *gorm.DB, opts BetterAuthOptions) *BetterAuthHandler {
	secret := strings.TrimSpace(opts.Secret)
	providers := make(map[string]OAuthProviderConfig)
	for name, config := range opts.OAuthProviders {
		if config.ClientID != "" && config.ClientSecret != "" {
			providers[strings.ToLower(name)] = config
		} else if config.ClientID != "" || config.ClientSecret != "" {
			log.Printf("auth: OAuth provider %q skipped: missing ClientID or ClientSecret", name)
		}
	}
	return &BetterAuthHandler{
		db:                       db,
		disableSignUp:            opts.DisableSignUp,
		requireEmailVerification: opts.RequireEmailVerification,
		trustedOrigins:           deriveBetterAuthTrustedOrigins(opts.PublicBaseURL, opts.AllowedHostnames),
		publicBaseURL:            strings.TrimRight(strings.TrimSpace(opts.PublicBaseURL), "/"),
		secret:                   secret,
		useSecureCookies:         strings.HasPrefix(strings.ToLower(strings.TrimSpace(opts.PublicBaseURL)), "https://"),
		oauthProviders:           providers,
	}
}

func (h *BetterAuthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	method := r.Method

	switch {
	case method == http.MethodPost && strings.HasSuffix(path, "/sign-up/email"):
		h.handleSignUp(w, r)
	case method == http.MethodPost && strings.HasSuffix(path, "/sign-in/email"):
		h.handleSignIn(w, r)
	case method == http.MethodPost && strings.HasSuffix(path, "/sign-out"):
		h.handleSignOut(w, r)
	case (method == http.MethodGet || method == http.MethodPost) && strings.HasSuffix(path, "/verify-email"):
		h.handleVerifyEmail(w, r)
	// OAuth routes
	case method == http.MethodGet && strings.Contains(path, "/signin/"):
		h.handleOAuthSignIn(w, r)
	case method == http.MethodGet && strings.Contains(path, "/callback/"):
		h.handleOAuthCallback(w, r)
	// Account linking routes
	case method == http.MethodPost && strings.Contains(path, "/link/"):
		h.handleLinkAccount(w, r)
	case method == http.MethodDelete && strings.Contains(path, "/unlink/"):
		h.handleUnlinkAccount(w, r)
	case method == http.MethodGet && strings.HasSuffix(path, "/accounts"):
		h.handleListAccounts(w, r)
	case method == http.MethodGet && (strings.HasSuffix(path, "/me") || strings.HasSuffix(path, "/session")):
		h.handleMe(w, r)
	default:
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "not implemented"})
	}
}

type signUpRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	Name        string `json:"name"`
	CallbackURL string `json:"callbackURL"`
}

func (h *BetterAuthHandler) handleSignUp(w http.ResponseWriter, r *http.Request) {
	if err := h.validateRequestOrigin(r); err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		return
	}
	if h.disableSignUp {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Email and password sign up is not enabled"})
		return
	}

	var req signUpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	email, err := normalizeEmail(req.Email)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid email"})
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing required fields"})
		return
	}
	if err := validatePassword(req.Password); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	var existingUser models.User
	if err := h.db.Where("LOWER(email) = ?", email).First(&existingUser).Error; err == nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "User already exists. Use another email."})
		return
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Printf("auth: signup existing-user lookup error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create user"})
		return
	}

	hashedPassword, err := hashPassword(req.Password)
	if err != nil {
		log.Printf("auth: password hash error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create user"})
		return
	}

	userID := uuid.NewString()
	now := time.Now().UTC()
	user := models.User{
		ID:            userID,
		Name:          strings.TrimSpace(req.Name),
		Email:         email,
		EmailVerified: false,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		account := models.Account{
			ID:         uuid.NewString(),
			AccountID:  userID,
			ProviderID: "credential",
			UserID:     userID,
			Password:   &hashedPassword,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		return tx.Create(&account).Error
	})
	if err != nil {
		log.Printf("auth: signup transaction error: %v", err)
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "Failed to create user"})
		return
	}

	if h.requireEmailVerification {
		h.logVerificationURL(r, user, req.CallbackURL)
		writeJSON(w, http.StatusOK, map[string]any{
			"token": nil,
			"user":  buildBetterAuthUserPayload(user),
		})
		return
	}

	session, err := h.createSession(r, user.ID)
	if err != nil {
		log.Printf("auth: signup session creation error: %v", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Failed to create session"})
		return
	}
	h.setSessionCookies(w, r, session)
	writeJSON(w, http.StatusOK, map[string]any{
		"token": session.Token,
		"user":  buildBetterAuthUserPayload(user),
	})
}

type signInRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	CallbackURL string `json:"callbackURL"`
}

func (h *BetterAuthHandler) handleSignIn(w http.ResponseWriter, r *http.Request) {
	if err := h.validateRequestOrigin(r); err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		return
	}

	var req signInRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	email, err := normalizeEmail(req.Email)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid email"})
		return
	}

	var user models.User
	if err := h.db.Where("LOWER(email) = ?", email).First(&user).Error; err != nil {
		_, _ = hashPassword(req.Password)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid email or password"})
		return
	}

	var account models.Account
	if err := h.db.Where("user_id = ? AND provider_id = ?", user.ID, "credential").First(&account).Error; err != nil || account.Password == nil {
		_, _ = hashPassword(req.Password)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid email or password"})
		return
	}

	ok, upgradedHash, needsUpgrade := verifyStoredPassword(req.Password, *account.Password)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid email or password"})
		return
	}
	if needsUpgrade {
		now := time.Now().UTC()
		if err := h.db.Model(&account).Updates(map[string]any{
			"password":   upgradedHash,
			"updated_at": now,
		}).Error; err != nil {
			log.Printf("auth: password upgrade error: %v", err)
		}
	}

	if h.requireEmailVerification && !user.EmailVerified {
		h.logVerificationURL(r, user, req.CallbackURL)
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Email not verified"})
		return
	}
	if callbackURL := strings.TrimSpace(req.CallbackURL); callbackURL != "" {
		if err := h.validateCallbackURL(callbackURL); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}
	}

	session, err := h.createSession(r, user.ID)
	if err != nil {
		log.Printf("auth: session creation error: %v", err)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Failed to create session"})
		return
	}
	h.setSessionCookies(w, r, session)
	if callbackURL := strings.TrimSpace(req.CallbackURL); callbackURL != "" {
		w.Header().Set("Location", callbackURL)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"redirect": strings.TrimSpace(req.CallbackURL) != "",
		"token":    session.Token,
		"url":      strings.TrimSpace(req.CallbackURL),
		"user":     buildBetterAuthUserPayload(user),
	})
}

func (h *BetterAuthHandler) handleSignOut(w http.ResponseWriter, r *http.Request) {
	if err := h.validateRequestOrigin(r); err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		return
	}

	if token, ok := resolveBetterAuthSessionToken(r, h.secret); ok {
		h.db.Where("token = ?", token).Delete(&models.Session{})
	}

	h.clearSessionCookies(w)
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

type verifyEmailRequest struct {
	Token       string `json:"token"`
	CallbackURL string `json:"callbackURL"`
}

func (h *BetterAuthHandler) handleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	var token string
	var callbackURL string
	if r.Method == http.MethodGet {
		token = strings.TrimSpace(r.URL.Query().Get("token"))
		callbackURL = strings.TrimSpace(r.URL.Query().Get("callbackURL"))
	} else {
		var req verifyEmailRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		token = strings.TrimSpace(req.Token)
		callbackURL = strings.TrimSpace(req.CallbackURL)
	}

	if callbackURL != "" {
		if err := h.validateCallbackURL(callbackURL); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}
	}

	email, tokenErr := verifyEmailVerificationToken(token, h.secret)
	if tokenErr != nil {
		h.respondVerifyEmailError(w, r, callbackURL, tokenErr)
		return
	}

	var user models.User
	if err := h.db.Where("LOWER(email) = ?", email).First(&user).Error; err != nil {
		h.respondVerifyEmailError(w, r, callbackURL, errors.New("user_not_found"))
		return
	}

	if !user.EmailVerified {
		now := time.Now().UTC()
		if err := h.db.Model(&models.User{}).Where("id = ?", user.ID).Updates(map[string]any{
			"email_verified": true,
			"updated_at":     now,
		}).Error; err != nil {
			log.Printf("auth: verify-email update error: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update user"})
			return
		}
		user.EmailVerified = true
		user.UpdatedAt = now
	}

	if callbackURL != "" && r.Method == http.MethodGet {
		http.Redirect(w, r, callbackURL, http.StatusFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": true,
		"user":   nil,
	})
}

func (h *BetterAuthHandler) handleMe(w http.ResponseWriter, r *http.Request) {
	token, ok := resolveBetterAuthSessionToken(r, h.secret)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	var session models.Session
	if err := h.db.Preload("User").Where("token = ?", token).First(&session).Error; err != nil {
		h.clearSessionCookies(w)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	if session.ExpiresAt.Before(time.Now().UTC()) {
		h.db.Delete(&session)
		h.clearSessionCookies(w)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Session expired"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"user":    buildBetterAuthUserPayload(session.User),
		"session": buildBetterAuthSessionPayload(session),
	})
}

func (h *BetterAuthHandler) respondVerifyEmailError(w http.ResponseWriter, r *http.Request, callbackURL string, err error) {
	code := "invalid_token"
	status := http.StatusUnauthorized
	switch err.Error() {
	case "token_expired":
		code = "token_expired"
	case "user_not_found":
		code = "user_not_found"
	}
	if callbackURL != "" && r.Method == http.MethodGet {
		target := callbackURL
		if strings.Contains(target, "?") {
			target += "&error=" + url.QueryEscape(code)
		} else {
			target += "?error=" + url.QueryEscape(code)
		}
		http.Redirect(w, r, target, http.StatusFound)
		return
	}
	writeJSON(w, status, map[string]string{"error": code})
}

func (h *BetterAuthHandler) createSession(r *http.Request, userID string) (*models.Session, error) {
	now := time.Now().UTC()
	expiresAt := now.Add(betterAuthSessionDays * 24 * time.Hour)
	token := hex.EncodeToString(generateRandomBytes(32))
	sessionID := uuid.NewString()
	userAgent := r.Header.Get("User-Agent")
	ipAddress := requestIPAddress(r)

	session := &models.Session{
		ID:        sessionID,
		UserID:    userID,
		Token:     token,
		ExpiresAt: expiresAt,
		CreatedAt: now,
		UpdatedAt: now,
		UserAgent: stringPtr(userAgent),
		IPAddress: stringPtr(ipAddress),
	}
	return session, h.db.Create(session).Error
}

func (h *BetterAuthHandler) setSessionCookies(w http.ResponseWriter, r *http.Request, session *models.Session) {
	cookieName := betterAuthCookieBaseName
	secure := h.useSecureCookies || r.TLS != nil
	if secure {
		cookieName = secureBetterAuthCookieName()
	}

	cookieValue := session.Token
	if h.secret != "" {
		signedValue, err := signBetterAuthCookieValue(session.Token, h.secret)
		if err == nil {
			cookieValue = signedValue
		}
	}

	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    cookieValue,
		Path:     "/",
		Expires:  session.ExpiresAt,
		MaxAge:   int(time.Until(session.ExpiresAt).Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *BetterAuthHandler) clearSessionCookies(w http.ResponseWriter) {
	for _, name := range []string{
		betterAuthCookieBaseName,
		secureBetterAuthCookieName(),
		"better-auth.session_data",
		"__Secure-better-auth.session_data",
		"better-auth.dont_remember",
		"__Secure-better-auth.dont_remember",
	} {
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/",
			Expires:  time.Unix(0, 0),
			MaxAge:   0,
			HttpOnly: true,
			Secure:   strings.HasPrefix(name, "__Secure-"),
			SameSite: http.SameSiteLaxMode,
		})
	}
}

func (h *BetterAuthHandler) validateRequestOrigin(r *http.Request) error {
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
		return nil
	}
	if len(h.trustedOrigins) == 0 {
		return nil
	}

	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		referer := strings.TrimSpace(r.Header.Get("Referer"))
		if referer != "" {
			if parsed, err := url.Parse(referer); err == nil && parsed.Scheme != "" && parsed.Host != "" {
				origin = parsed.Scheme + "://" + parsed.Host
			}
		}
	}
	if origin == "" || origin == "null" {
		return errors.New("Missing or null Origin")
	}
	if _, ok := h.trustedOrigins[strings.ToLower(origin)]; !ok {
		return errors.New("Invalid origin")
	}
	return nil
}

func (h *BetterAuthHandler) validateCallbackURL(callbackURL string) error {
	callbackURL = strings.TrimSpace(callbackURL)
	if callbackURL == "" {
		return nil
	}
	if strings.HasPrefix(callbackURL, "/") {
		return nil
	}
	parsed, err := url.Parse(callbackURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("Invalid callbackURL")
	}
	if len(h.trustedOrigins) == 0 {
		return nil
	}
	if _, ok := h.trustedOrigins[strings.ToLower(parsed.Scheme+"://"+parsed.Host)]; !ok {
		return errors.New("Invalid callbackURL")
	}
	return nil
}

func (h *BetterAuthHandler) logVerificationURL(r *http.Request, user models.User, callbackURL string) {
	if h.secret == "" {
		log.Printf("auth: verification requested for %s but BETTER_AUTH_SECRET is not configured", user.Email)
		return
	}
	token, err := createEmailVerificationToken(h.secret, user.Email, emailVerificationTTL)
	if err != nil {
		log.Printf("auth: failed to create verification token for %s: %v", user.Email, err)
		return
	}
	if callbackURL == "" {
		callbackURL = "/"
	}
	verificationURL := fmt.Sprintf(
		"%s/verify-email?token=%s&callbackURL=%s",
		h.authBaseURL(r),
		url.QueryEscape(token),
		url.QueryEscape(callbackURL),
	)
	log.Printf("auth: verify email for %s via %s", user.Email, verificationURL)
}

func (h *BetterAuthHandler) authBaseURL(r *http.Request) string {
	if h.publicBaseURL != "" {
		return h.publicBaseURL
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if forwardedProto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); forwardedProto != "" {
		scheme = strings.ToLower(forwardedProto)
	}
	return fmt.Sprintf("%s://%s/api/auth", scheme, r.Host)
}

func hashPassword(password string) (string, error) {
	saltHex := hex.EncodeToString(generateRandomBytes(passwordSaltLen))
	hash, err := scrypt.Key([]byte(normalizePassword(password)), []byte(saltHex), betterAuthPasswordN, betterAuthPasswordR, betterAuthPasswordP, betterAuthPasswordKeyLen)
	if err != nil {
		return "", err
	}
	return saltHex + ":" + hex.EncodeToString(hash), nil
}

func verifyStoredPassword(password, stored string) (bool, string, bool) {
	if verifyBetterAuthPassword(password, stored) {
		return true, "", false
	}
	if verifyLegacyPassword(password, stored) {
		upgradedHash, err := hashPassword(password)
		if err != nil {
			return true, "", false
		}
		return true, upgradedHash, true
	}
	return false, "", false
}

func verifyBetterAuthPassword(password, stored string) bool {
	saltHex, existingHash, ok := parsePasswordHash(stored)
	if !ok {
		return false
	}
	hash, err := scrypt.Key([]byte(normalizePassword(password)), []byte(saltHex), betterAuthPasswordN, betterAuthPasswordR, betterAuthPasswordP, betterAuthPasswordKeyLen)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(hash, existingHash) == 1
}

func verifyLegacyPassword(password, stored string) bool {
	saltHex, existingHash, ok := parsePasswordHash(stored)
	if !ok {
		return false
	}
	salt, err := hex.DecodeString(saltHex)
	if err != nil {
		return false
	}
	hash, err := scrypt.Key([]byte(password), salt, legacyPasswordN, legacyPasswordR, legacyPasswordP, legacyPasswordKeyLen)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(hash, existingHash) == 1
}

func parsePasswordHash(stored string) (string, []byte, bool) {
	parts := strings.Split(stored, ":")
	if len(parts) != 2 {
		return "", nil, false
	}
	existingHash, err := hex.DecodeString(parts[1])
	if err != nil {
		return "", nil, false
	}
	return parts[0], existingHash, true
}

func normalizePassword(password string) string {
	return norm.NFKC.String(password)
}

func deriveBetterAuthTrustedOrigins(publicBaseURL string, allowedHostnames []string) map[string]struct{} {
	trustedOrigins := make(map[string]struct{})
	if baseURL := strings.TrimSpace(publicBaseURL); baseURL != "" {
		if parsed, err := url.Parse(baseURL); err == nil && parsed.Scheme != "" && parsed.Host != "" {
			trustedOrigins[strings.ToLower(parsed.Scheme+"://"+parsed.Host)] = struct{}{}
		}
	}
	for _, hostname := range allowedHostnames {
		trimmed := strings.ToLower(strings.TrimSpace(hostname))
		if trimmed == "" {
			continue
		}
		trustedOrigins["https://"+trimmed] = struct{}{}
		trustedOrigins["http://"+trimmed] = struct{}{}
	}
	return trustedOrigins
}

func resolveBetterAuthSessionToken(r *http.Request, secret string) (string, bool) {
	for _, name := range []string{secureBetterAuthCookieName(), betterAuthCookieBaseName} {
		cookie, err := r.Cookie(name)
		if err != nil || cookie.Value == "" {
			continue
		}
		if secret != "" {
			if signedValue, ok := verifyBetterAuthCookieValue(cookie.Value, secret); ok {
				return signedValue, true
			}
			decodedValue, decodeErr := url.QueryUnescape(cookie.Value)
			if decodeErr == nil && strings.Contains(decodedValue, ".") {
				continue
			}
		}
		return cookie.Value, true
	}
	return "", false
}

func signBetterAuthCookieValue(value, secret string) (string, error) {
	signature := computeBetterAuthCookieSignature(value, secret)
	return url.QueryEscape(value + "." + signature), nil
}

func verifyBetterAuthCookieValue(rawValue, secret string) (string, bool) {
	decodedValue, err := url.QueryUnescape(rawValue)
	if err != nil {
		return "", false
	}
	signatureStart := strings.LastIndex(decodedValue, ".")
	if signatureStart < 1 {
		return "", false
	}
	value := decodedValue[:signatureStart]
	signature := decodedValue[signatureStart+1:]
	if len(signature) != signedCookieSigLength || !strings.HasSuffix(signature, "=") {
		return "", false
	}
	expected := computeBetterAuthCookieSignature(value, secret)
	if subtle.ConstantTimeCompare([]byte(signature), []byte(expected)) != 1 {
		return "", false
	}
	return value, true
}

func computeBetterAuthCookieSignature(value, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(value))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func secureBetterAuthCookieName() string {
	return "__Secure-" + betterAuthCookieBaseName
}

func createEmailVerificationToken(secret, email string, ttl time.Duration) (string, error) {
	if secret == "" {
		return "", errors.New("missing auth secret")
	}
	headerJSON := `{"alg":"HS256","typ":"JWT"}`
	now := time.Now().Unix()
	payloadJSON := fmt.Sprintf(`{"email":%q,"iat":%d,"exp":%d}`, strings.ToLower(email), now, now+int64(ttl.Seconds()))
	header := base64.RawURLEncoding.EncodeToString([]byte(headerJSON))
	payload := base64.RawURLEncoding.EncodeToString([]byte(payloadJSON))
	signingInput := header + "." + payload
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingInput))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signingInput + "." + signature, nil
}

func verifyEmailVerificationToken(token, secret string) (string, error) {
	if secret == "" || strings.TrimSpace(token) == "" {
		return "", errors.New("invalid_token")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", errors.New("invalid_token")
	}
	signingInput := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingInput))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(parts[2]), []byte(expected)) != 1 {
		return "", errors.New("invalid_token")
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", errors.New("invalid_token")
	}
	var payload struct {
		Email string `json:"email"`
		Exp   int64  `json:"exp"`
	}
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return "", errors.New("invalid_token")
	}
	if payload.Exp > 0 && time.Now().Unix() > payload.Exp {
		return "", errors.New("token_expired")
	}
	email, err := normalizeEmail(payload.Email)
	if err != nil {
		return "", errors.New("invalid_token")
	}
	return email, nil
}

func buildBetterAuthUserPayload(user models.User) betterAuthUserPayload {
	createdAt := user.CreatedAt
	updatedAt := user.UpdatedAt
	return betterAuthUserPayload{
		ID:            user.ID,
		Email:         user.Email,
		Name:          user.Name,
		EmailVerified: user.EmailVerified,
		Image:         user.Image,
		CreatedAt:     &createdAt,
		UpdatedAt:     &updatedAt,
	}
}

func buildBetterAuthSessionPayload(session models.Session) betterAuthSessionPayload {
	return betterAuthSessionPayload{
		ID:        session.ID,
		ExpiresAt: session.ExpiresAt,
		Token:     session.Token,
		CreatedAt: session.CreatedAt,
		UpdatedAt: session.UpdatedAt,
		IPAddress: session.IPAddress,
		UserAgent: session.UserAgent,
		UserID:    session.UserID,
	}
}

func validatePassword(password string) error {
	switch {
	case len(password) < minPasswordLength:
		return errors.New("Password too short")
	case len(password) > maxPasswordLength:
		return errors.New("Password too long")
	default:
		return nil
	}
}

func normalizeEmail(email string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(email))
	parsed, err := mail.ParseAddress(normalized)
	if err != nil || parsed.Address != normalized {
		return "", errors.New("invalid email")
	}
	return normalized, nil
}

func requestIPAddress(r *http.Request) string {
	if forwardedFor := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwardedFor != "" {
		parts := strings.Split(forwardedFor, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	if forwarded := strings.TrimSpace(r.Header.Get("X-Real-IP")); forwarded != "" {
		return forwarded
	}
	return r.RemoteAddr
}

func stringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload) //nolint:errcheck
}

func generateRandomBytes(n int) []byte {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		panic(err)
	}
	return b
}

// ============================================================================
// OAuth 2.0 Implementation
// ============================================================================

const (
	oauthStateCookieName    = "oauth_state"
	oauthStateTTL           = 10 * time.Minute
	oauthLinkingCookieName  = "oauth_linking"
	oauthCallbackURLCookie  = "oauth_callback_url"
)

// oauthState stores the state parameter for OAuth CSRF protection
type oauthState struct {
	State       string `json:"state"`
	Provider    string `json:"provider"`
	ExpiresAt   int64  `json:"expiresAt"`
	CallbackURL string `json:"callbackUrl,omitempty"`
	// LinkUserID is set when this is an account linking flow (user already authenticated)
	LinkUserID string `json:"linkUserId,omitempty"`
}

// extractProviderFromPath extracts the provider name from paths like /api/auth/signin/{provider}
func extractProviderFromPath(path string) string {
	parts := strings.Split(strings.TrimSuffix(path, "/"), "/")
	if len(parts) > 0 {
		return strings.ToLower(parts[len(parts)-1])
	}
	return ""
}

// handleOAuthSignIn initiates the OAuth 2.0 authorization flow
// GET /api/auth/signin/{provider}?callbackURL=...
func (h *BetterAuthHandler) handleOAuthSignIn(w http.ResponseWriter, r *http.Request) {
	provider := extractProviderFromPath(r.URL.Path)
	if provider == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Provider not specified"})
		return
	}

	config, ok := h.oauthProviders[provider]
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("OAuth provider '%s' not configured", provider)})
		return
	}

	callbackURL := r.URL.Query().Get("callbackURL")
	if callbackURL != "" {
		if err := h.validateCallbackURL(callbackURL); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}
	}

	// Generate state for CSRF protection
	state := hex.EncodeToString(generateRandomBytes(32))
	stateData := oauthState{
		State:       state,
		Provider:    provider,
		ExpiresAt:   time.Now().Add(oauthStateTTL).Unix(),
		CallbackURL: callbackURL,
	}

	// Sign and encode the state
	stateToken, err := h.createOAuthStateToken(stateData)
	if err != nil {
		log.Printf("auth: failed to create OAuth state token: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to initiate OAuth"})
		return
	}

	// Set state cookie
	h.setOAuthStateCookie(w, r, stateToken)

	// Build the authorization URL
	redirectURI := h.oauthRedirectURI(r, provider)
	authURL := h.buildOAuthAuthorizationURL(config, state, redirectURI)

	// Redirect to OAuth provider
	http.Redirect(w, r, authURL, http.StatusFound)
}

// handleOAuthCallback handles the OAuth 2.0 callback from the provider
// GET /api/auth/callback/{provider}?code=...&state=...
func (h *BetterAuthHandler) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	provider := extractProviderFromPath(r.URL.Path)
	if provider == "" {
		h.oauthErrorRedirect(w, r, "", "Provider not specified")
		return
	}

	config, ok := h.oauthProviders[provider]
	if !ok {
		h.oauthErrorRedirect(w, r, "", fmt.Sprintf("OAuth provider '%s' not configured", provider))
		return
	}

	// Check for error from OAuth provider
	if errCode := r.URL.Query().Get("error"); errCode != "" {
		errDesc := r.URL.Query().Get("error_description")
		if errDesc == "" {
			errDesc = errCode
		}
		h.oauthErrorRedirect(w, r, "", errDesc)
		return
	}

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		h.oauthErrorRedirect(w, r, "", "Missing code or state parameter")
		return
	}

	// Verify state token
	stateData, err := h.verifyOAuthStateToken(r, state, provider)
	if err != nil {
		log.Printf("auth: OAuth state verification failed: %v", err)
		h.oauthErrorRedirect(w, r, "", "Invalid or expired state")
		return
	}

	// Exchange code for tokens
	redirectURI := h.oauthRedirectURI(r, provider)
	tokens, err := h.exchangeOAuthCode(r.Context(), config, code, redirectURI)
	if err != nil {
		log.Printf("auth: OAuth token exchange failed for %s: %v", provider, err)
		h.oauthErrorRedirect(w, r, stateData.CallbackURL, "Failed to exchange authorization code")
		return
	}

	// Get user info from provider
	userInfo, err := h.getOAuthUserInfo(r.Context(), config, tokens.AccessToken)
	if err != nil {
		log.Printf("auth: OAuth user info fetch failed for %s: %v", provider, err)
		h.oauthErrorRedirect(w, r, stateData.CallbackURL, "Failed to get user info")
		return
	}

	// Clear the state cookie
	h.clearOAuthStateCookie(w)

	// Handle linking flow vs sign-in flow
	if stateData.LinkUserID != "" {
		h.completeAccountLinking(w, r, provider, stateData.LinkUserID, userInfo, tokens, stateData.CallbackURL)
		return
	}

	// Sign-in/sign-up flow
	h.completeOAuthSignIn(w, r, provider, config, userInfo, tokens, stateData.CallbackURL)
}

// oauthTokenResponse represents the token response from an OAuth provider
type oauthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
}

// oauthUserInfo represents normalized user info from any OAuth provider
type oauthUserInfo struct {
	AccountID string
	Email     string
	Name      string
	Image     string
}

// exchangeOAuthCode exchanges the authorization code for access tokens
func (h *BetterAuthHandler) exchangeOAuthCode(ctx context.Context, config OAuthProviderConfig, code, redirectURI string) (*oauthTokenResponse, error) {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {config.ClientID},
		"client_secret": {config.ClientSecret},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, config.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token endpoint returned status %d", resp.StatusCode)
	}

	var tokens oauthTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokens); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	return &tokens, nil
}

// getOAuthUserInfo fetches user info from the OAuth provider
func (h *BetterAuthHandler) getOAuthUserInfo(ctx context.Context, config OAuthProviderConfig, accessToken string) (*oauthUserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, config.UserInfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create userinfo request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("userinfo request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo endpoint returned status %d", resp.StatusCode)
	}

	var data map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode userinfo response: %w", err)
	}

	userInfo := &oauthUserInfo{
		AccountID: extractStringClaim(data, config.AccountIDClaim, "sub", "id"),
		Email:     extractStringClaim(data, config.EmailClaim, "email"),
		Name:      extractStringClaim(data, config.NameClaim, "name", "login"),
		Image:     extractStringClaim(data, config.ImageClaim, "picture", "avatar_url"),
	}

	if userInfo.AccountID == "" {
		return nil, errors.New("missing account ID in userinfo response")
	}

	return userInfo, nil
}

// extractStringClaim extracts a string value from a map, trying multiple keys
func extractStringClaim(data map[string]any, keys ...string) string {
	for _, key := range keys {
		if key == "" {
			continue
		}
		if val, ok := data[key]; ok {
			switch v := val.(type) {
			case string:
				return v
			case float64:
				return fmt.Sprintf("%.0f", v)
			case int:
				return fmt.Sprintf("%d", v)
			}
		}
	}
	return ""
}

// completeOAuthSignIn handles the final OAuth sign-in/sign-up logic
func (h *BetterAuthHandler) completeOAuthSignIn(w http.ResponseWriter, r *http.Request, provider string, config OAuthProviderConfig, userInfo *oauthUserInfo, tokens *oauthTokenResponse, callbackURL string) {
	now := time.Now().UTC()

	// Look for existing account with this OAuth provider + accountID
	var existingAccount models.Account
	err := h.db.Where("provider_id = ? AND account_id = ?", provider, userInfo.AccountID).
		First(&existingAccount).Error

	if err == nil {
		// Existing OAuth account found - sign in
		h.signInExistingOAuthUser(w, r, existingAccount, tokens, userInfo, now, callbackURL)
		return
	}

	if !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Printf("auth: OAuth account lookup error: %v", err)
		h.oauthErrorRedirect(w, r, callbackURL, "Failed to process sign-in")
		return
	}

	// No existing OAuth account - check if email matches an existing user
	if userInfo.Email != "" {
		email, _ := normalizeEmail(userInfo.Email)
		if email != "" {
			var existingUser models.User
			if err := h.db.Where("LOWER(email) = ?", email).First(&existingUser).Error; err == nil {
				// Link OAuth to existing user with matching email
				h.linkOAuthToExistingUser(w, r, provider, existingUser, userInfo, tokens, now, callbackURL)
				return
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				log.Printf("auth: user lookup error: %v", err)
			}
		}
	}

	// Check if sign-up is disabled
	if h.disableSignUp {
		h.oauthErrorRedirect(w, r, callbackURL, "Sign up is disabled")
		return
	}

	// Create new user and OAuth account
	h.createOAuthUser(w, r, provider, userInfo, tokens, now, callbackURL)
}

// signInExistingOAuthUser handles signing in an existing OAuth user
func (h *BetterAuthHandler) signInExistingOAuthUser(w http.ResponseWriter, r *http.Request, account models.Account, tokens *oauthTokenResponse, userInfo *oauthUserInfo, now time.Time, callbackURL string) {
	// Update tokens
	if err := h.updateAccountTokens(account.ID, tokens, now); err != nil {
		log.Printf("auth: failed to update OAuth tokens: %v", err)
	}

	// Create session
	session, err := h.createSession(r, account.UserID)
	if err != nil {
		log.Printf("auth: OAuth session creation error: %v", err)
		h.oauthErrorRedirect(w, r, callbackURL, "Failed to create session")
		return
	}
	h.setSessionCookies(w, r, session)

	// Redirect to callback or home
	redirectURL := callbackURL
	if redirectURL == "" {
		redirectURL = "/"
	}
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// linkOAuthToExistingUser links an OAuth account to an existing user (email match)
func (h *BetterAuthHandler) linkOAuthToExistingUser(w http.ResponseWriter, r *http.Request, provider string, user models.User, userInfo *oauthUserInfo, tokens *oauthTokenResponse, now time.Time, callbackURL string) {
	account := h.buildOAuthAccount(provider, user.ID, userInfo, tokens, now)

	if err := h.db.Create(&account).Error; err != nil {
		log.Printf("auth: failed to link OAuth account: %v", err)
		h.oauthErrorRedirect(w, r, callbackURL, "Failed to link account")
		return
	}

	// Update user's email verification if the OAuth email matches
	if !user.EmailVerified && strings.EqualFold(user.Email, userInfo.Email) {
		h.db.Model(&models.User{}).Where("id = ?", user.ID).
			Updates(map[string]any{"email_verified": true, "updated_at": now})
	}

	// Create session
	session, err := h.createSession(r, user.ID)
	if err != nil {
		log.Printf("auth: OAuth session creation error: %v", err)
		h.oauthErrorRedirect(w, r, callbackURL, "Failed to create session")
		return
	}
	h.setSessionCookies(w, r, session)

	redirectURL := callbackURL
	if redirectURL == "" {
		redirectURL = "/"
	}
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// createOAuthUser creates a new user from OAuth profile
func (h *BetterAuthHandler) createOAuthUser(w http.ResponseWriter, r *http.Request, provider string, userInfo *oauthUserInfo, tokens *oauthTokenResponse, now time.Time, callbackURL string) {
	userID := uuid.NewString()
	email, _ := normalizeEmail(userInfo.Email)

	name := userInfo.Name
	if name == "" && email != "" {
		name = strings.Split(email, "@")[0]
	}
	if name == "" {
		name = "User"
	}

	user := models.User{
		ID:            userID,
		Name:          name,
		Email:         email,
		EmailVerified: email != "", // Trust OAuth provider's email
		Image:         stringPtr(userInfo.Image),
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	account := h.buildOAuthAccount(provider, userID, userInfo, tokens, now)

	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		return tx.Create(&account).Error
	})

	if err != nil {
		log.Printf("auth: OAuth user creation error: %v", err)
		h.oauthErrorRedirect(w, r, callbackURL, "Failed to create user")
		return
	}

	// Create session
	session, err := h.createSession(r, userID)
	if err != nil {
		log.Printf("auth: OAuth session creation error: %v", err)
		h.oauthErrorRedirect(w, r, callbackURL, "Failed to create session")
		return
	}
	h.setSessionCookies(w, r, session)

	redirectURL := callbackURL
	if redirectURL == "" {
		redirectURL = "/"
	}
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// buildOAuthAccount constructs an Account model for OAuth provider
func (h *BetterAuthHandler) buildOAuthAccount(provider, userID string, userInfo *oauthUserInfo, tokens *oauthTokenResponse, now time.Time) models.Account {
	account := models.Account{
		ID:           uuid.NewString(),
		AccountID:    userInfo.AccountID,
		ProviderID:   provider,
		UserID:       userID,
		AccessToken:  stringPtr(tokens.AccessToken),
		RefreshToken: stringPtr(tokens.RefreshToken),
		IDToken:      stringPtr(tokens.IDToken),
		Scope:        stringPtr(tokens.Scope),
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if tokens.ExpiresIn > 0 {
		expiresAt := now.Add(time.Duration(tokens.ExpiresIn) * time.Second)
		account.AccessTokenExpiresAt = &expiresAt
	}

	return account
}

// updateAccountTokens updates OAuth tokens for an existing account
func (h *BetterAuthHandler) updateAccountTokens(accountID string, tokens *oauthTokenResponse, now time.Time) error {
	updates := map[string]any{
		"access_token": tokens.AccessToken,
		"updated_at":   now,
	}
	if tokens.RefreshToken != "" {
		updates["refresh_token"] = tokens.RefreshToken
	}
	if tokens.IDToken != "" {
		updates["id_token"] = tokens.IDToken
	}
	if tokens.Scope != "" {
		updates["scope"] = tokens.Scope
	}
	if tokens.ExpiresIn > 0 {
		updates["access_token_expires_at"] = now.Add(time.Duration(tokens.ExpiresIn) * time.Second)
	}

	return h.db.Model(&models.Account{}).Where("id = ?", accountID).Updates(updates).Error
}

// ============================================================================
// Account Linking Implementation
// ============================================================================

// handleLinkAccount initiates linking a new OAuth provider to the current user
// POST /api/auth/link/{provider}
func (h *BetterAuthHandler) handleLinkAccount(w http.ResponseWriter, r *http.Request) {
	if err := h.validateRequestOrigin(r); err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		return
	}

	// Get current user from session
	token, ok := resolveBetterAuthSessionToken(r, h.secret)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
		return
	}

	var session models.Session
	if err := h.db.Where("token = ? AND expires_at > ?", token, time.Now()).First(&session).Error; err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid session"})
		return
	}

	provider := extractProviderFromPath(r.URL.Path)
	if provider == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Provider not specified"})
		return
	}

	config, ok := h.oauthProviders[provider]
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("OAuth provider '%s' not configured", provider)})
		return
	}

	// Check if account already linked
	var existingCount int64
	h.db.Model(&models.Account{}).
		Where("user_id = ? AND provider_id = ?", session.UserID, provider).
		Count(&existingCount)

	if existingCount > 0 {
		writeJSON(w, http.StatusConflict, map[string]string{"error": fmt.Sprintf("Account already linked to %s", provider)})
		return
	}

	// Parse callback URL from request body
	var body struct {
		CallbackURL string `json:"callbackURL"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	callbackURL := body.CallbackURL
	if callbackURL != "" {
		if err := h.validateCallbackURL(callbackURL); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}
	}

	// Generate state with linking user ID
	state := hex.EncodeToString(generateRandomBytes(32))
	stateData := oauthState{
		State:       state,
		Provider:    provider,
		ExpiresAt:   time.Now().Add(oauthStateTTL).Unix(),
		CallbackURL: callbackURL,
		LinkUserID:  session.UserID,
	}

	stateToken, err := h.createOAuthStateToken(stateData)
	if err != nil {
		log.Printf("auth: failed to create OAuth state token: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to initiate account linking"})
		return
	}

	h.setOAuthStateCookie(w, r, stateToken)

	redirectURI := h.oauthRedirectURI(r, provider)
	authURL := h.buildOAuthAuthorizationURL(config, state, redirectURI)

	writeJSON(w, http.StatusOK, map[string]string{"url": authURL})
}

// completeAccountLinking handles the OAuth callback for account linking
func (h *BetterAuthHandler) completeAccountLinking(w http.ResponseWriter, r *http.Request, provider, userID string, userInfo *oauthUserInfo, tokens *oauthTokenResponse, callbackURL string) {
	now := time.Now().UTC()

	// Check if this OAuth account is already linked to another user
	var existingAccount models.Account
	if err := h.db.Where("provider_id = ? AND account_id = ?", provider, userInfo.AccountID).
		First(&existingAccount).Error; err == nil {
		if existingAccount.UserID != userID {
			// Account already belongs to another user
			h.oauthErrorRedirect(w, r, callbackURL, "This account is already linked to another user")
			return
		}
		// Already linked to the same user - just redirect
		redirectURL := callbackURL
		if redirectURL == "" {
			redirectURL = "/"
		}
		http.Redirect(w, r, redirectURL, http.StatusFound)
		return
	}

	// Create the linked account
	account := h.buildOAuthAccount(provider, userID, userInfo, tokens, now)

	if err := h.db.Create(&account).Error; err != nil {
		log.Printf("auth: failed to create linked account: %v", err)
		h.oauthErrorRedirect(w, r, callbackURL, "Failed to link account")
		return
	}

	redirectURL := callbackURL
	if redirectURL == "" {
		redirectURL = "/"
	}
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// handleUnlinkAccount removes a linked OAuth provider from the current user
// DELETE /api/auth/unlink/{provider}
func (h *BetterAuthHandler) handleUnlinkAccount(w http.ResponseWriter, r *http.Request) {
	if err := h.validateRequestOrigin(r); err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		return
	}

	// Get current user from session
	token, ok := resolveBetterAuthSessionToken(r, h.secret)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
		return
	}

	var session models.Session
	if err := h.db.Where("token = ? AND expires_at > ?", token, time.Now()).First(&session).Error; err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid session"})
		return
	}

	provider := extractProviderFromPath(r.URL.Path)
	if provider == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Provider not specified"})
		return
	}

	// Check if account exists
	var account models.Account
	if err := h.db.Where("user_id = ? AND provider_id = ?", session.UserID, provider).
		First(&account).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": fmt.Sprintf("No %s account linked", provider)})
			return
		}
		log.Printf("auth: unlink account lookup error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to unlink account"})
		return
	}

	// Prevent unlinking last auth method
	var accountCount int64
	h.db.Model(&models.Account{}).Where("user_id = ?", session.UserID).Count(&accountCount)

	if accountCount <= 1 {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "Cannot unlink last authentication method"})
		return
	}

	// Delete the account
	if err := h.db.Delete(&account).Error; err != nil {
		log.Printf("auth: failed to unlink account: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to unlink account"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// handleListAccounts lists all linked accounts for the current user
// GET /api/auth/accounts
func (h *BetterAuthHandler) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	// Get current user from session
	token, ok := resolveBetterAuthSessionToken(r, h.secret)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
		return
	}

	var session models.Session
	if err := h.db.Where("token = ? AND expires_at > ?", token, time.Now()).First(&session).Error; err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid session"})
		return
	}

	var accounts []models.Account
	if err := h.db.Where("user_id = ?", session.UserID).Find(&accounts).Error; err != nil {
		log.Printf("auth: list accounts error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to list accounts"})
		return
	}

	type accountResponse struct {
		ID         string     `json:"id"`
		ProviderID string     `json:"providerId"`
		AccountID  string     `json:"accountId"`
		CreatedAt  time.Time  `json:"createdAt"`
		HasToken   bool       `json:"hasToken"`
	}

	result := make([]accountResponse, 0, len(accounts))
	for _, acc := range accounts {
		result = append(result, accountResponse{
			ID:         acc.ID,
			ProviderID: acc.ProviderID,
			AccountID:  acc.AccountID,
			CreatedAt:  acc.CreatedAt,
			HasToken:   acc.AccessToken != nil && *acc.AccessToken != "",
		})
	}

	writeJSON(w, http.StatusOK, result)
}

// ============================================================================
// OAuth Helpers
// ============================================================================

// buildOAuthAuthorizationURL constructs the OAuth authorization URL
func (h *BetterAuthHandler) buildOAuthAuthorizationURL(config OAuthProviderConfig, state, redirectURI string) string {
	params := url.Values{
		"client_id":     {config.ClientID},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"state":         {state},
	}
	if len(config.Scopes) > 0 {
		params.Set("scope", strings.Join(config.Scopes, " "))
	}

	return config.AuthURL + "?" + params.Encode()
}

// oauthRedirectURI returns the callback URL for OAuth
func (h *BetterAuthHandler) oauthRedirectURI(r *http.Request, provider string) string {
	if h.publicBaseURL != "" {
		return h.publicBaseURL + "/api/auth/callback/" + provider
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if forwardedProto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); forwardedProto != "" {
		scheme = strings.ToLower(forwardedProto)
	}
	return fmt.Sprintf("%s://%s/api/auth/callback/%s", scheme, r.Host, provider)
}

// createOAuthStateToken creates a signed JWT-like token for OAuth state
func (h *BetterAuthHandler) createOAuthStateToken(stateData oauthState) (string, error) {
	if h.secret == "" {
		return "", errors.New("missing auth secret")
	}
	stateJSON, err := json.Marshal(stateData)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(stateJSON)
	mac := hmac.New(sha256.New, []byte(h.secret))
	mac.Write([]byte(encoded))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return encoded + "." + signature, nil
}

// verifyOAuthStateToken verifies and decodes the OAuth state token
func (h *BetterAuthHandler) verifyOAuthStateToken(r *http.Request, state, provider string) (*oauthState, error) {
	// Get the state token from cookie
	cookie, err := r.Cookie(oauthStateCookieName)
	if err != nil {
		return nil, errors.New("missing state cookie")
	}

	decoded, err := url.QueryUnescape(cookie.Value)
	if err != nil {
		return nil, errors.New("invalid state cookie encoding")
	}

	parts := strings.Split(decoded, ".")
	if len(parts) != 2 {
		return nil, errors.New("invalid state token format")
	}

	// Verify signature
	mac := hmac.New(sha256.New, []byte(h.secret))
	mac.Write([]byte(parts[0]))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(parts[1]), []byte(expected)) != 1 {
		return nil, errors.New("invalid state signature")
	}

	// Decode state data
	stateJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, errors.New("invalid state encoding")
	}

	var stateData oauthState
	if err := json.Unmarshal(stateJSON, &stateData); err != nil {
		return nil, errors.New("invalid state data")
	}

	// Verify state matches
	if stateData.State != state {
		return nil, errors.New("state mismatch")
	}

	// Verify provider matches
	if stateData.Provider != provider {
		return nil, errors.New("provider mismatch")
	}

	// Verify not expired
	if time.Now().Unix() > stateData.ExpiresAt {
		return nil, errors.New("state expired")
	}

	return &stateData, nil
}

// setOAuthStateCookie sets the OAuth state cookie
func (h *BetterAuthHandler) setOAuthStateCookie(w http.ResponseWriter, r *http.Request, stateToken string) {
	secure := h.useSecureCookies || r.TLS != nil
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookieName,
		Value:    url.QueryEscape(stateToken),
		Path:     "/",
		MaxAge:   int(oauthStateTTL.Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// clearOAuthStateCookie clears the OAuth state cookie
func (h *BetterAuthHandler) clearOAuthStateCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
}

// oauthErrorRedirect redirects to the callback URL with an error parameter
func (h *BetterAuthHandler) oauthErrorRedirect(w http.ResponseWriter, r *http.Request, callbackURL, errMsg string) {
	h.clearOAuthStateCookie(w)
	target := callbackURL
	if target == "" {
		target = "/"
	}
	errParam := url.QueryEscape(errMsg)
	if strings.Contains(target, "?") {
		target += "&error=" + errParam
	} else {
		target += "?error=" + errParam
	}
	http.Redirect(w, r, target, http.StatusFound)
}
