package optimizers

import (
	"context"
	"testing"

	"github.com/scottdavis/dsgo/internal/testutil"
	"github.com/scottdavis/dsgo/pkg/core"
	"github.com/scottdavis/dsgo/pkg/modules"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockDataset is a mock implementation of the Dataset interface for testing.
type MockDataset struct {
	Examples []core.Example
	Index    int
	mock.Mock
}

func NewMockDataset(examples []core.Example) *MockDataset {
	return &MockDataset{
		Examples: examples,
		Index:    0,
	}
}

func (m *MockDataset) Next() (core.Example, bool) {
	if m.Index < len(m.Examples) {
		example := m.Examples[m.Index]
		m.Index++
		return example, true
	}
	return core.Example{}, false
}

func (m *MockDataset) Reset() {
	m.Index = 0
}

func setupMockLLM() *testutil.MockLLM {
	mockLLM := new(testutil.MockLLM)
	mockLLM.On("Generate", mock.Anything, mock.Anything, mock.Anything).Return(&core.LLMResponse{Content: "Instruction for the test"}, nil)
	mockLLM.On("GenerateWithJSON", mock.Anything, mock.Anything, mock.Anything).Return(map[string]any{"output": "result"}, nil)
	return mockLLM
}

func createMockDataset() *MockDataset {
	examples := []core.Example{
		{
			Input:  map[string]any{"input": "test1"},
			Output: map[string]any{"output": "result1"},
		},
		{
			Input:  map[string]any{"input": "test2"},
			Output: map[string]any{"output": "result2"},
		},
	}
	return NewMockDataset(examples)
}

func TestNewMIPRO(t *testing.T) {
	metric := func(example, prediction map[string]any, ctx context.Context) float64 { return 0 }
	mockLLM := setupMockLLM()

	mipro := NewMIPRO(metric,
		WithNumCandidates(20),
		WithMaxBootstrappedDemos(10),
		WithMaxLabeledDemos(10),
		WithNumTrials(200),
		WithPromptModel(mockLLM),
		WithTaskModel(mockLLM),
		WithMiniBatchSize(64),
		WithFullEvalSteps(20),
		WithVerbose(true),
	)

	assert.Equal(t, 20, mipro.NumCandidates)
	assert.Equal(t, 10, mipro.MaxBootstrappedDemos)
	assert.Equal(t, 10, mipro.MaxLabeledDemos)
	assert.Equal(t, 200, mipro.NumTrials)
	assert.Equal(t, 64, mipro.MiniBatchSize)
	assert.Equal(t, 20, mipro.FullEvalSteps)
	assert.True(t, mipro.Verbose)
	assert.Equal(t, mockLLM, mipro.PromptModel)
	assert.Equal(t, mockLLM, mipro.TaskModel)
}

func TestMIPRO_Compile(t *testing.T) {
	ctx := core.WithExecutionState(context.Background())
	mockLLM := setupMockLLM()
	dataset := createMockDataset()

	// Set up a simple metric function
	metric := func(ctx context.Context, result map[string]any) (bool, string) {
		return true, ""
	}

	// Create MIPRO with minimal trials for faster testing
	mipro := NewMIPRO(
		func(example, prediction map[string]any, ctx context.Context) float64 { return 1.0 },
		WithNumCandidates(1),
		WithMaxBootstrappedDemos(1),
		WithMaxLabeledDemos(0),
		WithNumTrials(1),
		WithPromptModel(mockLLM),
		WithTaskModel(mockLLM),
		WithMiniBatchSize(1),
	)

	// Create a test program
	config := core.NewDSPYConfig().WithDefaultLLM(mockLLM)
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

	// Execute the test
	compiled, err := mipro.Compile(ctx, program, dataset, metric)

	assert.NoError(t, err)
	assert.NotNil(t, compiled)
}

func TestMIPRO_CompileErrors(t *testing.T) {
	t.Run("Empty dataset", func(t *testing.T) {
		ctx := context.Background()
		mockLLM := setupMockLLM()
		emptyDataset := NewMockDataset([]core.Example{})

		metric := func(ctx context.Context, result map[string]any) (bool, string) {
			return true, ""
		}

		mipro := NewMIPRO(
			func(example, prediction map[string]any, ctx context.Context) float64 { return 1.0 },
			WithNumCandidates(1),
			WithMaxBootstrappedDemos(1),
			WithPromptModel(mockLLM),
		)

		config := core.NewDSPYConfig().WithDefaultLLM(mockLLM)
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

		// Should handle empty dataset gracefully
		_, err := mipro.Compile(ctx, program, emptyDataset, metric)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no valid examples")
	})
}

func TestMIPRO_generateTrial(t *testing.T) {
	mipro := NewMIPRO(nil)

	config := core.NewDSPYConfig()
	sig := core.NewSignature(
		[]core.InputField{{Field: core.Field{Name: "input"}}},
		[]core.OutputField{{Field: core.Field{Name: "output"}}},
	)

	modules := []core.Module{
		modules.NewPredict(sig, config),
		modules.NewPredict(sig, config),
	}

	trial := mipro.generateTrial(modules, 5, 5)

	assert.Len(t, trial.Params, 4)
	assert.Contains(t, trial.Params, "instruction_0")
	assert.Contains(t, trial.Params, "instruction_1")
	assert.Contains(t, trial.Params, "demo_0")
	assert.Contains(t, trial.Params, "demo_1")

	// Check that values are within expected ranges
	assert.GreaterOrEqual(t, trial.Params["instruction_0"], 0)
	assert.Less(t, trial.Params["instruction_0"], 5)
	assert.GreaterOrEqual(t, trial.Params["demo_0"], 0)
	assert.Less(t, trial.Params["demo_0"], 5)
}

func TestMIPRO_constructProgram(t *testing.T) {
	mipro := NewMIPRO(nil)
	config := core.NewDSPYConfig()

	// Create a base program
	sig1 := core.NewSignature(
		[]core.InputField{{Field: core.Field{Name: "input1"}}},
		[]core.OutputField{{Field: core.Field{Name: "output1"}}},
	)
	sig2 := core.NewSignature(
		[]core.InputField{{Field: core.Field{Name: "input2"}}},
		[]core.OutputField{{Field: core.Field{Name: "output2"}}},
	)

	predict1 := modules.NewPredict(sig1, config)
	predict2 := modules.NewPredict(sig2, config)

	baseProgram := core.NewProgram(
		map[string]core.Module{
			"predict1": predict1,
			"predict2": predict2,
		},
		nil,
		config,
	)

	// Create a trial with specific parameters
	trial := Trial{
		Params: map[string]int{
			"instruction_0": 0,
			"demo_0":        0,
			"instruction_1": 1,
			"demo_1":        1,
		},
	}

	// Create instruction candidates
	instructionCandidates := [][]string{
		{"Instruction1", "Instruction2"},
		{"Instruction3", "Instruction4"},
	}

	// Create demo candidates
	demoCandidates := [][][]core.Example{
		{
			{
				{Input: map[string]any{"input1": "test1"}, Output: map[string]any{"output1": "result1"}},
			},
			{
				{Input: map[string]any{"input1": "test2"}, Output: map[string]any{"output1": "result2"}},
			},
		},
		{
			{
				{Input: map[string]any{"input2": "test3"}, Output: map[string]any{"output2": "result3"}},
			},
			{
				{Input: map[string]any{"input2": "test4"}, Output: map[string]any{"output2": "result4"}},
			},
		},
	}

	// Construct a new program using the trial
	result := mipro.constructProgram(baseProgram, trial, instructionCandidates, demoCandidates)

	// Verify the result
	assert.NotNil(t, result)
	assert.NotSame(t, baseProgram, result, "Should return a new program instance")
	assert.Len(t, result.Modules, 2)
}
