package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/chifamba/vashandi/vashandi/backend/shared"
)

type doctorStatus string

const (
	doctorPass doctorStatus = "pass"
	doctorWarn doctorStatus = "warn"
	doctorFail doctorStatus = "fail"
)

type doctorCheckResult struct {
	Name    string
	Status  doctorStatus
	Details string
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check your system for dependencies and potential issues",
	Long:  `Runs a suite of diagnostic checks on your environment, configuration, and dependencies.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(" paperclip doctor ")
		fmt.Println("--------------------")

		results := runDoctorChecks()
		for _, result := range results {
			fmt.Printf("[%s] %s: %s\n", strings.ToUpper(string(result.Status)), result.Name, result.Details)
		}

		passed, warned, failed := summarizeDoctorChecks(results)
		fmt.Println("--------------------")
		fmt.Printf("Summary: %d passed, %d warnings, %d failed\n", passed, warned, failed)
		if failed > 0 {
			os.Exit(1)
		}
	},
}

func runDoctorChecks() []doctorCheckResult {
	var results []doctorCheckResult

	loaded, err := loadCLIConfig()
	if err != nil {
		results = append(results, doctorCheckResult{
			Name:    "Config file",
			Status:  doctorFail,
			Details: err.Error(),
		})
		return results
	}

	cfg := loaded.Config
	results = append(results, doctorCheckResult{
		Name:    "Config file",
		Status:  doctorPass,
		Details: fmt.Sprintf("Loaded valid config from %s", loaded.Path),
	})
	results = append(results, checkDeploymentMode(cfg))
	results = append(results, checkAgentJWT(cfg))
	results = append(results, checkSecretsAdapter(cfg, loaded.Path))
	results = append(results, checkStorage(cfg, loaded.Path))
	results = append(results, checkDatabase(cfg, loaded.Path))
	results = append(results, checkLLM(cfg))
	results = append(results, checkLogDirectory(cfg, loaded.Path))
	results = append(results, checkPortAvailability(cfg))

	return results
}

func summarizeDoctorChecks(results []doctorCheckResult) (int, int, int) {
	passed, warned, failed := 0, 0, 0
	for _, result := range results {
		switch result.Status {
		case doctorPass:
			passed++
		case doctorWarn:
			warned++
		case doctorFail:
			failed++
		}
	}
	return passed, warned, failed
}

func checkDeploymentMode(cfg *shared.PaperclipConfig) doctorCheckResult {
	switch strings.TrimSpace(cfg.Server.DeploymentMode) {
	case "local_trusted", "authenticated":
		return doctorCheckResult{
			Name:    "Deployment/auth mode",
			Status:  doctorPass,
			Details: fmt.Sprintf("Configured for %s", cfg.Server.DeploymentMode),
		}
	default:
		return doctorCheckResult{
			Name:    "Deployment/auth mode",
			Status:  doctorFail,
			Details: fmt.Sprintf("Unsupported deployment mode %q", cfg.Server.DeploymentMode),
		}
	}
}

func checkAgentJWT(cfg *shared.PaperclipConfig) doctorCheckResult {
	if cfg.Server.DeploymentMode != "authenticated" {
		return doctorCheckResult{
			Name:    "Agent JWT",
			Status:  doctorPass,
			Details: "Not required outside authenticated mode",
		}
	}

	secret := strings.TrimSpace(os.Getenv("BETTER_AUTH_SECRET"))
	if secret == "" {
		secret = strings.TrimSpace(os.Getenv("PAPERCLIP_AGENT_JWT_SECRET"))
	}
	if secret != "" {
		if len(secret) < 32 {
			return doctorCheckResult{
				Name:    "Agent JWT",
				Status:  doctorFail,
				Details: "Authentication secret must be at least 32 characters long",
			}
		}
		return doctorCheckResult{
			Name:    "Agent JWT",
			Status:  doctorPass,
			Details: "Authentication secret is configured in the environment",
		}
	}

	return doctorCheckResult{
		Name:    "Agent JWT",
		Status:  doctorFail,
		Details: "Set BETTER_AUTH_SECRET or PAPERCLIP_AGENT_JWT_SECRET for authenticated mode",
	}
}

func checkSecretsAdapter(cfg *shared.PaperclipConfig, configPath string) doctorCheckResult {
	if strings.TrimSpace(cfg.Secrets.Provider) != "local_encrypted" {
		return doctorCheckResult{
			Name:    "Secrets adapter",
			Status:  doctorFail,
			Details: fmt.Sprintf("Unsupported secrets provider %q", cfg.Secrets.Provider),
		}
	}

	if envKey := strings.TrimSpace(os.Getenv("PAPERCLIP_SECRETS_MASTER_KEY")); envKey != "" {
		if !validateSecretMaterial(envKey) {
			return doctorCheckResult{
				Name:    "Secrets adapter",
				Status:  doctorFail,
				Details: "PAPERCLIP_SECRETS_MASTER_KEY is not valid 32-byte key material",
			}
		}
		return doctorCheckResult{
			Name:    "Secrets adapter",
			Status:  doctorPass,
			Details: "Using PAPERCLIP_SECRETS_MASTER_KEY from the environment",
		}
	}

	keyPath := strings.TrimSpace(os.Getenv("PAPERCLIP_SECRETS_MASTER_KEY_FILE"))
	if keyPath == "" {
		keyPath = cfg.Secrets.LocalEncrypted.KeyFilePath
	}
	keyPath = resolveRuntimeLikePath(keyPath, configPath)
	if keyPath == "" {
		return doctorCheckResult{
			Name:    "Secrets adapter",
			Status:  doctorFail,
			Details: "No secrets key file path is configured",
		}
	}

	data, err := os.ReadFile(keyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return doctorCheckResult{
				Name:    "Secrets adapter",
				Status:  doctorWarn,
				Details: fmt.Sprintf("Secrets key file does not exist yet: %s", keyPath),
			}
		}
		return doctorCheckResult{
			Name:    "Secrets adapter",
			Status:  doctorFail,
			Details: fmt.Sprintf("Could not read secrets key file %s: %v", keyPath, err),
		}
	}
	if !validateSecretMaterial(string(data)) {
		return doctorCheckResult{
			Name:    "Secrets adapter",
			Status:  doctorFail,
			Details: fmt.Sprintf("Secrets key file %s does not contain valid 32-byte key material", keyPath),
		}
	}

	return doctorCheckResult{
		Name:    "Secrets adapter",
		Status:  doctorPass,
		Details: fmt.Sprintf("Using local encrypted secrets key file %s", keyPath),
	}
}

func checkStorage(cfg *shared.PaperclipConfig, configPath string) doctorCheckResult {
	switch strings.TrimSpace(cfg.Storage.Provider) {
	case "local_disk":
		baseDir := resolveRuntimeLikePath(cfg.Storage.LocalDisk.BaseDir, configPath)
		if baseDir == "" {
			return doctorCheckResult{
				Name:    "Storage",
				Status:  doctorFail,
				Details: "storage.localDisk.baseDir is not configured",
			}
		}
		if err := ensureWritableDir(baseDir); err != nil {
			return doctorCheckResult{
				Name:    "Storage",
				Status:  doctorFail,
				Details: fmt.Sprintf("Local storage directory is not writable: %s (%v)", baseDir, err),
			}
		}
		return doctorCheckResult{
			Name:    "Storage",
			Status:  doctorPass,
			Details: fmt.Sprintf("Local disk storage is writable: %s", baseDir),
		}
	case "s3":
		if strings.TrimSpace(cfg.Storage.S3.Bucket) == "" || strings.TrimSpace(cfg.Storage.S3.Region) == "" {
			return doctorCheckResult{
				Name:    "Storage",
				Status:  doctorFail,
				Details: "S3 storage requires non-empty bucket and region",
			}
		}
		return doctorCheckResult{
			Name:    "Storage",
			Status:  doctorWarn,
			Details: fmt.Sprintf("S3 storage configured for bucket %s in %s; reachability not checked", cfg.Storage.S3.Bucket, cfg.Storage.S3.Region),
		}
	default:
		return doctorCheckResult{
			Name:    "Storage",
			Status:  doctorFail,
			Details: fmt.Sprintf("Unsupported storage provider %q", cfg.Storage.Provider),
		}
	}
}

func checkDatabase(cfg *shared.PaperclipConfig, configPath string) doctorCheckResult {
	dsn, source := resolvePostgresConnection(cfg)
	if dsn == "" {
		dsn, source = resolveEmbeddedPostgresConnection(cfg)
	}
	if dsn == "" {
		return doctorCheckResult{
			Name:    "Database",
			Status:  doctorFail,
			Details: "No database connection could be derived from config or DATABASE_URL",
		}
	}

	db, err := openPostgresConnection(dsn)
	if err != nil {
		return doctorCheckResult{
			Name:    "Database",
			Status:  doctorFail,
			Details: fmt.Sprintf("Could not create database client from %s: %v", source, err),
		}
	}
	defer db.Close()

	if cfg.Database.Mode == "embedded-postgres" {
		dataDir := resolveRuntimeLikePath(cfg.Database.EmbeddedPostgresDataDir, configPath)
		if dataDir != "" {
			if err := os.MkdirAll(dataDir, 0o755); err != nil {
				return doctorCheckResult{
					Name:    "Database",
					Status:  doctorFail,
					Details: fmt.Sprintf("Embedded postgres data directory is not ready: %s (%v)", dataDir, err),
				}
			}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return doctorCheckResult{
			Name:    "Database",
			Status:  doctorFail,
			Details: fmt.Sprintf("Database ping failed via %s: %v", source, err),
		}
	}

	return doctorCheckResult{
		Name:    "Database",
		Status:  doctorPass,
		Details: fmt.Sprintf("Database connection succeeded via %s", source),
	}
}

func checkLLM(cfg *shared.PaperclipConfig) doctorCheckResult {
	if cfg.Llm == nil || strings.TrimSpace(cfg.Llm.Provider) == "" {
		return doctorCheckResult{
			Name:    "LLM",
			Status:  doctorFail,
			Details: "No LLM adapter is configured",
		}
	}

	switch strings.TrimSpace(cfg.Llm.Provider) {
	case "claude", "openai":
		if strings.TrimSpace(cfg.Llm.ApiKey) == "" {
			return doctorCheckResult{
				Name:    "LLM",
				Status:  doctorWarn,
				Details: fmt.Sprintf("%s adapter is configured without an API key", cfg.Llm.Provider),
			}
		}
		return doctorCheckResult{
			Name:    "LLM",
			Status:  doctorPass,
			Details: fmt.Sprintf("%s adapter is configured", cfg.Llm.Provider),
		}
	default:
		return doctorCheckResult{
			Name:    "LLM",
			Status:  doctorFail,
			Details: fmt.Sprintf("Unsupported LLM provider %q", cfg.Llm.Provider),
		}
	}
}

func checkLogDirectory(cfg *shared.PaperclipConfig, configPath string) doctorCheckResult {
	logDir := resolveRuntimeLikePath(cfg.Logging.LogDir, configPath)
	if logDir == "" {
		return doctorCheckResult{
			Name:    "Log directory",
			Status:  doctorFail,
			Details: "logging.logDir is not configured",
		}
	}
	if err := ensureWritableDir(logDir); err != nil {
		return doctorCheckResult{
			Name:    "Log directory",
			Status:  doctorFail,
			Details: fmt.Sprintf("Log directory is not writable: %s (%v)", logDir, err),
		}
	}
	return doctorCheckResult{
		Name:    "Log directory",
		Status:  doctorPass,
		Details: fmt.Sprintf("Log directory is writable: %s", logDir),
	}
}

func checkPortAvailability(cfg *shared.PaperclipConfig) doctorCheckResult {
	addr := fmt.Sprintf("127.0.0.1:%d", cfg.Server.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return doctorCheckResult{
			Name:    "Port",
			Status:  doctorWarn,
			Details: fmt.Sprintf("Port %d is not available: %v", cfg.Server.Port, err),
		}
	}
	if err := listener.Close(); err != nil {
		return doctorCheckResult{
			Name:    "Port",
			Status:  doctorWarn,
			Details: fmt.Sprintf("Port %d accepted a bind but could not be released cleanly: %v", cfg.Server.Port, err),
		}
	}

	return doctorCheckResult{
		Name:    "Port",
		Status:  doctorPass,
		Details: fmt.Sprintf("Port %d is available", cfg.Server.Port),
	}
}

func ensureWritableDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tempFile, err := os.CreateTemp(dir, ".paperclip-doctor-*")
	if err != nil {
		return err
	}
	name := tempFile.Name()
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	return os.Remove(name)
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
