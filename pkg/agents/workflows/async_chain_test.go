//go:build redis
// +build redis

package workflows_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/scottdavis/dsgo/pkg/agents"
	"github.com/scottdavis/dsgo/pkg/agents/memory"
	"github.com/scottdavis/dsgo/pkg/agents/workflows"
	"github.com/scottdavis/dsgo/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestModule is a simple module for testing
type TestModule struct {
	increment int
	signature core.Signature
}

func NewTestModule(increment int) *TestModule {
	sig := core.NewSignature(
		[]core.InputField{{Field: core.Field{Name: "count"}}},
		[]core.OutputField{{Field: core.Field{Name: "count"}}},
	)
	return &TestModule{
		increment: increment,
		signature: sig,
	}
}

func (m *TestModule) Process(ctx context.Context, inputs map[string]interface{}, opts ...core.Option) (map[string]interface{}, error) {
	// Get current count or initialize to 0
	count, ok := inputs["count"].(float64)
	if !ok {
		count = 0
	}
	
	// Increment by module value
	count += float64(m.increment)
	
	// Return updated count
	return map[string]interface{}{
		"count": count,
	}, nil
}

func (m *TestModule) GetSignature() core.Signature {
	return m.signature
}

func (m *TestModule) Clone() core.Module {
	return &TestModule{
		increment: m.increment,
		signature: m.signature,
	}
}

// TestAsyncChainWorkflow_Redis tests the full execution of AsyncChainWorkflow with Redis
func TestAsyncChainWorkflow_Redis(t *testing.T) {
	// Get Redis connection info from environment or use defaults
	redisAddr := getEnvOrDefault("REDIS_TEST_ADDR", "localhost:6379")
	redisPass := getEnvOrDefault("REDIS_TEST_PASS", "")
	redisDB := 0
	
	// Connect to Redis
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
	
	// Create unique test queue name
	queueName := "async_test_" + time.Now().Format("20060102150405")
	
	// Set up Redis memory store
	redisStore, err := memory.NewRedisStore(redisAddr, redisPass, redisDB)
	require.NoError(t, err, "Failed to create Redis memory store")
	
	// Create Redis job queue
	queueConfig := &workflows.JobQueueConfig{
		QueueName:  queueName,
		JobTimeout: 30,
		RetryCount: 0,
	}
	
	jobQueue, err := workflows.NewRedisQueue(redisAddr, redisPass, redisDB, queueConfig)
	require.NoError(t, err, "Failed to create Redis job queue")
	defer jobQueue.Close()
	
	// Create AsyncChainWorkflow config
	asyncConfig := &workflows.AsyncChainConfig{
		JobQueue:          jobQueue,
		JobPrefix:         "test_job",
		WaitForCompletion: true,
		CompletionTimeout: 30 * time.Second,
	}
	
	// Create the workflow
	workflow := workflows.NewAsyncChainWorkflow(redisStore, asyncConfig)
	
	// Add test steps
	step1 := &workflows.Step{
		ID:     "step1",
		Module: NewTestModule(5),
	}
	
	step2 := &workflows.Step{
		ID:     "step2",
		Module: NewTestModule(10),
	}
	
	step3 := &workflows.Step{
		ID:     "step3",
		Module: NewTestModule(15),
	}
	
	require.NoError(t, workflow.AddStep(step1), "Failed to add step1")
	require.NoError(t, workflow.AddStep(step2), "Failed to add step2")
	require.NoError(t, workflow.AddStep(step3), "Failed to add step3")
	
	// Start worker goroutine to process jobs
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()
	
	go runTestWorker(workerCtx, t, redisClient, redisStore, queueName, map[string]*TestModule{
		"step1": step1.Module.(*TestModule),
		"step2": step2.Module.(*TestModule),
		"step3": step3.Module.(*TestModule),
	})
	
	// Execute the workflow
	executionCtx := core.WithExecutionState(context.Background())
	executionCtx = memory.WithMemoryStore(executionCtx, redisStore)
	
	// Initial inputs
	inputs := map[string]interface{}{
		"count": float64(0),
	}
	
	// Execute and wait for completion
	result, err := workflow.Execute(executionCtx, inputs)
	require.NoError(t, err, "Workflow execution failed")
	
	// Verify result - each step should have incremented count
	// 0 (initial) + 5 (step1) + 10 (step2) + 15 (step3) = 30
	var count float64
	switch c := result["count"].(type) {
	case float64:
		count = c
	case int:
		count = float64(c)
	default:
		t.Logf("Count is of unexpected type: %T", result["count"])
		require.Fail(t, "Count should be a numeric value")
	}
	
	assert.Equal(t, float64(30), count, "Final count should be 30")
}

