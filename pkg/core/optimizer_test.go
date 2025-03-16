package core

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockOptimizer is a mock implementation of the Optimizer interface.
type MockOptimizer struct {
	mock.Mock
}

func (m *MockOptimizer) Compile(ctx context.Context, program *Program, dataset Dataset, metric Metric) (*Program, error) {
	args := m.Called(ctx, program, dataset, metric)
	return args.Get(0).(*Program), args.Error(1)
}

// MockDataset is a mock implementation of the Dataset interface.
type MockDataset struct {
	mock.Mock
	examples []Example
	position int
}

func NewMockDataset(examples []Example) *MockDataset {
	return &MockDataset{
		examples: examples,
		position: 0,
	}
}

func (m *MockDataset) Next() (Example, bool) {
	if m.position < len(m.examples) {
		example := m.examples[m.position]
		m.position++
		return example, true
	}
	return Example{}, false
}

func (m *MockDataset) Reset() {
	m.position = 0
}

// TestOptimizerRegistry tests the OptimizerRegistry.
func TestOptimizerRegistry(t *testing.T) {
	registry := NewOptimizerRegistry()

	// Test registering an Optimizer
	registry.Register("test", func() (Optimizer, error) {
		return &MockOptimizer{}, nil
	})

	// Test creating a registered Optimizer
	optimizer, err := registry.Create("test")
	assert.NoError(t, err)
	_, ok := optimizer.(*MockOptimizer)
	assert.True(t, ok, "Created Optimizer is not of expected type")

	// Test creating an unregistered Optimizer
	_, err = registry.Create("nonexistent")
	assert.Error(t, err)
}

// TestCompileOptions tests the CompileOptions and related functions.
func TestCompileOptions(t *testing.T) {
	opts := &CompileOptions{}

	WithMaxTrials(10)(opts)
	assert.Equal(t, 10, opts.MaxTrials)

	config := NewDSPYConfig()
	sig := NewSignature(
		[]InputField{{Field: Field{Name: "input"}}},
		[]OutputField{{Field: Field{Name: "output"}}},
	)
	
	mockModule := new(MockProgramModule)
	mockModule.On("GetSignature").Return(sig).Maybe()
	
	teacherProgram := NewProgram(
		map[string]Module{"test": mockModule},
		func(ctx context.Context, inputs map[string]any) (map[string]any, error) {
			return inputs, nil
		},
		config,
	)

	WithTeacher(teacherProgram)(opts)
	assert.NotNil(t, opts.Teacher)
	assert.Len(t, opts.Teacher.Modules, 1)
	assert.NotNil(t, opts.Teacher.Forward)
}

// TestBootstrapFewShot tests the BootstrapFewShot optimizer.
func TestBootstrapFewShot(t *testing.T) {
	optimizer := NewBootstrapFewShot(5)

	assert.Equal(t, 5, optimizer.MaxExamples)

	// Create a simple program for testing
	config := NewDSPYConfig()
	sig := NewSignature(
		[]InputField{{Field: Field{Name: "input"}}},
		[]OutputField{{Field: Field{Name: "output"}}},
	)
	
	mockModule := new(MockProgramModule)
	mockModule.On("GetSignature").Return(sig).Maybe()
	mockModule.On("Process", mock.Anything, mock.Anything).Return(map[string]any{"output": "test"}, nil).Maybe()
	
	program := NewProgram(
		map[string]Module{"test": mockModule},
		nil,
		config,
	)

	// Create a simple dataset for testing
	examples := []Example{
		{
			Input:  map[string]any{"input": "test1"},
			Output: map[string]any{"output": "result1"},
		},
		{
			Input:  map[string]any{"input": "test2"},
			Output: map[string]any{"output": "result2"},
		},
	}
	dataset := NewMockDataset(examples)

	// Create a simple metric for testing
	metric := func(ctx context.Context, result map[string]any) (bool, string) {
		return true, ""
	}

	// Execute the test
	_, err := optimizer.Compile(context.Background(), program, dataset, metric)
	assert.NoError(t, err)
}

// TestOptimizer tests the optimizer functionality.
func TestOptimizer(t *testing.T) {
	mockOptimizer := new(MockOptimizer)
	
	config := NewDSPYConfig()
	sig := NewSignature(
		[]InputField{{Field: Field{Name: "input"}}},
		[]OutputField{{Field: Field{Name: "output"}}},
	)
	
	mockModule := new(MockProgramModule)
	mockModule.On("GetSignature").Return(sig).Maybe()
	
	program := NewProgram(
		map[string]Module{"test": mockModule},
		nil,
		config,
	)
	
	examples := []Example{
		{
			Input:  map[string]any{"input": "test1"},
			Output: map[string]any{"output": "result1"},
		},
	}
	dataset := NewMockDataset(examples)
	
	metric := func(ctx context.Context, result map[string]any) (bool, string) {
		return true, ""
	}
	
	// Set up expectations
	mockOptimizer.On("Compile", mock.Anything, program, dataset, mock.Anything).Return(program, nil)
	
	// Test the optimizer
	result, err := mockOptimizer.Compile(context.Background(), program, dataset, metric)
	
	assert.NoError(t, err)
	assert.Equal(t, program, result)
	mockOptimizer.AssertExpectations(t)
}

// TestOptimizeProgram tests the OptimizeProgram function.
func TestOptimizeProgram(t *testing.T) {
	mockOptimizer := new(MockOptimizer)
	
	config := NewDSPYConfig()
	sig := NewSignature(
		[]InputField{{Field: Field{Name: "input"}}},
		[]OutputField{{Field: Field{Name: "output"}}},
	)
	
	mockModule := new(MockProgramModule)
	mockModule.On("GetSignature").Return(sig).Maybe()
	
	program := NewProgram(
		map[string]Module{"test": mockModule},
		nil,
		config,
	)
	
	examples := []Example{
		{
			Input:  map[string]any{"input": "test1"},
			Output: map[string]any{"output": "result1"},
		},
	}
	dataset := NewMockDataset(examples)
	
	metric := func(ctx context.Context, result map[string]any) (bool, string) {
		return true, ""
	}
	
	// Set up expectations
	mockOptimizer.On("Compile", mock.Anything, program, dataset, mock.Anything).Return(program, nil)
	
	// Test OptimizeProgram
	result, err := OptimizeProgram(context.Background(), program, mockOptimizer, dataset, metric)
	
	assert.NoError(t, err)
	assert.Equal(t, program, result)
	mockOptimizer.AssertExpectations(t)
} 