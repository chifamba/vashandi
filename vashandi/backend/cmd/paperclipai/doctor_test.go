package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/shared"
)

func TestLoadCLIConfigSupportsYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	configBody := `
$meta:
  version: 1
  updatedAt: 2026-04-16T19:13:40Z
  source: doctor
llm:
  provider: claude
  apiKey: test-key
database:
  mode: postgres
  connectionString: postgres://localhost/paperclip
  embeddedPostgresDataDir: db
  embeddedPostgresPort: 54329
  backup:
    enabled: true
    intervalMinutes: 60
    retentionDays: 30
    dir: backups
logging:
  mode: file
  logDir: logs
server:
  deploymentMode: authenticated
  exposure: private
  host: 127.0.0.1
  port: 3100
  serveUi: true
auth:
  baseUrlMode: auto
  disableSignUp: false
  requireEmailVerification: false
storage:
  provider: local_disk
  localDisk:
    baseDir: storage
  s3:
    bucket: ""
    region: ""
secrets:
  provider: local_encrypted
  strictMode: true
  localEncrypted:
    keyFilePath: secrets/master.key
telemetry:
  enabled: false
`
	if err := os.WriteFile(configPath, []byte(configBody), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("PAPERCLIP_CONFIG", configPath)
	loaded, err := loadCLIConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loaded.Path != configPath {
		t.Fatalf("expected config path %s, got %s", configPath, loaded.Path)
	}
	if loaded.Config.Server.DeploymentMode != "authenticated" {
		t.Fatalf("expected authenticated deployment mode, got %s", loaded.Config.Server.DeploymentMode)
	}
}

func TestCheckAgentJWTRequiresSecretInAuthenticatedMode(t *testing.T) {
	t.Setenv("BETTER_AUTH_SECRET", "")
	t.Setenv("PAPERCLIP_AGENT_JWT_SECRET", "")

	result := checkAgentJWT(&shared.PaperclipConfig{
		Server: shared.ServerConfig{DeploymentMode: "authenticated"},
	})
	if result.Status != doctorFail {
		t.Fatalf("expected fail, got %s", result.Status)
	}
}

func TestPruneOldBackupsRemovesExpiredFiles(t *testing.T) {
	tmpDir := t.TempDir()
	oldFile := filepath.Join(tmpDir, "paperclip-old.sql")
	newFile := filepath.Join(tmpDir, "paperclip-new.sql")
	if err := os.WriteFile(oldFile, []byte("old"), 0o600); err != nil {
		t.Fatalf("write old file: %v", err)
	}
	if err := os.WriteFile(newFile, []byte("new"), 0o600); err != nil {
		t.Fatalf("write new file: %v", err)
	}

	oldTime := time.Now().AddDate(0, 0, -10)
	if err := os.Chtimes(oldFile, oldTime, oldTime); err != nil {
		t.Fatalf("touch old file: %v", err)
	}

	pruned, err := pruneOldBackups(tmpDir, "paperclip", 3)
	if err != nil {
		t.Fatalf("prune backups: %v", err)
	}
	if pruned != 1 {
		t.Fatalf("expected 1 file pruned, got %d", pruned)
	}
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Fatalf("expected old backup to be removed")
	}
	if _, err := os.Stat(newFile); err != nil {
		t.Fatalf("expected new backup to remain: %v", err)
	}
}
