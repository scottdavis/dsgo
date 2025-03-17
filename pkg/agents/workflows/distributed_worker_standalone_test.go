//go:build distribworker
// +build distribworker

package workflows_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scottdavis/dsgo/pkg/agents"
	"github.com/scottdavis/dsgo/pkg/agents/memory"
	"github.com/scottdavis/dsgo/pkg/agents/workflows"
	"github.com/scottdavis/dsgo/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestModule implements core.Module for testing
type TestModule struct {
	Signature core.Signature
	ProcessFn func(ctx context.Context, input map[string]any, opts ...core.Option) (map[string]any, error)
}

func (m *TestModule) GetSignature() core.Signature {
	return m.Signature
}

func (m *TestModule) Process(ctx context.Context, input map[string]any, opts ...core.Option) (map[string]any, error) {
	if m.ProcessFn != nil {
		return m.ProcessFn(ctx, input, opts...)
	}
	// Echo the input by default
	return input, nil
}

func (m *TestModule) Clone() core.Module {
	return &TestModule{
		Signature: m.Signature,
		ProcessFn: m.ProcessFn,
	}
}

// testMemory is a simple in-memory implementation for testing
type testMemory struct {
	data map[string]interface{}
	mu   sync.Mutex
}

func (m *testMemory) Store(key string, value interface{}, opts ...agents.StoreOption) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil {
		m.data = make(map[string]interface{})
	}
	m.data[key] = value
	return nil
}

func (m *testMemory) Retrieve(key string) (interface{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil {
		return nil, errors.New("key not found")
	}
	val, ok := m.data[key]
	if !ok {
		return nil, errors.New("key not found")
	}
	return val, nil
}

func (m *testMemory) List() ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	keys := make([]string, 0, len(m.data))
	for k := range m.data {
		keys = append(keys, k)
	}
	return keys, nil
}

func (m *testMemory) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data = make(map[string]interface{})
	return nil
}

func (m *testMemory) CleanExpired(ctx context.Context) (int64, error) {
	return 0, nil
}

func (m *testMemory) Close() error {
	return nil
}

// testListMemory extends testMemory to implement memory.ListMemory
type testListMemory struct {
	testMemory
	lists map[string][]interface{}
}

func (m *testListMemory) PushList(key string, value interface{}, opts ...agents.StoreOption) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.lists == nil {
		m.lists = make(map[string][]interface{})
	}
	m.lists[key] = append(m.lists[key], value)
	return nil
}

