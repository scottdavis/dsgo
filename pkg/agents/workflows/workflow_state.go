package workflows

import (
	"encoding/json"
	"time"
)

// WorkflowStatus defines the current state of a workflow
type WorkflowStatus string

const (
	// WorkflowStatusPending indicates the workflow is waiting to start
	WorkflowStatusPending WorkflowStatus = "pending"
	
	// WorkflowStatusRunning indicates the workflow is actively executing
	WorkflowStatusRunning WorkflowStatus = "running"
	
	// WorkflowStatusCompleted indicates the workflow has completed successfully
	WorkflowStatusCompleted WorkflowStatus = "completed"
	
	// WorkflowStatusFailed indicates the workflow has failed
	WorkflowStatusFailed WorkflowStatus = "failed"
	
	// WorkflowStatusCanceled indicates the workflow was canceled
	WorkflowStatusCanceled WorkflowStatus = "canceled"
	
	// WorkflowStatusStalled indicates the workflow has not made progress for an extended period
	WorkflowStatusStalled WorkflowStatus = "stalled"
)

// ErrorSeverity and ErrorPolicy types and constants are defined in errors.go

// WorkflowError represents a detailed error that occurred during workflow execution
type WorkflowError struct {
	// StepID is the ID of the step where the error occurred
	StepID string `json:"step_id"`
	
	// Error is the error message
	Error string `json:"error"`
	
	// Timestamp is when the error occurred
	Timestamp time.Time `json:"timestamp"`
	
	// Severity indicates how severe the error is
	Severity ErrorSeverity `json:"severity"`
	
	// AttemptCount indicates how many attempts have been made
	AttemptCount int `json:"attempt_count"`
	
	// Policy indicates how this error should be handled
	Policy ErrorPolicy `json:"policy"`
	
	// Details contains additional error details
	Details map[string]interface{} `json:"details,omitempty"`
}

// WorkflowState represents the complete state of a workflow
type WorkflowState struct {
	// ID is the unique identifier for the workflow
	ID string `json:"id"`
	
	// Status is the current status of the workflow
	Status WorkflowStatus `json:"status"`
	
	// CreatedAt is when the workflow was created
	CreatedAt time.Time `json:"created_at"`
	
	// StartedAt is when the workflow started executing
	StartedAt *time.Time `json:"started_at,omitempty"`
	
	// CompletedAt is when the workflow completed (successfully or not)
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	
	// LastUpdateAt is when the workflow was last updated
	LastUpdateAt time.Time `json:"last_update_at"`
	
	// LastHeartbeat is when the workflow last reported activity
	LastHeartbeat time.Time `json:"last_heartbeat"`
	
	// CurrentStepID is the ID of the currently executing step
	CurrentStepID string `json:"current_step_id,omitempty"`
	
	// CompletedStepIDs is a list of completed step IDs
	CompletedStepIDs []string `json:"completed_step_ids"`
	
	// FailedStepIDs is a list of failed step IDs
	FailedStepIDs []string `json:"failed_step_ids"`
	
	// RetryingStepIDs is a list of steps currently being retried
	RetryingStepIDs []string `json:"retrying_step_ids"`
	
	// SkippedStepIDs is a list of skipped step IDs
	SkippedStepIDs []string `json:"skipped_step_ids"`
	
	// TotalSteps is the total number of steps in the workflow
	TotalSteps int `json:"total_steps"`
	
	// Progress is the percentage of steps completed (0-100)
	Progress int `json:"progress"`
	
	// Errors contains any errors that occurred during execution
	Errors []WorkflowError `json:"errors,omitempty"`
	
	// Data contains the workflow's data/state
	Data map[string]interface{} `json:"data"`
}

// NewWorkflowState creates a new workflow state with the specified ID
func NewWorkflowState(id string, totalSteps int, initialData map[string]interface{}) *WorkflowState {
	now := time.Now()
	
	// Create a copy of the initial data to avoid sharing the map reference
	stateCopy := make(map[string]interface{})
	if initialData != nil {
		for k, v := range initialData {
			stateCopy[k] = v
		}
	}
	
	return &WorkflowState{
		ID:              id,
		Status:          WorkflowStatusPending,
		CreatedAt:       now,
		LastUpdateAt:    now,
		LastHeartbeat:   now,
		CompletedStepIDs: []string{},
		FailedStepIDs:    []string{},
		RetryingStepIDs:  []string{},
		SkippedStepIDs:   []string{},
		TotalSteps:       totalSteps,
		Progress:         0,
		Data:             stateCopy,
	}
}

