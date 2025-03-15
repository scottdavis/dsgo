package modules

import (
	"context"
	"testing"

	testutil "github.com/XiaoConstantine/dspy-go/internal/testutil"
	"github.com/XiaoConstantine/dspy-go/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestChainOfThought(t *testing.T) {
	// Create a mock LLM
	mockLLM := new(testutil.MockLLM)

	// Set up the expected behavior
	expectedResponse := &core.LLMResponse{
		Content: "rationale: Let me think about this\nanswer: 42",
	}
	mockLLM.On("Generate", mock.Anything, mock.Anything, mock.Anything).Return(expectedResponse, nil)

	// Create a config with the mock LLM
	config := core.NewDSPYConfig().WithDefaultLLM(mockLLM)

	// Create a ChainOfThought module
	signature := core.NewSignature(
		[]core.InputField{{Field: core.Field{Name: "question"}}},
		[]core.OutputField{{Field: core.NewField("answer")}},
	)
	cot := NewChainOfThought(signature, config)

	// Test the Process method
	ctx := context.Background()
	ctx = core.WithExecutionState(ctx)

	inputs := map[string]any{"question": "What is the meaning of life?"}
	outputs, err := cot.Process(ctx, inputs)

	// Assert the results
	assert.NoError(t, err)
	assert.Equal(t, "Let me think about this", outputs["rationale"])
	assert.Equal(t, "42", outputs["answer"])

	// Verify that the mock was called as expected
	mockLLM.AssertExpectations(t)
}
