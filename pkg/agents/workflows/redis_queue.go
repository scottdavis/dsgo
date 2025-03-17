package workflows

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisQueue implements JobQueue using Redis
type RedisQueue struct {
	client    *redis.Client
	config    *JobQueueConfig
	namespace string
}

// NewRedisQueue creates a new Redis-backed job queue
func NewRedisQueue(redisAddr string, password string, db int, config *JobQueueConfig) (*RedisQueue, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: password,
		DB:       db,
	})
	
	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	if _, err := client.Ping(ctx).Result(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}
	
	return &RedisQueue{
		client:    client,
		config:    config,
		namespace: "dsgo:async:jobs:",
	}, nil
}

// Push adds a job to the Redis queue
func (q *RedisQueue) Push(ctx context.Context, jobID string, stepID string, payload map[string]any) error {
	// Create job structure
	job := map[string]any{
		"id":        jobID,
		"step_id":   stepID,
		"payload":   payload,
		"queue":     q.config.QueueName,
		"retry":     q.config.RetryCount,
		"timeout":   q.config.JobTimeout,
		"enqueued":  time.Now().UTC().Format(time.RFC3339),
	}
	
	// Serialize job
	jobBytes, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to serialize job: %w", err)
	}
	
	// Push to Redis list
	queueKey := q.namespace + q.config.QueueName
	if err := q.client.RPush(ctx, queueKey, jobBytes).Err(); err != nil {
		return fmt.Errorf("failed to push job to Redis: %w", err)
	}
	
	return nil
}

// Pop retrieves the next job from the Redis queue
func (q *RedisQueue) Pop(ctx context.Context) (*Job, error) {
	queueKey := q.namespace + q.config.QueueName
	
	// Use BLPOP to get the next job (with timeout)
	result, err := q.client.BLPop(ctx, 1*time.Second, queueKey).Result()
	if err != nil {
		if err == redis.Nil {
			// No jobs available
			return nil, nil
		}
		return nil, fmt.Errorf("failed to pop job from Redis: %w", err)
	}
	
	// If we got here, we have a job
	if len(result) < 2 {
		return nil, fmt.Errorf("invalid job format from Redis")
	}
	
	// Parse job data
	jobBytes := result[1]
	var jobData map[string]any
	if err := json.Unmarshal([]byte(jobBytes), &jobData); err != nil {
		return nil, fmt.Errorf("failed to deserialize job: %w", err)
	}
	
	// Create job object
	job := &Job{
		ID:    jobData["id"].(string),
		StepID: jobData["step_id"].(string),
		Queue: jobData["queue"].(string),
	}
	
	// Get payload
	if payload, ok := jobData["payload"].(map[string]any); ok {
		job.Payload = payload
	} else {
		job.Payload = make(map[string]any)
	}
	
	// Get other fields if present
	if retry, ok := jobData["retry"].(float64); ok {
		job.Retry = int(retry)
	}
	
	if timeout, ok := jobData["timeout"].(float64); ok {
		job.Timeout = int(timeout)
	}
	
	if enqueued, ok := jobData["enqueued"].(string); ok {
		if t, err := time.Parse(time.RFC3339, enqueued); err == nil {
			job.EnqueuedAt = t
		}
	}
	
	return job, nil
}

// Ping verifies connectivity to the Redis server
func (q *RedisQueue) Ping() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	if _, err := q.client.Ping(ctx).Result(); err != nil {
		return fmt.Errorf("failed to ping Redis: %w", err)
	}
	
	return nil
}

// Close releases Redis connection
func (q *RedisQueue) Close() error {
	return q.client.Close()
} 