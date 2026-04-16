package services

import (
	"testing"
)

// ─── TestIsSensitiveEnvKey ────────────────────────────────────────────────────

func TestIsSensitiveEnvKey(t *testing.T) {
	sensitive := []string{
		"TOKEN", "API_KEY", "apikey", "ACCESS_TOKEN", "AUTH",
		"AUTH_TOKEN", "AUTHORIZATION", "BEARER", "SECRET",
		"PASSWORD", "PASSWD", "CREDENTIAL", "JWT",
		"PRIVATE_KEY", "COOKIE", "CONNECTIONSTRING",
		"MY_API_KEY", "GH_TOKEN", "DB_PASSWORD",
	}
	for _, k := range sensitive {
		if !isSensitiveEnvKey(k) {
			t.Errorf("expected %q to be sensitive", k)
		}
	}

	notSensitive := []string{
		"EDITOR", "HOME", "LANG", "NODE_ENV", "PORT",
		"DATABASE_URL", "LOG_LEVEL", "TIMEOUT_SEC",
	}
	for _, k := range notSensitive {
		if isSensitiveEnvKey(k) {
			t.Errorf("expected %q NOT to be sensitive", k)
		}
	}
}

// ─── TestIsAbsoluteCommand ────────────────────────────────────────────────────

func TestIsAbsoluteCommand(t *testing.T) {
	absolute := []string{
		"/usr/bin/python", "/Users/alice/bin/run.sh", "C:\\foo\\bar.exe", "D:/app/run",
	}
	for _, v := range absolute {
		if !isAbsoluteCommand(v) {
			t.Errorf("expected %q to be absolute", v)
		}
	}

	notAbsolute := []string{
		"python", "npm run dev", "node index.js", "./start.sh", "",
	}
	for _, v := range notAbsolute {
		if isAbsoluteCommand(v) {
			t.Errorf("expected %q NOT to be absolute", v)
		}
	}
}

// ─── TestPruneDefaultLikeValue ────────────────────────────────────────────────

func TestPruneDefaultLikeValue(t *testing.T) {
	rules := []defaultRule{
		{path: []string{"timeoutSec"}, value: float64(0)},
		{path: []string{"graceSec"}, value: float64(15)},
	}

	// Default values should be dropped.
	val := map[string]interface{}{
		"timeoutSec": float64(0),
		"graceSec":   float64(15),
		"model":      "claude-3-opus",
	}
	pruned, _ := pruneDefaultLikeValue(val, []string{}, true, rules)
	m, ok := pruned.(map[string]interface{})
	if !ok {
		t.Fatal("expected map result")
	}
	if _, found := m["timeoutSec"]; found {
		t.Error("timeoutSec should have been pruned as default")
	}
	if _, found := m["graceSec"]; found {
		t.Error("graceSec should have been pruned as default")
	}
	if m["model"] != "claude-3-opus" {
		t.Error("model should be preserved")
	}

	// Non-default value must be kept.
	val2 := map[string]interface{}{
		"timeoutSec": float64(30),
	}
	pruned2, _ := pruneDefaultLikeValue(val2, []string{}, true, rules)
	m2 := pruned2.(map[string]interface{})
	if m2["timeoutSec"] != float64(30) {
		t.Error("non-default timeoutSec should be kept")
	}

	// dropFalseBooleans trims false permissions.
	perm := map[string]interface{}{
		"canReadFiles": false,
		"canWrite":     true,
	}
	pruned3, _ := pruneDefaultLikeValue(perm, []string{}, true, nil)
	m3 := pruned3.(map[string]interface{})
	if _, found := m3["canReadFiles"]; found {
		t.Error("false boolean should be pruned when dropFalseBooleans=true")
	}
	if m3["canWrite"] != true {
		t.Error("true boolean should be preserved")
	}
}

// ─── TestExtractPortableEnvInputs ─────────────────────────────────────────────