func (m *testListMemory) PopList(key string) (interface{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.lists == nil {
		m.lists = make(map[string][]interface{})
		return nil, nil
	}
	list := m.lists[key]
	if len(list) == 0 {
		return nil, nil
	}
	item := list[0]
	m.lists[key] = list[1:]
	return item, nil
}

func (m *testListMemory) RemoveFromList(key string, value interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.lists == nil {
		m.lists = make(map[string][]any)
		return nil
	}
	list, exists := m.lists[key]
	if !exists || len(list) == 0 {
		return nil
	}
	
	// Convert value to string for comparison
	valueBytes, _ := json.Marshal(value)
	valueStr := string(valueBytes)
	
	newList := make([]interface{}, 0, len(list))
	for _, item := range list {
		itemBytes, _ := json.Marshal(item)
		if string(itemBytes) != valueStr {
			newList = append(newList, item)
		}
	}
	m.lists[key] = newList
	return nil
}

func (m *testListMemory) ListLength(key string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.lists == nil {
		return 0, nil
	}
	return len(m.lists[key]), nil
}

func (m *testListMemory) ListItems(key string, start, end int) ([]interface{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.lists == nil {
		return nil, nil
	}
	list := m.lists[key]
	if len(list) == 0 {
		return nil, nil
	}
	
	if end < 0 || end >= len(list) {
		end = len(list) - 1
	}
	if start < 0 {
		start = 0
	}
	if start > end {
		return nil, nil
	}
	
	return list[start : end+1], nil
}

func setupWorkerTest(t *testing.T) (*workflows.DistributedWorker, *agents.ModuleRegistry, memory.ListMemory) {
	// Create a memory store that implements ListMemory
	memStore := &testListMemory{}
	
	// Clean the store to ensure no data from previous tests
	err := memStore.Clear()
	require.NoError(t, err, "Clearing memory store should work")
	
	// Verify the memory implementation works correctly
	testKey := "test-key-" + uuid.New().String()
	testValue := "test-value"
	err = memStore.PushList(testKey, testValue)
	require.NoError(t, err, "PushList should work")
	
	length, err := memStore.ListLength(testKey)
	require.NoError(t, err, "ListLength should work")
	require.Equal(t, 1, length, "ListLength should return correct count")
	
	items, err := memStore.ListItems(testKey, 0, -1)
	require.NoError(t, err, "ListItems should work")
	require.Equal(t, 1, len(items), "ListItems should return all items")
	require.Equal(t, testValue, items[0], "ListItems should return correct items")
	
	// Create a module registry
	registry := agents.NewModuleRegistry()
	
	// Create test module
	testModule := &TestModule{
		Signature: core.Signature{
			Inputs: []core.InputField{
				{Field: core.Field{Name: "input"}},
			},
			Outputs: []core.OutputField{
				{Field: core.Field{Name: "output"}},
			},
		},
	}
	
	// Register the module
	registry.Register("test-module", testModule)
	
	// Create the worker
	worker, err := workflows.NewDistributedWorker(&workflows.DistributedWorkerConfig{
		QueueStore:     memStore,
		StateStore:     memStore,
		QueueName:      "test-queue",
		ModuleRegistry: registry,
		PollInterval:   10 * time.Millisecond,
		Concurrency:    1,
		WorkerID:       "test-worker-" + uuid.New().String(),
	})
	
	require.NoError(t, err)
	return worker, registry, memStore
}

// TestDistributedWorker_PushAndProcess tests that a job can be pushed to the queue and processed successfully
func TestDistributedWorker_PushAndProcess(t *testing.T) {
	worker, registry, memStore := setupWorkerTest(t)
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	// Start the worker
	err := worker.Start(ctx)
	require.NoError(t, err)
	defer worker.Stop()
	
	// Register a module with custom behavior
	module := &TestModule{
		Signature: core.Signature{
			Inputs: []core.InputField{
				{Field: core.Field{Name: "input"}},
			},
			Outputs: []core.OutputField{
				{Field: core.Field{Name: "output"}},
			},
		},
		ProcessFn: func(ctx context.Context, input map[string]any, opts ...core.Option) (map[string]any, error) {
			// Process the input and return a transformed output
			val, _ := input["input"].(string)
			result := map[string]any{
				"output": "processed-" + val,
			}
			t.Logf("Module processing complete, returning: %v", result)
			return result, nil
		},
	}
	
	// Register module to registry - directly to registry, not with a name
	typeStr := fmt.Sprintf("%T", module)
	registry.Register(typeStr, module)
	
	// Push a job
	job, err := worker.PushJob(ctx, module, map[string]any{
		"input": "test-data",
	})
	require.NoError(t, err)
	require.NotNil(t, job)
	t.Logf("Pushed job with ID: %s and type: %s", job.ID, job.Type)
	
	// Wait for job to be processed
	var result map[string]any
	var jobStatus string
	var foundResult bool
	maxAttempts := 50
	
	for i := 0; i < maxAttempts && !foundResult; i++ {
		time.Sleep(100 * time.Millisecond)
		
		// Check job status
		jobStatus, err = worker.GetJobStatus(ctx, job.ID)
		if err != nil {
			t.Logf("Error getting job status (attempt %d): %v", i, err)
			continue
		}
		
		t.Logf("Job status (attempt %d): %s", i, jobStatus)
		
		if jobStatus == "completed" {
			// Try getting result directly from memory store
			resultKey := fmt.Sprintf("job:%s:result", job.ID)
			resultRaw, err := memStore.Retrieve(resultKey)
			if err != nil {
				t.Logf("Error retrieving result directly from memory: %v", err)
			} else {
				t.Logf("Raw result from memory: %v (type: %T)", resultRaw, resultRaw)
			}
			
			// Try GetJobResult
			result, err = worker.GetJobResult(ctx, job.ID)
			if err != nil {
				t.Logf("Error getting job result: %v", err)
				continue
			}
			
			t.Logf("Got result: %+v", result)
			foundResult = true
		}
	}
	
	// Verify the result
	assert.Equal(t, "completed", jobStatus, "Job should be completed")
	assert.True(t, foundResult, "Job result should be found")
	if !foundResult {
		t.FailNow()
	}
	
	assert.NotNil(t, result, "Result should not be nil")
	if result == nil {
		t.FailNow()
	}
	
	assert.Contains(t, result, "output", "Result should contain output field")
	if output, ok := result["output"]; ok {
		assert.Equal(t, "processed-test-data", output, "Result output should match expected value")
	}
}

func TestDistributedWorker_ErrorHandling(t *testing.T) {
	worker, registry, _ := setupWorkerTest(t)
	
	// Keep track of errors
	var errorCalled bool
	var lastError error
	
	// Configure error handler
	worker.SetErrorHandler(func(job *workflows.WorkerJob, err error) {
		t.Logf("Error handler called: %v", err)
		errorCalled = true
		
		if job == nil {
			t.Logf("Warning: job is nil in error handler")
		} else {
			t.Logf("Job in error handler: %s", job.ID)
		}
		
		lastError = err
	})
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	// Start the worker
	err := worker.Start(ctx)
	require.NoError(t, err)
	defer worker.Stop()
	
	// Register a module that always fails
	failModule := &TestModule{
		Signature: core.Signature{
			Inputs: []core.InputField{
				{Field: core.Field{Name: "input"}},
			},
			Outputs: []core.OutputField{
				{Field: core.Field{Name: "output"}},
			},
		},
		ProcessFn: func(ctx context.Context, input map[string]any, opts ...core.Option) (map[string]any, error) {
			t.Log("Module is executing and will fail intentionally")
			return nil, assert.AnError
		},
	}
	
	// Register directly with type
	typeStr := fmt.Sprintf("%T", failModule)
	registry.Register(typeStr, failModule)
	
	// Push a job
	job, err := worker.PushJob(ctx, failModule, map[string]any{
		"input": "will-fail",
	})
	require.NoError(t, err)
	require.NotNil(t, job)
	
	t.Logf("Pushed job with ID: %s and type: %s", job.ID, job.Type)
	
	// Wait for error handler to be called
	var retryingFound bool
	maxAttempts := 50
	for i := 0; i < maxAttempts && !errorCalled; i++ {
		time.Sleep(100 * time.Millisecond)
	}
	
	// Check if job is in retrying state
	for i := 0; i < 10; i++ {
		jobStatus, err := worker.GetJobStatus(ctx, job.ID)
		if err == nil && jobStatus == "retrying" {
			retryingFound = true
			t.Logf("Job confirmed in retrying state: %s", job.ID)
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	
	// Verify the error handler was called and an error was captured
	assert.True(t, errorCalled, "Error handler should have been called")
	assert.True(t, retryingFound, "Job should be in retrying state")
	assert.NotNil(t, lastError, "Error should not be nil")
	if lastError != nil {
		assert.Contains(t, lastError.Error(), assert.AnError.Error(), "Error message should contain the original error")
	}
}

func TestDistributedWorker_Middleware(t *testing.T) {
	worker, registry, _ := setupWorkerTest(t)
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	// Track middleware execution
	var middlewareCalled bool
	var middlewareContext context.Context
	
	// Add middleware
	worker.Use(func(next workflows.WorkerHandler) workflows.WorkerHandler {
		return func(ctx context.Context, job *workflows.WorkerJob) error {
			middlewareCalled = true
			middlewareContext = ctx
			
			// Add something to context
			ctx = context.WithValue(ctx, "middleware_key", "middleware_value")
			
			t.Logf("Middleware executed for job: %s", job.ID)
			return next(ctx, job)
		}
	})
	
	// Register a module that checks context
	var moduleContext context.Context
	checkModule := &TestModule{
		Signature: core.Signature{
			Inputs: []core.InputField{
				{Field: core.Field{Name: "input"}},
			},
			Outputs: []core.OutputField{
				{Field: core.Field{Name: "output"}},
			},
		},
		ProcessFn: func(ctx context.Context, input map[string]any, opts ...core.Option) (map[string]any, error) {
			moduleContext = ctx
			t.Logf("Module executed with context containing middleware key: %v", ctx.Value("middleware_key"))
			return map[string]any{
				"output": "done",
			}, nil
		},
	}
	
	// Register with the type name
	typeStr := fmt.Sprintf("%T", checkModule)
	registry.Register(typeStr, checkModule)
	
	// Start the worker
	err := worker.Start(ctx)
	require.NoError(t, err)
	defer worker.Stop()
	
	// Push a job
	job, err := worker.PushJob(ctx, checkModule, map[string]any{
		"input": "test",
	})
	require.NoError(t, err)
	t.Logf("Pushed job with ID: %s and type: %s", job.ID, job.Type)
	
	// Wait for job to be processed
	var jobStatus string
	maxAttempts := 50
	for i := 0; i < maxAttempts; i++ {
		time.Sleep(100 * time.Millisecond)
		
		// Check job status
		jobStatus, err = worker.GetJobStatus(ctx, job.ID)
		t.Logf("Job status (attempt %d): %s", i, jobStatus)
		
		if err == nil && jobStatus == "completed" {
			break
		}
	}
	
	// Verify middleware was called and context was passed through
	assert.Equal(t, "completed", jobStatus, "Job should be completed")
	assert.True(t, middlewareCalled, "Middleware should have been called")
	assert.NotNil(t, middlewareContext, "Middleware context should not be nil")
	assert.NotNil(t, moduleContext, "Module context should not be nil")
	if moduleContext != nil {
		middlewareVal := moduleContext.Value("middleware_key")
		assert.Equal(t, "middleware_value", middlewareVal, "Middleware value should be passed through context")
	}
} 