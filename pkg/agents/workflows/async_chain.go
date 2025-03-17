package workflows

import (
	"context"
	"fmt"
	"maps"
	"time"

	"github.com/google/uuid"
	"github.com/scottdavis/dsgo/pkg/agents"
	"github.com/scottdavis/dsgo/pkg/agents/memory"
	"github.com/scottdavis/dsgo/pkg/core"
	"github.com/scottdavis/dsgo/pkg/errors"
)

// AsyncChainConfig holds configuration for the AsyncChainWorkflow
type AsyncChainConfig struct {
	// JobQueue is the backend to use for enqueueing jobs
	JobQueue JobQueue
	
	// JobPrefix is a prefix to use for job IDs
	JobPrefix string
	
	// WaitForCompletion determines if the workflow should wait for jobs to complete
	WaitForCompletion bool
	
	// CompletionTimeout is the maximum time to wait for job completion
	CompletionTimeout time.Duration
	
	// HeartbeatInterval is how often to update the workflow heartbeat
	HeartbeatInterval time.Duration
	
	// ErrorPolicy defines how to handle errors in workflow steps
	DefaultErrorPolicy ErrorPolicy
	
	// EnableRetries enables automatic retries for failed steps
	EnableRetries bool
	
	// MaxRetryAttempts is the maximum number of retry attempts
	MaxRetryAttempts int
	
	// RetryBackoff is the base backoff duration for retries (grows exponentially)
	RetryBackoff time.Duration
}

// AsyncChainWorkflow executes steps asynchronously by pushing them to a job queue
// Steps are executed in sequence by workers, with state shared through memory
type AsyncChainWorkflow struct {
	*BaseWorkflow
	config *AsyncChainConfig
}

// NewAsyncChainWorkflow creates a new AsyncChainWorkflow
func NewAsyncChainWorkflow(memory agents.Memory, config *AsyncChainConfig) *AsyncChainWorkflow {
	if config.JobPrefix == "" {
		config.JobPrefix = "async_job"
	}
	
	if config.HeartbeatInterval == 0 {
		config.HeartbeatInterval = 30 * time.Second
	}
	
	if config.DefaultErrorPolicy == "" {
		config.DefaultErrorPolicy = ErrorPolicyRetry
	}
	
	if config.EnableRetries && config.MaxRetryAttempts == 0 {
		config.MaxRetryAttempts = 3
	}
	
	if config.RetryBackoff == 0 {
		config.RetryBackoff = 5 * time.Second
	}
	
	return &AsyncChainWorkflow{
		BaseWorkflow: NewBaseWorkflow(memory),
		config:       config,
	}
}