func TestExtractPortableEnvInputs(t *testing.T) {
	adapterCfg := map[string]interface{}{
		"env": map[string]interface{}{
			"API_KEY":  map[string]interface{}{"type": "plain", "value": "abc123"},
			"GH_TOKEN": map[string]interface{}{"type": "secret_ref"},
			"EDITOR":   "vim",
			"RUNNER":   "/usr/bin/python",
		},
	}
	var warnings []string
	inputs := extractPortableEnvInputs("my-agent", adapterCfg, &warnings)
	byKey := map[string]PortabilityEnvInput{}
	for _, v := range inputs {
		byKey[v.Key] = v
	}

	// API_KEY has sensitive name → kind=secret, default cleared.
	apiKey := byKey["API_KEY"]
	if apiKey.Kind != "secret" {
		t.Errorf("API_KEY: expected kind=secret, got %q", apiKey.Kind)
	}
	if apiKey.DefaultValue == nil || *apiKey.DefaultValue != "" {
		t.Error("API_KEY: sensitive key should have empty defaultValue")
	}

	// GH_TOKEN is a secret_ref → kind=secret, description "Provide..."
	ghToken := byKey["GH_TOKEN"]
	if ghToken.Kind != "secret" {
		t.Errorf("GH_TOKEN: expected kind=secret, got %q", ghToken.Kind)
	}
	if ghToken.Description == nil || len(*ghToken.Description) == 0 {
		t.Error("GH_TOKEN: expected a description")
	}

	// EDITOR is a plain string, not sensitive.
	editor := byKey["EDITOR"]
	if editor.Kind != "plain" {
		t.Errorf("EDITOR: expected kind=plain, got %q", editor.Kind)
	}
	if editor.DefaultValue == nil || *editor.DefaultValue != "vim" {
		t.Error("EDITOR: expected defaultValue=vim")
	}
	if editor.Portability != "portable" {
		t.Errorf("EDITOR: expected portability=portable, got %q", editor.Portability)
	}

	// RUNNER is an absolute path → system_dependent warning emitted.
	runner := byKey["RUNNER"]
	if runner.Portability != "system_dependent" {
		t.Errorf("RUNNER: expected system_dependent, got %q", runner.Portability)
	}
	if len(warnings) == 0 {
		t.Error("expected at least one warning for absolute path env value")
	}
}

// ─── TestDedupeEnvInputs ──────────────────────────────────────────────────────

func TestDedupeEnvInputs(t *testing.T) {
	slug1 := "agent-a"
	slug2 := "agent-b"
	dv := ""
	inputs := []PortabilityEnvInput{
		{Key: "TOKEN", AgentSlug: &slug1, Kind: "secret", DefaultValue: &dv, Portability: "portable"},
		{Key: "token", AgentSlug: &slug1, Kind: "secret", DefaultValue: &dv, Portability: "portable"}, // duplicate (case-insensitive key)
		{Key: "TOKEN", AgentSlug: &slug2, Kind: "secret", DefaultValue: &dv, Portability: "portable"}, // different agent → not a dup
	}
	deduped := dedupeEnvInputs(inputs)
	if len(deduped) != 2 {
		t.Errorf("expected 2 unique entries, got %d", len(deduped))
	}
}

// ─── TestDisableImportedTimerHeartbeat ────────────────────────────────────────

func TestDisableImportedTimerHeartbeat(t *testing.T) {
	// nil input → returns map with heartbeat.enabled=false
	result := disableImportedTimerHeartbeat(nil)
	hb, ok := result["heartbeat"].(map[string]interface{})
	if !ok {
		t.Fatal("expected heartbeat map")
	}
	if hb["enabled"] != false {
		t.Error("expected heartbeat.enabled=false")
	}

	// Existing true heartbeat → overridden to false.
	existing := map[string]interface{}{
		"heartbeat": map[string]interface{}{
			"enabled":     true,
			"intervalSec": float64(3600),
		},
	}
	result2 := disableImportedTimerHeartbeat(existing)
	hb2 := result2["heartbeat"].(map[string]interface{})
	if hb2["enabled"] != false {
		t.Error("expected heartbeat.enabled to be overridden to false")
	}
	if hb2["intervalSec"] != float64(3600) {
		t.Error("other heartbeat fields should be preserved")
	}

	// Original map must not be mutated.
	origHb := existing["heartbeat"].(map[string]interface{})
	if origHb["enabled"] != true {
		t.Error("original config must not be mutated")
	}
}
