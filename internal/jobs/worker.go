package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Worker processes jobs from a queue
type Worker struct {
	id       int
	queue    Queue
	registry *Registry
	config   *WorkerConfig
	logger   *slog.Logger
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// WorkerConfig holds worker configuration
type WorkerConfig struct {
	// Job timeout
	JobTimeout time.Duration

	// Poll interval when queue is empty
	PollInterval time.Duration

	// Logger for structured logging
	Logger *slog.Logger

	// Error handler
	ErrorHandler func(ctx context.Context, job *Job, err error)

	// Success handler
	SuccessHandler func(ctx context.Context, job *Job)
}

// DefaultWorkerConfig returns a default worker configuration
func DefaultWorkerConfig() *WorkerConfig {
	return &WorkerConfig{
		JobTimeout:     5 * time.Minute,
		PollInterval:   1 * time.Second,
		Logger:         nil,
		ErrorHandler:   nil,
		SuccessHandler: nil,
	}
}

// NewWorker creates a new worker
func NewWorker(id int, queue Queue, registry *Registry, config *WorkerConfig) *Worker {
	if config == nil {
		config = DefaultWorkerConfig()
	}

	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &Worker{
		id:       id,
		queue:    queue,
		registry: registry,
		config:   config,
		logger:   logger.With("worker_id", id),
		stopCh:   make(chan struct{}),
	}
}

// Start starts the worker
func (w *Worker) Start(ctx context.Context) {
	w.wg.Add(1)
	go w.run(ctx)
}

// Stop stops the worker gracefully
func (w *Worker) Stop() {
	close(w.stopCh)
	w.wg.Wait()
}

func (w *Worker) run(ctx context.Context) {
	defer w.wg.Done()

	w.logger.Info("worker started")

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("worker stopped (context cancelled)")
			return
		case <-w.stopCh:
			w.logger.Info("worker stopped")
			return
		default:
			w.processNext(ctx)
		}
	}
}

func (w *Worker) processNext(ctx context.Context) {
	// Dequeue next job
	job, err := w.queue.Dequeue(ctx)
	if err != nil {
		w.logger.Error("failed to dequeue job", "error", err)
		time.Sleep(w.config.PollInterval)
		return
	}

	if job == nil {
		// No jobs available, wait before polling again
		time.Sleep(w.config.PollInterval)
		return
	}

	// Process the job
	w.process(ctx, job)
}

func (w *Worker) process(ctx context.Context, job *Job) {
	w.logger.Info("processing job",
		"job_id", job.ID,
		"type", job.Type,
		"attempts", job.Attempts,
	)

	start := time.Now()

	// Get handler
	handler, ok := w.registry.Get(job.Type)
	if !ok {
		err := fmt.Errorf("no handler registered for job type: %s", job.Type)
		w.logger.Error("job handler not found", "job_id", job.ID, "type", job.Type)
		w.queue.Fail(ctx, job.ID, err)
		return
	}

	// Create timeout context
	jobCtx, cancel := context.WithTimeout(ctx, w.config.JobTimeout)
	defer cancel()

	// Execute handler
	err := handler(jobCtx, job)

	duration := time.Since(start)

	if err != nil {
		w.handleError(ctx, job, err, duration)
	} else {
		w.handleSuccess(ctx, job, duration)
	}
}

func (w *Worker) handleSuccess(ctx context.Context, job *Job, duration time.Duration) {
	w.logger.Info("job completed successfully",
		"job_id", job.ID,
		"type", job.Type,
		"duration", duration.String(),
	)

	// Mark as completed
	w.queue.Complete(ctx, job.ID, job.Result)

	// Call success handler if configured
	if w.config.SuccessHandler != nil {
		w.config.SuccessHandler(ctx, job)
	}
}

func (w *Worker) handleError(ctx context.Context, job *Job, err error, duration time.Duration) {
	w.logger.Error("job failed",
		"job_id", job.ID,
		"type", job.Type,
		"error", err,
		"attempts", job.Attempts,
		"duration", duration.String(),
	)

	// Check if should retry
	if job.Attempts < job.MaxRetries {
		w.logger.Info("retrying job",
			"job_id", job.ID,
			"attempts", job.Attempts,
			"max_retries", job.MaxRetries,
		)

		// Store backoff strategy in metadata if not present
		if job.Metadata == nil {
			job.Metadata = make(map[string]interface{})
		}
		if _, ok := job.Metadata["backoff_strategy"]; !ok {
			job.Metadata["backoff_strategy"] = ExponentialBackoff
		}

		w.queue.Retry(ctx, job)
	} else {
		w.logger.Error("job failed permanently",
			"job_id", job.ID,
			"type", job.Type,
			"attempts", job.Attempts,
		)
		w.queue.Fail(ctx, job.ID, err)
	}

	// Call error handler if configured
	if w.config.ErrorHandler != nil {
		w.config.ErrorHandler(ctx, job, err)
	}
}

