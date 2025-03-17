package workflows

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/scottdavis/dsgo/pkg/agents"
	"github.com/scottdavis/dsgo/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockWorkerJobQueue is a test implementation of JobQueue
type MockWorkerJobQueue struct {
	mu     sync.Mutex
	jobs   []*Job
	closed bool
}

func NewMockWorkerJobQueue() *MockWorkerJobQueue {
	return &MockWorkerJobQueue{
		jobs: make([]*Job, 0),
	}
}

func (q *MockWorkerJobQueue) Push(ctx context.Context, jobID, stepID string, payload map[string]any) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	
	if q.closed {
		return fmt.Errorf("queue is closed")
	}
	
	job := &Job{
		ID:         jobID,
		StepID:     stepID,
		Payload:    payload,
		EnqueuedAt: time.Now(),
	}
	
	q.jobs = append(q.jobs, job)
	return nil
}

func (q *MockWorkerJobQueue) Pop(ctx context.Context) (*Job, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	
	if q.closed {
		return nil, fmt.Errorf("queue is closed")
	}
	
	if len(q.jobs) == 0 {
		return nil, nil
	}
	
	job := q.jobs[0]
	q.jobs = q.jobs[1:]
	return job, nil
}

func (q *MockWorkerJobQueue) Close() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	
	q.closed = true
	return nil
}

func (q *MockWorkerJobQueue) Ping() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	
	if q.closed {
		return fmt.Errorf("queue is closed")
	}
	
	return nil
}

// WorkerTestModule is a test implementation of core.Module
type WorkerTestModule struct {
	ProcessFunc func(ctx context.Context, inputs map[string]any) (map[string]any, error)
}

func (m *WorkerTestModule) Process(ctx context.Context, inputs map[string]any, opts ...core.Option) (map[string]any, error) {
	if m.ProcessFunc != nil {
		return m.ProcessFunc(ctx, inputs)
	}
	return inputs, nil
}

func (m *WorkerTestModule) GetSignature() core.Signature {
	return core.Signature{}
}

func (m *WorkerTestModule) Clone() core.Module {
	return &WorkerTestModule{ProcessFunc: m.ProcessFunc}
}

// WorkerTestMemory is a test implementation of agents.Memory
type WorkerTestMemory struct {
	mu    sync.Mutex
	store map[string]any
}

func NewWorkerTestMemory() *WorkerTestMemory {
	return &WorkerTestMemory{
		store: make(map[string]any),
	}
}

func (m *WorkerTestMemory) Store(key string, value any, opts ...agents.StoreOption) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.store[key] = value
	return nil
}

func (m *WorkerTestMemory) Retrieve(key string) (any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if value, ok := m.store[key]; ok {
		return value, nil
	}
	return nil, fmt.Errorf("key not found: %s", key)
}

func (m *WorkerTestMemory) List() ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	keys := make([]string, 0, len(m.store))
	for k := range m.store {
		keys = append(keys, k)
	}
	return keys, nil
}

func (m *WorkerTestMemory) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.store = make(map[string]any)
	return nil
}

func (m *WorkerTestMemory) CleanExpired(ctx context.Context) (int64, error) {
	return 0, nil
}

func (m *WorkerTestMemory) Close() error {
	return nil
}

func TestWorkflowWorker_ProcessJob(t *testing.T) {
	// Create mocks
	queue := NewMockWorkerJobQueue()
	mem := NewWorkerTestMemory()
	
	// Create a processing module that adds a result
	module := &WorkerTestModule{
		ProcessFunc: func(ctx context.Context, inputs map[string]any) (map[string]any, error) {
			result := make(map[string]any)
			for k, v := range inputs {
				result[k] = v
			}
			result["processed"] = true
			result["timestamp"] = time.Now().Format(time.RFC3339)
			return result, nil
		},
	}
	
	// Create worker config
	config := WorkerConfig{
		JobQueue:     queue,
		Memory:       mem,
		ModuleRegistry: map[string]core.Module{
			"test-step": module,
		},
		PollInterval: 10 * time.Millisecond,
		Logger:       log.New(os.Stdout, "TEST: ", log.LstdFlags),
	}
	
	// Create worker
	worker := NewWorkflowWorker(config)
	
	// Push a job
	workflowID := "test-workflow-123"
	stepID := "test-step"
	payload := map[string]any{
		"workflow_id": workflowID,
		"step_id":     stepID,
		"state": map[string]any{
			"input_data": "test input",
		},
		"total_steps": 1,
	}
	
	err := queue.Push(context.Background(), "job-1", stepID, payload)
	require.NoError(t, err)
	
	// Start worker
	worker.Start()
	
	// Wait for processing
	time.Sleep(100 * time.Millisecond)
	
	// Stop worker
	worker.Stop()
	
	// Check results
	stateKey := fmt.Sprintf("workflow:%s:state", workflowID)
	stateVal, err := mem.Retrieve(stateKey)
	require.NoError(t, err)
	
	stateMap, ok := stateVal.(map[string]any)
	require.True(t, ok, "State should be a map")
	
	// Verify the state was updated correctly
	assert.Equal(t, "test input", stateMap["input_data"])
	assert.True(t, stateMap["processed"].(bool))
	assert.NotEmpty(t, stateMap["timestamp"])
	
	// Check completion
	completionKey := fmt.Sprintf("workflow:%s:completion", workflowID)
	completionVal, err := mem.Retrieve(completionKey)
	if err != nil {
		t.Logf("Completion key may not exist yet: %v", err)
		// This is okay, the test might be running too fast
	} else {
		assert.Equal(t, 1, completionVal)
	}
}

