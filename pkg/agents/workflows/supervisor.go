package workflows

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/scottdavis/dsgo/pkg/agents"
	"github.com/scottdavis/dsgo/pkg/errors"
)

// SupervisorConfig configures the workflow supervisor
type SupervisorConfig struct {
	// Memory is the memory store where workflow states are stored
	Memory agents.Memory
	
	// CheckInterval is how often the supervisor checks for stalled workflows
	CheckInterval time.Duration
	
	// StallThreshold is how long a workflow can go without heartbeats before considered stalled
	StallThreshold time.Duration
	
	// RetentionPeriod is how long to keep completed workflows before cleanup
	RetentionPeriod time.Duration
	
	// JobQueue is the queue for enqueuing recovery jobs
	JobQueue JobQueue
	
	// MaxConcurrentRecoveries is the maximum number of recoveries to perform concurrently
	MaxConcurrentRecoveries int
	
	// DeadLetterQueue is where to store unrecoverable jobs
	DeadLetterQueue string
	
	// EnableAutoRecovery automatically attempts to recover stalled workflows
	EnableAutoRecovery bool
	
	// RecoveryCallback is called when recovery is needed
	RecoveryCallback func(ctx context.Context, workflowID string, state *WorkflowState) error
	
	// Logger is the logger to use for supervision events
	Logger *log.Logger
}

// WorkflowSupervisor monitors workflow execution and handles recovery of stalled workflows
type WorkflowSupervisor struct {
	config SupervisorConfig
	mutex  sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
}

