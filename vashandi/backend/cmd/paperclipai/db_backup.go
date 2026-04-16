package main

import (
	"encoding/json"
	"fmt"
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

		command := exec.Command("pg_dump", "--format=plain", "--no-owner", "--no-privileges", "--file", backupPath, dsn)
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

func init() {
	dbBackupCmd.Flags().String("dir", "", "Backup output directory (overrides config)")
	dbBackupCmd.Flags().Int("retention-days", 0, "Retention window used for pruning")
	dbBackupCmd.Flags().String("filename-prefix", "paperclip", "Backup filename prefix")
	dbBackupCmd.Flags().Bool("json", false, "Print backup metadata as JSON")
	rootCmd.AddCommand(dbBackupCmd)
}
