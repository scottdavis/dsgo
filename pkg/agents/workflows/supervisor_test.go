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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockSupervisorJobQueue implements JobQueue for testing
type MockSupervisorJobQueue struct {
	mutex sync.RWMutex
	jobs  map[string]map[string]any
	// Track push calls
	pushCalls int
}

// NewMockSupervisorJobQueue creates a new mock job queue for testing
func NewMockSupervisorJobQueue() *MockSupervisorJobQueue {
	return &MockSupervisorJobQueue{
		jobs: make(map[string]map[string]any),
	}
}

// Push implements JobQueue.Push
func (q *MockSupervisorJobQueue) Push(ctx context.Context, jobID, stepID string, payload map[string]any) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	
	q.jobs[jobID] = payload
	q.pushCalls++
	return nil
}

// Pop implements JobQueue.Pop
func (q *MockSupervisorJobQueue) Pop(ctx context.Context) (*Job, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	
	// Return nil if no jobs
	if len(q.jobs) == 0 {
		return nil, fmt.Errorf("no jobs available")
	}
	
	// Get the first job (non-deterministic in a map, but ok for tests)
	var jobID string
	var payload map[string]any
	for id, p := range q.jobs {
		jobID = id
		payload = p
		break
	}
	
	// Remove the job from the queue
	delete(q.jobs, jobID)
	
	// Create and return the job
	return &Job{
		ID:         jobID,
		StepID:     payload["step_id"].(string),
		Payload:    payload,
		EnqueuedAt: time.Now(),
	}, nil
}

// Close implements JobQueue.Close
func (q *MockSupervisorJobQueue) Close() error {
	return nil
}

// Ping implements JobQueue.Ping
func (q *MockSupervisorJobQueue) Ping() error {
	return nil
}

// PushCalls returns the number of push calls
func (q *MockSupervisorJobQueue) PushCalls() int {
	q.mutex.RLock()
	defer q.mutex.RUnlock()
	return q.pushCalls
}

// GetJobs returns all jobs
func (q *MockSupervisorJobQueue) GetJobs() map[string]map[string]any {
	q.mutex.RLock()
	defer q.mutex.RUnlock()
	
	// Return a copy to avoid race conditions
	result := make(map[string]map[string]any, len(q.jobs))
	for k, v := range q.jobs {
		result[k] = v
	}
	
	return result
}

// RecordRecoveryCallback is a helper for testing supervisor recovery callbacks
type RecordRecoveryCallback struct {
	mutex      sync.RWMutex
	callCount  int
	workflowID string
	state      *WorkflowState
}

// Call implements the recovery callback
func (r *RecordRecoveryCallback) Call(ctx context.Context, workflowID string, state *WorkflowState) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	
	r.callCount++
	r.workflowID = workflowID
	r.state = state
	return nil
}

// GetCallCount returns the number of times the callback was called
func (r *RecordRecoveryCallback) GetCallCount() int {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.callCount
}

// GetLastWorkflowID returns the last workflow ID passed to the callback
func (r *RecordRecoveryCallback) GetLastWorkflowID() string {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.workflowID
}

// TestWorkflowSupervisor_DetectsStalledWorkflows tests that the supervisor detects stalled workflows
func TestWorkflowSupervisor_DetectsStalledWorkflows(t *testing.T) {
	// Create memory store and job queue
	mem := agents.NewInMemoryStore()
	queue := NewMockSupervisorJobQueue()
	recovery := &RecordRecoveryCallback{}
	
	// Create supervisor with short check interval and stall threshold
	supervisor := NewWorkflowSupervisor(SupervisorConfig{
		Memory:         mem,
		JobQueue:       queue,
		CheckInterval:  50 * time.Millisecond,
		StallThreshold: 100 * time.Millisecond,
		// Disable auto-recovery, use callback instead
		EnableAutoRecovery: false,
		RecoveryCallback:   recovery.Call,
		Logger:             log.New(os.Stdout, "TEST: ", log.LstdFlags),
	})
	
	// Create a stalled workflow
	workflowID := "stalled-workflow-123"
	state := NewWorkflowState(workflowID, 3, map[string]any{"test": "data"})
	state.Status = WorkflowStatusRunning
	state.CurrentStepID = "step1"
	
	// Set the last heartbeat to be older than the stall threshold
	state.LastHeartbeat = time.Now().Add(-200 * time.Millisecond)
	
	// Store the workflow state
	stateStr, err := state.Serialize()
	require.NoError(t, err)
	
	stateKey := fmt.Sprintf("wf:%s:state", workflowID)
	err = mem.Store(stateKey, stateStr, agents.WithTTL(24*time.Hour))
	require.NoError(t, err)
	
	// Start the supervisor
	supervisor.Start()
	
	// Wait for the supervisor to check for stalled workflows
	time.Sleep(150 * time.Millisecond)
	
	// Stop the supervisor
	supervisor.Stop()
	
	// Check if the recovery callback was called
	assert.Equal(t, 1, recovery.GetCallCount(), "Recovery callback should be called once")
	assert.Equal(t, workflowID, recovery.GetLastWorkflowID(), "Recovery callback should receive correct workflow ID")
	
	// Check if the workflow state was updated
	stateValue, err := mem.Retrieve(stateKey)
	require.NoError(t, err)
	
	updatedState, err := DeserializeWorkflowState(stateValue.(string))
	require.NoError(t, err)
	
	assert.Equal(t, WorkflowStatusStalled, updatedState.Status, "Workflow status should be marked as stalled")
}