// Execute runs the workflow asynchronously by pushing jobs to the queue
func (w *AsyncChainWorkflow) Execute(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	// Initialize workflow state with input values
	state := make(map[string]any)
	maps.Copy(state, inputs)
	
	// Create a workflow ID
	workflowID := uuid.New().String()
	
	// Create workflow state tracking object
	workflowState := NewWorkflowState(workflowID, len(w.steps), state)
	
	// Add memory to context for modules to access
	ctx = w.ExecuteWithContext(ctx)
	
	// Get memory store
	memStore, err := memory.GetMemoryStore(ctx)
	if err != nil {
		return nil, fmt.Errorf("memory store not found in context")
	}
	
	// Store workflow state
	stateKey := fmt.Sprintf("wf:%s:state", workflowID)
	stateStr, err := workflowState.Serialize()
	if err != nil {
		return nil, fmt.Errorf("failed to serialize workflow state: %w", err)
	}
	
	if err := memStore.Store(stateKey, stateStr, agents.WithTTL(24*time.Hour)); err != nil {
		return nil, fmt.Errorf("failed to store workflow state: %w", err)
	}
	
	// Mark workflow as running
	workflowState.MarkRunning()
	
	// Update stored state
	stateStr, err = workflowState.Serialize()
	if err != nil {
		return nil, fmt.Errorf("failed to serialize updated workflow state: %w", err)
	}
	
	if err := memStore.Store(stateKey, stateStr, agents.WithTTL(24*time.Hour)); err != nil {
		return nil, fmt.Errorf("failed to store updated workflow state: %w", err)
	}
	
	// Enqueue the first step
	if len(w.steps) > 0 {
		firstStep := w.steps[0]
		jobID := fmt.Sprintf("%s:%s:%s:0", w.config.JobPrefix, workflowID, firstStep.ID)
		
		stepCtx, stepSpan := core.StartSpan(ctx, "AsyncChainStep_0")
		stepSpan.WithAnnotation("async_chain_step", map[string]any{
			"name":        firstStep.ID,
			"index":       0,
			"total":       len(w.steps),
			"workflow_id": workflowID,
		})
		
		// Set first step as current
		workflowState.CurrentStepID = firstStep.ID
		
		// Update state before pushing job
		stateStr, _ = workflowState.Serialize()
		_ = memStore.Store(stateKey, stateStr, agents.WithTTL(24*time.Hour))
		
		// Create job payload
		jobPayload := map[string]any{
			"workflow_id":     workflowID,
			"step_id":         firstStep.ID,
			"step_index":      0,
			"total_steps":     len(w.steps),
			"next_step_index": 1,
			"state":           workflowState.Data,
			"retry_policy":    string(w.config.DefaultErrorPolicy),
			"retry_count":     0,
			"max_retries":     w.config.MaxRetryAttempts,
		}
		
		// Enqueue the job
		if err := w.config.JobQueue.Push(stepCtx, jobID, firstStep.ID, jobPayload); err != nil {
			core.EndSpan(stepCtx)
			
			// Mark workflow as failed
			workflowState.MarkFailed()
			workflowState.AddFailedStep(firstStep.ID, err, ErrorSeverityWorkflowFatal, ErrorPolicyFail)
			
			// Store updated state
			stateStr, _ := workflowState.Serialize()
			_ = memStore.Store(stateKey, stateStr, agents.WithTTL(24*time.Hour))
			
			return nil, errors.WithFields(
				errors.Wrap(err, errors.LLMGenerationFailed, "failed to enqueue job"),
				errors.Fields{
					"step_id":     firstStep.ID,
					"workflow_id": workflowID,
				})
		}
		
		core.EndSpan(stepCtx)
	}
	
	// If not waiting for completion, return immediately
	if !w.config.WaitForCompletion {
		return map[string]any{
			"workflow_id": workflowID,
			"status":      string(workflowState.Status),
			"steps_total": len(w.steps),
		}, nil
	}
	
	// Wait for workflow completion
	timeout := w.config.CompletionTimeout
	if timeout == 0 {
		timeout = 1 * time.Hour
	}
	
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	
	// Start heartbeat goroutine
	heartbeatDone := make(chan struct{})
	if w.config.HeartbeatInterval > 0 {
		go func() {
			defer close(heartbeatDone)
			ticker := time.NewTicker(w.config.HeartbeatInterval)
			defer ticker.Stop()
			
			for {
				select {
				case <-timeoutCtx.Done():
					return
				case <-ticker.C:
					// Retrieve current state
					stateValue, err := memStore.Retrieve(stateKey)
					if err != nil {
						continue
					}
					
					// Update heartbeat
					stateStr, ok := stateValue.(string)
					if !ok {
						continue
					}
					
					state, err := DeserializeWorkflowState(stateStr)
					if err != nil {
						continue
					}
					
					if state.Status == WorkflowStatusRunning {
						state.UpdateHeartbeat()
						updatedStateStr, _ := state.Serialize()
						_ = memStore.Store(stateKey, updatedStateStr, agents.WithTTL(24*time.Hour))
					} else {
						// Workflow is no longer running, stop heartbeat
						return
					}
				}
			}
		}()
	}
	
	// Check completion status
	completionTicker := time.NewTicker(500 * time.Millisecond)
	defer completionTicker.Stop()
	
	for {
		select {
		case <-timeoutCtx.Done():
			// Timeout occurred, mark workflow as stalled
			stateValue, err := memStore.Retrieve(stateKey)
			if err == nil {
				if stateStr, ok := stateValue.(string); ok {
					if state, err := DeserializeWorkflowState(stateStr); err == nil {
						state.MarkStalled()
						updatedStateStr, _ := state.Serialize()
						_ = memStore.Store(stateKey, updatedStateStr, agents.WithTTL(24*time.Hour))
					}
				}
			}
			
			return nil, fmt.Errorf("workflow execution timed out after %v", timeout)
			
		case <-completionTicker.C:
			// Check current workflow state
			stateValue, err := memStore.Retrieve(stateKey)
			if err != nil {
				return nil, fmt.Errorf("failed to retrieve workflow state: %w", err)
			}
			
			// Convert to JSON string if needed
			var stateStr string
			switch v := stateValue.(type) {
			case string:
				stateStr = v
			case []byte:
				stateStr = string(v)
			case map[string]interface{}:
				// If the stored value is already a map, handle it as final state data
				return v, nil
			case map[string]int:
				// Convert map[string]int to map[string]interface{}
				result := make(map[string]interface{})
				for k, val := range v {
					result[k] = val
				}
				return result, nil
			default:
				return nil, fmt.Errorf("invalid workflow state type: %T", stateValue)
			}
			
			// Deserialize the workflow state
			workflowState, err := DeserializeWorkflowState(stateStr)
			if err != nil {
				return nil, fmt.Errorf("failed to deserialize workflow state: %w", err)
			}
			
			// Check if workflow has failed
			if workflowState.Status == WorkflowStatusFailed || workflowState.HasFatalErrors() {
				// Stop heartbeat goroutine
				cancel()
				<-heartbeatDone
				
				// Return error information
				if len(workflowState.Errors) > 0 {
					return map[string]any{
						"workflow_id": workflowID,
						"status":      string(workflowState.Status),
						"error":       workflowState.Errors[0].Error,
						"step_id":     workflowState.Errors[0].StepID,
					}, fmt.Errorf("workflow failed: %s", workflowState.Errors[0].Error)
				}
				
				return map[string]any{
					"workflow_id": workflowID,
					"status":      string(workflowState.Status),
				}, fmt.Errorf("workflow failed")
			}
			
			// Check if workflow is complete
			if workflowState.Status == WorkflowStatusCompleted || 
				workflowState.IsComplete() || 
				len(workflowState.CompletedStepIDs) >= workflowState.TotalSteps {
				
				// Stop heartbeat goroutine
				cancel()
				<-heartbeatDone
				
				// If not already marked as completed, do so now
				if workflowState.Status != WorkflowStatusCompleted {
					workflowState.MarkCompleted()
					updatedStateStr, _ := workflowState.Serialize()
					_ = memStore.Store(stateKey, updatedStateStr, agents.WithTTL(24*time.Hour))
				}
				
				// Return final state data
				return workflowState.Data, nil
			}
		}
	}
} 