// WorkerPool manages multiple workers
type WorkerPool struct {
	workers  []*Worker
	queue    Queue
	registry *Registry
	config   *WorkerPoolConfig
	logger   *slog.Logger
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// WorkerPoolConfig holds worker pool configuration
type WorkerPoolConfig struct {
	// Number of workers
	NumWorkers int

	// Worker configuration
	WorkerConfig *WorkerConfig

	// Logger for structured logging
	Logger *slog.Logger
}

// DefaultWorkerPoolConfig returns a default worker pool configuration
func DefaultWorkerPoolConfig() *WorkerPoolConfig {
	return &WorkerPoolConfig{
		NumWorkers:   5,
		WorkerConfig: DefaultWorkerConfig(),
		Logger:       nil,
	}
}

// NewWorkerPool creates a new worker pool
func NewWorkerPool(queue Queue, registry *Registry, config *WorkerPoolConfig) *WorkerPool {
	if config == nil {
		config = DefaultWorkerPoolConfig()
	}

	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	workers := make([]*Worker, config.NumWorkers)
	for i := 0; i < config.NumWorkers; i++ {
		workers[i] = NewWorker(i+1, queue, registry, config.WorkerConfig)
	}

	return &WorkerPool{
		workers:  workers,
		queue:    queue,
		registry: registry,
		config:   config,
		logger:   logger,
		stopCh:   make(chan struct{}),
	}
}

// Start starts all workers in the pool
func (wp *WorkerPool) Start(ctx context.Context) {
	wp.logger.Info("starting worker pool", "num_workers", wp.config.NumWorkers)

	for _, worker := range wp.workers {
		worker.Start(ctx)
	}

	wp.logger.Info("worker pool started")
}

// Stop stops all workers gracefully
func (wp *WorkerPool) Stop() {
	wp.logger.Info("stopping worker pool")

	for _, worker := range wp.workers {
		worker.Stop()
	}

	wp.logger.Info("worker pool stopped")
}

// Stats returns worker pool statistics
func (wp *WorkerPool) Stats(ctx context.Context) (*JobStats, error) {
	return wp.queue.Stats(ctx)
}

// Client provides a high-level interface for job management
type Client struct {
	queue    Queue
	registry *Registry
	pool     *WorkerPool
	logger   *slog.Logger
}

// ClientConfig holds client configuration
type ClientConfig struct {
	// Queue configuration
	Queue Queue

	// Registry for job handlers
	Registry *Registry

	// Worker pool configuration
	WorkerPoolConfig *WorkerPoolConfig

	// Logger for structured logging
	Logger *slog.Logger
}

// NewClient creates a new job client
func NewClient(config *ClientConfig) *Client {
	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	pool := NewWorkerPool(config.Queue, config.Registry, config.WorkerPoolConfig)

	return &Client{
		queue:    config.Queue,
		registry: config.Registry,
		pool:     pool,
		logger:   logger,
	}
}

// Start starts the job processing
func (c *Client) Start(ctx context.Context) {
	c.pool.Start(ctx)
}

// Stop stops the job processing
func (c *Client) Stop() {
	c.pool.Stop()
}

// Enqueue adds a job to the queue
func (c *Client) Enqueue(ctx context.Context, jobType string, payload interface{}, config *JobConfig) (string, error) {
	if config == nil {
		config = DefaultJobConfig()
	}

	// Serialize payload
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}

	job := &Job{
		Type:        jobType,
		Payload:     payloadBytes,
		MaxRetries:  config.MaxRetries,
		Priority:    config.Priority,
		Metadata:    config.Metadata,
		ScheduledAt: time.Now().Add(config.Delay),
	}

	// Store backoff strategy in metadata
	if job.Metadata == nil {
		job.Metadata = make(map[string]interface{})
	}
	job.Metadata["backoff_strategy"] = config.RetryBackoff

	err = c.queue.Enqueue(ctx, job)
	if err != nil {
		return "", err
	}

	c.logger.Info("job enqueued", "job_id", job.ID, "type", jobType)
	return job.ID, nil
}

// Get retrieves a job by ID
func (c *Client) Get(ctx context.Context, jobID string) (*Job, error) {
	return c.queue.Get(ctx, jobID)
}

// Cancel cancels a job
func (c *Client) Cancel(ctx context.Context, jobID string) error {
	return c.queue.Cancel(ctx, jobID)
}

// Stats returns job statistics
func (c *Client) Stats(ctx context.Context) (*JobStats, error) {
	return c.queue.Stats(ctx)
}
