package services

import (
	"context"
	"strings"
	"testing"

	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
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

func setupPortabilityServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&portability_service_test=1"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Company{},
		&models.Agent{},
		&models.Project{},
		&models.Issue{},
		&models.CompanySkill{},
	); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	return db
}

func TestExportBundle_GeneratesReadmeOrgChartAndSkillMetadata(t *testing.T) {
	db := setupPortabilityServiceTestDB(t)
	company := models.Company{ID: "company-1", Name: "Acme Labs", Status: "active"}
	if err := db.Create(&company).Error; err != nil {
		t.Fatalf("create company: %v", err)
	}
	if err := db.Create(&models.Agent{
		ID:            "agent-1",
		CompanyID:     company.ID,
		Name:          "Chief Exec",
		Role:          "ceo",
		Status:        "idle",
		AdapterType:   "process",
		AdapterConfig: datatypes.JSON(`{"skills":["octo/demo/research"]}`),
		RuntimeConfig: datatypes.JSON(`{}`),
		Permissions:   datatypes.JSON(`{}`),
	}).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	description := "Research skill"
	if err := db.Create(&models.CompanySkill{
		ID:            "skill-1",
		CompanyID:     company.ID,
		Key:           "octo/demo/research",
		Slug:          "research",
		Name:          "Research",
		Description:   &description,
		Markdown:      "Use trusted sources.",
		SourceType:    "github",
		SourceLocator: portStrPtr("https://github.com/octo/demo/tree/main/skills/research"),
		SourceRef:     portStrPtr("abc123"),
		Metadata:      datatypes.JSON(`{"sourceKind":"github","owner":"octo","repo":"demo","repoSkillDir":"skills/research","trackingRef":"main"}`),
	}).Error; err != nil {
		t.Fatalf("create skill: %v", err)
	}

	svc := NewPortabilityService(db)
	result, err := svc.ExportBundle(context.Background(), company.ID, ExportRequest{
		Include: map[string]bool{
			"company": true,
			"agents":  true,
			"skills":  true,
		},
	})
	if err != nil {
		t.Fatalf("export bundle: %v", err)
	}

	readme, ok := result.Files["README.md"].(string)
	if !ok {
		t.Fatalf("expected README.md text file, got %T", result.Files["README.md"])
	}
	if !strings.Contains(readme, "![Org Chart](images/org-chart.svg)") {
		t.Fatalf("expected README org chart image, got %q", readme)
	}
	if !strings.Contains(readme, "| Research | Research skill | [github](https://github.com/octo/demo/tree/main/skills/research) |") {
		t.Fatalf("expected README skill source link, got %q", readme)
	}

	orgChart, ok := result.Files["images/org-chart.svg"].(string)
	if !ok || !strings.Contains(orgChart, "<svg") {
		t.Fatalf("expected org chart svg export, got %T %q", result.Files["images/org-chart.svg"], orgChart)
	}
	if len(result.Manifest.Skills) != 1 {
		t.Fatalf("expected one skill in manifest, got %d", len(result.Manifest.Skills))
	}
	skill := result.Manifest.Skills[0]
	if skill.SourceType != "github" {
		t.Fatalf("expected github source type, got %q", skill.SourceType)
	}
	if skill.SourceLocator == nil || *skill.SourceLocator != "https://github.com/octo/demo/tree/main/skills/research" {
		t.Fatalf("unexpected source locator: %#v", skill.SourceLocator)
	}
	if len(skill.FileInventory) != 1 || skill.FileInventory[0].Kind != "skill" {
		t.Fatalf("unexpected skill file inventory: %#v", skill.FileInventory)
	}
}

