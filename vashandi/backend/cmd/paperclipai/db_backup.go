package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var dbBackupCmd = &cobra.Command{
	Use:   "db:backup",
	Short: "Create a one-off database backup using current config",
	Run: func(cmd *cobra.Command, args []string) {
		loaded, err := loadCLIConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		cfg := loaded.Config
		outputJSON, _ := cmd.Flags().GetBool("json")
		outputDir, _ := cmd.Flags().GetString("dir")
		retentionDays, _ := cmd.Flags().GetInt("retention-days")
		filenamePrefix, _ := cmd.Flags().GetString("filename-prefix")

		if cfg.Database.Mode != "postgres" {
			msg := fmt.Sprintf("Skipping backup for %s mode; pg_dump is only used for external postgres", cfg.Database.Mode)
			if outputJSON {
				_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
					"skipped": true,
					"reason":  msg,
				})
				return
			}
			fmt.Println(msg)
			return
		}

		dsn, source := resolvePostgresConnection(cfg)
		if dsn == "" {
			fmt.Fprintln(os.Stderr, "no postgres connection string found in DATABASE_URL or config.database.connectionString")
			os.Exit(1)
		}

		backupDir := strings.TrimSpace(outputDir)
		if backupDir == "" {
			backupDir = strings.TrimSpace(cfg.Database.Backup.Dir)
		}
		if backupDir == "" {
			backupDir = defaultBackupDir()
		}
		backupDir = resolveRuntimeLikePath(backupDir, loaded.Path)
		if err := os.MkdirAll(backupDir, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "failed to create backup directory %s: %v\n", backupDir, err)
			os.Exit(1)
		}

		if strings.TrimSpace(filenamePrefix) == "" {
			filenamePrefix = "paperclip"
		}
		filename := fmt.Sprintf("%s-%s.sql", filenamePrefix, time.Now().Format("20060102-150405"))
		backupPath := filepath.Join(backupDir, filename)

		pgEnv, err := postgresEnvFromDSN(dsn)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to derive secure pg_dump environment: %v\n", err)
			os.Exit(1)
		}

		command := exec.Command("pg_dump", "--format=plain", "--no-owner", "--no-privileges", "--file", backupPath)
		command.Env = append(os.Environ(), pgEnv...)
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
		if err := command.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "pg_dump failed: %v\n", err)
			os.Exit(1)
		}

		if retentionDays <= 0 {
			retentionDays = cfg.Database.Backup.RetentionDays
		}
		prunedCount, err := pruneOldBackups(backupDir, filenamePrefix, retentionDays)
		if err != nil {
			fmt.Fprintf(os.Stderr, "backup created but pruning failed: %v\n", err)
			os.Exit(1)
		}

		if outputJSON {
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
				"backupFile":       backupPath,
				"backupDir":        backupDir,
				"connectionSource": source,
				"prunedCount":      prunedCount,
				"retentionDays":    retentionDays,
			})
			return
		}

		fmt.Printf("Backup created: %s\n", backupPath)
		fmt.Printf("Connection source: %s\n", source)
		if retentionDays > 0 {
			fmt.Printf("Pruned %d backup(s) older than %d day(s)\n", prunedCount, retentionDays)
		}
	},
}

func pruneOldBackups(dir, prefix string, retentionDays int) (int, error) {
	if retentionDays <= 0 {
		return 0, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}

	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	pruned := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, prefix+"-") || !strings.HasSuffix(name, ".sql") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return pruned, err
		}
		if info.ModTime().After(cutoff) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			return pruned, err
		}
		pruned++
	}
	return pruned, nil
}

func postgresEnvFromDSN(dsn string) ([]string, error) {
	trimmed := strings.TrimSpace(dsn)
	if trimmed == "" {
		return nil, fmt.Errorf("empty postgres DSN")
	}

	if strings.HasPrefix(trimmed, "postgres://") || strings.HasPrefix(trimmed, "postgresql://") {
		parsed, err := url.Parse(trimmed)
		if err != nil {
			return nil, err
		}
		database := strings.TrimPrefix(parsed.Path, "/")
		if database == "" {
			return nil, fmt.Errorf("postgres DSN is missing a database name")
		}

		env := []string{
			"PGDATABASE=" + database,
		}
		if host := parsed.Hostname(); host != "" {
			env = append(env, "PGHOST="+host)
		}
		if port := parsed.Port(); port != "" {
			env = append(env, "PGPORT="+port)
		}
		if user := parsed.User.Username(); user != "" {
			env = append(env, "PGUSER="+user)
		}
		if password, ok := parsed.User.Password(); ok && password != "" {
			env = append(env, "PGPASSWORD="+password)
		}

		query := parsed.Query()
		supported := map[string]string{
			"application_name":     "PGAPPNAME",
			"connect_timeout":      "PGCONNECT_TIMEOUT",
			"sslcert":              "PGSSLCERT",
			"sslkey":               "PGSSLKEY",
			"sslmode":              "PGSSLMODE",
			"sslrootcert":          "PGSSLROOTCERT",
			"target_session_attrs": "PGTARGETSESSIONATTRS",
		}
		for key, envKey := range supported {
			if value := strings.TrimSpace(query.Get(key)); value != "" {
				env = append(env, envKey+"="+value)
			}
			query.Del(key)
		}
		if len(query) > 0 {
			unsupported := make([]string, 0, len(query))
			for key := range query {
				unsupported = append(unsupported, key)
			}
			return nil, fmt.Errorf("unsupported postgres DSN query parameters: %s", strings.Join(unsupported, ", "))
		}

		return env, nil
	}

	env := []string{}
	for _, part := range strings.Fields(trimmed) {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			return nil, fmt.Errorf("unsupported postgres DSN segment %q", part)
		}
		value = strings.Trim(value, `'`)
		switch key {
		case "dbname":
			env = append(env, "PGDATABASE="+value)
		case "host":
			env = append(env, "PGHOST="+value)
		case "password":
			env = append(env, "PGPASSWORD="+value)
		case "port":
			env = append(env, "PGPORT="+value)
		case "sslmode":
			env = append(env, "PGSSLMODE="+value)
		case "user":
			env = append(env, "PGUSER="+value)
		default:
			return nil, fmt.Errorf("unsupported postgres DSN key %q", key)
		}
	}
	for _, entry := range env {
		if strings.HasPrefix(entry, "PGDATABASE=") {
			return env, nil
		}
	}
	return nil, fmt.Errorf("postgres DSN is missing dbname")
}

func init() {
	dbBackupCmd.Flags().String("dir", "", "Backup output directory (overrides config)")
	dbBackupCmd.Flags().Int("retention-days", 0, "Retention window used for pruning")
	dbBackupCmd.Flags().String("filename-prefix", "paperclip", "Backup filename prefix")
	dbBackupCmd.Flags().Bool("json", false, "Print backup metadata as JSON")
	rootCmd.AddCommand(dbBackupCmd)
}
