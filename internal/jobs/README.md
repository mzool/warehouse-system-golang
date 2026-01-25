# Jobs Package

Production-ready background job processing system with Redis-backed queue, worker pools, retry logic, and cron scheduling.

## Features

- **Redis-Backed Queue**: Reliable job storage with priority support
- **Worker Pools**: Concurrent job processing with configurable workers
- **Retry Logic**: Automatic retry with exponential backoff
- **Cron Scheduler**: Schedule recurring jobs (daily, weekly, monthly, custom intervals)
- **Job Monitoring**: Real-time statistics and job tracking
- **Priority Queues**: High-priority jobs processed first
- **Graceful Shutdown**: Proper cleanup and job completion
- **Type Safety**: Strongly-typed job payloads with generics
- **Structured Logging**: slog integration for observability

## Quick Start

### 1. Setup Job Client

```go
import (
    "your-project/internal/jobs"
    "github.com/redis/go-redis/v9"
)

// Create Redis client
redisClient := redis.NewClient(&redis.Options{
    Addr: "localhost:6379",
})

// Create job queue
queueConfig := jobs.DefaultRedisQueueConfig()
queueConfig.Client = redisClient
queue, _ := jobs.NewRedisQueue(queueConfig)

// Create job registry
registry := jobs.NewRegistry()

// Register job handlers
registry.RegisterFunc("send_email", handleSendEmail)
registry.RegisterFunc("process_order", handleProcessOrder)

// Create job client
client := jobs.NewClient(&jobs.ClientConfig{
    Queue:    queue,
    Registry: registry,
    WorkerPoolConfig: &jobs.WorkerPoolConfig{
        NumWorkers: 10,
    },
})

// Start processing
ctx := context.Background()
client.Start(ctx)
defer client.Stop()
```

### 2. Define Job Handlers

```go
func handleSendEmail(ctx context.Context, payload json.RawMessage) (interface{}, error) {
    var email struct {
        To      string `json:"to"`
        Subject string `json:"subject"`
        Body    string `json:"body"`
    }
    
    if err := json.Unmarshal(payload, &email); err != nil {
        return nil, err
    }
    
    // Send email
    err := sendEmail(email.To, email.Subject, email.Body)
    if err != nil {
        return nil, err
    }
    
    return map[string]string{"status": "sent"}, nil
}
```

### 3. Enqueue Jobs

```go
// Simple enqueue
payload := map[string]string{
    "to":      "user@example.com",
    "subject": "Welcome!",
    "body":    "Thank you for signing up",
}

jobID, err := client.Enqueue(ctx, "send_email", payload, nil)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Job enqueued: %s\n", jobID)
```

## Job Configuration

### Custom Job Configuration

```go
config := &jobs.JobConfig{
    MaxRetries:   5,
    RetryBackoff: jobs.ExponentialBackoff,
    Timeout:      10 * time.Minute,
    Priority:     10, // Higher = more important
    Delay:        5 * time.Minute, // Delay before first execution
    Metadata: map[string]interface{}{
        "user_id": 123,
        "source":  "api",
    },
}

jobID, _ := client.Enqueue(ctx, "process_order", payload, config)
```

### Retry Strategies

```go
// No backoff - retry immediately
config.RetryBackoff = jobs.NoBackoff

// Linear backoff - 1s, 2s, 3s, 4s...
config.RetryBackoff = jobs.LinearBackoff

// Exponential backoff - 1s, 2s, 4s, 8s, 16s... (recommended)
config.RetryBackoff = jobs.ExponentialBackoff
```

## Cron Scheduling

### Create Scheduler

```go
scheduler := jobs.NewScheduler(client, logger)
scheduler.Start(ctx)
defer scheduler.Stop()
```

### Schedule Jobs

```go
// Run every 5 minutes
scheduler.Register(&jobs.CronJob{
    ID:       "cleanup-temp-files",
    Schedule: jobs.Every(5 * time.Minute),
    JobType:  "cleanup",
    Payload:  map[string]string{"type": "temp"},
    Config:   jobs.DefaultJobConfig(),
    Enabled:  true,
})

// Run daily at 2:00 AM
scheduler.Register(&jobs.CronJob{
    ID:       "daily-report",
    Schedule: jobs.Daily(2, 0),
    JobType:  "generate_report",
    Payload:  map[string]string{"report_type": "daily"},
    Enabled:  true,
})

// Run every Monday at 9:00 AM
scheduler.Register(&jobs.CronJob{
    ID:       "weekly-summary",
    Schedule: jobs.Weekly(time.Monday, 9, 0),
    JobType:  "generate_report",
    Payload:  map[string]string{"report_type": "weekly"},
    Enabled:  true,
})

// Run on the 1st of each month at midnight
scheduler.Register(&jobs.CronJob{
    ID:       "monthly-invoice",
    Schedule: jobs.Monthly(1, 0, 0),
    JobType:  "generate_invoice",
    Payload:  nil,
    Enabled:  true,
})
```

