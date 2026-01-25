package jobs

import (
"context"
"encoding/json"
"time"
)

// Job represents a background job
type Job struct {
ID          string                 `json:"id"`
Type        string                 `json:"type"`
Payload     json.RawMessage        `json:"payload"`
Status      JobStatus              `json:"status"`
Priority    int                    `json:"priority"`
MaxRetries  int                    `json:"max_retries"`
Attempts    int                    `json:"attempts"`
CreatedAt   time.Time              `json:"created_at"`
ScheduledAt time.Time              `json:"scheduled_at"`
StartedAt   *time.Time             `json:"started_at,omitempty"`
CompletedAt *time.Time             `json:"completed_at,omitempty"`
FailedAt    *time.Time             `json:"failed_at,omitempty"`
Error       string                 `json:"error,omitempty"`
Result      json.RawMessage        `json:"result,omitempty"`
Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// JobStatus represents the status of a job
type JobStatus string

const (
JobStatusPending    JobStatus = "pending"
JobStatusScheduled  JobStatus = "scheduled"
JobStatusProcessing JobStatus = "processing"
JobStatusCompleted  JobStatus = "completed"
JobStatusFailed     JobStatus = "failed"
JobStatusRetrying   JobStatus = "retrying"
JobStatusCancelled  JobStatus = "cancelled"
)

// Handler is a function that processes a job
type Handler func(ctx context.Context, job *Job) error

// HandlerFunc wraps a function to implement Handler interface
type HandlerFunc func(ctx context.Context, payload json.RawMessage) (interface{}, error)

// JobConfig holds job configuration
type JobConfig struct {
// Maximum number of retries
MaxRetries int

// Retry backoff strategy
RetryBackoff BackoffStrategy

// Job timeout
Timeout time.Duration

// Priority (higher = more important)
Priority int

// Delay before execution
Delay time.Duration

// Metadata for the job
Metadata map[string]interface{}
}

// DefaultJobConfig returns a default job configuration
func DefaultJobConfig() *JobConfig {
return &JobConfig{
MaxRetries:   3,
RetryBackoff: ExponentialBackoff,
Timeout:      5 * time.Minute,
Priority:     0,
Delay:        0,
Metadata:     make(map[string]interface{}),
}
}

// BackoffStrategy defines retry backoff behavior
type BackoffStrategy string

const (
NoBackoff          BackoffStrategy = "none"
LinearBackoff      BackoffStrategy = "linear"
ExponentialBackoff BackoffStrategy = "exponential"
)

// CalculateBackoff calculates the delay before next retry
func CalculateBackoff(strategy BackoffStrategy, attempt int) time.Duration {
switch strategy {
case NoBackoff:
return 0
case LinearBackoff:
return time.Duration(attempt) * time.Second
case ExponentialBackoff:
// 2^attempt seconds, capped at 1 hour
delay := time.Duration(1<<uint(attempt)) * time.Second
if delay > time.Hour {
return time.Hour
}
return delay
default:
return 0
}
}

// JobError represents a job processing error
type JobError struct {
JobID   string
JobType string
Err     error
Retry   bool
}

func (e *JobError) Error() string {
if e.Retry {
return "job " + e.JobID + " (" + e.JobType + ") failed, will retry: " + e.Err.Error()
}
return "job " + e.JobID + " (" + e.JobType + ") failed: " + e.Err.Error()
}

func (e *JobError) Unwrap() error {
return e.Err
}

// JobStats represents job statistics
type JobStats struct {
TotalJobs      int64         `json:"total_jobs"`
PendingJobs    int64         `json:"pending_jobs"`
ProcessingJobs int64         `json:"processing_jobs"`
CompletedJobs  int64         `json:"completed_jobs"`
FailedJobs     int64         `json:"failed_jobs"`
AvgDuration    time.Duration `json:"avg_duration"`
LastProcessed  time.Time     `json:"last_processed"`
}

// Registry holds registered job handlers
type Registry struct {
handlers map[string]Handler
}

// NewRegistry creates a new job registry
func NewRegistry() *Registry {
return &Registry{
handlers: make(map[string]Handler),
}
}

// Register registers a job handler
func (r *Registry) Register(jobType string, handler Handler) {
r.handlers[jobType] = handler
}

// RegisterFunc registers a job handler function
func (r *Registry) RegisterFunc(jobType string, fn HandlerFunc) {
r.handlers[jobType] = func(ctx context.Context, job *Job) error {
result, err := fn(ctx, job.Payload)
if err != nil {
return err
}

// Store result
if result != nil {
resultBytes, marshalErr := json.Marshal(result)
if marshalErr == nil {
job.Result = resultBytes
}
}

return nil
}
}

// Get retrieves a job handler by type
func (r *Registry) Get(jobType string) (Handler, bool) {
handler, ok := r.handlers[jobType]
return handler, ok
}

// Types returns all registered job types
func (r *Registry) Types() []string {
types := make([]string, 0, len(r.handlers))
for t := range r.handlers {
types = append(types, t)
}
return types
}
