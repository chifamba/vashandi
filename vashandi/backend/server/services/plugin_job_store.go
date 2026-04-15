package services

import (
	"context"
	"fmt"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"gorm.io/gorm"
)

type PluginJobDeclaration struct {
	JobKey   string `json:"jobKey"`
	Schedule string `json:"schedule"`
}

type PluginJobStore struct {
	DB *gorm.DB
}

func NewPluginJobStore(db *gorm.DB) *PluginJobStore {
	return &PluginJobStore{DB: db}
}

// SyncJobDeclarations ensures the database matches the jobs declared in the plugin manifest.
func (s *PluginJobStore) SyncJobDeclarations(ctx context.Context, pluginID string, declarations []PluginJobDeclaration) error {
	// 1. Fetch existing jobs for this plugin.
	var existingJobs []models.PluginJob
	if err := s.DB.WithContext(ctx).Where("plugin_id = ?", pluginID).Find(&existingJobs).Error; err != nil {
		return fmt.Errorf("failed to fetch existing jobs: %w", err)
	}

	existingByKey := make(map[string]models.PluginJob)
	for _, j := range existingJobs {
		existingByKey[j.JobKey] = j
	}

	declaredKeys := make(map[string]bool)

	// 2. Upsert each declared job.
	for _, decl := range declarations {
		declaredKeys[decl.JobKey] = true
		schedule := decl.Schedule
		if schedule == "" {
			continue
		}

		if existing, ok := existingByKey[decl.JobKey]; ok {
			// Update if changed or if it was paused.
			updates := map[string]interface{}{
				"updated_at": time.Now(),
			}
			if existing.Schedule != schedule {
				updates["schedule"] = schedule
			}
			if existing.Status == "paused" {
				updates["status"] = "active"
			}

			if len(updates) > 1 { // more than just updated_at
				if err := s.DB.WithContext(ctx).Model(&models.PluginJob{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
					return fmt.Errorf("failed to update job %s: %w", decl.JobKey, err)
				}
			}
		} else {
			// Insert new job.
			job := models.PluginJob{
				PluginID: pluginID,
				JobKey:   decl.JobKey,
				Schedule: schedule,
				Status:   "active",
			}
			if err := s.DB.WithContext(ctx).Create(&job).Error; err != nil {
				return fmt.Errorf("failed to create job %s: %w", decl.JobKey, err)
			}
		}
	}

	// 3. Pause jobs that are no longer declared.
	for _, existing := range existingJobs {
		if !declaredKeys[existing.JobKey] && existing.Status != "paused" {
			if err := s.DB.WithContext(ctx).Model(&models.PluginJob{}).Where("id = ?", existing.ID).Updates(map[string]interface{}{
				"status":     "paused",
				"updated_at": time.Now(),
			}).Error; err != nil {
				return fmt.Errorf("failed to pause job %s: %w", existing.JobKey, err)
			}
		}
	}

	return nil
}

func (s *PluginJobStore) GetJobByID(ctx context.Context, id string) (*models.PluginJob, error) {
	var job models.PluginJob
	if err := s.DB.WithContext(ctx).First(&job, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &job, nil
}

func (s *PluginJobStore) ListJobs(ctx context.Context, pluginID string, status string) ([]models.PluginJob, error) {
	var jobs []models.PluginJob
	db := s.DB.WithContext(ctx).Where("plugin_id = ?", pluginID)
	if status != "" {
		db = db.Where("status = ?", status)
	}
	if err := db.Find(&jobs).Error; err != nil {
		return nil, err
	}
	return jobs, nil
}

func (s *PluginJobStore) UpdateRunTimestamps(ctx context.Context, jobID string, lastRunAt time.Time, nextRunAt *time.Time) error {
	updates := map[string]interface{}{
		"last_run_at": lastRunAt,
		"next_run_at": nextRunAt,
		"updated_at":  time.Now(),
	}
	return s.DB.WithContext(ctx).Model(&models.PluginJob{}).Where("id = ?", jobID).Updates(updates).Error
}

func (s *PluginJobStore) DeleteAllJobs(ctx context.Context, pluginID string) error {
	return s.DB.WithContext(ctx).Delete(&models.PluginJob{}, "plugin_id = ?", pluginID).Error
}

// --- Runs ---

func (s *PluginJobStore) CreateRun(ctx context.Context, jobID, pluginID, trigger string) (*models.PluginJobRun, error) {
	run := models.PluginJobRun{
		JobID:    jobID,
		PluginID: pluginID,
		Trigger:  trigger,
		Status:   "queued",
	}
	if err := s.DB.WithContext(ctx).Create(&run).Error; err != nil {
		return nil, err
	}
	return &run, nil
}

func (s *PluginJobStore) MarkRunning(ctx context.Context, runID string) error {
	now := time.Now()
	return s.DB.WithContext(ctx).Model(&models.PluginJobRun{}).Where("id = ?", runID).Updates(map[string]interface{}{
		"status":     "running",
		"started_at": &now,
	}).Error
}

type CompleteRunInput struct {
	Status     string
	Error      *string
	DurationMs *int
}

func (s *PluginJobStore) CompleteRun(ctx context.Context, runID string, input CompleteRunInput) error {
	now := time.Now()
	updates := map[string]interface{}{
		"status":      input.Status,
		"error":       input.Error,
		"duration_ms": input.DurationMs,
		"finished_at": &now,
	}
	return s.DB.WithContext(ctx).Model(&models.PluginJobRun{}).Where("id = ?", runID).Updates(updates).Error
}
