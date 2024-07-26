package core

import (
	"context"
	"testing"

	"github.com/XiaoConstantine/dspy-go/pkg/utils"
)

// TestProgram tests the Program struct and its methods.
func TestProgram(t *testing.T) {
	modules := map[string]Module{
		"module1": NewModule(NewSignature(
			[]InputField{{Field: Field{Name: "input"}}},
			[]OutputField{{Field: Field{Name: "output"}}},
		)),
	}

	forward := func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{"output": inputs["input"]}, nil
	}

	program := NewProgram(modules, forward)

	if len(program.Modules) != 1 {
		t.Errorf("Expected 1 module in program, got %d", len(program.Modules))
	}
	// Create a context with the traces slice
	traces := &[]Trace{}
	ctx := context.WithValue(context.Background(), utils.TracesContextKey, traces)

	result, err := program.Execute(ctx, map[string]interface{}{"input": "test"})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result["output"] != "test" {
		t.Errorf("Expected output 'test', got '%v'", result["output"])
	}

	clone := program.Clone()
	if !program.Equal(clone) {
		t.Error("Cloned program is not equal to original program")
	}
}
