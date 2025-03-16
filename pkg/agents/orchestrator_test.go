package agents

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/scottdavis/dsgo/internal/testutil"
	"github.com/scottdavis/dsgo/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mock implementations for testing.
type MockTaskProcessor struct {
	mock.Mock
}

func (m *MockTaskProcessor) Process(ctx context.Context, task Task, context map[string]any) (any, error) {
	args := m.Called(ctx, task, context)
	return args.Get(0), args.Error(1)
}

type MockTaskParser struct {
	mock.Mock
}

func (m *MockTaskParser) Parse(analyzerOutput map[string]any) ([]Task, error) {
	args := m.Called(analyzerOutput)
	if tasks, ok := args.Get(0).([]Task); ok {
		return tasks, args.Error(1)
	}
	return nil, args.Error(1)
}

type MockPlanCreator struct {
	mock.Mock
}

func (m *MockPlanCreator) CreatePlan(tasks []Task) ([][]Task, error) {
	args := m.Called(tasks)
	if plan, ok := args.Get(0).([][]Task); ok {
		return plan, args.Error(1)
	}
	return nil, args.Error(1)
}

// MockMemory implements the Memory interface for testing.
type MockMemory struct {
	mock.Mock
}

func (m *MockMemory) Store(key string, value any) error {
	args := m.Called(key, value)
	return args.Error(0)
}

func (m *MockMemory) Retrieve(key string) (any, error) {
	args := m.Called(key)
	return args.Get(0), args.Error(1)
}

func (m *MockMemory) List() ([]string, error) {
	args := m.Called()
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockMemory) Clear() error {
	args := m.Called()
	return args.Error(0)
}

// testTaskParser is a simple implementation of TaskParser for testing.
type testTaskParser struct{}

func (p *testTaskParser) Parse(analyzerOutput map[string]any) ([]Task, error) {
	// Extract the subtasks from the analyzer output
	subtasksRaw, ok := analyzerOutput["subtasks"]
	if !ok {
		return nil, nil
	}

	// For this test, we expect a simple array with one task
	subtasks, ok := subtasksRaw.([]interface{})
	if !ok || len(subtasks) == 0 {
		return nil, nil
	}

	// Create a task from the first subtask
	task := Task{
		ID:            "1",
		Type:          "test",
		ProcessorType: "test",
		Priority:      1,
	}

	return []Task{task}, nil
}

// testPlanCreator is a simple implementation of PlanCreator for testing.
type testPlanCreator struct{}

func (p *testPlanCreator) CreatePlan(tasks []Task) ([][]Task, error) {
	// For this test, we just return all tasks in a single phase
	return [][]Task{tasks}, nil
}

// testProcessor is a simple implementation of TaskProcessor for testing.
type testProcessor struct{}

func (p *testProcessor) Process(ctx context.Context, task Task, taskContext map[string]any) (any, error) {
	// For this test, we just return a simple string
	return "processed", nil
}

// MockTaskAnalyzer is a mock implementation of an analyzer for testing.
type MockTaskAnalyzer struct {
	mock.Mock
}

func (m *MockTaskAnalyzer) Process(ctx context.Context, input map[string]any) (map[string]any, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(map[string]any), args.Error(1)
}

