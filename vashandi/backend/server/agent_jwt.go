package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"
)

const jwtAlgorithm = "HS256"

// LocalAgentJwtClaims represents the claims in a local agent JWT token.
type LocalAgentJwtClaims struct {
	Sub         string `json:"sub"`
	CompanyID   string `json:"company_id"`
	AdapterType string `json:"adapter_type"`
	RunID       string `json:"run_id"`
	Iat         int64  `json:"iat"`
	Exp         int64  `json:"exp"`
	Iss         string `json:"iss,omitempty"`
	Aud         string `json:"aud,omitempty"`
	Jti         string `json:"jti,omitempty"`
}

// jwtHeader represents the JWT header.
type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ,omitempty"`
}

// jwtConfig holds JWT configuration from environment variables.
type jwtConfig struct {
	Secret     string
	TTLSeconds int64
	Issuer     string
	Audience   string
}

// getJwtConfig resolves JWT configuration from environment variables.
// Returns nil if no secret is configured.
func getJwtConfig() *jwtConfig {
	secret := strings.TrimSpace(os.Getenv("PAPERCLIP_AGENT_JWT_SECRET"))
	if secret == "" {
		secret = strings.TrimSpace(os.Getenv("BETTER_AUTH_SECRET"))
	}
	if secret == "" {
		return nil
	}

	ttl := int64(60 * 60 * 48) // 48 hours default
	if ttlEnv := os.Getenv("PAPERCLIP_AGENT_JWT_TTL_SECONDS"); ttlEnv != "" {
		if parsed, err := strconv.ParseInt(ttlEnv, 10, 64); err == nil && parsed > 0 {
			ttl = parsed
		}
	}

	issuer := os.Getenv("PAPERCLIP_AGENT_JWT_ISSUER")
	if issuer == "" {
		issuer = "paperclip"
	}

	audience := os.Getenv("PAPERCLIP_AGENT_JWT_AUDIENCE")
	if audience == "" {
		audience = "paperclip-api"
	}

	return &jwtConfig{
		Secret:     secret,
		TTLSeconds: ttl,
		Issuer:     issuer,
		Audience:   audience,
	}
}

// base64UrlEncode encodes data to base64url without padding.
func base64UrlEncode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

// base64UrlDecode decodes base64url data without padding.
func base64UrlDecode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}

// signPayload creates an HMAC-SHA256 signature for the signing input.
func signPayload(secret, signingInput string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(signingInput))
	return base64UrlEncode(h.Sum(nil))
}

// safeCompare performs a constant-time comparison of two strings.
func safeCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// CreateLocalAgentJwt creates a signed JWT for local agent authentication.
func CreateLocalAgentJwt(agentID, companyID, adapterType, runID string) string {
	config := getJwtConfig()
	if config == nil {
		return ""
	}

	now := time.Now().Unix()
	claims := LocalAgentJwtClaims{
		Sub:         agentID,
		CompanyID:   companyID,
		AdapterType: adapterType,
		RunID:       runID,
		Iat:         now,
		Exp:         now + config.TTLSeconds,
		Iss:         config.Issuer,
		Aud:         config.Audience,
	}

	header := jwtHeader{
		Alg: jwtAlgorithm,
		Typ: "JWT",
	}

	headerBytes, _ := json.Marshal(header)
	claimsBytes, _ := json.Marshal(claims)

	signingInput := base64UrlEncode(headerBytes) + "." + base64UrlEncode(claimsBytes)
	signature := signPayload(config.Secret, signingInput)

	return signingInput + "." + signature
}

// VerifyLocalAgentJwt verifies a JWT token and returns the claims if valid.
// Returns nil if the token is invalid, expired, or verification fails.
func VerifyLocalAgentJwt(token string) *LocalAgentJwtClaims {
	if token == "" {
		return nil
	}

	config := getJwtConfig()
	if config == nil {
		return nil
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil
	}
	headerB64, claimsB64, signature := parts[0], parts[1], parts[2]

	// Verify header
	headerBytes, err := base64UrlDecode(headerB64)
	if err != nil {
		return nil
	}
	var header jwtHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil
	}
	if header.Alg != jwtAlgorithm {
		return nil
	}

	// Verify signature
	signingInput := headerB64 + "." + claimsB64
	expectedSig := signPayload(config.Secret, signingInput)
	if !safeCompare(signature, expectedSig) {
		return nil
	}

	// Decode and validate claims
	claimsBytes, err := base64UrlDecode(claimsB64)
	if err != nil {
		return nil
	}
	var claims LocalAgentJwtClaims
	if err := json.Unmarshal(claimsBytes, &claims); err != nil {
		return nil
	}

	// Validate required fields
	if claims.Sub == "" || claims.CompanyID == "" || claims.AdapterType == "" || claims.RunID == "" {
		return nil
	}
	if claims.Iat == 0 || claims.Exp == 0 {
		return nil
	}

	// Check expiration
	now := time.Now().Unix()
	if claims.Exp < now {
		return nil
	}

	// Validate issuer and audience if present
	if claims.Iss != "" && claims.Iss != config.Issuer {
		return nil
	}
	if claims.Aud != "" && claims.Aud != config.Audience {
		return nil
	}

	return &claims
}