// TestWorkflowSupervisor_AutoRecovery tests the supervisor's auto-recovery capability
func TestWorkflowSupervisor_AutoRecovery(t *testing.T) {
	// Create memory store and job queue
	mem := agents.NewInMemoryStore()
	queue := NewMockSupervisorJobQueue()
	
	// Create supervisor with short check interval and stall threshold
	supervisor := NewWorkflowSupervisor(SupervisorConfig{
		Memory:               mem,
		JobQueue:             queue,
		CheckInterval:        50 * time.Millisecond,
		StallThreshold:       100 * time.Millisecond,
		EnableAutoRecovery:   true,
		MaxConcurrentRecoveries: 2,
		Logger:               log.New(os.Stdout, "TEST: ", log.LstdFlags),
	})
	
	// Create a stalled workflow
	workflowID := "stalled-workflow-456"
	state := NewWorkflowState(workflowID, 3, map[string]any{"test": "data"})
	state.Status = WorkflowStatusRunning
	state.CurrentStepID = "step2"
	state.CompletedStepIDs = []string{"step1"}
	
	// Set the last heartbeat to be older than the stall threshold
	state.LastHeartbeat = time.Now().Add(-200 * time.Millisecond)
	
	// Store the workflow state
	stateStr, err := state.Serialize()
	require.NoError(t, err)
	
	stateKey := fmt.Sprintf("wf:%s:state", workflowID)
	err = mem.Store(stateKey, stateStr, agents.WithTTL(24*time.Hour))
	require.NoError(t, err)
	
	// Start the supervisor
	supervisor.Start()
	
	// Wait for the supervisor to check for stalled workflows
	time.Sleep(150 * time.Millisecond)
	
	// Stop the supervisor
	supervisor.Stop()
	
	// Check if a recovery job was pushed to the queue
	assert.Equal(t, 1, queue.PushCalls(), "Should push one recovery job")
	
	jobs := queue.GetJobs()
	require.Len(t, jobs, 1, "Should have one recovery job")
	
	// Get the recovery job
	var recoveryJob map[string]any
	for _, job := range jobs {
		recoveryJob = job
		break
	}
	
	// Verify recovery job details
	assert.Equal(t, workflowID, recoveryJob["workflow_id"])
	assert.Equal(t, 1, recoveryJob["step_index"])
	assert.Equal(t, 3, recoveryJob["total_steps"])
	assert.Equal(t, 2, recoveryJob["next_step_index"])
	assert.True(t, recoveryJob["recovery"].(bool))
	
	// Check if the workflow state was updated
	stateValue, err := mem.Retrieve(stateKey)
	require.NoError(t, err)
	
	updatedState, err := DeserializeWorkflowState(stateValue.(string))
	require.NoError(t, err)
	
	assert.Equal(t, WorkflowStatusRunning, updatedState.Status, "Workflow status should be reset to running")
}