// NewWorkflowSupervisor creates a new workflow supervisor
func NewWorkflowSupervisor(config SupervisorConfig) *WorkflowSupervisor {
	// Set default values
	if config.CheckInterval == 0 {
		config.CheckInterval = 1 * time.Minute
	}
	
	if config.StallThreshold == 0 {
		config.StallThreshold = 5 * time.Minute
	}
	
	if config.RetentionPeriod == 0 {
		config.RetentionPeriod = 7 * 24 * time.Hour // 1 week
	}
	
	if config.MaxConcurrentRecoveries == 0 {
		config.MaxConcurrentRecoveries = 5
	}
	
	if config.DeadLetterQueue == "" {
		config.DeadLetterQueue = "dead_letter_queue"
	}
	
	if config.Logger == nil {
		config.Logger = log.Default()
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	
	return &WorkflowSupervisor{
		config: config,
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start begins supervision of workflows
func (s *WorkflowSupervisor) Start() {
	go s.supervisorLoop()
}

// Stop stops the supervisor
func (s *WorkflowSupervisor) Stop() {
	s.cancel()
}

// supervisorLoop is the main supervision loop
func (s *WorkflowSupervisor) supervisorLoop() {
	ticker := time.NewTicker(s.config.CheckInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-s.ctx.Done():
			return
			
		case <-ticker.C:
			// Perform supervision tasks
			s.checkStalledWorkflows()
			s.cleanupCompletedWorkflows()
		}
	}
}

// checkStalledWorkflows looks for stalled workflows and attempts recovery
func (s *WorkflowSupervisor) checkStalledWorkflows() {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	
	// Get all keys from memory store
	keys, err := s.config.Memory.List()
	if err != nil {
		s.config.Logger.Printf("Error listing keys: %v", err)
		return
	}
	
	// Find workflow state keys
	var workflowStateKeys []string
	for _, key := range keys {
		if strings.HasPrefix(key, "wf:") && strings.HasSuffix(key, ":state") {
			workflowStateKeys = append(workflowStateKeys, key)
		}
	}
	
	// Limit concurrent recoveries
	semaphore := make(chan struct{}, s.config.MaxConcurrentRecoveries)
	var wg sync.WaitGroup
	
	// Check each workflow
	for _, key := range workflowStateKeys {
		semaphore <- struct{}{} // Acquire
		wg.Add(1)
		
		go func(stateKey string) {
			defer func() {
				<-semaphore // Release
				wg.Done()
			}()
			
			// Extract workflow ID from key pattern "wf:{id}:state"
			parts := strings.Split(stateKey, ":")
			if len(parts) != 3 {
				return
			}
			workflowID := parts[1]
			
			// Get the workflow state
			stateValue, err := s.config.Memory.Retrieve(stateKey)
			if err != nil {
				s.config.Logger.Printf("Error retrieving workflow state for %s: %v", workflowID, err)
				return
			}
			
			// Convert to JSON string if needed
			var stateStr string
			switch v := stateValue.(type) {
			case string:
				stateStr = v
			case []byte:
				stateStr = string(v)
			default:
				jsonBytes, err := json.Marshal(stateValue)
				if err != nil {
					s.config.Logger.Printf("Error marshaling workflow state for %s: %v", workflowID, err)
					return
				}
				stateStr = string(jsonBytes)
			}
			
			// Deserialize the workflow state
			state, err := DeserializeWorkflowState(stateStr)
			if err != nil {
				s.config.Logger.Printf("Error deserializing workflow state for %s: %v", workflowID, err)
				return
			}
			
			// Check if it's stalled
			if state.Status == WorkflowStatusRunning && state.IsStalled(s.config.StallThreshold) {
				s.config.Logger.Printf("Found stalled workflow: %s, last heartbeat: %v", workflowID, state.LastHeartbeat)
				
				// Update state to stalled
				state.MarkStalled()
				
				// Serialize and store updated state
				updatedStateStr, err := state.Serialize()
				if err != nil {
					s.config.Logger.Printf("Error serializing updated state for %s: %v", workflowID, err)
					return
				}
				
				err = s.config.Memory.Store(stateKey, updatedStateStr, agents.WithTTL(24*time.Hour))
				if err != nil {
					s.config.Logger.Printf("Error storing updated state for %s: %v", workflowID, err)
					return
				}
				
				// Attempt recovery if enabled
				if s.config.EnableAutoRecovery {
					s.recoverWorkflow(workflowID, state)
				} else if s.config.RecoveryCallback != nil {
					// Call recovery callback if auto-recovery is disabled
					err := s.config.RecoveryCallback(s.ctx, workflowID, state)
					if err != nil {
						s.config.Logger.Printf("Recovery callback failed for %s: %v", workflowID, err)
					}
				}
			}
		}(key)
	}
	
	wg.Wait()
}

// recoverWorkflow attempts to recover a stalled workflow
func (s *WorkflowSupervisor) recoverWorkflow(workflowID string, state *WorkflowState) {
	s.config.Logger.Printf("Attempting to recover workflow: %s", workflowID)
	
	// Get the current step ID
	currentStepID := state.CurrentStepID
	if currentStepID == "" && len(state.CompletedStepIDs) < state.TotalSteps {
		// Determine which step should be running
		completedSteps := make(map[string]bool)
		for _, id := range state.CompletedStepIDs {
			completedSteps[id] = true
		}
		
		// Current step should be the first non-completed step (basic recovery)
		// In a real implementation, we would need to know all step IDs and their order
		currentStepID = fmt.Sprintf("step%d", len(state.CompletedStepIDs)+1)
	}
	
	if currentStepID == "" {
		s.config.Logger.Printf("Cannot determine current step for recovery of workflow %s", workflowID)
		return
	}
	
	// Create recovery job
	jobID := fmt.Sprintf("recovery:%s:%s", workflowID, currentStepID)
	stepIndex := len(state.CompletedStepIDs)
	
	// Recovery payload
	payload := map[string]any{
		"workflow_id":     workflowID,
		"step_index":      stepIndex,
		"total_steps":     state.TotalSteps,
		"next_step_index": stepIndex + 1,
		"state":           state.Data,
		"recovery":        true,
	}
	
	// Reset workflow state for recovery
	state.Status = WorkflowStatusRunning
	state.LastHeartbeat = time.Now()
	state.LastUpdateAt = time.Now()
	
	// Serialize and store updated state
	updatedStateStr, err := state.Serialize()
	if err != nil {
		s.config.Logger.Printf("Error serializing recovery state for %s: %v", workflowID, err)
		return
	}
	
	stateKey := fmt.Sprintf("wf:%s:state", workflowID)
	err = s.config.Memory.Store(stateKey, updatedStateStr, agents.WithTTL(24*time.Hour))
	if err != nil {
		s.config.Logger.Printf("Error storing recovery state for %s: %v", workflowID, err)
		return
	}
	
	// Enqueue recovery job
	err = s.config.JobQueue.Push(s.ctx, jobID, currentStepID, payload)
	if err != nil {
		s.config.Logger.Printf("Error enqueueing recovery job for %s: %v", workflowID, err)
		
		// Move to dead letter queue if recovery fails
		deadLetterKey := fmt.Sprintf("dlq:%s:%s", s.config.DeadLetterQueue, workflowID)
		dlqData := map[string]any{
			"workflow_id": workflowID,
			"step_id":     currentStepID,
			"error":       err.Error(),
			"timestamp":   time.Now().Unix(),
			"state":       state,
		}
		
		// Serialize dead letter data
		dlqBytes, _ := json.Marshal(dlqData)
		dlqErr := s.config.Memory.Store(deadLetterKey, string(dlqBytes), agents.WithTTL(s.config.RetentionPeriod))
		if dlqErr != nil {
			s.config.Logger.Printf("Error storing dead letter for %s: %v", workflowID, dlqErr)
		}
	} else {
		s.config.Logger.Printf("Successfully enqueued recovery job for workflow %s", workflowID)
	}
}

// cleanupCompletedWorkflows removes old completed workflows based on retention period
func (s *WorkflowSupervisor) cleanupCompletedWorkflows() {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	
	// Get all keys from memory store
	keys, err := s.config.Memory.List()
	if err != nil {
		s.config.Logger.Printf("Error listing keys: %v", err)
		return
	}
	
	// Find workflow state keys
	var workflowStateKeys []string
	for _, key := range keys {
		if strings.HasPrefix(key, "wf:") && strings.HasSuffix(key, ":state") {
			workflowStateKeys = append(workflowStateKeys, key)
		}
	}
	
	now := time.Now()
	retentionThreshold := now.Add(-s.config.RetentionPeriod)
	
	// Limit concurrent cleanups
	semaphore := make(chan struct{}, s.config.MaxConcurrentRecoveries)
	var wg sync.WaitGroup
	
	// Check each workflow
	for _, key := range workflowStateKeys {
		semaphore <- struct{}{} // Acquire
		wg.Add(1)
		
		go func(stateKey string) {
			defer func() {
				<-semaphore // Release
				wg.Done()
			}()
			
			// Extract workflow ID from key pattern "wf:{id}:state"
			parts := strings.Split(stateKey, ":")
			if len(parts) != 3 {
				return
			}
			workflowID := parts[1]
			
			// Get the workflow state
			stateValue, err := s.config.Memory.Retrieve(stateKey)
			if err != nil {
				return // Skip if error retrieving
			}
			
			// Convert to JSON string if needed
			var stateStr string
			switch v := stateValue.(type) {
			case string:
				stateStr = v
			case []byte:
				stateStr = string(v)
			default:
				jsonBytes, err := json.Marshal(stateValue)
				if err != nil {
					return
				}
				stateStr = string(jsonBytes)
			}
			
			// Deserialize the workflow state
			state, err := DeserializeWorkflowState(stateStr)
			if err != nil {
				return
			}
			
			// Check if it's completed and older than retention period
			if (state.Status == WorkflowStatusCompleted || 
				state.Status == WorkflowStatusFailed || 
				state.Status == WorkflowStatusCanceled) && 
				state.CompletedAt != nil && 
				state.CompletedAt.Before(retentionThreshold) {
				
				// Cleanup all keys for this workflow
				prefixToClean := fmt.Sprintf("wf:%s:", workflowID)
				
				// Find all keys for this workflow
				var workflowKeys []string
				for _, k := range keys {
					if strings.HasPrefix(k, prefixToClean) {
						workflowKeys = append(workflowKeys, k)
					}
				}
				
				// Delete all keys
				for _, k := range workflowKeys {
					err := s.config.Memory.Store(k, nil, agents.WithTTL(time.Millisecond)) // Very short TTL to effectively delete
					if err != nil {
						s.config.Logger.Printf("Error cleaning up workflow key %s: %v", k, err)
					}
				}
				
				s.config.Logger.Printf("Cleaned up completed workflow: %s", workflowID)
			}
		}(key)
	}
	
	wg.Wait()
}

// GetStatus gets the current status of a workflow
func (s *WorkflowSupervisor) GetStatus(workflowID string) (*WorkflowState, error) {
	stateKey := fmt.Sprintf("wf:%s:state", workflowID)
	
	// Get the workflow state
	stateValue, err := s.config.Memory.Retrieve(stateKey)
	if err != nil {
		return nil, errors.WithFields(
			errors.Wrap(err, errors.ResourceNotFound, "workflow state not found"),
			errors.Fields{"workflow_id": workflowID},
		)
	}
	
	// Convert to JSON string if needed
	var stateStr string
	switch v := stateValue.(type) {
	case string:
		stateStr = v
	case []byte:
		stateStr = string(v)
	default:
		jsonBytes, err := json.Marshal(stateValue)
		if err != nil {
			return nil, errors.Wrap(err, errors.InvalidResponse, "failed to marshal workflow state")
		}
		stateStr = string(jsonBytes)
	}
	
	// Deserialize the workflow state
	state, err := DeserializeWorkflowState(stateStr)
	if err != nil {
		return nil, errors.Wrap(err, errors.InvalidResponse, "failed to deserialize workflow state")
	}
	
	return state, nil
}

// CancelWorkflow cancels a running workflow
func (s *WorkflowSupervisor) CancelWorkflow(workflowID string) error {
	state, err := s.GetStatus(workflowID)
	if err != nil {
		return err
	}
	
	if state.Status != WorkflowStatusRunning && state.Status != WorkflowStatusStalled {
		return errors.WithFields(
			errors.New(errors.Unknown, "workflow is not running or stalled"),
			errors.Fields{
				"workflow_id": workflowID,
				"status":      string(state.Status),
			},
		)
	}
	
	// Mark as canceled
	state.MarkCanceled()
	
	// Serialize and store updated state
	updatedStateStr, err := state.Serialize()
	if err != nil {
		return errors.Wrap(err, errors.Unknown, "failed to serialize canceled state")
	}
	
	stateKey := fmt.Sprintf("wf:%s:state", workflowID)
	err = s.config.Memory.Store(stateKey, updatedStateStr, agents.WithTTL(24*time.Hour))
	if err != nil {
		return errors.Wrap(err, errors.Unknown, "failed to store canceled state")
	}
	
	return nil
}

// ResetStuckJob attempts to reset a specific stuck job in a workflow
func (s *WorkflowSupervisor) ResetStuckJob(workflowID, stepID string) error {
	state, err := s.GetStatus(workflowID)
	if err != nil {
		return err
	}
	
	if state.Status != WorkflowStatusStalled && state.Status != WorkflowStatusFailed {
		return errors.WithFields(
			errors.New(errors.Unknown, "workflow is not stalled or failed"),
			errors.Fields{
				"workflow_id": workflowID,
				"status":      string(state.Status),
			},
		)
	}
	
	// Create recovery job
	jobID := fmt.Sprintf("recovery:%s:%s", workflowID, stepID)
	
	// Find step index
	stepIndex := -1
	for i, completedStep := range state.CompletedStepIDs {
		if completedStep == stepID {
			stepIndex = i
			break
		}
	}
	
	if stepIndex == -1 {
		stepIndex = len(state.CompletedStepIDs)
	}
	
	// Recovery payload
	payload := map[string]any{
		"workflow_id":     workflowID,
		"step_index":      stepIndex,
		"total_steps":     state.TotalSteps,
		"next_step_index": stepIndex + 1,
		"state":           state.Data,
		"recovery":        true,
	}
	
	// Reset workflow state for recovery
	state.Status = WorkflowStatusRunning
	state.LastHeartbeat = time.Now()
	state.LastUpdateAt = time.Now()
	state.CurrentStepID = stepID
	
	// Remove from failed steps if present
	newFailedSteps := make([]string, 0)
	for _, id := range state.FailedStepIDs {
		if id != stepID {
			newFailedSteps = append(newFailedSteps, id)
		}
	}
	state.FailedStepIDs = newFailedSteps
	
	// Serialize and store updated state
	updatedStateStr, err := state.Serialize()
	if err != nil {
		return errors.Wrap(err, errors.Unknown, "failed to serialize recovery state")
	}
	
	stateKey := fmt.Sprintf("wf:%s:state", workflowID)
	err = s.config.Memory.Store(stateKey, updatedStateStr, agents.WithTTL(24*time.Hour))
	if err != nil {
		return errors.Wrap(err, errors.Unknown, "failed to store recovery state")
	}
	
	// Enqueue recovery job
	err = s.config.JobQueue.Push(s.ctx, jobID, stepID, payload)
	if err != nil {
		return errors.WithFields(
			errors.Wrap(err, errors.Unknown, "failed to enqueue recovery job"),
			errors.Fields{
				"workflow_id": workflowID,
				"step_id":     stepID,
			},
		)
	}
	
	return nil
}

// ListDeadLetterJobs returns all jobs in the dead letter queue
func (s *WorkflowSupervisor) ListDeadLetterJobs() ([]map[string]any, error) {
	// Get all keys from memory store
	keys, err := s.config.Memory.List()
	if err != nil {
		return nil, errors.Wrap(err, errors.Unknown, "failed to list keys")
	}
	
	// Find dead letter queue keys
	dlqPrefix := fmt.Sprintf("dlq:%s:", s.config.DeadLetterQueue)
	var dlqKeys []string
	for _, key := range keys {
		if strings.HasPrefix(key, dlqPrefix) {
			dlqKeys = append(dlqKeys, key)
		}
	}
	
	// Get dead letter jobs
	var jobs []map[string]any
	for _, key := range dlqKeys {
		value, err := s.config.Memory.Retrieve(key)
		if err != nil {
			continue
		}
		
		var job map[string]any
		var jsonStr string
		
		switch v := value.(type) {
		case string:
			jsonStr = v
		case []byte:
			jsonStr = string(v)
		default:
			continue
		}
		
		err = json.Unmarshal([]byte(jsonStr), &job)
		if err != nil {
			continue
		}
		
		jobs = append(jobs, job)
	}
	
	return jobs, nil
}

// RetryDeadLetterJob attempts to retry a job from the dead letter queue
func (s *WorkflowSupervisor) RetryDeadLetterJob(workflowID, stepID string) error {
	// Get the dead letter job
	dlqKey := fmt.Sprintf("dlq:%s:%s", s.config.DeadLetterQueue, workflowID)
	value, err := s.config.Memory.Retrieve(dlqKey)
	if err != nil {
		return errors.WithFields(
			errors.Wrap(err, errors.ResourceNotFound, "dead letter job not found"),
			errors.Fields{
				"workflow_id": workflowID,
				"step_id":     stepID,
			},
		)
	}
	
	var dlqData map[string]any
	var jsonStr string
	
	switch v := value.(type) {
	case string:
		jsonStr = v
	case []byte:
		jsonStr = string(v)
	default:
		jsonBytes, err := json.Marshal(value)
		if err != nil {
			return errors.Wrap(err, errors.InvalidResponse, "failed to marshal dead letter job")
		}
		jsonStr = string(jsonBytes)
	}
	
	err = json.Unmarshal([]byte(jsonStr), &dlqData)
	if err != nil {
		return errors.Wrap(err, errors.InvalidResponse, "failed to unmarshal dead letter job")
	}
	
	// Create recovery job
	jobID := fmt.Sprintf("recovery:%s:%s", workflowID, stepID)
	
	// Get state from dead letter
	stateObj, ok := dlqData["state"]
	if !ok {
		return errors.New(errors.InvalidResponse, "dead letter job has no state")
	}
	
	var state *WorkflowState
	stateBytes, err := json.Marshal(stateObj)
	if err != nil {
		return errors.Wrap(err, errors.InvalidResponse, "failed to marshal state from dead letter")
	}
	
	err = json.Unmarshal(stateBytes, &state)
	if err != nil {
		return errors.Wrap(err, errors.InvalidResponse, "failed to unmarshal state from dead letter")
	}
	
	// Reset workflow state for recovery
	state.Status = WorkflowStatusRunning
	state.LastHeartbeat = time.Now()
	state.LastUpdateAt = time.Now()
	state.CurrentStepID = stepID
	
	// Remove from failed steps if present
	newFailedSteps := make([]string, 0)
	for _, id := range state.FailedStepIDs {
		if id != stepID {
			newFailedSteps = append(newFailedSteps, id)
		}
	}
	state.FailedStepIDs = newFailedSteps
	
	// Recovery payload
	stepIndex := len(state.CompletedStepIDs)
	payload := map[string]any{
		"workflow_id":     workflowID,
		"step_index":      stepIndex,
		"total_steps":     state.TotalSteps,
		"next_step_index": stepIndex + 1,
		"state":           state.Data,
		"recovery":        true,
	}
	
	// Serialize and store updated state
	updatedStateStr, err := state.Serialize()
	if err != nil {
		return errors.Wrap(err, errors.Unknown, "failed to serialize recovery state")
	}
	
	stateKey := fmt.Sprintf("wf:%s:state", workflowID)
	err = s.config.Memory.Store(stateKey, updatedStateStr, agents.WithTTL(24*time.Hour))
	if err != nil {
		return errors.Wrap(err, errors.Unknown, "failed to store recovery state")
	}
	
	// Enqueue recovery job
	err = s.config.JobQueue.Push(s.ctx, jobID, stepID, payload)
	if err != nil {
		return errors.WithFields(
			errors.Wrap(err, errors.Unknown, "failed to enqueue recovery job"),
			errors.Fields{
				"workflow_id": workflowID,
				"step_id":     stepID,
			},
		)
	}
	
	// Remove from dead letter queue
	err = s.config.Memory.Store(dlqKey, nil, agents.WithTTL(time.Millisecond))
	if err != nil {
		s.config.Logger.Printf("Error removing job from dead letter queue: %v", err)
	}
	
	return nil
} 