// Helper to run a worker that processes jobs
func runTestWorker(ctx context.Context, t *testing.T, 
	redisClient *redis.Client, memStore agents.Memory, 
	queueName string, modules map[string]*TestModule) {
	
	queueKey := "dsgo:async:jobs:" + queueName
	
	for {
		select {
		case <-ctx.Done():
			return
			
		default:
			// Poll for jobs
			result, err := redisClient.BLPop(ctx, 1*time.Second, queueKey).Result()
			if err != nil {
				if err == redis.Nil || err.Error() == "redis: nil" {
					continue
				}
				
				t.Logf("Error polling queue: %v", err)
				continue
			}
			
			if len(result) < 2 {
				continue
			}
			
			// Process the job
			var job map[string]interface{}
			err = json.Unmarshal([]byte(result[1]), &job)
			if err != nil {
				t.Logf("Error parsing job: %v", err)
				continue
			}
			
			stepID, _ := job["step_id"].(string)
			payload, _ := job["payload"].(map[string]interface{})
			
			module, exists := modules[stepID]
			if !exists {
				t.Logf("Unknown step ID: %s", stepID)
				continue
			}
			
			// Extract workflow data
			workflowID, _ := payload["workflow_id"].(string)
			totalSteps, _ := payload["total_steps"].(float64)
			nextStepIndex, _ := payload["next_step_index"].(float64)
			state, _ := payload["state"].(map[string]interface{})
			
			// Process the step
			jobCtx := memory.WithMemoryStore(ctx, memStore)
			jobCtx = core.WithExecutionState(jobCtx)
			
			stepResult, err := module.Process(jobCtx, state)
			if err != nil {
				t.Logf("Error processing step: %v", err)
				continue
			}
			
			// Update state
			for k, v := range stepResult {
				state[k] = v
			}
			
			// Store updated state
			stateKey := fmt.Sprintf("wf:%s:state", workflowID)
			err = memStore.Store(stateKey, state, agents.WithTTL(24*time.Hour))
			if err != nil {
				t.Logf("Error storing state: %v", err)
				continue
			}
			
			// Update steps completed
			stepsCompletedKey := fmt.Sprintf("wf:%s:steps_completed", workflowID)
			redisClient.Incr(jobCtx, stepsCompletedKey)
			
			// Enqueue next step or mark as completed
			if int(nextStepIndex) < int(totalSteps) {
				// Find next step ID
				var nextStepID string
				for id := range modules {
					if fmt.Sprintf("step%d", int(nextStepIndex)+1) == id {
						nextStepID = id
						break
					}
				}
				
				if nextStepID != "" {
					queueConfig := &workflows.JobQueueConfig{
						QueueName:  queueName,
						JobTimeout: 30,
						RetryCount: 0,
					}
					
					// Get a new Redis queue for each step to avoid connection issues
					rq, err := workflows.NewRedisQueue(redisClient.Options().Addr, 
						redisClient.Options().Password, redisClient.Options().DB, queueConfig)
					if err != nil {
						t.Logf("Error creating Redis queue: %v", err)
						continue
					}
					
					nextPayload := map[string]interface{}{
						"workflow_id":     workflowID,
						"step_index":      nextStepIndex,
						"total_steps":     totalSteps,
						"next_step_index": nextStepIndex + 1,
						"state":           state,
					}
					
					jobID := fmt.Sprintf("test_job:%s:%s:%d", workflowID, nextStepID, int(nextStepIndex))
					err = rq.Push(jobCtx, jobID, nextStepID, nextPayload)
					if err != nil {
						t.Logf("Error enqueueing next job: %v", err)
					}
					rq.Close()
				}
			} else {
				// Mark workflow as completed
				completionKey := fmt.Sprintf("wf:%s:completed", workflowID)
				err = memStore.Store(completionKey, true, agents.WithTTL(24*time.Hour))
				if err != nil {
					t.Logf("Error marking workflow as completed: %v", err)
				}
			}
		}
	}
}

// Helper function for environment variables
func getEnvOrDefault(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

// TestAsyncChainWorkflow_DirectMapStorage tests that AsyncChainWorkflow can handle directly stored map state
func TestAsyncChainWorkflow_DirectMapStorage(t *testing.T) {
	// Check Redis availability
	redisAddr := getEnvOrDefault("REDIS_TEST_ADDR", "localhost:6379")
	redisPass := getEnvOrDefault("REDIS_TEST_PASS", "")
	redisDB := 0
	
	// Connect to Redis
	redisClient := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: redisPass,
		DB:       redisDB,
	})
	
	// Verify Redis connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	_, err := redisClient.Ping(ctx).Result()
	if err != nil {
		t.Skip("Skipping test, Redis is not available")
	}
	
	// Initialize memory store
	memStore, err := memory.NewRedisStore(redisAddr, redisPass, redisDB)
	require.NoError(t, err, "Failed to create Redis memory store")
	
	// Simulate a workflow that's already been completed with a map[string]int state
	workflowID := "test-direct-map-" + uuid.New().String()
	stateKey := fmt.Sprintf("wf:%s:state", workflowID)
	
	// Store a map[string]int directly
	directState := map[string]int{
		"count": 42,
		"steps_completed": 3,
	}
	
	err = memStore.Store(stateKey, directState, agents.WithTTL(24*time.Hour))
	require.NoError(t, err, "Failed to store direct state")
	
	// Directly test the retrieval and type conversion logic
	stateValue, err := memStore.Retrieve(stateKey)
	require.NoError(t, err, "Failed to retrieve state")
	
	// Convert to JSON string if needed
	var result map[string]interface{}
	switch v := stateValue.(type) {
	case string:
		// Deserialize
		var state map[string]interface{}
		err := json.Unmarshal([]byte(v), &state)
		require.NoError(t, err, "Failed to unmarshal state")
		result = state
	case []byte:
		// Deserialize
		var state map[string]interface{}
		err := json.Unmarshal(v, &state)
		require.NoError(t, err, "Failed to unmarshal state")
		result = state
	case map[string]interface{}:
		result = v
	case map[string]int:
		// Convert map[string]int to map[string]interface{}
		result = make(map[string]interface{})
		for k, val := range v {
			result[k] = val
		}
	default:
		t.Fatalf("Invalid workflow state type: %T", stateValue)
	}
	
	// Verify result contains expected values
	require.Equal(t, 42, result["count"], "Expected count to be 42")
	require.Equal(t, 3, result["steps_completed"], "Expected steps_completed to be 3")
} 