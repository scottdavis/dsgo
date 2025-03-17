package workflows

import (
	"context"
)

// JobQueue represents a backend for async job processing
type JobQueue interface {
	// Push adds a job to the queue with the given ID and payload
	Push(ctx context.Context, jobID string, stepID string, payload map[string]any) error
	
	// Close releases any resources used by the queue
	Close() error
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