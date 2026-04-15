package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"gorm.io/gorm"
)

const (
	DefaultJobTickInterval = 30 * time.Second
	DefaultJobTimeout     = 5 * time.Minute
	DefaultMaxConcurrentJobs = 10
)

type PluginJobScheduler struct {
	DB            *gorm.DB
	Store         *PluginJobStore
	WorkerManager *PluginWorkerManager
	
	activeJobs    map[string]bool
	activeMu      sync.RWMutex
	
	tickInterval  time.Duration
	jobTimeout    time.Duration
	maxConcurrent int
	
	tickCount     int
	lastTickAt    *time.Time
	running       bool
	stopChan      chan struct{}
}

func NewPluginJobScheduler(db *gorm.DB, store *PluginJobStore, wm *PluginWorkerManager) *PluginJobScheduler {
	return &PluginJobScheduler{
		DB:            db,
		Store:         store,
		WorkerManager: wm,
		activeJobs:    make(map[string]bool),
		tickInterval:  DefaultJobTickInterval,
		jobTimeout:    DefaultJobTimeout,
		maxConcurrent: DefaultMaxConcurrentJobs,
	}
}

func (s *PluginJobScheduler) Start(ctx context.Context) {
	if s.running {
		return
	}
	s.running = true
	s.stopChan = make(chan struct{})

	ticker := time.NewTicker(s.tickInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-s.stopChan:
				return
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				s.Tick(ctx, now)
			}
		}
	}()
	slog.Info("Plugin job scheduler started", "interval", s.tickInterval, "maxConcurrent", s.maxConcurrent)
}

func (s *PluginJobScheduler) Stop() {
	if !s.running {
		return
	}
	close(s.stopChan)
	s.running = false
	slog.Info("Plugin job scheduler stopped")
}

func (s *PluginJobScheduler) Tick(ctx context.Context, now time.Time) {
	s.tickCount++
	s.lastTickAt = &now

	// 1. Fetch due jobs: status active, next_run_at <= now.
	var dueJobs []models.PluginJob
	if err := s.DB.WithContext(ctx).Where("status = ? AND next_run_at <= ?", "active", now).Find(&dueJobs).Error; err != nil {
		slog.Error("Plugin job scheduler: failed to fetch due jobs", "error", err)
		return
	}

	if len(dueJobs) == 0 {
		return
	}

	slog.Debug("Plugin job scheduler: found due jobs", "count", len(dueJobs))

	for _, job := range dueJobs {
		// Check concurrency limit.
		s.activeMu.RLock()
		activeCount := len(s.activeJobs)
		s.activeMu.RUnlock()

		if activeCount >= s.maxConcurrent {
			slog.Warn("Plugin job scheduler: max concurrent jobs reached, deferring remainder")
			break
		}

		// Overlap prevention.
		s.activeMu.RLock()
		alreadyRunning := s.activeJobs[job.ID]
		s.activeMu.RUnlock()

		if alreadyRunning {
			continue
		}

		// Check worker availability.
		if !s.WorkerManager.IsRunning(job.PluginID) {
			continue
		}

		// Dispatch.
		go s.DispatchJob(context.Background(), job)
	}
}

func (s *PluginJobScheduler) DispatchJob(ctx context.Context, job models.PluginJob) {
	s.activeMu.Lock()
	s.activeJobs[job.ID] = true
	s.activeMu.Unlock()

	defer func() {
		s.activeMu.Lock()
		delete(s.activeJobs, job.ID)
		s.activeMu.Unlock()
	}()

	startedAt := time.Now()
	
	// 1. Create run record.
	run, err := s.Store.CreateRun(ctx, job.ID, job.PluginID, "schedule")
	if err != nil {
		slog.Error("Plugin job scheduler: failed to create run record", "jobID", job.ID, "error", err)
		return
	}

	// 2. Mark running.
	if err := s.Store.MarkRunning(ctx, run.ID); err != nil {
		slog.Error("Plugin job scheduler: failed to mark run as running", "runID", run.ID, "error", err)
	}

	// 3. Call worker via RPC.
	params := map[string]interface{}{
		"job": map[string]interface{}{
			"jobKey":      job.JobKey,
			"runId":       run.ID,
			"trigger":     "schedule",
			"scheduledAt": job.NextRunAt.UTC().Format(time.RFC3339),
		},
	}
	
	_, rpcErr := s.WorkerManager.Call(ctx, job.PluginID, "runJob", params, s.jobTimeout)

	// 4. Record result.
	durationMs := int(time.Since(startedAt).Milliseconds())
	status := "succeeded"
	var errMsg *string
	if rpcErr != nil {
		status = "failed"
		s := rpcErr.Error()
		errMsg = &s
		slog.Error("Plugin job execution failed", "runID", run.ID, "error", rpcErr)
	}

	if err := s.Store.CompleteRun(ctx, run.ID, CompleteRunInput{
		Status:     status,
		Error:      errMsg,
		DurationMs: &durationMs,
	}); err != nil {
		slog.Error("Plugin job scheduler: failed to complete run record", "runID", run.ID, "error", err)
	}

	// 5. Advance schedule pointer.
	if err := s.AdvanceSchedulePointer(ctx, job); err != nil {
		slog.Error("Plugin job scheduler: failed to advance schedule pointer", "jobID", job.ID, "error", err)
	}
}

