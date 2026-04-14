package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupSecretsServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&svc_secrets=1"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.Exec("DROP TABLE IF EXISTS company_secret_versions")
	db.Exec("DROP TABLE IF EXISTS company_secrets")
	db.Exec(`CREATE TABLE company_secrets (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		name text NOT NULL,
		provider text NOT NULL DEFAULT 'local_encrypted',
		external_ref text,
		latest_version integer NOT NULL DEFAULT 1,
		description text,
		created_by_agent_id text,
		created_by_user_id text,
		created_at datetime,
		updated_at datetime
	)`)
	db.Exec(`CREATE TABLE company_secret_versions (
		id text PRIMARY KEY,
		secret_id text NOT NULL,
		version integer NOT NULL,
		material text NOT NULL,
		value_sha256 text NOT NULL DEFAULT '',
		created_by_agent_id text,
		created_by_user_id text,
		created_at datetime,
		revoked_at datetime
	)`)
	return db
}

func TestResolveEnvBindings_PlainType(t *testing.T) {
	db := setupSecretsServiceTestDB(t)
	svc := NewSecretService(db, nil)

	envValue := map[string]interface{}{
		"MY_VAR": map[string]interface{}{
			"type":  "plain",
			"value": "hello-world",
		},
	}

	resolved, err := svc.ResolveEnvBindings(context.Background(), "comp-1", envValue)
	if err != nil {
		t.Fatalf("ResolveEnvBindings: %v", err)
	}

	if resolved["MY_VAR"] != "hello-world" {
		t.Errorf("expected MY_VAR='hello-world', got %q", resolved["MY_VAR"])
	}
}

func TestResolveEnvBindings_MultiplePlainValues(t *testing.T) {
	db := setupSecretsServiceTestDB(t)
	svc := NewSecretService(db, nil)

	envValue := map[string]interface{}{
		"FOO": map[string]interface{}{"type": "plain", "value": "foo-val"},
		"BAR": map[string]interface{}{"type": "plain", "value": "bar-val"},
	}

	resolved, err := svc.ResolveEnvBindings(context.Background(), "comp-1", envValue)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved["FOO"] != "foo-val" {
		t.Errorf("expected FOO='foo-val', got %q", resolved["FOO"])
	}
	if resolved["BAR"] != "bar-val" {
		t.Errorf("expected BAR='bar-val', got %q", resolved["BAR"])
	}
}

func TestResolveEnvBindings_SkipsNonMapValues(t *testing.T) {
	db := setupSecretsServiceTestDB(t)
	svc := NewSecretService(db, nil)

	// A raw string value (not a binding map) should be skipped.
	envValue := map[string]interface{}{
		"SKIP_ME": "raw-string",
	}

	resolved, err := svc.ResolveEnvBindings(context.Background(), "comp-1", envValue)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := resolved["SKIP_ME"]; ok {
		t.Errorf("expected SKIP_ME to be skipped, but it appeared in resolved map")
	}
}

func TestResolveAdapterConfigForRuntime_PassThrough(t *testing.T) {
	db := setupSecretsServiceTestDB(t)
	svc := NewSecretService(db, nil)

	config := map[string]interface{}{
		"model":      "gpt-4",
		"maxTokens":  4096,
		"nestedList": []interface{}{"a", "b"},
	}

	result, err := svc.ResolveAdapterConfigForRuntime(context.Background(), "comp-1", config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result["model"] != "gpt-4" {
		t.Errorf("expected model 'gpt-4', got %v", result["model"])
	}
}

func TestResolveAdapterConfigForRuntime_NestedPlainValues(t *testing.T) {
	db := setupSecretsServiceTestDB(t)
	svc := NewSecretService(db, nil)

	// Nested maps without secret_ref markers should pass through.
	config := map[string]interface{}{
		"options": map[string]interface{}{
			"timeout": 30,
			"retry":   true,
		},
	}

	result, err := svc.ResolveAdapterConfigForRuntime(context.Background(), "comp-1", config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	options, ok := result["options"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected options to be a map, got %T", result["options"])
	}
	if options["timeout"] != 30 {
		t.Errorf("expected timeout=30, got %v", options["timeout"])
	}
}

func TestGenerateOpenBrainToken_Structure(t *testing.T) {
	db := setupSecretsServiceTestDB(t)
	svc := NewSecretService(db, nil)

	token, err := svc.GenerateOpenBrainToken("ns-1", "agent-1", 2)
	if err != nil {
		t.Fatalf("GenerateOpenBrainToken: %v", err)
	}

	if len(token) == 0 {
		t.Error("expected non-empty token")
	}

	// Token format is: "openbrain.<payload>.<sig>"
	if token[:10] != "openbrain." {
		t.Errorf("expected token to start with 'openbrain.', got: %s", token[:10])
	}
}

func TestGenerateOpenBrainToken_ContainsClaims(t *testing.T) {
	db := setupSecretsServiceTestDB(t)
	svc := NewSecretService(db, nil)

	token, err := svc.GenerateOpenBrainToken("ns-abc", "agent-xyz", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Parse the payload section (second dot-segment).
	parts := splitToken(token)
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts, got %d: %v", len(parts), parts)
	}

	import64 := parts[1]
	decoded, err := decodeBase64URL(import64)
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}

	var claims map[string]interface{}
	if err := json.Unmarshal(decoded, &claims); err != nil {
		t.Fatalf("parse claims: %v", err)
	}

	if claims["namespaceId"] != "ns-abc" {
		t.Errorf("expected namespaceId 'ns-abc', got %v", claims["namespaceId"])
	}
	if claims["agentId"] != "agent-xyz" {
		t.Fatalf("expected agentId 'agent-xyz', got %v", claims["agentId"])
	}
}

// splitToken splits a dot-delimited token into its parts.
func splitToken(token string) []string {
	return strings.Split(token, ".")
}

// decodeBase64URL decodes a raw base64url-encoded string.
func decodeBase64URL(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}
