package core

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// // MockLLM is a mock implementation of the LLM interface for testing.
type MockLLM struct{}

func (m *MockLLM) Generate(ctx context.Context, prompt string, options ...GenerateOption) (*LLMResponse, error) {
	return &LLMResponse{Content: "mock response"}, nil
}

func (m *MockLLM) GenerateWithJSON(ctx context.Context, prompt string, options ...GenerateOption) (map[string]any, error) {
	return map[string]any{"response": "mock response"}, nil
}

func (m *MockLLM) GenerateWithFunctions(ctx context.Context, prompt string, functions []map[string]any, options ...GenerateOption) (map[string]any, error) {
	return nil, nil
}

func (m *MockLLM) CreateEmbedding(ctx context.Context, input string, options ...EmbeddingOption) (*EmbeddingResult, error) {
	return &EmbeddingResult{
		// Using float32 for the vector as embeddings are typically floating point numbers
		Vector: []float32{0.1, 0.2, 0.3},
		// Include token count to simulate real embedding behavior
		TokenCount: len(strings.Fields(input)),
		// Add metadata to simulate real response
		Metadata: map[string]any{},
	}, nil
}

func (m *MockLLM) CreateEmbeddings(ctx context.Context, inputs []string, options ...EmbeddingOption) (*BatchEmbeddingResult, error) {
	opts := NewEmbeddingOptions()
	for _, opt := range options {
		opt(opts)
	}

	// Create mock results for each input
	embeddings := make([]EmbeddingResult, len(inputs))
	for i, input := range inputs {
		embeddings[i] = EmbeddingResult{
			// Each embedding gets slightly different values to simulate real behavior
			Vector:     []float32{0.1 * float32(i+1), 0.2 * float32(i+1), 0.3 * float32(i+1)},
			TokenCount: len(strings.Fields(input)),
			Metadata: map[string]any{
				"model":        opts.Model,
				"input_length": len(input),
				"batch_index":  i,
			},
		}
	}

	// Return the batch result
	return &BatchEmbeddingResult{
		Embeddings: embeddings,
		Error:      nil,
		ErrorIndex: -1, // -1 indicates no error
	}, nil
}

func (m *MockLLM) StreamGenerate(ctx context.Context, prompt string, opts ...GenerateOption) (*StreamResponse, error) {
	return nil, nil
}

func (m *MockLLM) ProviderName() string {
	return "mock"
}

func (m *MockLLM) ModelID() string {
	return "mock"
}

func (m *MockLLM) Capabilities() []Capability {
	return []Capability{}
}

// // TestBaseModule tests the BaseModule struct and its methods.
func TestBaseModule(t *testing.T) {
	sig := NewSignature(
		[]InputField{{Field: Field{Name: "input"}}},
		[]OutputField{{Field: Field{Name: "output"}}},
	)
	config := NewDSPYConfig()
	bm := NewModule(sig, config)

	if !reflect.DeepEqual(bm.GetSignature(), sig) {
		t.Error("GetSignature did not return the correct signature")
	}

	if bm.Config != config {
		t.Error("Config not set correctly")
	}

	_, err := bm.Process(context.Background(), map[string]any{"input": "test"})
	if err == nil || err.Error() != "Process method not implemented" {
		t.Error("Expected 'Process method not implemented' error")
	}

	clone := bm.Clone()
	if !reflect.DeepEqual(clone.GetSignature(), bm.GetSignature()) {
		t.Error("Cloned module does not have the same signature")
	}
	
	// Test with nil config
	bmNilConfig := NewModule(sig, nil)
	if bmNilConfig.Config == nil {
		t.Error("Module should create a default config when nil is provided")
	}
}

// TestModuleChain tests the ModuleChain struct and its methods.
func TestModuleChain(t *testing.T) {
	sig1 := NewSignature(
		[]InputField{{Field: Field{Name: "input1"}}},
		[]OutputField{{Field: Field{Name: "output1"}}},
	)
	sig2 := NewSignature(
		[]InputField{{Field: Field{Name: "input2"}}},
		[]OutputField{{Field: Field{Name: "output2"}}},
	)

	// Create test modules
	config := NewDSPYConfig()
	module1 := NewModule(sig1, config)
	module2 := NewModule(sig2, config)

	chain := &ModuleChain{
		Modules: []Module{module1, module2},
	}

	if len(chain.Modules) != 2 {
		t.Error("ModuleChain should have 2 modules")
	}

	// Check that the modules are correctly ordered
	if !reflect.DeepEqual(chain.Modules[0].GetSignature(), sig1) {
		t.Error("First module in chain has incorrect signature")
	}

	if !reflect.DeepEqual(chain.Modules[1].GetSignature(), sig2) {
		t.Error("Second module in chain has incorrect signature")
	}
}
