//go:build redis
// +build redis

package workflows_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/scottdavis/dsgo/pkg/agents/workflows"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedisQueue(t *testing.T) {
	// Get Redis connection info from environment or use defaults
	redisAddr := getEnvOrDefault("REDIS_TEST_ADDR", "localhost:6379")
	redisPass := getEnvOrDefault("REDIS_TEST_PASS", "")
	redisDB := 0
	
	// Create a direct Redis client for verification
	redisClient := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: redisPass,
		DB:       redisDB,
	})
	
	// Verify Redis connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	_, err := redisClient.Ping(ctx).Result()
	require.NoError(t, err, "Failed to connect to Redis")
	defer redisClient.Close()
	
	// Create a unique test queue name
	queueName := "redis_test_" + time.Now().Format("20060102150405")
	
	// Create queue config
	queueConfig := &workflows.JobQueueConfig{
		QueueName:  queueName,
		JobTimeout: 30,
		RetryCount: 3,
	}
	
	// Create Redis queue
	queue, err := workflows.NewRedisQueue(redisAddr, redisPass, redisDB, queueConfig)
	require.NoError(t, err, "Failed to create Redis queue")
	defer queue.Close()
	
	// Test push job
	jobID := "test_job_1"
	stepID := "test_step"
	payload := map[string]interface{}{
		"data": "test data",
		"timestamp": time.Now().Unix(),
	}
	
	err = queue.Push(ctx, jobID, stepID, payload)
	require.NoError(t, err, "Failed to push job to Redis queue")
	
	// Verify the job is in the queue using the Redis client directly
	queueKey := "dsgo:async:jobs:" + queueName
	result, err := redisClient.LIndex(ctx, queueKey, 0).Result()
	require.NoError(t, err, "Failed to retrieve job from Redis")
	
	// Decode the job
	var job map[string]interface{}
	err = json.Unmarshal([]byte(result), &job)
	require.NoError(t, err, "Failed to parse job JSON")
	
	// Verify job contents
	assert.Equal(t, jobID, job["id"], "Job ID mismatch")
	assert.Equal(t, stepID, job["step_id"], "Step ID mismatch")
	
	// Verify job payload
	jobPayload, ok := job["payload"].(map[string]interface{})
	require.True(t, ok, "Job payload should be a map")
	assert.Equal(t, "test data", jobPayload["data"], "Payload data mismatch")
	
	// Verify job metadata
	assert.NotNil(t, job["enqueued"], "Enqueued time should be set")
	assert.Equal(t, queueConfig.RetryCount, int(job["retry"].(float64)), "Retry count mismatch")
	assert.Equal(t, queueConfig.JobTimeout, int(job["timeout"].(float64)), "Job timeout mismatch")
	
	// Test pop job (simulate a worker fetching the job)
	popCtx, popCancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer popCancel()
	
	results, err := redisClient.BLPop(popCtx, 1*time.Second, queueKey).Result()
	require.NoError(t, err, "Failed to pop job from Redis queue")
	require.Len(t, results, 2, "Expected queue name and value in BLPop result")
	
	// Result contains both the queue name and the value, we want the value (index 1)
	result = results[1]
	
	// Decode the popped job
	var poppedJob map[string]interface{}
	err = json.Unmarshal([]byte(result), &poppedJob)
	require.NoError(t, err, "Failed to parse popped job JSON")
	
	// Verify popped job is the same as the one we pushed
	assert.Equal(t, jobID, poppedJob["id"], "Popped job ID mismatch")
	assert.Equal(t, stepID, poppedJob["step_id"], "Popped step ID mismatch")
} 