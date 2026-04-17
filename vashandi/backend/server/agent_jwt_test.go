package server

import (
	"os"
	"testing"
	"time"
)

func TestCreateAndVerifyLocalAgentJwt(t *testing.T) {
	// Set up test environment
	t.Setenv("PAPERCLIP_AGENT_JWT_SECRET", "test-secret-key-for-jwt-testing-32b")

	agentID := "agent-123"
	companyID := "company-456"
	adapterType := "process"
	runID := "run-789"

	// Create a JWT
	token := CreateLocalAgentJwt(agentID, companyID, adapterType, runID)
	if token == "" {
		t.Fatal("Expected non-empty JWT token")
	}

	// Verify the JWT
	claims := VerifyLocalAgentJwt(token)
	if claims == nil {
		t.Fatal("Expected valid claims from JWT verification")
	}

	if claims.Sub != agentID {
		t.Errorf("Expected sub=%q, got %q", agentID, claims.Sub)
	}
	if claims.CompanyID != companyID {
		t.Errorf("Expected company_id=%q, got %q", companyID, claims.CompanyID)
	}
	if claims.AdapterType != adapterType {
		t.Errorf("Expected adapter_type=%q, got %q", adapterType, claims.AdapterType)
	}
	if claims.RunID != runID {
		t.Errorf("Expected run_id=%q, got %q", runID, claims.RunID)
	}
	if claims.Iss != "paperclip" {
		t.Errorf("Expected iss=%q, got %q", "paperclip", claims.Iss)
	}
	if claims.Aud != "paperclip-api" {
		t.Errorf("Expected aud=%q, got %q", "paperclip-api", claims.Aud)
	}
}

func TestCreateAndVerifyLocalAgentJwt_OptionalRunID(t *testing.T) {
	// Set up test environment
	t.Setenv("PAPERCLIP_AGENT_JWT_SECRET", "test-secret-key-for-jwt-testing-32b")

	agentID := "agent-123"
	companyID := "company-456"
	adapterType := "process"
	runID := "" // Empty run_id should be allowed

	// Create a JWT with empty run_id
	token := CreateLocalAgentJwt(agentID, companyID, adapterType, runID)
	if token == "" {
		t.Fatal("Expected non-empty JWT token even with empty run_id")
	}

	// Verify the JWT - should succeed even without run_id
	claims := VerifyLocalAgentJwt(token)
	if claims == nil {
		t.Fatal("Expected valid claims from JWT verification even without run_id")
	}

	if claims.Sub != agentID {
		t.Errorf("Expected sub=%q, got %q", agentID, claims.Sub)
	}
	if claims.CompanyID != companyID {
		t.Errorf("Expected company_id=%q, got %q", companyID, claims.CompanyID)
	}
	if claims.RunID != "" {
		t.Errorf("Expected empty run_id, got %q", claims.RunID)
	}
}

func TestVerifyLocalAgentJwt_NoSecret(t *testing.T) {
	t.Setenv("PAPERCLIP_AGENT_JWT_SECRET", "")
	t.Setenv("BETTER_AUTH_SECRET", "")

	claims := VerifyLocalAgentJwt("any-token")
	if claims != nil {
		t.Error("Expected nil claims when no secret is configured")
	}
}

func TestVerifyLocalAgentJwt_InvalidToken(t *testing.T) {
	t.Setenv("PAPERCLIP_AGENT_JWT_SECRET", "test-secret-key-for-jwt-testing-32b")

	tests := []struct {
		name  string
		token string
	}{
		{"empty token", ""},
		{"invalid format", "not-a-jwt"},
		{"too few parts", "header.claims"},
		{"too many parts", "a.b.c.d"},
		{"invalid base64 header", "!!!.claims.signature"},
		{"invalid base64 claims", "eyJhbGciOiJIUzI1NiJ9.!!!.signature"},
		{"wrong algorithm", "eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJ0ZXN0In0.signature"},
		{"invalid signature", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ0ZXN0In0.invalid-sig"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := VerifyLocalAgentJwt(tt.token)
			if claims != nil {
				t.Error("Expected nil claims for invalid token")
			}
		})
	}
}

func TestVerifyLocalAgentJwt_ExpiredToken(t *testing.T) {
	t.Setenv("PAPERCLIP_AGENT_JWT_SECRET", "test-secret-key-for-jwt-testing-32b")
	// Set TTL to 1 second
	t.Setenv("PAPERCLIP_AGENT_JWT_TTL_SECONDS", "1")

	token := CreateLocalAgentJwt("agent", "company", "process", "run")
	if token == "" {
		t.Fatal("Expected non-empty JWT token")
	}

	// Wait for token to expire
	time.Sleep(2 * time.Second)

	claims := VerifyLocalAgentJwt(token)
	if claims != nil {
		t.Error("Expected nil claims for expired token")
	}
}