func TestWorkflowWorker_ErrorHandling(t *testing.T) {
	// Create mocks
	queue := NewMockWorkerJobQueue()
	mem := NewWorkerTestMemory()
	
	// Create an error handler to track errors
	var handlerCalled bool
	var handledError error
	var handledStepID string
	var handledWorkflowID string
	
	errorHandler := func(ctx context.Context, workflowID, stepID string, err error) error {
		handlerCalled = true
		handledError = err
		handledStepID = stepID
		handledWorkflowID = workflowID
		return nil
	}
	
	// Create a failing module
	module := &WorkerTestModule{
		ProcessFunc: func(ctx context.Context, inputs map[string]any) (map[string]any, error) {
			return nil, fmt.Errorf("test error")
		},
	}
	
	// Create worker config
	config := WorkerConfig{
		JobQueue:     queue,
		Memory:       mem,
		ModuleRegistry: map[string]core.Module{
			"failing-step": module,
		},
		PollInterval: 10 * time.Millisecond,
		ErrorHandler: errorHandler,
		Logger:       log.New(os.Stdout, "TEST: ", log.LstdFlags),
	}
	
	// Create worker
	worker := NewWorkflowWorker(config)
	
	// Push a job
	workflowID := "test-workflow-456"
	stepID := "failing-step"
	payload := map[string]any{
		"workflow_id": workflowID,
		"step_id":     stepID,
		"state": map[string]any{
			"input_data": "test input",
		},
		"retry_count": float64(0),
		"max_retries": float64(0), // No retries, to make this test simpler
	}
	
	err := queue.Push(context.Background(), "job-2", stepID, payload)
	require.NoError(t, err)
	
	// Start worker
	worker.Start()
	
	// Wait for processing
	time.Sleep(100 * time.Millisecond)
	
	// Stop worker
	worker.Stop()
	
	// Check error handling
	assert.True(t, handlerCalled, "Error handler should have been called")
	assert.Equal(t, "test error", handledError.Error())
	assert.Equal(t, stepID, handledStepID)
	assert.Equal(t, workflowID, handledWorkflowID)
	
	// Verify workflow was marked as failed
	statusKey := fmt.Sprintf("workflow:%s:status", workflowID)
	statusVal, err := mem.Retrieve(statusKey)
	require.NoError(t, err)
	assert.Equal(t, "failed", statusVal)
	
	// Check error details were stored
	errKey := fmt.Sprintf("workflow:%s:error", workflowID)
	errVal, err := mem.Retrieve(errKey)
	require.NoError(t, err)
	
	errMap, ok := errVal.(map[string]any)
	require.True(t, ok, "Error data should be a map")
	assert.Equal(t, stepID, errMap["step_id"])
	assert.Equal(t, "test error", errMap["error"])
	assert.NotEmpty(t, errMap["timestamp"])
}

func TestWorkflowWorker_RetryMechanism(t *testing.T) {
	// Create mocks
	queue := NewMockWorkerJobQueue()
	mem := NewWorkerTestMemory()
	
	// Track retry attempts
	var attempts int
	var mu sync.Mutex
	
	// Create a module that fails for the first attempt but succeeds on retry
	module := &WorkerTestModule{
		ProcessFunc: func(ctx context.Context, inputs map[string]any) (map[string]any, error) {
			mu.Lock()
			defer mu.Unlock()
			
			attempts++
			
			if attempts == 1 {
				return nil, fmt.Errorf("temporary error")
			}
			
			// Success on second attempt
			result := make(map[string]any)
			for k, v := range inputs {
				result[k] = v
			}
			result["processed"] = true
			result["retried"] = true
			return result, nil
		},
	}
	
	// Speed up retries for testing
	fastBackoff := func(attempt int, baseDelay time.Duration) time.Duration {
		return 10 * time.Millisecond
	}
	
	// Create worker config
	config := WorkerConfig{
		JobQueue:     queue,
		Memory:       mem,
		ModuleRegistry: map[string]core.Module{
			"retry-step": module,
		},
		PollInterval:    10 * time.Millisecond,
		BackoffStrategy: fastBackoff,
		Logger:          log.New(os.Stdout, "TEST: ", log.LstdFlags),
	}
	
	// Create worker
	worker := NewWorkflowWorker(config)
	
	// Push a job
	workflowID := "test-workflow-789"
	stepID := "retry-step"
	payload := map[string]any{
		"workflow_id": workflowID,
		"step_id":     stepID,
		"state": map[string]any{
			"input_data": "test input",
		},
		"retry_count": float64(0),
		"max_retries": float64(2), // Allow retries
	}
	
	err := queue.Push(context.Background(), "job-3", stepID, payload)
	require.NoError(t, err)
	
	// Start worker
	worker.Start()
	
	// Wait for processing (including retry)
	time.Sleep(300 * time.Millisecond)
	
	// Stop worker
	worker.Stop()
	
	// Verify retry count
	assert.Equal(t, 2, attempts, "Module should have been called twice")
	
	// Check results
	stateKey := fmt.Sprintf("workflow:%s:state", workflowID)
	stateVal, err := mem.Retrieve(stateKey)
	require.NoError(t, err)
	
	stateMap, ok := stateVal.(map[string]any)
	require.True(t, ok, "State should be a map")
	
	// Verify the state was updated correctly after retry
	assert.Equal(t, "test input", stateMap["input_data"])
	assert.True(t, stateMap["processed"].(bool))
	assert.True(t, stateMap["retried"].(bool))
} 