// TestWorkflowSupervisor_DeadLetterQueue tests the dead letter queue functionality
func TestWorkflowSupervisor_DeadLetterQueue(t *testing.T) {
	// Create memory store
	mem := agents.NewInMemoryStore()
	
	// Create a failing job queue that always returns an error on Push
	failingQueue := NewMockSupervisorJobQueue()
	
	// Override the Push method with a custom implementation that always fails
	originalPush := failingQueue.Push
	failQueuePush := func(ctx context.Context, jobID, stepID string, payload map[string]any) error {
		// Still record the call via the original method
		_ = originalPush(ctx, jobID, stepID, payload)
		// But return an error
		return fmt.Errorf("queue push failed")
	}
	
	// Create supervisor with auto-recovery
	supervisor := NewWorkflowSupervisor(SupervisorConfig{
		Memory:               mem,
		JobQueue:             &failingQueueWrapper{failingQueue, failQueuePush},
		CheckInterval:        50 * time.Millisecond,
		StallThreshold:       100 * time.Millisecond,
		EnableAutoRecovery:   true,
		DeadLetterQueue:      "test_dlq",
		Logger:               log.New(os.Stdout, "TEST: ", log.LstdFlags),
	})
	
	// Create a stalled workflow
	workflowID := "stalled-workflow-789"
	state := NewWorkflowState(workflowID, 3, map[string]any{"test": "data"})
	state.Status = WorkflowStatusRunning
	state.CurrentStepID = "step2"
	
	// Set the last heartbeat to be older than the stall threshold
	state.LastHeartbeat = time.Now().Add(-200 * time.Millisecond)
	
	// Store the workflow state
	stateStr, err := state.Serialize()
	require.NoError(t, err)
	
	stateKey := fmt.Sprintf("wf:%s:state", workflowID)
	err = mem.Store(stateKey, stateStr, agents.WithTTL(24*time.Hour))
	require.NoError(t, err)
	
	// Start the supervisor
	supervisor.Start()
	
	// Wait for the supervisor to check for stalled workflows
	time.Sleep(150 * time.Millisecond)
	
	// Stop the supervisor
	supervisor.Stop()
	
	// Check if Push was called
	assert.Equal(t, 1, failingQueue.PushCalls(), "Should have attempted to push one job")
	
	// Check if the job was moved to the dead letter queue
	dlqKey := fmt.Sprintf("dlq:%s:%s", "test_dlq", workflowID)
	dlqValue, err := mem.Retrieve(dlqKey)
	require.NoError(t, err, "Should have a dead letter queue entry")
	
	// Deserialize the DLQ entry
	require.NotNil(t, dlqValue, "Dead letter queue entry should not be nil")
	
	// List dead letter jobs
	dlqJobs, err := supervisor.ListDeadLetterJobs()
	require.NoError(t, err)
	require.Len(t, dlqJobs, 1, "Should have one dead letter job")
	assert.Equal(t, workflowID, dlqJobs[0]["workflow_id"])
}

// failingQueueWrapper is a wrapper that allows us to override the Push method
type failingQueueWrapper struct {
	queue     *MockSupervisorJobQueue
	pushOverride func(ctx context.Context, jobID, stepID string, payload map[string]any) error
}

// Push uses the override
func (w *failingQueueWrapper) Push(ctx context.Context, jobID, stepID string, payload map[string]any) error {
	return w.pushOverride(ctx, jobID, stepID, payload)
}

// Pop delegates to the wrapped queue
func (w *failingQueueWrapper) Pop(ctx context.Context) (*Job, error) {
	return w.queue.Pop(ctx)
}

// Close delegates to the wrapped queue
func (w *failingQueueWrapper) Close() error {
	return w.queue.Close()
}

// Ping delegates to the wrapped queue
func (w *failingQueueWrapper) Ping() error {
	return w.queue.Ping()
}

// TestWorkflowSupervisor_CleanupCompletedWorkflows tests the cleanup of completed workflows
func TestWorkflowSupervisor_CleanupCompletedWorkflows(t *testing.T) {
	// Create memory store
	mem := agents.NewInMemoryStore()
	queue := NewMockSupervisorJobQueue()
	
	// Create supervisor with very short retention period
	supervisor := NewWorkflowSupervisor(SupervisorConfig{
		Memory:          mem,
		JobQueue:        queue,
		CheckInterval:   50 * time.Millisecond,
		RetentionPeriod: 1 * time.Millisecond, // Very short for testing
		Logger:          log.New(os.Stdout, "TEST: ", log.LstdFlags),
	})
	
	// Create a completed workflow that's older than the retention period
	workflowID := "completed-workflow-123"
	state := NewWorkflowState(workflowID, 2, map[string]any{"test": "data"})
	state.Status = WorkflowStatusCompleted
	completedTime := time.Now().Add(-10 * time.Millisecond) // Older than retention period
	state.CompletedAt = &completedTime
	
	// Store the workflow state
	stateStr, err := state.Serialize()
	require.NoError(t, err)
	
	stateKey := fmt.Sprintf("wf:%s:state", workflowID)
	err = mem.Store(stateKey, stateStr, agents.WithTTL(24*time.Hour))
	require.NoError(t, err)
	
	// Store some other workflow data
	dataKey := fmt.Sprintf("wf:%s:data", workflowID)
	err = mem.Store(dataKey, "some data", agents.WithTTL(24*time.Hour))
	require.NoError(t, err)
	
	// Start the supervisor
	supervisor.Start()
	
	// Wait for the supervisor to cleanup completed workflows
	time.Sleep(150 * time.Millisecond)
	
	// Stop the supervisor
	supervisor.Stop()
	
	// Check if the workflow state was cleaned up
	_, err = mem.Retrieve(stateKey)
	assert.Error(t, err, "Workflow state should be removed")
	
	// Check if the workflow data was cleaned up
	_, err = mem.Retrieve(dataKey)
	assert.Error(t, err, "Workflow data should be removed")
}

