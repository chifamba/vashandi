package services

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"gorm.io/gorm"
)

type DatabaseBackupService struct {
	db          *gorm.DB
	settings    *InstanceSettingsService
	backupDir   string
	intervalMin int
	enabled     bool
}

func NewDatabaseBackupService(db *gorm.DB, settings *InstanceSettingsService, backupDir string, intervalMin int, enabled bool) *DatabaseBackupService {
	return &DatabaseBackupService{
		db:          db,
		settings:    settings,
		backupDir:   backupDir,
		intervalMin: intervalMin,
		enabled:     enabled,
	}
}

func (s *DatabaseBackupService) Start(ctx context.Context) {
	if !s.enabled {
		slog.Info("[backup-service] disabled")
		return
	}
	slog.Info("[backup-service] starting", "intervalMinutes", s.intervalMin, "backupDir", s.backupDir)

	ticker := time.NewTicker(time.Duration(s.intervalMin) * time.Minute)
	defer ticker.Stop()

	// Initial run
	go s.runMaintenance(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runMaintenance(ctx)
		}
	}
}

func (s *DatabaseBackupService) runMaintenance(ctx context.Context) {
	slog.Info("[backup-service] running maintenance")

	general, err := s.settings.GetGeneral(ctx)
	if err != nil {
		slog.Error("[backup-service] failed to load settings", "error", err)
		return
	}

	// 1. Run File Backup
	if err := s.runFileBackup(ctx, general.BackupRetention); err != nil {
		slog.Error("[backup-service] file backup failed", "error", err)
	}

	// 2. Prune Plugin Logs
	if err := s.prunePluginLogs(ctx, general.BackupRetention.DailyDays); err != nil {
		slog.Error("[backup-service] log pruning failed", "error", err)
	}
}