// Serialize converts the workflow state to a JSON string
func (ws *WorkflowState) Serialize() (string, error) {
	bytes, err := json.Marshal(ws)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// DeserializeWorkflowState converts a JSON string back to a WorkflowState
func DeserializeWorkflowState(data string) (*WorkflowState, error) {
	var state WorkflowState
	err := json.Unmarshal([]byte(data), &state)
	if err != nil {
		return nil, err
	}
	return &state, nil
}

// UpdateProgress recalculates the workflow progress percentage
func (ws *WorkflowState) UpdateProgress() {
	if ws.TotalSteps == 0 {
		ws.Progress = 0
		return
	}
	
	completedCount := len(ws.CompletedStepIDs) + len(ws.SkippedStepIDs)
	ws.Progress = int((float64(completedCount) / float64(ws.TotalSteps)) * 100)
	
	// Cap at 100%
	if ws.Progress > 100 {
		ws.Progress = 100
	}
}

// AddCompletedStep marks a step as completed
func (ws *WorkflowState) AddCompletedStep(stepID string) {
	// Check if already in completed steps
	for _, id := range ws.CompletedStepIDs {
		if id == stepID {
			return
		}
	}
	
	ws.CompletedStepIDs = append(ws.CompletedStepIDs, stepID)
	ws.LastUpdateAt = time.Now()
	ws.UpdateProgress()
}

// AddFailedStep marks a step as failed
func (ws *WorkflowState) AddFailedStep(stepID string, err error, severity ErrorSeverity, policy ErrorPolicy) {
	// Add to failed steps
	for _, id := range ws.FailedStepIDs {
		if id == stepID {
			return
		}
	}
	
	ws.FailedStepIDs = append(ws.FailedStepIDs, stepID)
	
	// Add error details
	workflowError := WorkflowError{
		StepID:      stepID,
		Error:       err.Error(),
		Timestamp:   time.Now(),
		Severity:    severity,
		AttemptCount: 1, // Initial failure
		Policy:      policy,
	}
	
	// If there are existing errors for this step, update the attempt count
	for i, existingErr := range ws.Errors {
		if existingErr.StepID == stepID {
			workflowError.AttemptCount = existingErr.AttemptCount + 1
			ws.Errors[i] = workflowError
			ws.LastUpdateAt = time.Now()
			return
		}
	}
	
	// No existing errors for this step
	ws.Errors = append(ws.Errors, workflowError)
	ws.LastUpdateAt = time.Now()
}

// UpdateHeartbeat updates the last heartbeat time
func (ws *WorkflowState) UpdateHeartbeat() {
	ws.LastHeartbeat = time.Now()
}

// IsStalled checks if the workflow is stalled based on heartbeat threshold
func (ws *WorkflowState) IsStalled(threshold time.Duration) bool {
	return time.Since(ws.LastHeartbeat) > threshold
}

// HasErrors checks if the workflow has any errors
func (ws *WorkflowState) HasErrors() bool {
	return len(ws.Errors) > 0
}

// HasFatalErrors checks if the workflow has any workflow-fatal errors
func (ws *WorkflowState) HasFatalErrors() bool {
	for _, err := range ws.Errors {
		if err.Severity == ErrorSeverityWorkflowFatal {
			return true
		}
	}
	return false
}

// IsComplete checks if all steps are either completed, failed, or skipped
func (ws *WorkflowState) IsComplete() bool {
	totalHandled := len(ws.CompletedStepIDs) + len(ws.FailedStepIDs) + len(ws.SkippedStepIDs)
	return totalHandled >= ws.TotalSteps
}

// MarkRunning marks the workflow as running
func (ws *WorkflowState) MarkRunning() {
	ws.Status = WorkflowStatusRunning
	now := time.Now()
	ws.StartedAt = &now
	ws.LastUpdateAt = now
	ws.LastHeartbeat = now
}

// MarkCompleted marks the workflow as completed
func (ws *WorkflowState) MarkCompleted() {
	ws.Status = WorkflowStatusCompleted
	now := time.Now()
	ws.CompletedAt = &now
	ws.LastUpdateAt = now
	ws.Progress = 100
}

// MarkFailed marks the workflow as failed
func (ws *WorkflowState) MarkFailed() {
	ws.Status = WorkflowStatusFailed
	now := time.Now()
	ws.CompletedAt = &now
	ws.LastUpdateAt = now
}

// MarkCanceled marks the workflow as canceled
func (ws *WorkflowState) MarkCanceled() {
	ws.Status = WorkflowStatusCanceled
	now := time.Now()
	ws.CompletedAt = &now
	ws.LastUpdateAt = now
}

// MarkStalled marks the workflow as stalled
func (ws *WorkflowState) MarkStalled() {
	ws.Status = WorkflowStatusStalled
	ws.LastUpdateAt = time.Now()
} 