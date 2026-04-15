package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/scrypt"
	"gorm.io/gorm"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
)

const (
	scryptN      = 16384
	scryptR      = 8
	scryptP      = 1
	scryptKeyLen = 64
	saltLen      = 16
	cookieName   = "better-auth.session_token"
)

type BetterAuthHandler struct {
	db *gorm.DB
}

func NewBetterAuthHandler(db *gorm.DB) *BetterAuthHandler {
	return &BetterAuthHandler{db: db}
}

func (h *BetterAuthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	method := r.Method

	switch {
	case method == "POST" && strings.HasSuffix(path, "/sign-up/email"):
		h.handleSignUp(w, r)
	case method == "POST" && strings.HasSuffix(path, "/sign-in/email"):
		h.handleSignIn(w, r)
	case method == "POST" && strings.HasSuffix(path, "/sign-out"):
		h.handleSignOut(w, r)
	default:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotImplemented)
		json.NewEncoder(w).Encode(map[string]string{"error": "not implemented"})
	}
}

type signUpRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

func (h *BetterAuthHandler) handleSignUp(w http.ResponseWriter, r *http.Request) {
	var req signUpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Email == "" || req.Password == "" || req.Name == "" {
		http.Error(w, "missing required fields", http.StatusBadRequest)
		return
	}

	// Check if user already exists
	var existingUser models.User
	if err := h.db.Where("email = ?", req.Email).First(&existingUser).Error; err == nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "user already exists"})
		return
	}

	// Hash password
	hashedPassword, err := hashPassword(req.Password)
	if err != nil {
		http.Error(w, "hashing error", http.StatusInternalServerError)
		return
	}

	userID := uuid.New().String()
	now := time.Now()

	err = h.db.Transaction(func(tx *gorm.DB) error {
		user := models.User{
			ID:            userID,
			Name:          req.Name,
			Email:         req.Email,
			EmailVerified: false,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if err := tx.Create(&user).Error; err != nil {
			return err
		}

		account := models.Account{
			ID:         uuid.New().String(),
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
		log.Printf("auth: signup error: %v", err)
		http.Error(w, "signup failed", http.StatusInternalServerError)
		return
	}

	h.createAndSetSession(w, r, userID)
}

type signInRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *BetterAuthHandler) handleSignIn(w http.ResponseWriter, r *http.Request) {
	var req signInRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	var user models.User
	if err := h.db.Where("email = ?", req.Email).First(&user).Error; err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid email or password"})
		return
	}

	var account models.Account
	if err := h.db.Where("user_id = ? AND provider_id = ?", user.ID, "credential").First(&account).Error; err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid email or password"})
		return
	}

	if account.Password == nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid email or password"})
		return
	}

	if !verifyPassword(req.Password, *account.Password) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid email or password"})
		return
	}

	h.createAndSetSession(w, r, user.ID)
}

func (h *BetterAuthHandler) handleSignOut(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(cookieName)
	if err == nil && cookie.Value != "" {
		h.db.Where("token = ?", cookie.Value).Delete(&models.Session{})
	}

	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *BetterAuthHandler) createAndSetSession(w http.ResponseWriter, r *http.Request, userID string) {
	token := hex.EncodeToString(generateRandomBytes(32))
	sessionID := uuid.New().String()
	now := time.Now()
	expiresAt := now.Add(30 * 24 * time.Hour)

	userAgent := r.Header.Get("User-Agent")
	ipAddress := r.RemoteAddr // Simplified

	session := models.Session{
		ID:        sessionID,
		UserID:    userID,
		Token:     token,
		ExpiresAt: expiresAt,
		CreatedAt: now,
		UpdatedAt: now,
		UserAgent: &userAgent,
		IPAddress: &ipAddress,
	}

	if err := h.db.Create(&session).Error; err != nil {
		log.Printf("auth: session creation error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   r.TLS != nil,
	})

	// Return same response as get-session
	var user models.User
	h.db.First(&user, "id = ?", userID)

	resp := map[string]interface{}{
		"session": map[string]string{
			"id":     sessionID,
			"userId": userID,
		},
		"user": map[string]interface{}{
			"id":    user.ID,
			"email": user.Email,
			"name":  user.Name,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func hashPassword(password string) (string, error) {
	salt := generateRandomBytes(saltLen)
	hash, err := scrypt.Key([]byte(password), salt, scryptN, scryptR, scryptP, scryptKeyLen)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(salt) + ":" + hex.EncodeToString(hash), nil
}

func verifyPassword(password, hashedPassword string) bool {
	parts := strings.Split(hashedPassword, ":")
	if len(parts) != 2 {
		return false
	}

	salt, err := hex.DecodeString(parts[0])
	if err != nil {
		return false
	}

	existingHash, err := hex.DecodeString(parts[1])
	if err != nil {
		return false
	}

	hash, err := scrypt.Key([]byte(password), salt, scryptN, scryptR, scryptP, scryptKeyLen)
	if err != nil {
		return false
	}

	if len(hash) != len(existingHash) {
		return false
	}

	for i := range hash {
		if hash[i] != existingHash[i] {
			return false
		}
	}

	return true
}

func generateRandomBytes(n int) []byte {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		panic(err)
	}
	return b
}
