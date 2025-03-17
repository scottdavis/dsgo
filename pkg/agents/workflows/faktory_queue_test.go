//go:build faktory
// +build faktory

package workflows_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/contribsys/faktory/client"
	"github.com/scottdavis/dsgo/pkg/agents/workflows"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFaktoryQueue(t *testing.T) {
	// Get Faktory URL from environment or use default
	faktoryURL := getEnvOrDefault("FAKTORY_URL", "localhost:7419")
	
	// Create a queue config
	queueConfig := &workflows.JobQueueConfig{
		QueueName:  "test_queue",
		JobTimeout: 30,
		RetryCount: 0,
	}
	
	// Create a Faktory queue
	queue, err := workflows.NewFaktoryQueue(faktoryURL, queueConfig)
	require.NoError(t, err, "Failed to create Faktory queue")
	defer queue.Close()
	
	// No Ping method in FaktoryQueue, we'll verify connection through job push
	// Test push job
	jobID := "test_job_1"
	stepID := "test_step"
	payload := map[string]interface{}{
		"data": "test data",
		"timestamp": time.Now().Unix(),
	}
	
	ctx := context.Background()
	err = queue.Push(ctx, jobID, stepID, payload)
	require.NoError(t, err, "Failed to push job to Faktory")
	
	// Create a client to fetch and verify the job
	// Parse the URL to get host and port
	server := &client.Server{
		Network: "tcp",
		Address: faktoryURL,
		// No password for test server
	}
	
	c, err := server.Open()
	require.NoError(t, err, "Failed to connect to Faktory server")
	defer c.Close()
	
	// Fetch job and verify contents
	queues := []string{queueConfig.QueueName}
	job, err := c.Fetch(queues...)
	require.NoError(t, err, "Failed to fetch job from Faktory")
	
	if job == nil {
		t.Fatal("Expected to fetch a job but got nil")
	}
	
	// Verify job metadata
	assert.Equal(t, jobID, job.Jid, "Job ID mismatch")
	assert.Equal(t, stepID, job.Type, "Job type (step ID) mismatch")
	assert.Equal(t, queueConfig.QueueName, job.Queue, "Queue name mismatch")
	
	// Check retry count, which is a pointer to an int
	if job.Retry != nil {
		assert.Equal(t, queueConfig.RetryCount, *job.Retry, "Retry count value mismatch")
	} else {
		t.Error("Expected retry count to be set, but it was nil")
	}
	
	// Verify job payload
	for _, arg := range job.Args {
		if argMap, ok := arg.(map[string]interface{}); ok {
			assert.Contains(t, argMap, "data", "Job payload missing 'data' field")
			assert.Contains(t, argMap, "timestamp", "Job payload missing 'timestamp' field")
		}
	}
	
	// Acknowledge the job
	err = c.Ack(job.Jid)
	require.NoError(t, err, "Failed to acknowledge job")
}

// Helper function for environment variables (reused from async_chain_test.go)
func getEnvOrDefault(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
} 