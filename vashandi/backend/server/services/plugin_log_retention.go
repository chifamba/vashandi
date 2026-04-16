package services

import (
	"context"
	"log/slog"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"gorm.io/gorm"
)

const (
	pluginLogDefaultRetentionDays = 7
	pluginLogDeleteBatchSize      = 5_000
	pluginLogMaxIterations        = 100
	pluginLogDefaultIntervalHours = 1
)

// PrunePluginLogs deletes plugin_logs rows older than retentionDays.
// It deletes in batches of pluginLogDeleteBatchSize to avoid long-running
// transactions and lock contention, mirroring the Node.js implementation.
// Returns the total number of rows deleted.
func PrunePluginLogs(ctx context.Context, db *gorm.DB, retentionDays int) (int, error) {
	if retentionDays <= 0 {
		retentionDays = pluginLogDefaultRetentionDays
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)

	totalDeleted := 0
	for i := 0; i < pluginLogMaxIterations; i++ {
		// Delete a batch of rows older than the cutoff using a sub-query so
		// that the batch size is respected cleanly across databases.
		result := db.WithContext(ctx).
			Where("created_at < ?", cutoff).
			Limit(pluginLogDeleteBatchSize).
			Delete(&models.PluginLog{})
		if result.Error != nil {
			return totalDeleted, result.Error
		}
		deleted := int(result.RowsAffected)
		totalDeleted += deleted
		if deleted < pluginLogDeleteBatchSize {
			break
		}
		if i == pluginLogMaxIterations-1 {
			slog.Warn("Plugin log retention hit iteration limit; some logs may remain",
				"totalDeleted", totalDeleted,
				"iterations", pluginLogMaxIterations,
				"cutoffDate", cutoff,
			)
		}
	}

	if totalDeleted > 0 {
		slog.Info("Pruned expired plugin logs",
			"totalDeleted", totalDeleted,
			"retentionDays", retentionDays,
		)
	}
	return totalDeleted, nil
}

// StartPluginLogRetention starts a background goroutine that periodically
// deletes plugin log rows older than retentionDays. The goroutine runs an
// initial sweep immediately on startup, then ticks every intervalHours hours.
// It shuts down gracefully when ctx is cancelled.
//
// If retentionDays <= 0 the default of 7 days is used.
// If intervalHours <= 0 the default of 1 hour is used.
func StartPluginLogRetention(ctx context.Context, db *gorm.DB, retentionDays, intervalHours int) {
	if retentionDays <= 0 {
		retentionDays = pluginLogDefaultRetentionDays
	}
	if intervalHours <= 0 {
		intervalHours = pluginLogDefaultIntervalHours
	}

	// Run an initial sweep immediately, matching the Node.js behaviour.
	if n, err := PrunePluginLogs(ctx, db, retentionDays); err != nil {
		slog.Warn("Initial plugin log retention sweep failed", "error", err)
	} else if n > 0 {
		slog.Info("Initial plugin log retention sweep complete", "deleted", n)
	}

	ticker := time.NewTicker(time.Duration(intervalHours) * time.Hour)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := PrunePluginLogs(ctx, db, retentionDays); err != nil {
					slog.Warn("Plugin log retention sweep failed", "error", err)
				}
			}
		}
	}()
}