### Manage Scheduled Jobs

```go
// Disable a job temporarily
scheduler.Disable("daily-report")

// Enable it again
scheduler.Enable("daily-report")

// Remove a job
scheduler.Unregister("daily-report")

// List all scheduled jobs
cronJobs := scheduler.List()

// Get next run time
nextRun, ok := scheduler.NextRun("daily-report")
```

## Advanced Usage

### Job Priorities

```go
// High priority job (processed first)
config := &jobs.JobConfig{
    Priority: 100,
}
client.Enqueue(ctx, "urgent_task", payload, config)

// Normal priority
config.Priority = 0

// Low priority (processed last)
config.Priority = -100
```

### Delayed Jobs

```go
// Execute in 1 hour
config := &jobs.JobConfig{
    Delay: 1 * time.Hour,
}
client.Enqueue(ctx, "reminder", payload, config)
```

### Job Tracking

```go
// Get job by ID
job, err := client.Get(ctx, jobID)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Status: %s\n", job.Status)
fmt.Printf("Attempts: %d/%d\n", job.Attempts, job.MaxRetries)

if job.CompletedAt != nil {
    fmt.Printf("Completed at: %s\n", job.CompletedAt)
}

if job.Error != "" {
    fmt.Printf("Error: %s\n", job.Error)
}
```

### Cancel Jobs

```go
err := client.Cancel(ctx, jobID)
if err != nil {
    log.Fatal(err)
}
```

### Queue Statistics

```go
stats, err := client.Stats(ctx)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Total Jobs: %d\n", stats.TotalJobs)
fmt.Printf("Pending: %d\n", stats.PendingJobs)
fmt.Printf("Processing: %d\n", stats.ProcessingJobs)
fmt.Printf("Completed: %d\n", stats.CompletedJobs)
fmt.Printf("Failed: %d\n", stats.FailedJobs)
fmt.Printf("Last Processed: %s\n", stats.LastProcessed)
```

## Error Handling

### Custom Error Handler

```go
workerConfig := jobs.DefaultWorkerConfig()
workerConfig.ErrorHandler = func(ctx context.Context, job *jobs.Job, err error) {
    // Log to monitoring service
    monitoring.ReportError(job.Type, err)
    
    // Send alert for critical jobs
    if job.Priority > 50 {
        alerts.Send("High priority job failed: " + job.ID)
    }
}
```

### Custom Success Handler

```go
workerConfig.SuccessHandler = func(ctx context.Context, job *jobs.Job) {
    // Update metrics
    metrics.IncrementJobSuccess(job.Type)
    
    // Trigger dependent jobs
    if job.Type == "process_order" {
        client.Enqueue(ctx, "send_confirmation", nil, nil)
    }
}
```

## Integration Examples

### Email Sending Job

```go
type EmailPayload struct {
    To      string `json:"to"`
    Subject string `json:"subject"`
    Body    string `json:"body"`
}

registry.RegisterFunc("send_email", func(ctx context.Context, payload json.RawMessage) (interface{}, error) {
    var email EmailPayload
    if err := json.Unmarshal(payload, &email); err != nil {
        return nil, err
    }
    
    // Send email using SMTP
    if err := smtp.SendEmail(email.To, email.Subject, email.Body); err != nil {
        return nil, err
    }
    
    return map[string]string{
        "status":    "sent",
        "message_id": generateMessageID(),
    }, nil
})

// Usage
client.Enqueue(ctx, "send_email", EmailPayload{
    To:      "user@example.com",
    Subject: "Welcome!",
    Body:    "Thanks for signing up",
}, nil)
```

### Image Processing Job

```go
type ImageProcessPayload struct {
    SourceURL string   `json:"source_url"`
    Sizes     []string `json:"sizes"` // e.g., ["thumbnail", "medium", "large"]
}

registry.RegisterFunc("process_image", func(ctx context.Context, payload json.RawMessage) (interface{}, error) {
    var img ImageProcessPayload
    json.Unmarshal(payload, &img)
    
    // Download image
    data, _ := downloadImage(img.SourceURL)
    
    // Generate thumbnails
    results := make(map[string]string)
    for _, size := range img.Sizes {
        url, _ := resizeAndUpload(data, size)
        results[size] = url
    }
    
    return results, nil
})

// Enqueue with higher priority and longer timeout
config := &jobs.JobConfig{
    Priority:   10,
    Timeout:    30 * time.Minute,
    MaxRetries: 5,
}

client.Enqueue(ctx, "process_image", ImageProcessPayload{
    SourceURL: "https://example.com/photo.jpg",
    Sizes:     []string{"thumbnail", "medium", "large"},
}, config)
```

