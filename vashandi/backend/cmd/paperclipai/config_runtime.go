package main

import (
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/lib/pq"
	"gopkg.in/yaml.v3"

	"github.com/chifamba/vashandi/vashandi/backend/shared"
)

type loadedPaperclipConfig struct {
	Path   string
	Config *shared.PaperclipConfig
}

func expandHomePrefix(value string) string {
	if value == "~" {
		home, _ := os.UserHomeDir()
		return home
	}
	if strings.HasPrefix(value, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, value[2:])
	}
	return value
}

func resolveCLIConfigPath() string {
	if raw := strings.TrimSpace(os.Getenv("PAPERCLIP_CONFIG")); raw != "" {
		abs, err := filepath.Abs(expandHomePrefix(raw))
		if err == nil {
			return abs
		}
		return expandHomePrefix(raw)
	}

	instanceRoot := shared.ResolvePaperclipInstanceRoot()
	for _, candidate := range []string{
		filepath.Join(instanceRoot, "config.json"),
		filepath.Join(instanceRoot, "config.yaml"),
		filepath.Join(instanceRoot, "config.yml"),
	} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	return filepath.Join(instanceRoot, "config.json")
}

func loadCLIConfig() (*loadedPaperclipConfig, error) {
	configPath := resolveCLIConfigPath()
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("could not read config file %s: %w", configPath, err)
	}

	cfg := &shared.PaperclipConfig{}
	switch strings.ToLower(filepath.Ext(configPath)) {
	case ".yaml", ".yml":
		var raw any
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("could not parse YAML config file %s: %w", configPath, err)
		}
		jsonBytes, err := json.Marshal(raw)
		if err != nil {
			return nil, fmt.Errorf("could not normalize YAML config file %s: %w", configPath, err)
		}
		if err := json.Unmarshal(jsonBytes, cfg); err != nil {
			return nil, fmt.Errorf("could not decode YAML config file %s: %w", configPath, err)
		}
	default:
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("could not parse JSON config file %s: %w", configPath, err)
		}
	}

	return &loadedPaperclipConfig{
		Path:   configPath,
		Config: cfg,
	}, nil
}

func resolveRuntimeLikePath(value string, configPath string) string {
	expanded := expandHomePrefix(strings.TrimSpace(value))
	if expanded == "" {
		return ""
	}
	if filepath.IsAbs(expanded) {
		return filepath.Clean(expanded)
	}
	return filepath.Join(filepath.Dir(configPath), expanded)
}

func resolvePostgresConnection(cfg *shared.PaperclipConfig) (string, string) {
	if envDSN := strings.TrimSpace(os.Getenv("DATABASE_URL")); envDSN != "" {
		return envDSN, "DATABASE_URL"
	}
	if cfg != nil && cfg.Database.Mode == "postgres" {
		if dsn := strings.TrimSpace(cfg.Database.ConnectionString); dsn != "" {
			return dsn, "config.database.connectionString"
		}
	}
	return "", ""
}

func resolveEmbeddedPostgresConnection(cfg *shared.PaperclipConfig) (string, string) {
	if cfg == nil || cfg.Database.Mode != "embedded-postgres" {
		return "", ""
	}
	port := cfg.Database.EmbeddedPostgresPort
	if port == 0 {
		port = 54329
	}
	return fmt.Sprintf("postgres://paperclip:paperclip@127.0.0.1:%d/paperclip?sslmode=disable", port), fmt.Sprintf("embedded-postgres@%d", port)
}

func validateSecretMaterial(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false
	}
	if len(trimmed) == 64 {
		if decoded, err := hex.DecodeString(trimmed); err == nil && len(decoded) == 32 {
			return true
		}
	}
	if decoded, err := base64.StdEncoding.DecodeString(trimmed); err == nil && len(decoded) == 32 {
		return true
	}
	return len(trimmed) == 32
}

func openPostgresConnection(dsn string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	// Callers must verify connectivity with Ping/PingContext because sql.Open
	// validates driver registration and DSN parsing, not the live connection.
	return db, nil
}

func defaultBackupDir() string {
	return filepath.Join(shared.ResolvePaperclipInstanceRoot(), "data", "backups")
}
