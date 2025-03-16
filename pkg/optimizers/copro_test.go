package optimizers

import (
	"context"
	"testing"

	"github.com/scottdavis/dsgo/pkg/core"
	"github.com/scottdavis/dsgo/pkg/modules"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewCopro(t *testing.T) {
	mockLLM := setupMockLLM()
	config := core.NewDSPYConfig().WithDefaultLLM(mockLLM)

	metric := func(example, prediction map[string]any, ctx context.Context) bool {
		return true
	}

	mockOptimizer := new(MockSubOptimizer)

	copro := NewCopro(metric, 5, mockOptimizer, config)

	assert.Equal(t, 5, copro.MaxBootstrapped)
	assert.Same(t, mockOptimizer, copro.SubOptimizer)
	assert.Equal(t, config, copro.Config)
}

// MockSubOptimizer for testing.
type MockSubOptimizer struct {
	mock.Mock
}

func (m *MockSubOptimizer) Compile(ctx context.Context, program *core.Program, dataset core.Dataset, metric core.Metric) (*core.Program, error) {
	args := m.Called(ctx, program, dataset, metric)
	return args.Get(0).(*core.Program), args.Error(1)
}

func TestCopro_Compile(t *testing.T) {
	// Create a mock sub-optimizer
	mockSubOptimizer := new(MockSubOptimizer)

	// Create the test objects
	ctx := context.Background()
	mockLLM := setupMockLLM()
	config := core.NewDSPYConfig().WithDefaultLLM(mockLLM)

	// Create a test program
	sig := core.NewSignature(
		[]core.InputField{{Field: core.Field{Name: "input"}}},
		[]core.OutputField{{Field: core.Field{Name: "output"}}},
	)

	predict := modules.NewPredict(sig, config)

	program := core.NewProgram(
		map[string]core.Module{"predict": predict},
		func(ctx context.Context, inputs map[string]any) (map[string]any, error) {
			return mockLLM.GenerateWithJSON(ctx, inputs["input"].(string))
		},
		config,
	)

	// Create test data
	dataset := NewMockDataset([]core.Example{
		{
			Input:  map[string]any{"input": "test1"},
			Output: map[string]any{"output": "result1"},
		},
	})

	// Set up metric function
	metric := func(ctx context.Context, result map[string]any) (bool, string) {
		return true, ""
	}

	// Set up expectations
	mockSubOptimizer.On("Compile", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(program, nil)

	// Create and test Copro
	metric2 := func(example, prediction map[string]any, ctx context.Context) bool {
		return true
	}

	copro := NewCopro(metric2, 5, mockSubOptimizer, config)

	result, err := copro.Compile(ctx, program, dataset, metric)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	mockSubOptimizer.AssertExpectations(t)
}

func TestCopro_CompileWithTeacher(t *testing.T) {
	// For a more comprehensive test, we'll simulate a teacher program
	ctx := context.Background()
	mockLLM := setupMockLLM()
	teacherLLM := setupMockLLM() // Using another mock for teacher

	config := core.NewDSPYConfig().
		WithDefaultLLM(mockLLM).
		WithTeacherLLM(teacherLLM)

	mockSubOptimizer := new(MockSubOptimizer)

	// Create a test program
	sig := core.NewSignature(
		[]core.InputField{{Field: core.Field{Name: "input"}}},
		[]core.OutputField{{Field: core.Field{Name: "output"}}},
	)

	predict := modules.NewPredict(sig, config)

	program := core.NewProgram(
		map[string]core.Module{"predict": predict},
		func(ctx context.Context, inputs map[string]any) (map[string]any, error) {
			return mockLLM.GenerateWithJSON(ctx, inputs["input"].(string))
		},
		config,
	)

	// Create test data
	dataset := NewMockDataset([]core.Example{
		{
			Input:  map[string]any{"input": "test1"},
			Output: map[string]any{"output": "result1"},
		},
	})

	// Set up metric function
	metric := func(ctx context.Context, result map[string]any) (bool, string) {
		return true, ""
	}

	// Set expectation for sub-optimizer
	mockSubOptimizer.On("Compile", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(program, nil)

	// Create and test Copro
	metric2 := func(example, prediction map[string]any, ctx context.Context) bool {
		return true
	}

	copro := NewCopro(metric2, 5, mockSubOptimizer, config)

	result, err := copro.Compile(ctx, program, dataset, metric)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	mockSubOptimizer.AssertExpectations(t)
}