### Data Export Job

```go
registry.RegisterFunc("export_data", func(ctx context.Context, payload json.RawMessage) (interface{}, error) {
    var export struct {
        UserID int    `json:"user_id"`
        Format string `json:"format"` // csv, json, excel
    }
    json.Unmarshal(payload, &export)
    
    // Fetch data from database
    data, _ := db.GetUserData(export.UserID)
    
    // Generate file
    filename, _ := generateExport(data, export.Format)
    
    // Upload to storage
    url, _ := storage.Upload(filename)
    
    // Send notification
    notifyUser(export.UserID, url)
    
    return map[string]string{"url": url}, nil
})

// Schedule daily exports
scheduler.Register(&jobs.CronJob{
    ID:       "daily-export",
    Schedule: jobs.Daily(1, 0), // 1:00 AM
    JobType:  "export_data",
    Payload: map[string]interface{}{
        "user_id": 123,
        "format":  "csv",
    },
    Enabled: true,
})
```

## Best Practices

### 1. Idempotent Jobs

Make jobs idempotent so they can be safely retried:

```go
func handleProcessPayment(ctx context.Context, payload json.RawMessage) (interface{}, error) {
    var payment PaymentRequest
    json.Unmarshal(payload, &payment)
    
    // Check if already processed
    if isPaymentProcessed(payment.ID) {
        return map[string]string{"status": "already_processed"}, nil
    }
    
    // Process payment
    result, err := processPayment(payment)
    if err != nil {
        return nil, err
    }
    
    // Mark as processed
    markPaymentProcessed(payment.ID)
    
    return result, nil
}
```

### 2. Timeout Configuration

Set appropriate timeouts based on job type:

```go
// Quick jobs
quickConfig := &jobs.JobConfig{
    Timeout: 30 * time.Second,
}

// Long-running jobs
longConfig := &jobs.JobConfig{
    Timeout: 30 * time.Minute,
}
```

### 3. Graceful Shutdown

```go
// Handle shutdown signals
sigCh := make(chan os.Signal, 1)
signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

<-sigCh
log.Println("Shutting down...")

// Stop scheduler first
scheduler.Stop()

// Then stop workers
client.Stop()

// Close queue
queue.Close()
```

### 4. Monitor Job Performance

```go
import "your-project/internal/observability"

// Register health check
healthConfig.CustomChecks["jobs"] = func(ctx context.Context) (observability.HealthStatus, string, error) {
    stats, err := client.Stats(ctx)
    if err != nil {
        return observability.StatusUnhealthy, "Job queue unavailable", err
    }
    
    // Alert if too many pending jobs
    if stats.PendingJobs > 10000 {
        return observability.StatusDegraded, "High pending job count", nil
    }
    
    return observability.StatusHealthy, "Job processing healthy", nil
}
```

## Performance Tuning

### Worker Pool Size

```go
// CPU-intensive jobs
poolConfig := &jobs.WorkerPoolConfig{
    NumWorkers: runtime.NumCPU(),
}

// I/O-intensive jobs
poolConfig := &jobs.WorkerPoolConfig{
    NumWorkers: runtime.NumCPU() * 4,
}
```

### Redis Connection Pool

```go
redisClient := redis.NewClient(&redis.Options{
    Addr:     "localhost:6379",
    PoolSize: 50, // Increase for high throughput
})
```

### Batch Processing

```go
// Process multiple items in one job
type BatchPayload struct {
    Items []Item `json:"items"`
}

registry.RegisterFunc("batch_process", func(ctx context.Context, payload json.RawMessage) (interface{}, error) {
    var batch BatchPayload
    json.Unmarshal(payload, &batch)
    
    results := make([]Result, len(batch.Items))
    for i, item := range batch.Items {
        results[i] = processItem(item)
    }
    
    return results, nil
})
```

## Troubleshooting

### Jobs Not Processing

1. Check worker pool is started: `client.Start(ctx)`
2. Verify job handlers are registered
3. Check Redis connection
4. Review logs for errors

### High Failure Rate

1. Increase retry count and timeout
2. Check error logs for patterns
3. Make jobs idempotent
4. Add error handling in job handlers

### Performance Issues

1. Increase worker pool size
2. Optimize job handlers
3. Use batch processing
4. Scale horizontally with more workers

## License

Part of the goengine framework.
