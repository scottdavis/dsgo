package workflows

import "github.com/scottdavis/dsgo/pkg/errors"

var (
	// ErrStepConditionFailed indicates a step's condition check failed.
	ErrStepConditionFailed = errors.New(errors.InvalidWorkflowState, "step condition check failed")

	// ErrStepNotFound indicates a referenced step doesn't exist in workflow.
	ErrStepNotFound = errors.New(errors.ResourceNotFound, "step not found in workflow")

	// ErrInvalidInput indicates missing or invalid input parameters.
	ErrInvalidInput = errors.New(errors.InvalidInput, "invalid input parameters")

	// ErrDuplicateStepID indicates attempt to add step with existing ID.
	ErrDuplicateStepID = errors.New(errors.ValidationFailed, "duplicate step ID")

	// ErrCyclicDependency indicates circular dependencies between steps.
	ErrCyclicDependency = errors.New(errors.WorkflowExecutionFailed, "cyclic dependency detected in workflow")

	// ErrWorkflowNotFound indicates a workflow was not found
	ErrWorkflowNotFound = errors.New(errors.ResourceNotFound, "workflow not found")

	// ErrInvalidWorkflowState indicates the workflow state is invalid
	ErrInvalidWorkflowState = errors.New(errors.InvalidWorkflowState, "invalid workflow state")

	// ErrWorkflowAlreadyComplete indicates the workflow is already complete
	ErrWorkflowAlreadyComplete = errors.New(errors.InvalidWorkflowState, "workflow already complete")

	// ErrWorkflowFailed indicates the workflow has failed
	ErrWorkflowFailed = errors.New(errors.WorkflowExecutionFailed, "workflow failed")
)

// ErrorSeverity defines how critical an error is
type ErrorSeverity string

const (
	// ErrorSeverityNone indicates no error
	ErrorSeverityNone ErrorSeverity = "none"
	
	// ErrorSeverityRetryable indicates the error can be retried
	ErrorSeverityRetryable ErrorSeverity = "retryable"
	
	// ErrorSeverityStepFatal indicates the error is fatal for the current step
	ErrorSeverityStepFatal ErrorSeverity = "step_fatal"
	
	// ErrorSeverityWorkflowFatal indicates the error is fatal for the entire workflow
	ErrorSeverityWorkflowFatal ErrorSeverity = "workflow_fatal"
)

// ErrorPolicy defines how to handle errors
type ErrorPolicy string

const (
	// ErrorPolicyRetry indicates the error should be retried
	ErrorPolicyRetry ErrorPolicy = "retry"
	
	// ErrorPolicySkip indicates the error should be skipped
	ErrorPolicySkip ErrorPolicy = "skip"
	
	// ErrorPolicyFail indicates the error should fail the workflow
	ErrorPolicyFail ErrorPolicy = "fail"
)

func WrapWorkflowError(err error, fields map[string]any) error {
	if err == nil {
		return nil
	}
	return errors.WithFields(err, fields)
}
