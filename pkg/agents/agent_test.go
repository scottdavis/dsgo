package agents

import (
	"context"
	"testing"

	testutil "github.com/scottdavis/dsgo/internal/testutil"
	"github.com/scottdavis/dsgo/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockTool implements Tool interface for testing.
type MockTool struct {
	mock.Mock
}

func (m *MockTool) Name() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockTool) Description() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockTool) Execute(ctx context.Context, params map[string]any) (any, error) {
	args := m.Called(ctx, params)
	return args.Get(0), args.Error(1)
}

func (m *MockTool) ValidateParams(params map[string]any) error {
	args := m.Called(params)
	return args.Error(0)
}

// MockAgent implements Agent interface for testing.
type MockAgent struct {
	mock.Mock
}

func (m *MockAgent) Execute(ctx context.Context, input map[string]any) (map[string]any, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(map[string]any), args.Error(1)
}

func (m *MockAgent) GetCapabilities() []core.Tool {
	args := m.Called()
	return args.Get(0).([]core.Tool)
}

func (m *MockAgent) GetMemory() Memory {
	args := m.Called()
	return args.Get(0).(Memory)
}

func TestAgentInterface(t *testing.T) {
	t.Run("MockAgent Implementation", func(t *testing.T) {
		mockAgent := new(MockAgent)
		mockTool := testutil.NewMockTool("test_tool")
		mockMemory := NewInMemoryStore()

		// Setup expectations
		expectedOutput := map[string]any{
			"result": "success",
		}
		mockAgent.On("Execute", mock.Anything, mock.Anything).Return(expectedOutput, nil)
		mockAgent.On("GetCapabilities").Return([]core.Tool{mockTool})
		mockAgent.On("GetMemory").Return(mockMemory)

		// Test Execute
		ctx := context.Background()
		input := map[string]any{
			"test": "input",
		}
		output, err := mockAgent.Execute(ctx, input)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutput, output)

		// Test GetCapabilities
		capabilities := mockAgent.GetCapabilities()
		assert.Len(t, capabilities, 1)
		assert.Equal(t, mockTool, capabilities[0])

		// Test GetMemory
		memory := mockAgent.GetMemory()
		assert.Equal(t, mockMemory, memory)

		// Verify all expectations were met
		mockAgent.AssertExpectations(t)
	})

	t.Run("MockTool Implementation", func(t *testing.T) {
		mockTool := new(MockTool)

		// Setup expectations
		mockTool.On("Name").Return("TestTool")
		mockTool.On("Description").Return("Test tool description")
		mockTool.On("Execute", mock.Anything, mock.Anything).Return("result", nil)
		mockTool.On("ValidateParams", mock.Anything).Return(nil)

		// Test Name
		name := mockTool.Name()
		assert.Equal(t, "TestTool", name)

		// Test Description
		desc := mockTool.Description()
		assert.Equal(t, "Test tool description", desc)

		// Test Execute
		ctx := context.Background()
		params := map[string]any{
			"param": "value",
		}
		result, err := mockTool.Execute(ctx, params)
		assert.NoError(t, err)
		assert.Equal(t, "result", result)

		// Test ValidateParams
		err = mockTool.ValidateParams(params)
		assert.NoError(t, err)

		// Verify all expectations were met
		mockTool.AssertExpectations(t)
	})
}
