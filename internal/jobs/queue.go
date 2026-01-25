package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Queue represents a job queue
type Queue interface {
	// Enqueue adds a job to the queue
	Enqueue(ctx context.Context, job *Job) error

	// Dequeue retrieves the next job from the queue
	Dequeue(ctx context.Context) (*Job, error)

	// Complete marks a job as completed
	Complete(ctx context.Context, jobID string, result json.RawMessage) error

	// Fail marks a job as failed
	Fail(ctx context.Context, jobID string, err error) error

	// Retry marks a job for retry
	Retry(ctx context.Context, job *Job) error

	// Cancel cancels a job
	Cancel(ctx context.Context, jobID string) error

	// Get retrieves a job by ID
	Get(ctx context.Context, jobID string) (*Job, error)

	// Stats returns queue statistics
	Stats(ctx context.Context) (*JobStats, error)

	// Close closes the queue
	Close() error
}

// RedisQueue implements a Redis-backed job queue
type RedisQueue struct {
	client *redis.Client
	config *RedisQueueConfig
	logger *slog.Logger
}

// RedisQueueConfig holds Redis queue configuration
type RedisQueueConfig struct {
	// Redis client
	Client *redis.Client

	// Queue name prefix
	Prefix string

	// Logger for structured logging
	Logger *slog.Logger

	// Visibility timeout (time before job becomes available again if not completed)
	VisibilityTimeout time.Duration

	// Poll interval when queue is empty
	PollInterval time.Duration
}

// DefaultRedisQueueConfig returns a default Redis queue configuration
func DefaultRedisQueueConfig() *RedisQueueConfig {
	return &RedisQueueConfig{
		Client:            nil,
		Prefix:            "jobs:",
		Logger:            nil,
		VisibilityTimeout: 5 * time.Minute,
		PollInterval:      1 * time.Second,
	}
}

// NewRedisQueue creates a new Redis-backed job queue
func NewRedisQueue(config *RedisQueueConfig) (*RedisQueue, error) {
	if config == nil {
		config = DefaultRedisQueueConfig()
	}

	if config.Client == nil {
		return nil, fmt.Errorf("redis client is required")
	}

	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	logger.Info("redis queue initialized", "prefix", config.Prefix)

	return &RedisQueue{
		client: config.Client,
		config: config,
		logger: logger,
	}, nil
}

// Enqueue adds a job to the queue
func (q *RedisQueue) Enqueue(ctx context.Context, job *Job) error {
	if job.ID == "" {
		job.ID = uuid.New().String()
	}

	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now()
	}

	if job.ScheduledAt.IsZero() {
		job.ScheduledAt = time.Now()
	}

	job.Status = JobStatusPending

	// Serialize job
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	pipe := q.client.Pipeline()

	// Store job data
	jobKey := q.jobKey(job.ID)
	pipe.Set(ctx, jobKey, data, 0)

	// Add to priority queue (sorted set by priority and scheduled time)
	score := float64(job.ScheduledAt.Unix()) - float64(job.Priority)*1000000
	queueKey := q.queueKey()
	pipe.ZAdd(ctx, queueKey, redis.Z{Score: score, Member: job.ID})

	// Update stats
	pipe.Incr(ctx, q.statsKey("total"))
	pipe.Incr(ctx, q.statsKey("pending"))

	_, err = pipe.Exec(ctx)
	if err != nil {
		q.logger.Error("failed to enqueue job", "error", err, "job_id", job.ID)
		return fmt.Errorf("failed to enqueue job: %w", err)
	}

	q.logger.Debug("job enqueued", "job_id", job.ID, "type", job.Type, "priority", job.Priority)
	return nil
}

// Dequeue retrieves the next job from the queue
func (q *RedisQueue) Dequeue(ctx context.Context) (*Job, error) {
	now := time.Now()
	queueKey := q.queueKey()

	// Get jobs ready to process (scheduled time <= now)
	results, err := q.client.ZRangeByScoreWithScores(ctx, queueKey, &redis.ZRangeBy{
		Min:   "-inf",
		Max:   fmt.Sprintf("%f", float64(now.Unix())),
		Count: 1,
	}).Result()

	if err != nil {
		return nil, fmt.Errorf("failed to query queue: %w", err)
	}

	if len(results) == 0 {
		return nil, nil // No jobs available
	}

	jobID := results[0].Member.(string)

	// Remove from queue atomically
	removed, err := q.client.ZRem(ctx, queueKey, jobID).Result()
	if err != nil || removed == 0 {
		return nil, nil // Job was already taken by another worker
	}

	// Get job data
	job, err := q.Get(ctx, jobID)
	if err != nil {
		return nil, err
	}

	// Update job status
	job.Status = JobStatusProcessing
	job.Attempts++
	now = time.Now()
	job.StartedAt = &now

	// Save updated job
	data, _ := json.Marshal(job)
	q.client.Set(ctx, q.jobKey(job.ID), data, 0)

	// Update stats
	pipe := q.client.Pipeline()
	pipe.Decr(ctx, q.statsKey("pending"))
	pipe.Incr(ctx, q.statsKey("processing"))
	pipe.Exec(ctx)

	q.logger.Debug("job dequeued", "job_id", job.ID, "type", job.Type, "attempts", job.Attempts)
	return job, nil
}

