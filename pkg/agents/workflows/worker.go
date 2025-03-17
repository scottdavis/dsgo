package workflows

import (
	"context"
	"fmt"
	"log"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/scottdavis/dsgo/pkg/agents"
	"github.com/scottdavis/dsgo/pkg/agents/memory"
	"github.com/scottdavis/dsgo/pkg/core"
)

// WorkerConfig configures a workflow worker
type WorkerConfig struct {
	// JobQueue is the queue to poll for jobs
	JobQueue JobQueue
	
	// Memory is the memory store for workflow state
	Memory agents.Memory
	
	// ModuleRegistry maps step IDs to modules
	ModuleRegistry map[string]core.Module
	
	// PollInterval is how often to poll for new jobs
	PollInterval time.Duration
	
	// ErrorHandler is called when a job fails
	ErrorHandler func(ctx context.Context, workflowID, stepID string, err error) error
	
	// RecoveryHandler is called when recovering from a previous error
	RecoveryHandler func(ctx context.Context, workflowID, stepID string, state map[string]any) error
	
	// MaxConcurrentJobs is the maximum number of jobs to process concurrently
	MaxConcurrentJobs int
	
	// BackoffStrategy defines how to handle backoff for retries
	BackoffStrategy func(attempt int, baseDelay time.Duration) time.Duration
	
	// Logger is the logger to use for worker events
	Logger *log.Logger
}

// WorkflowWorker processes jobs from a queue and executes workflow steps
type WorkflowWorker struct {
	config    WorkerConfig
	running   bool
	mutex     sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
	semaphore chan struct{}
	wg        sync.WaitGroup
}