func TestFlexibleOrchestrator(t *testing.T) {
	t.Run("Simple test with mocked components", func(t *testing.T) {
		// Use a mock parser and creator instead of relying on the analyzer output
		mockParser := new(MockTaskParser)
		mockCreator := new(MockPlanCreator)
		mockProcessor := new(MockTaskProcessor)

		// Set up expectations
		tasks := []Task{
			{
				ID:            "1",
				Type:          "test",
				ProcessorType: "test",
				Priority:      1,
			},
		}

		// Configure the parser to return our tasks regardless of input
		mockParser.On("Parse", mock.Anything).Return(tasks, nil)

		// Expect creator to be called with tasks
		plan := [][]Task{tasks}
		mockCreator.On("CreatePlan", tasks).Return(plan, nil)

		// Expect processor to be called with task and context
		mockProcessor.On("Process", mock.Anything, tasks[0], mock.Anything).Return("processed", nil)

		// Create a mock memory implementation
		memory := new(MockMemory)
		memory.On("Store", mock.Anything, mock.Anything).Return(nil).Maybe()
		memory.On("Retrieve", mock.Anything).Return(nil, nil).Maybe()
		memory.On("List").Return([]string{}, nil).Maybe()
		memory.On("Clear").Return(nil).Maybe()

		// Create a mock LLM
		mockLLM := new(testutil.MockLLM)

		// Mock both Generate and GenerateWithJSON methods
		mockLLM.On("Generate", mock.Anything, mock.Anything, mock.Anything).Return(&core.LLMResponse{
			Content: `analysis: Task analyzed successfully
subtasks: [
  {
    "id": "1",
    "type": "test",
    "processor_type": "test",
    "priority": 1,
    "dependencies": []
  }
]`,
		}, nil)

		// The GenerateWithJSON method might not be called in all cases
		mockLLM.On("GenerateWithJSON", mock.Anything, mock.Anything, mock.Anything).Return(map[string]any{
			"subtasks": []map[string]any{
				{
					"id":             "1",
					"type":           "test",
					"processor_type": "test",
					"priority":       1,
				},
			},
			"analysis": "Task analyzed successfully",
		}, nil).Maybe()

		// Create a config with the mock LLM
		config := core.NewDSPYConfig().WithDefaultLLM(mockLLM)

		// Create the orchestration config with retry configured for only 1 attempt
		orchConfig := OrchestrationConfig{
			MaxConcurrent:  2,
			DefaultTimeout: 5 * time.Second,
			TaskParser:     mockParser,
			PlanCreator:    mockCreator,
			RetryConfig: &RetryConfig{
				MaxAttempts:       1, // Only try once since we're testing
				BackoffMultiplier: 1.0,
			},
			AnalyzerConfig: AnalyzerConfig{
				BaseInstruction: "Analyze this task",
			},
		}

		// Create the orchestrator
		orchestrator := NewFlexibleOrchestrator(memory, orchConfig, config)

		// Register the test processor
		orchestrator.RegisterProcessor("test", mockProcessor)

		// Test the orchestrator with a simple task
		ctx := context.Background()
		result, err := orchestrator.Process(ctx, "Test task", map[string]any{"context": "test"})

		// Verify the result
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Empty(t, result.FailedTasks)

		// Verify expectations were met
		mockParser.AssertExpectations(t)
		mockCreator.AssertExpectations(t)
		mockProcessor.AssertExpectations(t)
		memory.AssertExpectations(t)
		mockLLM.AssertExpectations(t)
	})

	t.Run("Error handling for LLM failure", func(t *testing.T) {
		// Create mocks
		mockParser := new(MockTaskParser)
		mockCreator := new(MockPlanCreator)

		// Create a mock memory implementation
		memory := new(MockMemory)
		memory.On("Store", mock.Anything, mock.Anything).Return(nil).Maybe()
		memory.On("Retrieve", mock.Anything).Return(nil, nil).Maybe()
		memory.On("List").Return([]string{}, nil).Maybe()
		memory.On("Clear").Return(nil).Maybe()

		// Create a mock LLM that returns an error
		mockLLM := new(testutil.MockLLM)

		// Mock both Generate and GenerateWithJSON methods to return errors
		mockLLM.On("Generate", mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("llm error"))

		// The GenerateWithJSON method might not be called in all cases
		mockLLM.On("GenerateWithJSON", mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("llm error")).Maybe()

		// Create a config with the mock LLM
		config := core.NewDSPYConfig().WithDefaultLLM(mockLLM)

		// Create the orchestration config with retry configured for only 1 attempt
		orchConfig := OrchestrationConfig{
			MaxConcurrent:  2,
			DefaultTimeout: 5 * time.Second,
			TaskParser:     mockParser,
			PlanCreator:    mockCreator,
			RetryConfig: &RetryConfig{
				MaxAttempts:       1, // Only try once since we're testing errors
				BackoffMultiplier: 1.0,
			},
			AnalyzerConfig: AnalyzerConfig{
				BaseInstruction: "Analyze this task",
			},
		}

		// Create the orchestrator
		orchestrator := NewFlexibleOrchestrator(memory, orchConfig, config)

		// Test the orchestrator with a simple task
		ctx := context.Background()
		_, err := orchestrator.Process(ctx, "Test task", map[string]any{"context": "test"})

		// Verify the error
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "analysis failed")

		// Verify expectations were met
		memory.AssertExpectations(t)
		mockLLM.AssertExpectations(t)
	})

	t.Run("Error handling for parser failure", func(t *testing.T) {
		// Create mocks with error responses
		mockParser := new(MockTaskParser)
		mockCreator := new(MockPlanCreator)

		// Configure the parser to return an error
		mockParser.On("Parse", mock.Anything).Return(nil, errors.New("parser error"))

		// Create a mock memory implementation
		memory := new(MockMemory)
		memory.On("Store", mock.Anything, mock.Anything).Return(nil).Maybe()
		memory.On("Retrieve", mock.Anything).Return(nil, nil).Maybe()
		memory.On("List").Return([]string{}, nil).Maybe()
		memory.On("Clear").Return(nil).Maybe()

		// Create a mock LLM
		mockLLM := new(testutil.MockLLM)

		// Mock both Generate and GenerateWithJSON methods
		mockLLM.On("Generate", mock.Anything, mock.Anything, mock.Anything).Return(&core.LLMResponse{
			Content: `analysis: Task analyzed successfully
subtasks: [
  {
    "id": "1",
    "type": "test",
    "processor_type": "test",
    "priority": 1,
    "dependencies": []
  }
]`,
		}, nil)

		// The GenerateWithJSON method might not be called in all cases
		mockLLM.On("GenerateWithJSON", mock.Anything, mock.Anything, mock.Anything).Return(map[string]any{
			"subtasks": []map[string]any{
				{
					"id":             "1",
					"type":           "test",
					"processor_type": "test",
					"priority":       1,
				},
			},
			"analysis": "Task analyzed successfully",
		}, nil).Maybe()

		// Create a config with the mock LLM
		config := core.NewDSPYConfig().WithDefaultLLM(mockLLM)

		// Create the orchestration config with retry configured for only 1 attempt
		orchConfig := OrchestrationConfig{
			MaxConcurrent:  2,
			DefaultTimeout: 5 * time.Second,
			TaskParser:     mockParser,
			PlanCreator:    mockCreator,
			RetryConfig: &RetryConfig{
				MaxAttempts:       1, // Only try once since we're testing errors
				BackoffMultiplier: 1.0,
			},
			AnalyzerConfig: AnalyzerConfig{
				BaseInstruction: "Analyze this task",
			},
		}

		// Create the orchestrator
		orchestrator := NewFlexibleOrchestrator(memory, orchConfig, config)

		// Test the orchestrator with a simple task
		ctx := context.Background()
		_, err := orchestrator.Process(ctx, "Test task", map[string]any{"context": "test"})

		// Verify the error
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "parser error")

		// Verify expectations were met
		mockParser.AssertExpectations(t)
		memory.AssertExpectations(t)
		mockLLM.AssertExpectations(t)
	})

	t.Run("GetProcessor", func(t *testing.T) {
		// Create a simple orchestrator for testing GetProcessor
		memory := new(MockMemory)
		memory.On("Store", mock.Anything, mock.Anything).Return(nil).Maybe()
		memory.On("Retrieve", mock.Anything).Return(nil, nil).Maybe()
		memory.On("List").Return([]string{}, nil).Maybe()
		memory.On("Clear").Return(nil).Maybe()

		config := core.NewDSPYConfig()

		orchConfig := OrchestrationConfig{
			MaxConcurrent: 2,
			TaskParser:    &testTaskParser{},
			PlanCreator:   &testPlanCreator{},
		}

		orchestrator := NewFlexibleOrchestrator(memory, orchConfig, config)

		// Register a test processor
		processor := &testProcessor{}
		orchestrator.RegisterProcessor("test", processor)

		// Test getting a registered processor
		proc, err := orchestrator.GetProcessor("test")
		assert.NoError(t, err)
		assert.NotNil(t, proc)
		assert.Same(t, processor, proc)

		// Test getting an unregistered processor
		_, err = orchestrator.GetProcessor("nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "processor not found")
	})
}