func TestVerifyLocalAgentJwt_WrongSignature(t *testing.T) {
	t.Setenv("PAPERCLIP_AGENT_JWT_SECRET", "secret-one")

	token := CreateLocalAgentJwt("agent", "company", "process", "run")
	if token == "" {
		t.Fatal("Expected non-empty JWT token")
	}

	// Change secret
	t.Setenv("PAPERCLIP_AGENT_JWT_SECRET", "secret-two")

	claims := VerifyLocalAgentJwt(token)
	if claims != nil {
		t.Error("Expected nil claims when signature doesn't match")
	}
}

func TestVerifyLocalAgentJwt_WrongIssuer(t *testing.T) {
	t.Setenv("PAPERCLIP_AGENT_JWT_SECRET", "test-secret-key-for-jwt-testing-32b")
	t.Setenv("PAPERCLIP_AGENT_JWT_ISSUER", "issuer-one")

	token := CreateLocalAgentJwt("agent", "company", "process", "run")
	if token == "" {
		t.Fatal("Expected non-empty JWT token")
	}

	// Change issuer
	t.Setenv("PAPERCLIP_AGENT_JWT_ISSUER", "issuer-two")

	claims := VerifyLocalAgentJwt(token)
	if claims != nil {
		t.Error("Expected nil claims when issuer doesn't match")
	}
}

func TestVerifyLocalAgentJwt_WrongAudience(t *testing.T) {
	t.Setenv("PAPERCLIP_AGENT_JWT_SECRET", "test-secret-key-for-jwt-testing-32b")
	t.Setenv("PAPERCLIP_AGENT_JWT_AUDIENCE", "audience-one")

	token := CreateLocalAgentJwt("agent", "company", "process", "run")
	if token == "" {
		t.Fatal("Expected non-empty JWT token")
	}

	// Change audience
	t.Setenv("PAPERCLIP_AGENT_JWT_AUDIENCE", "audience-two")

	claims := VerifyLocalAgentJwt(token)
	if claims != nil {
		t.Error("Expected nil claims when audience doesn't match")
	}
}

func TestGetJwtConfig_FallbackToBetterAuthSecret(t *testing.T) {
	t.Setenv("PAPERCLIP_AGENT_JWT_SECRET", "")
	t.Setenv("BETTER_AUTH_SECRET", "fallback-secret")

	config := getJwtConfig()
	if config == nil {
		t.Fatal("Expected config when BETTER_AUTH_SECRET is set")
	}
	if config.Secret != "fallback-secret" {
		t.Errorf("Expected secret=%q, got %q", "fallback-secret", config.Secret)
	}
}

func TestGetJwtConfig_CustomTTL(t *testing.T) {
	t.Setenv("PAPERCLIP_AGENT_JWT_SECRET", "test-secret")
	t.Setenv("PAPERCLIP_AGENT_JWT_TTL_SECONDS", "3600")

	config := getJwtConfig()
	if config == nil {
		t.Fatal("Expected config")
	}
	if config.TTLSeconds != 3600 {
		t.Errorf("Expected TTLSeconds=3600, got %d", config.TTLSeconds)
	}
}

func TestGetJwtConfig_CustomIssuerAndAudience(t *testing.T) {
	t.Setenv("PAPERCLIP_AGENT_JWT_SECRET", "test-secret")
	t.Setenv("PAPERCLIP_AGENT_JWT_ISSUER", "custom-issuer")
	t.Setenv("PAPERCLIP_AGENT_JWT_AUDIENCE", "custom-audience")

	config := getJwtConfig()
	if config == nil {
		t.Fatal("Expected config")
	}
	if config.Issuer != "custom-issuer" {
		t.Errorf("Expected Issuer=%q, got %q", "custom-issuer", config.Issuer)
	}
	if config.Audience != "custom-audience" {
		t.Errorf("Expected Audience=%q, got %q", "custom-audience", config.Audience)
	}
}

func TestCreateLocalAgentJwt_NoSecret(t *testing.T) {
	t.Setenv("PAPERCLIP_AGENT_JWT_SECRET", "")
	t.Setenv("BETTER_AUTH_SECRET", "")

	token := CreateLocalAgentJwt("agent", "company", "process", "run")
	if token != "" {
		t.Error("Expected empty token when no secret is configured")
	}
}

// cleanupEnv removes all JWT-related environment variables.
func cleanupEnv() {
	os.Unsetenv("PAPERCLIP_AGENT_JWT_SECRET")
	os.Unsetenv("BETTER_AUTH_SECRET")
	os.Unsetenv("PAPERCLIP_AGENT_JWT_TTL_SECONDS")
	os.Unsetenv("PAPERCLIP_AGENT_JWT_ISSUER")
	os.Unsetenv("PAPERCLIP_AGENT_JWT_AUDIENCE")
}
