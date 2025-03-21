package workflows

import (
	"context"
	"fmt"

	"github.com/scottdavis/dsgo/pkg/agents"
	"github.com/scottdavis/dsgo/pkg/core"
	"github.com/scottdavis/dsgo/pkg/errors"
)

// ChainWorkflow executes steps in a linear sequence, where each step's output
// can be used as input for subsequent steps.
type ChainWorkflow struct {
	*BaseWorkflow
}

func NewChainWorkflow(memory agents.Memory) *ChainWorkflow {
	return &ChainWorkflow{
		BaseWorkflow: NewBaseWorkflow(memory),
	}
}

// Execute runs steps sequentially, passing state from one step to the next.
func (w *ChainWorkflow) Execute(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	// Initialize workflow state with input values
	state := make(map[string]any)
	for k, v := range inputs {
		state[k] = v
	}

	// Add memory to context for modules to access
	ctx = w.ExecuteWithContext(ctx)

	totalSteps := len(w.steps)
	// Execute steps in sequence
	for i, step := range w.steps {

		stepCtx, stepSpan := core.StartSpan(ctx, fmt.Sprintf("ChainStep_%d", i))

		stepSpan.WithAnnotation("chain_step", map[string]any{
			"name":  step.ID,
			"index": i,
			"total": totalSteps,
		})

		signature := step.Module.GetSignature()

		// Create subset of state containing only the fields this step needs
		stepInputs := make(map[string]any)
		for _, field := range signature.Inputs {
			if val, ok := state[field.Name]; ok {
				stepInputs[field.Name] = val
			}
		}

		// Execute the step
		result, err := step.Execute(stepCtx, stepInputs)

		core.EndSpan(stepCtx)
		if err != nil {
			return nil, errors.WithFields(
				errors.Wrap(err, errors.StepExecutionFailed, "step execution failed"),
				errors.Fields{
					"step_id": step.ID,
					"step":    i + 1,
					"inputs":  stepInputs,
					"total":   totalSteps,
				})
		}

		// Update state with step outputs
		for k, v := range result.Outputs {
			state[k] = v
		}
	}

	return state, nil
}
