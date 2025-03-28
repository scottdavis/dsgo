package workflows

import (
	"context"
	"fmt"
	"sync"

	"github.com/scottdavis/dsgo/pkg/agents"
)

// ParallelWorkflow executes multiple steps concurrently.
type ParallelWorkflow struct {
	*BaseWorkflow
	// Maximum number of concurrent steps
	maxConcurrent int
}

func NewParallelWorkflow(memory agents.Memory, maxConcurrent int) *ParallelWorkflow {
	return &ParallelWorkflow{
		BaseWorkflow:  NewBaseWorkflow(memory),
		maxConcurrent: maxConcurrent,
	}
}

func (w *ParallelWorkflow) Execute(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	state := make(map[string]any)
	for k, v := range inputs {
		state[k] = v
	}

	// Add memory to context for modules to access
	ctx = w.ExecuteWithContext(ctx)

	// Create channel for collecting results
	results := make(chan *StepResult, len(w.steps))
	errors := make(chan error, len(w.steps))

	// Create semaphore to limit concurrency
	sem := make(chan struct{}, w.maxConcurrent)

	// Launch goroutine for each step
	var wg sync.WaitGroup
	for _, step := range w.steps {
		wg.Add(1)
		go doStep(ctx, &wg, sem, step, inputs, errors, results)
	}

	// Wait for all steps to complete
	go func() {
		wg.Wait()
		close(results)
		close(errors)
	}()

	// Collect results and errors
	var errs []error
	for err := range errors {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("parallel execution failed with %d errors: %v", len(errs), errs)
	}

	// Merge results into final state
	for result := range results {
		for k, v := range result.Outputs {
			state[k] = v
		}
	}

	return state, nil
}

func doStep(ctx context.Context, wg *sync.WaitGroup, sem chan struct{}, step *Step, inputs map[string]any, errors chan error, results chan *StepResult) {
	defer wg.Done()

	// Acquire semaphore
	sem <- struct{}{}
	defer func() { <-sem }()

	// Prepare inputs for this step
	stepInputs := make(map[string]any)
	signature := step.Module.GetSignature()

	for _, field := range signature.Inputs {
		if val, ok := inputs[field.Name]; ok {
			stepInputs[field.Name] = val
		}
	}

	// Execute step
	result, err := step.Execute(ctx, stepInputs)
	if err != nil {
		errors <- fmt.Errorf("step %s failed: %w", step.ID, err)
		return
	}
	results <- result

}
