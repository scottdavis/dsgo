package modules

import (
	"context"
	"testing"

	testutil "github.com/scottdavis/dsgo/internal/testutil"
	"github.com/scottdavis/dsgo/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestPredict(t *testing.T) {
	// Create a mock LLM
	mockLLM := new(testutil.MockLLM)

	// Expected response
	expectedResponse := &core.LLMResponse{
		Content: "answer:\n\t42\n",
	}

	// Set up the expected behavior
	mockLLM.On("Generate", mock.Anything, mock.Anything, mock.Anything).Return(expectedResponse, nil)

	// Create a config with the mock LLM
	config := core.NewDSPYConfig().WithDefaultLLM(mockLLM)

	// Create a Predict module with the config
	signature := core.NewSignature(
		[]core.InputField{{Field: core.Field{Name: "question"}}},
		[]core.OutputField{{Field: core.NewField("answer")}},
	)
	predict := NewPredict(signature, config)

	// Test the Process method
	ctx := context.Background()
	ctx = core.WithExecutionState(ctx)

	inputs := map[string]any{"question": "What is the meaning of life?"}
	outputs, err := predict.Process(ctx, inputs)

	// Assert the results
	assert.NoError(t, err)
	assert.Equal(t, "42", outputs["answer"])

	// Verify that the mock was called as expected
	mockLLM.AssertExpectations(t)

	// Verify traces
	spans := core.CollectSpans(ctx)
	require.Len(t, spans, 1)
	span := spans[0]

	inputsMap, _ := span.Annotations["inputs"].(map[string]any)
	question, _ := inputsMap["question"].(string)

	outputsMap, _ := span.Annotations["outputs"].(map[string]any)
	answer, _ := outputsMap["answer"].(string)

	assert.Contains(t, question, "What is the meaning of life?")
	assert.Contains(t, answer, "42")
}