func (s *DatabaseBackupService) runFileBackup(ctx context.Context, retention BackupRetention) error {
	if err := os.MkdirAll(s.backupDir, 0755); err != nil {
		return fmt.Errorf("failed to create backup dir: %w", err)
	}

	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("paperclip-%s.sql.gz", timestamp)
	fullPath := filepath.Join(s.backupDir, filename)

	f, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("failed to create backup file: %w", err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	if err := s.writeDump(ctx, gw); err != nil {
		_ = os.Remove(fullPath)
		return err
	}

	slog.Info("[backup-service] backup created", "file", filename)

	// Prune old backups
	s.pruneOldBackups(retention)

	return nil
}

func (s *DatabaseBackupService) writeDump(ctx context.Context, w io.Writer) error {
	fmt.Fprintln(w, "-- Paperclip database backup")
	fmt.Fprintf(w, "-- Created: %s\n\n", time.Now().Format(time.RFC3339))
	fmt.Fprintln(w, "BEGIN;")
	fmt.Fprintln(w, "SET LOCAL session_replication_role = replica;")
	fmt.Fprintln(w, "SET LOCAL client_min_messages = warning;")
	fmt.Fprintln(w, "")

	tables, err := s.db.Migrator().GetTables()
	if err != nil {
		return err
	}

	for _, table := range tables {
		// Skip internal postgres tables or drizzle ones if needed
		if strings.HasPrefix(table, "pg_") || strings.HasPrefix(table, "_drizzle") {
			continue
		}

		fmt.Fprintf(w, "-- Data for: %s\n", table)

		rows, err := s.db.Table(table).Rows()
		if err != nil {
			slog.Warn("[backup-service] failed to read table", "table", table, "error", err)
			continue
		}

		cols, _ := rows.Columns()
		placeholders := make([]string, len(cols))
		for i := range placeholders {
			placeholders[i] = "?"
		}
		insertStmt := fmt.Sprintf("INSERT INTO %s (%s) VALUES ", quoteIdentifier(table), strings.Join(quoteIdentifiers(cols), ", "))

		for rows.Next() {
			values := make([]interface{}, len(cols))
			valuePtrs := make([]interface{}, len(cols))
			for i := range values {
				valuePtrs[i] = &values[i]
			}
			if err := rows.Scan(valuePtrs...); err != nil {
				continue
			}

			formattedValues := make([]string, len(values))
			for i, val := range values {
				formattedValues[i] = formatValue(val)
			}
			fmt.Fprintf(w, "%s(%s);\n", insertStmt, strings.Join(formattedValues, ", "))
		}
		rows.Close()
		fmt.Fprintln(w, "")
	}

	fmt.Fprintln(w, "COMMIT;")
	return nil
}

func (s *DatabaseBackupService) pruneOldBackups(retention BackupRetention) {
	files, err := os.ReadDir(s.backupDir)
	if err != nil {
		return
	}

	type backupFile struct {
		name      string
		fullPath  string
		mtime     time.Time
		isMonthly bool
		isWeekly  bool
	}

	var backups []backupFile
	for _, f := range files {
		if !f.Type().IsRegular() || !strings.HasPrefix(f.Name(), "paperclip-") {
			continue
		}
		info, err := f.Info()
		if err != nil {
			continue
		}
		backups = append(backups, backupFile{
			name:     f.Name(),
			fullPath: filepath.Join(s.backupDir, f.Name()),
			mtime:    info.ModTime(),
		})
	}

	// Sort newest first
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].mtime.After(backups[j].mtime)
	})

	now := time.Now()
	dailyCutoff := now.AddDate(0, 0, -retention.DailyDays)
	weeklyCutoff := now.AddDate(0, 0, -retention.WeeklyWeeks*7)
	monthlyCutoff := now.AddDate(0, -retention.MonthlyMonths, 0)

	keepWeeks := make(map[string]bool)
	keepMonths := make(map[string]bool)

	for _, b := range backups {
		// Daily tier: keep everything
		if b.mtime.After(dailyCutoff) {
			continue
		}

		year, week := b.mtime.ISOWeek()
		weekKey := fmt.Sprintf("%d-W%d", year, week)
		monthKey := b.mtime.Format("2006-01")

		// Weekly tier: keep newest per week
		if b.mtime.After(weeklyCutoff) {
			if !keepWeeks[weekKey] {
				keepWeeks[weekKey] = true
				continue
			}
		}

		// Monthly tier: keep newest per month
		if b.mtime.After(monthlyCutoff) {
			if !keepMonths[monthKey] {
				keepMonths[monthKey] = true
				continue
			}
		}

		// Prune
		if err := os.Remove(b.fullPath); err == nil {
			slog.Info("[backup-service] pruned backup file", "file", b.name)
		}
	}
}

func (s *DatabaseBackupService) prunePluginLogs(ctx context.Context, dailyDays int) error {
	cutoff := time.Now().AddDate(0, 0, -dailyDays)
	totalDeleted := 0
	for i := 0; i < 100; i++ {
		res := s.db.WithContext(ctx).
			Where("created_at < ?", cutoff).
			Limit(5000).
			Delete(&models.PluginLog{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			break
		}
		totalDeleted += int(res.RowsAffected)
		if res.RowsAffected < 5000 {
			break
		}
	}
	if totalDeleted > 0 {
		slog.Info("[backup-service] pruned plugin logs", "count", totalDeleted)
	}
	return nil
}

func quoteIdentifier(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func quoteIdentifiers(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = quoteIdentifier(s)
	}
	return out
}

func formatValue(v interface{}) string {
	if v == nil {
		return "NULL"
	}
	switch t := v.(type) {
	case string:
		return "'" + strings.ReplaceAll(t, "'", "''") + "'"
	case time.Time:
		return "'" + t.Format(time.RFC3339Nano) + "'"
	case []byte:
		// Assume JSON or binary
		return "'" + strings.ReplaceAll(string(t), "'", "''") + "'"
	case bool:
		if t {
			return "TRUE"
		}
		return "FALSE"
	default:
		// Numbers, etc
		return fmt.Sprintf("%v", v)
	}
}
