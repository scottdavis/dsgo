package core

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockModule is a mock implementation of Module interface for testing.
type MockProgramModule struct {
	mock.Mock
}

func (m *MockProgramModule) Process(ctx context.Context, inputs map[string]any, opts ...Option) (map[string]any, error) {
	args := m.Called(ctx, inputs)
	return args.Get(0).(map[string]any), args.Error(1)
}

func (m *MockProgramModule) GetSignature() Signature {
	args := m.Called()
	return args.Get(0).(Signature)
}

func (m *MockProgramModule) Clone() Module {
	args := m.Called()
	return args.Get(0).(Module)
}

func TestProgram(t *testing.T) {
	t.Run("NewProgram", func(t *testing.T) {
		mockModule := new(MockProgramModule)
		modules := map[string]Module{"test": mockModule}
		forward := func(context.Context, map[string]any) (map[string]any, error) {
			return nil, nil
		}
		config := NewDSPYConfig()

		program := NewProgram(modules, forward, config)
		assert.Equal(t, modules, program.Modules)
		assert.NotNil(t, program.Forward)
		assert.Equal(t, config, program.Config)
	})

	t.Run("NewProgram with nil config", func(t *testing.T) {
		mockModule := new(MockProgramModule)
		modules := map[string]Module{"test": mockModule}
		forward := func(context.Context, map[string]any) (map[string]any, error) {
			return nil, nil
		}

		program := NewProgram(modules, forward, nil)
		assert.Equal(t, modules, program.Modules)
		assert.NotNil(t, program.Forward)
		assert.NotNil(t, program.Config) // Should create default config
	})
}

func TestProgramExecute(t *testing.T) {
	t.Run("Execute with valid inputs", func(t *testing.T) {
		mockModule := new(MockProgramModule)
		expectedOutputs := map[string]any{"result": "success"}
		forward := func(ctx context.Context, inputs map[string]any) (map[string]any, error) {
			return expectedOutputs, nil
		}
		config := NewDSPYConfig()

		program := NewProgram(map[string]Module{"test": mockModule}, forward, config)
		ctx := WithExecutionState(context.Background())

		outputs, err := program.Execute(ctx, map[string]any{"input": "test"})
		assert.NoError(t, err)
		assert.Equal(t, expectedOutputs, outputs)

		// Verify spans were created
		spans := CollectSpans(ctx)
		require.NotEmpty(t, spans)
		assert.Equal(t, "Program", spans[0].Operation)
	})

	t.Run("Execute with nil forward function", func(t *testing.T) {
		config := NewDSPYConfig()
		program := NewProgram(nil, nil, config)
		ctx := WithExecutionState(context.Background())

		_, err := program.Execute(ctx, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "forward function is not defined")
	})

	t.Run("Execute with forward error", func(t *testing.T) {
		expectedErr := errors.New("forward error")
		forward := func(ctx context.Context, inputs map[string]any) (map[string]any, error) {
			return nil, expectedErr
		}
		config := NewDSPYConfig()

		program := NewProgram(nil, forward, config)
		ctx := WithExecutionState(context.Background())

		_, err := program.Execute(ctx, nil)
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)

		// Verify error was recorded in span
		spans := CollectSpans(ctx)
		require.NotEmpty(t, spans)
		assert.Equal(t, expectedErr, spans[0].Error)
	})
}

func TestProgramEqual(t *testing.T) {
	t.Run("Equal with identical programs", func(t *testing.T) {
		mockModule1 := new(MockProgramModule)
		mockModule2 := new(MockProgramModule)
		sig := NewSignature(nil, nil)

		mockModule1.On("GetSignature").Return(sig)
		mockModule2.On("GetSignature").Return(sig)

		config := NewDSPYConfig()
		prog1 := NewProgram(map[string]Module{"test": mockModule1}, nil, config)
		prog2 := NewProgram(map[string]Module{"test": mockModule2}, nil, config)

		assert.True(t, prog1.Equal(prog2))
		mockModule1.AssertExpectations(t)
		mockModule2.AssertExpectations(t)
	})

	t.Run("Equal with different programs", func(t *testing.T) {
		mockModule1 := new(MockProgramModule)
		mockModule2 := new(MockProgramModule)
		sig1 := NewSignature([]InputField{{Field: Field{Name: "input1"}}}, nil)
		sig2 := NewSignature([]InputField{{Field: Field{Name: "input2"}}}, nil)

		mockModule1.On("GetSignature").Return(sig1)
		mockModule2.On("GetSignature").Return(sig2)

		config := NewDSPYConfig()
		prog1 := NewProgram(map[string]Module{"test": mockModule1}, nil, config)
		prog2 := NewProgram(map[string]Module{"test": mockModule2}, nil, config)

		assert.False(t, prog1.Equal(prog2))
		mockModule1.AssertExpectations(t)
		mockModule2.AssertExpectations(t)
	})

	t.Run("GetModules returns sorted modules", func(t *testing.T) {
		mockModule1 := new(MockProgramModule)
		mockModule2 := new(MockProgramModule)
		mockModule3 := new(MockProgramModule)

		sig1 := NewSignature(nil, nil)
		mockModule1.On("GetSignature").Return(sig1).Maybe()
		mockModule2.On("GetSignature").Return(sig1).Maybe()
		mockModule3.On("GetSignature").Return(sig1).Maybe()

		config := NewDSPYConfig()
		program := NewProgram(map[string]Module{
			"c": mockModule3,
			"a": mockModule1,
			"b": mockModule2,
		}, nil, config)

		modules := program.GetModules()
		assert.Len(t, modules, 3)
		// Verify modules are returned in alphabetical order by key
		assert.Same(t, mockModule1, modules[0])
		assert.Same(t, mockModule2, modules[1])
		assert.Same(t, mockModule3, modules[2])
	})

	t.Run("AddModule and SetForward", func(t *testing.T) {
		config := NewDSPYConfig()
		program := NewProgram(make(map[string]Module), nil, config)
		mockModule := new(MockProgramModule)

		program.AddModule("test", mockModule)
		assert.Contains(t, program.Modules, "test")
		assert.Same(t, mockModule, program.Modules["test"])

		forward := func(ctx context.Context, inputs map[string]any) (map[string]any, error) {
			return nil, nil
		}
		program.SetForward(forward)
		assert.NotNil(t, program.Forward)
	})
}