// NewWorkflowWorker creates a new worker for processing workflow jobs
func NewWorkflowWorker(config WorkerConfig) *WorkflowWorker {
	// Set defaults
	if config.PollInterval == 0 {
		config.PollInterval = 100 * time.Millisecond
	}
	
	if config.MaxConcurrentJobs == 0 {
		config.MaxConcurrentJobs = 10
	}
	
	if config.BackoffStrategy == nil {
		config.BackoffStrategy = DefaultBackoffStrategy
	}
	
	if config.Logger == nil {
		config.Logger = log.Default()
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	
	return &WorkflowWorker{
		config:    config,
		running:   false,
		ctx:       ctx,
		cancel:    cancel,
		semaphore: make(chan struct{}, config.MaxConcurrentJobs),
	}
}

// DefaultBackoffStrategy provides exponential backoff with jitter
func DefaultBackoffStrategy(attempt int, baseDelay time.Duration) time.Duration {
	// Exponential backoff with jitter
	backoff := float64(baseDelay.Nanoseconds()) * math.Pow(2, float64(attempt))
	jitter := rand.Float64() * 0.5 * backoff
	return time.Duration(backoff + jitter)
}

// Start begins processing jobs from the queue
func (w *WorkflowWorker) Start() {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	
	if w.running {
		return
	}
	
	w.running = true
	w.wg.Add(1)
	go w.processLoop()
}

// Stop halts job processing and waits for in-progress jobs to complete
func (w *WorkflowWorker) Stop() {
	w.mutex.Lock()
	if !w.running {
		w.mutex.Unlock()
		return
	}
	
	w.running = false
	w.cancel()
	w.mutex.Unlock()
	
	w.wg.Wait()
}

// processLoop continuously polls for and processes jobs
func (w *WorkflowWorker) processLoop() {
	defer w.wg.Done()
	
	for {
		// Check if we're still running
		w.mutex.RLock()
		if !w.running {
			w.mutex.RUnlock()
			return
		}
		w.mutex.RUnlock()
		
		// Try to acquire a slot in the semaphore
		select {
		case w.semaphore <- struct{}{}:
			// Got a slot, process a job
			w.wg.Add(1)
			go func() {
				defer w.wg.Done()
				defer func() { <-w.semaphore }()
				w.processNextJob()
			}()
		case <-w.ctx.Done():
			// Context canceled, we're shutting down
			return
		}
		
		// Sleep before next poll
		time.Sleep(w.config.PollInterval)
	}
}

// processNextJob fetches and processes the next job from the queue
func (w *WorkflowWorker) processNextJob() {
	// Context for this job
	jobCtx, jobCancel := context.WithCancel(w.ctx)
	defer jobCancel()
	
	// Add memory to context
	jobCtx = memory.WithMemoryStore(jobCtx, w.config.Memory)
	
	// Add execution state
	jobCtx = core.WithExecutionState(jobCtx)
	
	// Try to get a job
	job, err := w.config.JobQueue.Pop(jobCtx)
	if err != nil {
		// No jobs available or error
		return
	}
	
	if job == nil {
		// No jobs available
		return
	}
	
	w.config.Logger.Printf("Processing job: %s, step: %s", job.ID, job.StepID)
	
	// Get the module for this step
	module, ok := w.config.ModuleRegistry[job.StepID]
	if !ok {
		w.handleJobError(jobCtx, job, fmt.Errorf("module not found for step ID: %s", job.StepID), ErrorSeverityWorkflowFatal)
		return
	}
	
	// Extract job payload data
	var (
		workflowID     string
		totalSteps     int
		nextStepIndex  int
		state          map[string]any
		retryCount     int
		maxRetries     int
		isRecovery     bool
	)
	
	// Extract workflow ID
	workflowID, ok = job.Payload["workflow_id"].(string)
	if !ok {
		w.handleJobError(jobCtx, job, fmt.Errorf("missing workflow ID in payload"), ErrorSeverityWorkflowFatal)
		return
	}
	
	// Get next step if available
	if val, ok := job.Payload["next_step_index"].(float64); ok {
		nextStepIndex = int(val)
	}
	
	// Get total steps if available
	if val, ok := job.Payload["total_steps"].(float64); ok {
		totalSteps = int(val)
	}
	
	// Get state if available
	if val, ok := job.Payload["state"].(map[string]any); ok {
		state = val
	} else {
		state = make(map[string]any)
	}
	
	// Get retry info if available
	if val, ok := job.Payload["retry_count"].(float64); ok {
		retryCount = int(val)
	}
	
	if val, ok := job.Payload["max_retries"].(float64); ok {
		maxRetries = int(val)
	}
	
	// Check if this is a recovery job
	if val, ok := job.Payload["is_recovery"].(bool); ok {
		isRecovery = val
	}
	
	// Set up timeout if specified
	if job.Timeout > 0 {
		var cancel context.CancelFunc
		jobCtx, cancel = context.WithTimeout(jobCtx, time.Duration(job.Timeout)*time.Second)
		defer cancel()
	}
	
	// Process job based on whether it's recovery or normal execution
	var jobErr error
	if isRecovery && w.config.RecoveryHandler != nil {
		// Handle recovery
		jobErr = w.config.RecoveryHandler(jobCtx, workflowID, job.StepID, state)
	} else {
		// Normal execution
		// Set up the execution context with state
		// We can't directly set annotations, so we'll just use the state as is
		// and let the module access what it needs from the input map
		
		// Execute the module
		result, err := module.Process(jobCtx, state)
		if err != nil {
			jobErr = err
		} else {
			// Success! Update state with result
			for k, v := range result {
				state[k] = v
			}
			
			// If we have a next step, add it to the state
			if nextStepIndex > 0 {
				state["next_step_index"] = nextStepIndex
			}
			
			// Update completion tracking
			if totalSteps > 0 {
				completionKey := fmt.Sprintf("workflow:%s:completion", workflowID)
				// Use Store with numeric value instead of Increment
				currentVal, err := w.config.Memory.Retrieve(completionKey)
				if err != nil || currentVal == nil {
					err = w.config.Memory.Store(completionKey, 1)
				} else {
					var count int
					switch v := currentVal.(type) {
					case int:
						count = v + 1
					case float64:
						count = int(v) + 1
					default:
						count = 1
					}
					err = w.config.Memory.Store(completionKey, count)
				}
				if err != nil {
					w.config.Logger.Printf("Failed to update completion for workflow %s: %v", workflowID, err)
				}
			}
			
			// Successful execution
			// Store updated state
			stateKey := fmt.Sprintf("workflow:%s:state", workflowID)
			if err := w.config.Memory.Store(stateKey, state); err != nil {
				w.config.Logger.Printf("Failed to store state for workflow %s: %v", workflowID, err)
			}
		}
	}
	
	// Handle any errors
	if jobErr != nil {
		var severity ErrorSeverity
		
		// Determine severity based on retry settings
		if retryCount >= maxRetries {
			severity = ErrorSeverityStepFatal
		} else {
			severity = ErrorSeverityRetryable
		}
		
		w.handleJobError(jobCtx, job, jobErr, severity)
	}
}

// handleJobError processes errors that occur during job execution
func (w *WorkflowWorker) handleJobError(ctx context.Context, job *Job, err error, severity ErrorSeverity) {
	workflowID, _ := job.Payload["workflow_id"].(string)
	
	w.config.Logger.Printf("Error processing job %s (step %s): %v", job.ID, job.StepID, err)
	
	// Custom error handling if provided
	if w.config.ErrorHandler != nil {
		if handlerErr := w.config.ErrorHandler(ctx, workflowID, job.StepID, err); handlerErr != nil {
			w.config.Logger.Printf("Error handler failed: %v", handlerErr)
		}
	}
	
	// Check if we should retry based on severity
	if severity == ErrorSeverityRetryable {
		retryCount, _ := job.Payload["retry_count"].(float64)
		maxRetries, _ := job.Payload["max_retries"].(float64)
		
		// Increment retry count
		retryCount++
		job.Payload["retry_count"] = retryCount
		
		// Check if we've exceeded max retries
		if int(retryCount) <= int(maxRetries) {
			// Calculate backoff
			baseDelay := 1 * time.Second
			if job.Retry > 0 {
				// Use job's retry setting if available
				baseDelay = time.Duration(job.Retry) * time.Second
			}
			
			backoff := w.config.BackoffStrategy(int(retryCount), baseDelay)
			
			// Log retry attempt
			w.config.Logger.Printf("Retrying job %s (step %s) in %v (attempt %d/%d)",
				job.ID, job.StepID, backoff, int(retryCount), int(maxRetries))
			
			// Wait for backoff period
			time.Sleep(backoff)
			
			// Re-enqueue the job
			err := w.config.JobQueue.Push(ctx, job.ID, job.StepID, job.Payload)
			if err != nil {
				w.config.Logger.Printf("Failed to re-enqueue job for retry: %v", err)
			}
			return
		}
	}
	
	// If we got here, we're not retrying
	// Mark workflow as failed
	if workflowID != "" {
		errKey := fmt.Sprintf("workflow:%s:error", workflowID)
		errData := map[string]any{
			"step_id":   job.StepID,
			"error":     err.Error(),
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"severity":  string(severity),
		}
		
		if err := w.config.Memory.Store(errKey, errData); err != nil {
			w.config.Logger.Printf("Failed to store error for workflow %s: %v", workflowID, err)
		}
		
		statusKey := fmt.Sprintf("workflow:%s:status", workflowID)
		if err := w.config.Memory.Store(statusKey, "failed"); err != nil {
			w.config.Logger.Printf("Failed to update status for workflow %s: %v", workflowID, err)
		}
	}
} 