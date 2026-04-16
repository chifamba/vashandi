package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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

type BetterAuthOptions struct {
	DisableSignUp            bool
	RequireEmailVerification bool
	AllowedHostnames         []string
	PublicBaseURL            string
	Secret                   string
}

type BetterAuthHandler struct {
	db                       *gorm.DB
	disableSignUp            bool
	requireEmailVerification bool
	trustedOrigins           map[string]struct{}
	publicBaseURL            string
	secret                   string
	useSecureCookies         bool
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
	return &BetterAuthHandler{
		db:                       db,
		disableSignUp:            opts.DisableSignUp,
		requireEmailVerification: opts.RequireEmailVerification,
		trustedOrigins:           deriveBetterAuthTrustedOrigins(opts.PublicBaseURL, opts.AllowedHostnames),
		publicBaseURL:            strings.TrimRight(strings.TrimSpace(opts.PublicBaseURL), "/"),
		secret:                   secret,
		useSecureCookies:         strings.HasPrefix(strings.ToLower(strings.TrimSpace(opts.PublicBaseURL)), "https://"),
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