func (s *PluginJobScheduler) AdvanceSchedulePointer(ctx context.Context, job models.PluginJob) error {
	now := time.Now()
	var nextRunAt *time.Time

	if job.Schedule != "" {
		// We use UTC as the default timezone for plugin jobs if not specified.
		// NOTE: The Node.js implementation seems to use the system timezone or UTC.
		// Go's nextCronTickInTimezone handles the expansion.
		nt, err := nextCronTickInTimezone(job.Schedule, "UTC", now)
		if err != nil {
			return fmt.Errorf("invalid cron schedule %q: %w", job.Schedule, err)
		}
		nextRunAt = nt
	}

	return s.Store.UpdateRunTimestamps(ctx, job.ID, now, nextRunAt)
}

func (s *PluginJobScheduler) TriggerJob(ctx context.Context, jobID string, trigger string) (string, error) {
	job, err := s.Store.GetJobByID(ctx, jobID)
	if err != nil {
		return "", err
	}
	if job == nil {
		return "", fmt.Errorf("job not found")
	}

	if job.Status != "active" {
		return "", fmt.Errorf("job is not active (current status: %s)", job.Status)
	}

	// Check manual overlap.
	s.activeMu.RLock()
	alreadyRunning := s.activeJobs[job.ID]
	s.activeMu.RUnlock()

	if alreadyRunning {
		return "", fmt.Errorf("job is already running")
	}

	// Verify worker.
	if !s.WorkerManager.IsRunning(job.PluginID) {
		return "", fmt.Errorf("plugin worker is not running")
	}

	// Manual dispatch.
	run, err := s.Store.CreateRun(ctx, job.ID, job.PluginID, trigger)
	if err != nil {
		return "", err
	}

	// Dispatch in background.
	go s.DispatchManualRun(context.Background(), *job, run.ID, trigger)

	return run.ID, nil
}

func (s *PluginJobScheduler) DispatchManualRun(ctx context.Context, job models.PluginJob, runID string, trigger string) {
	s.activeMu.Lock()
	s.activeJobs[job.ID] = true
	s.activeMu.Unlock()

	defer func() {
		s.activeMu.Lock()
		delete(s.activeJobs, job.ID)
		s.activeMu.Unlock()
	}()

	startedAt := time.Now()
	
	if err := s.Store.MarkRunning(ctx, runID); err != nil {
		slog.Error("Plugin job scheduler: failed to mark manual run as running", "runID", runID, "error", err)
	}

	params := map[string]interface{}{
		"job": map[string]interface{}{
			"jobKey":      job.JobKey,
			"runId":       runID,
			"trigger":     trigger,
			"scheduledAt": startedAt.UTC().Format(time.RFC3339),
		},
	}
	
	_, rpcErr := s.WorkerManager.Call(ctx, job.PluginID, "runJob", params, s.jobTimeout)

	durationMs := int(time.Since(startedAt).Milliseconds())
	status := "succeeded"
	var errMsg *string
	if rpcErr != nil {
		status = "failed"
		s := rpcErr.Error()
		errMsg = &s
	}

	_ = s.Store.CompleteRun(ctx, runID, CompleteRunInput{
		Status:     status,
		Error:      errMsg,
		DurationMs: &durationMs,
	})
}

func (s *PluginJobScheduler) RegisterPlugin(ctx context.Context, pluginID string) error {
	slog.Info("Registering plugin with job scheduler", "pluginId", pluginID)
	
	// Ensure all active jobs have a nextRunAt.
	jobs, err := s.Store.ListJobs(ctx, pluginID, "active")
	if err != nil {
		return err
	}

	for _, job := range jobs {
		if job.NextRunAt == nil || job.NextRunAt.Before(time.Now()) {
			if err := s.AdvanceSchedulePointer(ctx, job); err != nil {
				slog.Error("Failed to compute initial nextRunAt for job", "jobID", job.ID, "error", err)
			}
		}
	}
	return nil
}

func (s *PluginJobScheduler) UnregisterPlugin(ctx context.Context, pluginID string) error {
	slog.Info("Unregistering plugin from job scheduler", "pluginId", pluginID)
	
	// In Node.js, it cancels in-flight runs in the DB.
	// We'll leave the activeJobs map check to the DispatchJob deferred cleanup.
	
	return nil
}
