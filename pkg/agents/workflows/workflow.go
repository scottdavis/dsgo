package workflows

import (
	"context"
	"fmt"

	"github.com/scottdavis/dsgo/pkg/agents"
	"github.com/scottdavis/dsgo/pkg/agents/memory"
)

// Workflow represents a sequence of steps that accomplish a task.
type Workflow interface {
	// Execute runs the workflow with the provided inputs
	Execute(ctx context.Context, inputs map[string]any) (map[string]any, error)

	// GetSteps returns all steps in this workflow
	GetSteps() []*Step

	// AddStep adds a new step to the workflow
	AddStep(step *Step) error
}

// BaseWorkflow provides common workflow functionality.
type BaseWorkflow struct {
	// steps stores all steps in the workflow
	steps []*Step

	// stepIndex provides quick lookup of steps by ID
	stepIndex map[string]*Step

	// memory provides persistence between workflow runs
	memory agents.Memory
}

func NewBaseWorkflow(mem agents.Memory) *BaseWorkflow {
	return &BaseWorkflow{
		steps:     make([]*Step, 0),
		stepIndex: make(map[string]*Step),
		memory:    mem,
	}
}

// ExecuteWithContext prepares a context with memory store for execution
func (w *BaseWorkflow) ExecuteWithContext(ctx context.Context) context.Context {
	// Add memory to context for modules to access
	return memory.WithMemoryStore(ctx, w.memory)
}

func (w *BaseWorkflow) AddStep(step *Step) error {
	// Validate step ID is unique
	if _, exists := w.stepIndex[step.ID]; exists {
		return fmt.Errorf("step with ID %s already exists", step.ID)
	}

	// Add step to workflow
	w.steps = append(w.steps, step)
	w.stepIndex[step.ID] = step
	return nil
}

func (w *BaseWorkflow) GetSteps() []*Step {
	return w.steps
}

// ValidateWorkflow checks if the workflow structure is valid.
func (w *BaseWorkflow) ValidateWorkflow() error {
	// Validate that all nextSteps references exist
	for _, step := range w.steps {
		for _, nextID := range step.NextSteps {
			if _, exists := w.stepIndex[nextID]; !exists {
				return fmt.Errorf("step %s references non-existent step %s", step.ID, nextID)
			}
		}
	}

	// Check for cycles in the workflow
	visited := make(map[string]bool)
	path := make(map[string]bool)

	// Helper function for DFS cycle detection
	var checkCycle func(stepID string) error
	checkCycle = func(stepID string) error {
		// If we've seen this node in the current path, we have a cycle
		if path[stepID] {
			return fmt.Errorf("cycle detected in workflow involving step %s", stepID)
		}

		// If we've already visited this node and found no cycles, skip it
		if visited[stepID] {
			return nil
		}

		// Mark as visited and add to current path
		visited[stepID] = true
		path[stepID] = true

		// Check all next steps
		step := w.stepIndex[stepID]
		for _, nextID := range step.NextSteps {
			if err := checkCycle(nextID); err != nil {
				return err
			}
		}

		// Remove from current path as we backtrack
		path[stepID] = false
		return nil
	}

	// Start DFS from each step to find cycles
	for _, step := range w.steps {
		if err := checkCycle(step.ID); err != nil {
			return err
		}
	}

	return nil
}

// GetMemory returns the memory associated with this workflow
func (w *BaseWorkflow) GetMemory() agents.Memory {
	return w.memory
}
