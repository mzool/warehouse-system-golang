package jobs

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// CronJob represents a scheduled job
type CronJob struct {
	ID       string
	Schedule Schedule
	JobType  string
	Payload  interface{}
	Config   *JobConfig
	Enabled  bool
}

// Schedule defines when a job should run
type Schedule interface {
	// Next returns the next execution time after the given time
	Next(t time.Time) time.Time
}

// IntervalSchedule runs a job at fixed intervals
type IntervalSchedule struct {
	Interval time.Duration
}

func (s *IntervalSchedule) Next(t time.Time) time.Time {
	return t.Add(s.Interval)
}

// DailySchedule runs a job at a specific time each day
type DailySchedule struct {
	Hour   int
	Minute int
}

func (s *DailySchedule) Next(t time.Time) time.Time {
	next := time.Date(t.Year(), t.Month(), t.Day(), s.Hour, s.Minute, 0, 0, t.Location())
	if next.Before(t) || next.Equal(t) {
		next = next.Add(24 * time.Hour)
	}
	return next
}

// WeeklySchedule runs a job on specific days of the week
type WeeklySchedule struct {
	Weekday time.Weekday
	Hour    int
	Minute  int
}

func (s *WeeklySchedule) Next(t time.Time) time.Time {
	next := time.Date(t.Year(), t.Month(), t.Day(), s.Hour, s.Minute, 0, 0, t.Location())

	// Find next occurrence of the weekday
	daysUntil := int(s.Weekday - t.Weekday())
	if daysUntil <= 0 {
		daysUntil += 7
	}

	next = next.Add(time.Duration(daysUntil) * 24 * time.Hour)

	if next.Before(t) || next.Equal(t) {
		next = next.Add(7 * 24 * time.Hour)
	}

	return next
}

// MonthlySchedule runs a job on a specific day of the month
type MonthlySchedule struct {
	Day    int
	Hour   int
	Minute int
}

func (s *MonthlySchedule) Next(t time.Time) time.Time {
	next := time.Date(t.Year(), t.Month(), s.Day, s.Hour, s.Minute, 0, 0, t.Location())
	if next.Before(t) || next.Equal(t) {
		// Move to next month
		next = time.Date(t.Year(), t.Month()+1, s.Day, s.Hour, s.Minute, 0, 0, t.Location())
	}
	return next
}

// Scheduler manages scheduled jobs
type Scheduler struct {
	client    *Client
	jobs      map[string]*CronJob
	mu        sync.RWMutex
	stopCh    chan struct{}
	wg        sync.WaitGroup
	logger    *slog.Logger
	nextRuns  map[string]time.Time
	runningMu sync.Mutex
}

// NewScheduler creates a new job scheduler
func NewScheduler(client *Client, logger *slog.Logger) *Scheduler {
	if logger == nil {
		logger = slog.Default()
	}

	return &Scheduler{
		client:   client,
		jobs:     make(map[string]*CronJob),
		logger:   logger,
		stopCh:   make(chan struct{}),
		nextRuns: make(map[string]time.Time),
	}
}

// Register registers a scheduled job
func (s *Scheduler) Register(cronJob *CronJob) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.jobs[cronJob.ID] = cronJob
	s.nextRuns[cronJob.ID] = cronJob.Schedule.Next(time.Now())

	s.logger.Info("cron job registered",
		"id", cronJob.ID,
		"type", cronJob.JobType,
		"next_run", s.nextRuns[cronJob.ID].Format(time.RFC3339),
	)
}

// Unregister removes a scheduled job
func (s *Scheduler) Unregister(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.jobs, id)
	delete(s.nextRuns, id)

	s.logger.Info("cron job unregistered", "id", id)
}

// Enable enables a scheduled job
func (s *Scheduler) Enable(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if job, ok := s.jobs[id]; ok {
		job.Enabled = true
		s.nextRuns[id] = job.Schedule.Next(time.Now())
		s.logger.Info("cron job enabled", "id", id)
	}
}

// Disable disables a scheduled job
func (s *Scheduler) Disable(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if job, ok := s.jobs[id]; ok {
		job.Enabled = false
		s.logger.Info("cron job disabled", "id", id)
	}
}

// Start starts the scheduler
func (s *Scheduler) Start(ctx context.Context) {
	s.wg.Add(1)
	go s.run(ctx)
	s.logger.Info("scheduler started")
}

// Stop stops the scheduler gracefully
func (s *Scheduler) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	s.logger.Info("scheduler stopped")
}

func (s *Scheduler) run(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case now := <-ticker.C:
			s.tick(ctx, now)
		}
	}
}

func (s *Scheduler) tick(ctx context.Context, now time.Time) {
	s.mu.RLock()
	jobsToRun := make([]*CronJob, 0)

	for id, nextRun := range s.nextRuns {
		if job, ok := s.jobs[id]; ok && job.Enabled && now.After(nextRun) {
			jobsToRun = append(jobsToRun, job)
		}
	}
	s.mu.RUnlock()

	// Execute jobs outside of lock
	for _, job := range jobsToRun {
		s.execute(ctx, job)
	}
}

func (s *Scheduler) execute(ctx context.Context, cronJob *CronJob) {
	s.logger.Info("executing scheduled job",
		"id", cronJob.ID,
		"type", cronJob.JobType,
	)

	// Enqueue the job
	jobID, err := s.client.Enqueue(ctx, cronJob.JobType, cronJob.Payload, cronJob.Config)
	if err != nil {
		s.logger.Error("failed to enqueue scheduled job",
			"id", cronJob.ID,
			"error", err,
		)
		return
	}

	s.logger.Info("scheduled job enqueued",
		"cron_id", cronJob.ID,
		"job_id", jobID,
	)

	// Update next run time
	s.mu.Lock()
	s.nextRuns[cronJob.ID] = cronJob.Schedule.Next(time.Now())
	s.mu.Unlock()
}

// List returns all registered cron jobs
func (s *Scheduler) List() []*CronJob {
	s.mu.RLock()
	defer s.mu.RUnlock()

	jobs := make([]*CronJob, 0, len(s.jobs))
	for _, job := range s.jobs {
		jobs = append(jobs, job)
	}
	return jobs
}

// NextRun returns the next run time for a cron job
func (s *Scheduler) NextRun(id string) (time.Time, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	nextRun, ok := s.nextRuns[id]
	return nextRun, ok
}

// Helper functions for creating schedules

// Every creates an interval schedule
func Every(interval time.Duration) Schedule {
	return &IntervalSchedule{Interval: interval}
}

// Daily creates a daily schedule
func Daily(hour, minute int) Schedule {
	return &DailySchedule{Hour: hour, Minute: minute}
}

// Weekly creates a weekly schedule
func Weekly(weekday time.Weekday, hour, minute int) Schedule {
	return &WeeklySchedule{Weekday: weekday, Hour: hour, Minute: minute}
}

// Monthly creates a monthly schedule
func Monthly(day, hour, minute int) Schedule {
	return &MonthlySchedule{Day: day, Hour: hour, Minute: minute}
}
