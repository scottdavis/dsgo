package workflows

import (
	"context"
	"time"
)

// Job represents a unit of work in the workflow system
type Job struct {
	// ID is the unique identifier for this job
	ID string
	
	// StepID is the ID of the workflow step this job represents
	StepID string
	
	// Payload contains all data needed to process the job
	Payload map[string]any
	
	// Queue is the name of the queue this job belongs to
	Queue string
	
	// EnqueuedAt is when this job was pushed to the queue
	EnqueuedAt time.Time
	
	// Retry is the number of times this job can be retried
	Retry int
	
	// Timeout is the maximum time this job can take to process
	Timeout int
}

// JobQueue represents a backend for async job processing
type JobQueue interface {
	// Push adds a job to the queue with the given ID and payload
	Push(ctx context.Context, jobID string, stepID string, payload map[string]any) error
	
	// Pop retrieves the next job from the queue
	Pop(ctx context.Context) (*Job, error)
	
	// Close releases any resources used by the queue
	Close() error
	
	// Ping verifies connectivity to the queue backend
	Ping() error
}

// JobQueueConfig holds common configuration for job queues
type JobQueueConfig struct {
	// QueueName is the name of the queue to use
	QueueName string
	
	// JobTimeout is the time in seconds before a job is considered failed
	JobTimeout int
	
	// RetryCount is the number of times to retry a failed job
	RetryCount int
}

// QueueStats provides statistics about a job queue
type QueueStats struct {
	// QueueName is the name of the queue
	QueueName string
	
	// Count is the number of jobs in the queue
	Count int64
	
	// ProcessedCount is the number of jobs processed
	ProcessedCount int64
	
	// FailedCount is the number of jobs that failed
	FailedCount int64
	
	// Timestamp is when these stats were collected
	Timestamp time.Time
} 