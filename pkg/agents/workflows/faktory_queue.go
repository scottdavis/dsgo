package workflows

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	faktory "github.com/contribsys/faktory/client"
)

// FaktoryQueue implements JobQueue using Faktory
type FaktoryQueue struct {
	client  *faktory.Client
	config  *JobQueueConfig
}

// NewFaktoryQueue creates a new Faktory-backed job queue
func NewFaktoryQueue(faktoryURL string, config *JobQueueConfig) (*FaktoryQueue, error) {
	// Parse the URL to create a server config
	server := &faktory.Server{
		Address: faktoryURL,
	}
	
	// Handle auth if provided in URL (format: faktory:password@localhost:7419)
	if strings.Contains(faktoryURL, ":") && strings.Contains(faktoryURL, "@") {
		parts := strings.Split(faktoryURL, "@")
		if len(parts) == 2 {
			authParts := strings.Split(parts[0], ":")
			if len(authParts) == 2 {
				server.Password = authParts[1]
			}
			server.Address = parts[1]
		}
	}
	
	// Set default timeout
	server.Timeout = 5 * time.Second
	
	// Connect to Faktory
	client, err := server.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Faktory: %w", err)
	}
	
	return &FaktoryQueue{
		client: client,
		config: config,
	}, nil
}

// Push adds a job to the Faktory queue
func (q *FaktoryQueue) Push(ctx context.Context, jobID string, stepID string, payload map[string]any) error {
	// Serialize payload for Faktory
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to serialize payload: %w", err)
	}
	
	// Create Faktory job
	job := faktory.NewJob(stepID, string(payloadBytes))
	job.Queue = q.config.QueueName
	job.ReserveFor = q.config.JobTimeout
	
	// Set retry if specified (use pointer to allow nil)
	retry := q.config.RetryCount
	job.Retry = &retry
	
	// Set job ID
	job.Jid = jobID
	
	// Push to Faktory
	err = q.client.Push(job)
	if err != nil {
		return fmt.Errorf("failed to push job to Faktory: %w", err)
	}
	
	return nil
}

// Close releases Faktory connection
func (q *FaktoryQueue) Close() error {
	return q.client.Close()
} 