// Complete marks a job as completed
func (q *RedisQueue) Complete(ctx context.Context, jobID string, result json.RawMessage) error {
	job, err := q.Get(ctx, jobID)
	if err != nil {
		return err
	}

	now := time.Now()
	job.Status = JobStatusCompleted
	job.CompletedAt = &now
	job.Result = result

	data, _ := json.Marshal(job)
	q.client.Set(ctx, q.jobKey(jobID), data, 24*time.Hour) // Keep for 24h

	// Update stats
	pipe := q.client.Pipeline()
	pipe.Decr(ctx, q.statsKey("processing"))
	pipe.Incr(ctx, q.statsKey("completed"))
	pipe.Set(ctx, q.statsKey("last_processed"), now.Unix(), 0)
	pipe.Exec(ctx)

	q.logger.Info("job completed", "job_id", jobID, "type", job.Type)
	return nil
}

// Fail marks a job as failed
func (q *RedisQueue) Fail(ctx context.Context, jobID string, jobErr error) error {
	job, err := q.Get(ctx, jobID)
	if err != nil {
		return err
	}

	now := time.Now()
	job.Status = JobStatusFailed
	job.FailedAt = &now
	job.Error = jobErr.Error()

	data, _ := json.Marshal(job)
	q.client.Set(ctx, q.jobKey(jobID), data, 24*time.Hour) // Keep for 24h

	// Update stats
	pipe := q.client.Pipeline()
	pipe.Decr(ctx, q.statsKey("processing"))
	pipe.Incr(ctx, q.statsKey("failed"))
	pipe.Exec(ctx)

	q.logger.Error("job failed", "job_id", jobID, "type", job.Type, "error", jobErr)
	return nil
}

// Retry marks a job for retry
func (q *RedisQueue) Retry(ctx context.Context, job *Job) error {
	// Calculate retry delay based on backoff strategy
	delay := CalculateBackoff(job.Metadata["backoff_strategy"].(BackoffStrategy), job.Attempts)
	job.ScheduledAt = time.Now().Add(delay)
	job.Status = JobStatusRetrying

	// Re-enqueue the job
	return q.Enqueue(ctx, job)
}

// Cancel cancels a job
func (q *RedisQueue) Cancel(ctx context.Context, jobID string) error {
	job, err := q.Get(ctx, jobID)
	if err != nil {
		return err
	}

	job.Status = JobStatusCancelled
	data, _ := json.Marshal(job)
	q.client.Set(ctx, q.jobKey(jobID), data, 24*time.Hour)

	// Remove from queue
	q.client.ZRem(ctx, q.queueKey(), jobID)

	q.logger.Info("job cancelled", "job_id", jobID, "type", job.Type)
	return nil
}

// Get retrieves a job by ID
func (q *RedisQueue) Get(ctx context.Context, jobID string) (*Job, error) {
	data, err := q.client.Get(ctx, q.jobKey(jobID)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("job not found: %s", jobID)
		}
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	var job Job
	if err := json.Unmarshal(data, &job); err != nil {
		return nil, fmt.Errorf("failed to unmarshal job: %w", err)
	}

	return &job, nil
}

// Stats returns queue statistics
func (q *RedisQueue) Stats(ctx context.Context) (*JobStats, error) {
	pipe := q.client.Pipeline()
	totalCmd := pipe.Get(ctx, q.statsKey("total"))
	pendingCmd := pipe.Get(ctx, q.statsKey("pending"))
	processingCmd := pipe.Get(ctx, q.statsKey("processing"))
	completedCmd := pipe.Get(ctx, q.statsKey("completed"))
	failedCmd := pipe.Get(ctx, q.statsKey("failed"))
	lastProcessedCmd := pipe.Get(ctx, q.statsKey("last_processed"))

	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("failed to get stats: %w", err)
	}

	stats := &JobStats{}
	stats.TotalJobs, _ = totalCmd.Int64()
	stats.PendingJobs, _ = pendingCmd.Int64()
	stats.ProcessingJobs, _ = processingCmd.Int64()
	stats.CompletedJobs, _ = completedCmd.Int64()
	stats.FailedJobs, _ = failedCmd.Int64()

	lastProcessed, _ := lastProcessedCmd.Int64()
	if lastProcessed > 0 {
		stats.LastProcessed = time.Unix(lastProcessed, 0)
	}

	return stats, nil
}

// Close closes the queue
func (q *RedisQueue) Close() error {
	return nil // Redis client is managed externally
}

func (q *RedisQueue) jobKey(jobID string) string {
	return q.config.Prefix + "job:" + jobID
}

func (q *RedisQueue) queueKey() string {
	return q.config.Prefix + "queue"
}

func (q *RedisQueue) statsKey(stat string) string {
	return q.config.Prefix + "stats:" + stat
}