// TestWorkflowSupervisor_CancelWorkflow tests workflow cancellation
func TestWorkflowSupervisor_CancelWorkflow(t *testing.T) {
	// Create memory store
	mem := agents.NewInMemoryStore()
	queue := NewMockSupervisorJobQueue()
	
	// Create supervisor
	supervisor := NewWorkflowSupervisor(SupervisorConfig{
		Memory:   mem,
		JobQueue: queue,
		Logger:   log.New(os.Stdout, "TEST: ", log.LstdFlags),
	})
	
	// Create a running workflow
	workflowID := "running-workflow-123"
	state := NewWorkflowState(workflowID, 3, map[string]any{"test": "data"})
	state.Status = WorkflowStatusRunning
	state.CurrentStepID = "step1"
	
	// Store the workflow state
	stateStr, err := state.Serialize()
	require.NoError(t, err)
	
	stateKey := fmt.Sprintf("wf:%s:state", workflowID)
	err = mem.Store(stateKey, stateStr, agents.WithTTL(24*time.Hour))
	require.NoError(t, err)
	
	// Cancel the workflow
	err = supervisor.CancelWorkflow(workflowID)
	require.NoError(t, err)
	
	// Check if the workflow state was updated
	stateValue, err := mem.Retrieve(stateKey)
	require.NoError(t, err)
	
	updatedState, err := DeserializeWorkflowState(stateValue.(string))
	require.NoError(t, err)
	
	assert.Equal(t, WorkflowStatusCanceled, updatedState.Status, "Workflow status should be canceled")
	assert.NotNil(t, updatedState.CompletedAt, "Completed timestamp should be set")
}

// TestWorkflowSupervisor_ResetStuckJob tests resetting a stuck job
func TestWorkflowSupervisor_ResetStuckJob(t *testing.T) {
	// Create memory store and job queue
	mem := agents.NewInMemoryStore()
	queue := NewMockSupervisorJobQueue()
	
	// Create supervisor
	supervisor := NewWorkflowSupervisor(SupervisorConfig{
		Memory:   mem,
		JobQueue: queue,
		Logger:   log.New(os.Stdout, "TEST: ", log.LstdFlags),
	})
	
	// Create a stalled workflow
	workflowID := "stalled-workflow-123"
	state := NewWorkflowState(workflowID, 3, map[string]any{"test": "data"})
	state.Status = WorkflowStatusStalled
	state.CurrentStepID = "step2"
	state.CompletedStepIDs = []string{"step1"}
	state.FailedStepIDs = []string{"step2"}
	
	// Store the workflow state
	stateStr, err := state.Serialize()
	require.NoError(t, err)
	
	stateKey := fmt.Sprintf("wf:%s:state", workflowID)
	err = mem.Store(stateKey, stateStr, agents.WithTTL(24*time.Hour))
	require.NoError(t, err)
	
	// Reset the stuck job
	err = supervisor.ResetStuckJob(workflowID, "step2")
	require.NoError(t, err)
	
	// Check if a recovery job was pushed to the queue
	assert.Equal(t, 1, queue.PushCalls(), "Should push one recovery job")
	
	jobs := queue.GetJobs()
	require.Len(t, jobs, 1, "Should have one recovery job")
	
	// Check if the workflow state was updated
	stateValue, err := mem.Retrieve(stateKey)
	require.NoError(t, err)
	
	updatedState, err := DeserializeWorkflowState(stateValue.(string))
	require.NoError(t, err)
	
	assert.Equal(t, WorkflowStatusRunning, updatedState.Status, "Workflow status should be running")
	assert.Equal(t, "step2", updatedState.CurrentStepID, "Current step ID should be set")
	assert.NotContains(t, updatedState.FailedStepIDs, "step2", "Failed step should be removed")
} 