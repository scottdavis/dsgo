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

// Close releases Redis connection
func (q *RedisQueue) Close() error {
	return q.client.Close()
} 