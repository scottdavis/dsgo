package workflows

import (
	"context"
	"fmt"
	"maps"
	"sync"
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
}

// AsyncChainWorkflow executes steps asynchronously by pushing them to a job queue
// Steps are executed in sequence by workers, with state shared through memory
type AsyncChainWorkflow struct {
	*BaseWorkflow
	config *AsyncChainConfig
	mutex  sync.Mutex
}

// NewAsyncChainWorkflow creates a new AsyncChainWorkflow
func NewAsyncChainWorkflow(memory agents.Memory, config *AsyncChainConfig) *AsyncChainWorkflow {
	if config.JobPrefix == "" {
		config.JobPrefix = "async_job"
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
	
	// Add memory to context for modules to access
	ctx = w.ExecuteWithContext(ctx)
	
	// Store initial state in memory
	memStore, err := memory.GetMemoryStore(ctx)
	if err != nil {
		return nil, fmt.Errorf("memory store not found in context")
	}
	
	stateKey := fmt.Sprintf("wf:%s:state", workflowID)
	if err := memStore.Store(stateKey, state, agents.WithTTL(24*time.Hour)); err != nil {
		return nil, fmt.Errorf("failed to store initial state: %w", err)
	}
	
	// Create completion tracking
	completionKey := fmt.Sprintf("wf:%s:completed", workflowID)
	stepsCompletedKey := fmt.Sprintf("wf:%s:steps_completed", workflowID)
	
	if err := memStore.Store(completionKey, false, agents.WithTTL(24*time.Hour)); err != nil {
		return nil, fmt.Errorf("failed to set completion flag: %w", err)
	}
	
	if err := memStore.Store(stepsCompletedKey, 0, agents.WithTTL(24*time.Hour)); err != nil {
		return nil, fmt.Errorf("failed to set steps completed counter: %w", err)
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
		
		// Create job payload
		jobPayload := map[string]any{
			"workflow_id":     workflowID,
			"step_index":      0,
			"total_steps":     len(w.steps),
			"next_step_index": 1,
			"state":           state,
		}
		
		// Enqueue the job
		if err := w.config.JobQueue.Push(stepCtx, jobID, firstStep.ID, jobPayload); err != nil {
			core.EndSpan(stepCtx)
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
			"status":      "enqueued",
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
	
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	
	for {
		select {
		case <-timeoutCtx.Done():
			return nil, fmt.Errorf("workflow execution timed out after %v", timeout)
			
		case <-ticker.C:
			// Check completion status
			completedVal, err := memStore.Retrieve(completionKey)
			if err != nil {
				return nil, fmt.Errorf("failed to check completion status: %w", err)
			}
			
			completed, ok := completedVal.(bool)
			if !ok {
				return nil, fmt.Errorf("invalid completion status type in memory: got %T", completedVal)
			}
			
			if completed {
				// Retrieve final state
				finalStateVal, err := memStore.Retrieve(stateKey)
				if err != nil {
					return nil, fmt.Errorf("failed to get final state: %w", err)
				}
				
				// Handle state type conversion
				var finalState map[string]any
				
				// Try to directly cast to the expected type
				if typedMap, ok := finalStateVal.(map[string]any); ok {
					finalState = typedMap
				} else if intMap, ok := finalStateVal.(map[string]int); ok {
					// Convert map[string]int to map[string]any
					finalState = make(map[string]any, len(intMap))
					for k, v := range intMap {
						finalState[k] = v
					}
				} else {
					// As a fallback, try a type assertion with reflection
					// This is needed because the JSON unmarshaling in Redis store 
					// might return map[string]interface{} which is interchangeable with map[string]any
					// in some contexts but not in type assertions
					return nil, fmt.Errorf("invalid state type in memory: %T", finalStateVal)
				}
				
				return finalState, nil
			}
		}
	}
} 