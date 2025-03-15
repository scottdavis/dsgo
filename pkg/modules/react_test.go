package modules

import (
	"context"
	"testing"

	testutil "github.com/XiaoConstantine/dspy-go/internal/testutil"
	"github.com/XiaoConstantine/dspy-go/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestReAct(t *testing.T) {
	// Create a mock LLM
	mockLLM := new(testutil.MockLLM)

	// Set up the expected behavior for the first call (action)
	firstResponse := &core.LLMResponse{
		Content: "thought: I need to search for information\naction: Search\nobservation: ",
	}
	
	// Set up the expected behavior for the second call (finish)
	secondResponse := &core.LLMResponse{
		Content: "thought: I found the answer\naction: Finish\nanswer: 42",
	}
	
	// Configure the mock to return different responses on consecutive calls
	mockLLM.On("Generate", mock.Anything, mock.Anything, mock.Anything).Return(firstResponse, nil).Once()
	mockLLM.On("Generate", mock.Anything, mock.Anything, mock.Anything).Return(secondResponse, nil).Once()

	// Create a mock tool
	mockTool := testutil.NewMockTool("Search")
	mockTool.On("Metadata").Return(&core.ToolMetadata{
		Name:        "Search",
		Description: "Search for information",
		Capabilities: []string{"search"},
	})
	mockTool.On("CanHandle", mock.Anything, "Search").Return(true)
	mockTool.On("Validate", mock.Anything).Return(nil)
	mockTool.On("Execute", mock.Anything, mock.Anything).Return(core.ToolResult{
		Data: "Found information about the meaning of life",
	}, nil)

	// Create a config with the mock LLM
	config := core.NewDSPYConfig().WithDefaultLLM(mockLLM)
	
	// Create a ReAct module
	signature := core.NewSignature(
		[]core.InputField{{Field: core.Field{Name: "question"}}},
		[]core.OutputField{{Field: core.NewField("answer")}},
	)
	react := NewReAct(signature, []core.Tool{mockTool}, 5, config)

	// Test the Process method
	ctx := context.Background()
	ctx = core.WithExecutionState(ctx)

	inputs := map[string]any{"question": "What is the meaning of life?"}
	outputs, err := react.Process(ctx, inputs)

	// Assert the results
	assert.NoError(t, err)
	assert.Equal(t, "42", outputs["answer"])
	assert.Equal(t, "I found the answer", outputs["thought"])
	assert.Equal(t, "Finish", outputs["action"])

	// Verify that the mocks were called as expected
	mockLLM.AssertExpectations(t)
	mockTool.AssertExpectations(t)
}