func TestPreviewImport_DetectsDetailedCollisions(t *testing.T) {
	db := setupPortabilityServiceTestDB(t)
	company := models.Company{ID: "company-target", Name: "Target", Status: "active"}
	if err := db.Create(&company).Error; err != nil {
		t.Fatalf("create company: %v", err)
	}
	if err := db.Create(&models.Agent{
		ID:            "existing-agent",
		CompanyID:     company.ID,
		Name:          "Alpha Agent",
		Role:          "ceo",
		Status:        "idle",
		AdapterType:   "process",
		AdapterConfig: datatypes.JSON(`{}`),
		RuntimeConfig: datatypes.JSON(`{}`),
		Permissions:   datatypes.JSON(`{}`),
	}).Error; err != nil {
		t.Fatalf("create existing agent: %v", err)
	}
	if err := db.Create(&models.Project{
		ID:        "existing-project",
		CompanyID: company.ID,
		Name:      "Launch Plan",
		Status:    "backlog",
	}).Error; err != nil {
		t.Fatalf("create existing project: %v", err)
	}
	identifier := "TASK-1"
	if err := db.Create(&models.Issue{
		ID:         "existing-issue",
		CompanyID:  company.ID,
		Title:      "Investigate",
		Status:     "backlog",
		Priority:   "medium",
		OriginKind: "manual",
		Identifier: &identifier,
	}).Error; err != nil {
		t.Fatalf("create existing issue: %v", err)
	}
	if err := db.Create(&models.CompanySkill{
		ID:        "existing-skill",
		CompanyID: company.ID,
		Key:       "octo/demo/research",
		Slug:      "research",
		Name:      "Research",
		Markdown:  "Existing skill",
	}).Error; err != nil {
		t.Fatalf("create existing skill: %v", err)
	}

	svc := NewPortabilityService(db)
	preview, err := svc.PreviewImport(context.Background(), ImportRequest{
		Source: ImportSource{
			Type: "inline",
			Files: map[string]interface{}{
				"COMPANY.md":                "---\nname: Imported\n---\n",
				"agents/alpha-agent/AGENTS.md": "---\nname: Alpha Agent\n---\nYou are alpha.\n",
				"projects/launch-plan/PROJECT.md": "---\nname: Launch Plan\n---\n",
				"tasks/investigate/TASK.md": "---\nname: Investigate\nidentifier: TASK-1\n---\n",
				"skills/research/SKILL.md": "---\nname: Research\nslug: research\nkey: octo/demo/research\ndescription: Research skill\nmetadata:\n  sourceKind: github\n  owner: octo\n  repo: demo\n  repoSkillDir: skills/research\n  sources:\n    - kind: github-dir\n      repo: octo/demo\n      path: skills/research\n      commit: abc123\n      url: https://github.com/octo/demo/tree/main/skills/research\n---\nUse trusted sources.\n",
			},
		},
		Include: map[string]bool{
			"company":  true,
			"agents":   true,
			"projects": true,
			"issues":   true,
			"skills":   true,
		},
		Target:            ImportTarget{Mode: "existing_company", CompanyID: company.ID},
		CollisionStrategy: "replace",
	}, ImportModeBoardFull)
	if err != nil {
		t.Fatalf("preview import: %v", err)
	}

	if len(preview.Collisions) < 4 {
		t.Fatalf("expected detailed collisions, got %#v", preview.Collisions)
	}
	collisionByType := map[string]ImportCollision{}
	for _, collision := range preview.Collisions {
		collisionByType[collision.EntityType] = collision
	}
	if collisionByType["agent"].PlannedAction != "update" || collisionByType["agent"].RecommendedCollisionStrategy != "replace" {
		t.Fatalf("unexpected agent collision: %#v", collisionByType["agent"])
	}
	if collisionByType["project"].PlannedAction != "update" || collisionByType["project"].RecommendedCollisionStrategy != "replace" {
		t.Fatalf("unexpected project collision: %#v", collisionByType["project"])
	}
	if collisionByType["skill"].PlannedAction != "update" || collisionByType["skill"].RecommendedCollisionStrategy != "replace" {
		t.Fatalf("unexpected skill collision: %#v", collisionByType["skill"])
	}
	if collisionByType["issue"].PlannedAction != "create" || collisionByType["issue"].RecommendedCollisionStrategy != "rename" {
		t.Fatalf("unexpected issue collision: %#v", collisionByType["issue"])
	}
	if len(preview.Plan.IssuePlans) != 1 || preview.Plan.IssuePlans[0].PlannedTitle == "Investigate" {
		t.Fatalf("expected renamed issue title in plan, got %#v", preview.Plan.IssuePlans)
	}
	if len(preview.Errors) != 0 {
		t.Fatalf("expected no preview errors, got %#v", preview.Errors)
	}
}
