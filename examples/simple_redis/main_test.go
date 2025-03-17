package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/scottdavis/dsgo/pkg/agents"
	"github.com/scottdavis/dsgo/pkg/agents/memory"
	"github.com/scottdavis/dsgo/pkg/agents/workflows"
	"github.com/scottdavis/dsgo/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockLLM implements the core.LLM interface for testing
type MockLLM struct {
	mock.Mock
}

func (m *MockLLM) Generate(ctx context.Context, prompt string, opts ...core.GenerateOption) (*core.LLMResponse, error) {
	args := m.Called(ctx, prompt)
	return args.Get(0).(*core.LLMResponse), args.Error(1)
}

func (m *MockLLM) StreamGenerate(ctx context.Context, prompt string, opts ...core.GenerateOption) (*core.StreamResponse, error) {
	args := m.Called(ctx, prompt)
	return args.Get(0).(*core.StreamResponse), args.Error(1)
}

func (m *MockLLM) GenerateWithJSON(ctx context.Context, prompt string, opts ...core.GenerateOption) (map[string]any, error) {
	args := m.Called(ctx, prompt)
	return args.Get(0).(map[string]any), args.Error(1)
}

func (m *MockLLM) GenerateWithFunctions(ctx context.Context, prompt string, functions []map[string]any, opts ...core.GenerateOption) (map[string]any, error) {
	args := m.Called(ctx, prompt, functions)
	return args.Get(0).(map[string]any), args.Error(1)
}

func (m *MockLLM) CreateEmbedding(ctx context.Context, input string, opts ...core.EmbeddingOption) (*core.EmbeddingResult, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(*core.EmbeddingResult), args.Error(1)
}

func (m *MockLLM) CreateEmbeddings(ctx context.Context, inputs []string, opts ...core.EmbeddingOption) (*core.BatchEmbeddingResult, error) {
	args := m.Called(ctx, inputs)
	return args.Get(0).(*core.BatchEmbeddingResult), args.Error(1)
}

func (m *MockLLM) ModelID() string {
	args := m.Called()
	return args.Get(0).(string)
}

func (m *MockLLM) Capabilities() []core.Capability {
	args := m.Called()
	return args.Get(0).([]core.Capability)
}

func (m *MockLLM) ProviderName() string {
	args := m.Called()
	return args.Get(0).(string)
}

// TestDownloadURL tests the URL download functionality
func TestDownloadURL(t *testing.T) {
	// Create a test server that returns a fixed response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Test content for download"))
	}))
	defer server.Close()

	// Test downloading from the test server
	content, err := downloadURL(server.URL)
	require.NoError(t, err)
	assert.Equal(t, "Test content for download", content)
}

func TestSummaryModule(t *testing.T) {
	// Create a mock LLM
	mockLLM := new(MockLLM)

	// Setup the mock LLM to return a fixed response
	mockResponse := &core.LLMResponse{
		Content: "This is a test summary",
	}
	mockLLM.On("Generate", mock.Anything, mock.Anything).Return(mockResponse, nil)

	// Create a signature for testing
	signature := core.NewSignature(
		[]core.InputField{
			{Field: core.Field{Name: "content", Description: "Content to summarize"}},
		},
		[]core.OutputField{
			{Field: core.Field{Name: "summary", Description: "Summarized content"}},
		},
	)

	// Create the module with injected LLM
	module := NewSummaryModule(signature, mockLLM)

	// Create test context with memory store
	ctx := core.WithExecutionState(context.Background())
	store := agents.NewInMemoryStore()
	ctx = memory.WithMemoryStore(ctx, store)

	// Process input
	result, err := module.Process(ctx, map[string]any{
		"content": "Test content to summarize",
	})

	// Verify results
	require.NoError(t, err)
	assert.Equal(t, "This is a test summary", result["summary"])

	// Verify LLM was called
	mockLLM.AssertExpectations(t)

	// Verify something was stored in Redis
	keys, err := store.List()
	require.NoError(t, err)
	assert.Len(t, keys, 1)
}

func TestTranslationModule(t *testing.T) {
	// Create a mock LLM
	mockLLM := new(MockLLM)

	// Setup the mock LLM to return a fixed response
	mockResponse := &core.LLMResponse{
		Content: "Spanish: This is a test translation",
	}
	mockLLM.On("Generate", mock.Anything, mock.Anything).Return(mockResponse, nil)

	// Create a signature for testing
	signature := core.NewSignature(
		[]core.InputField{
			{Field: core.Field{Name: "summary", Description: "English summary to translate"}},
		},
		[]core.OutputField{
			{Field: core.Field{Name: "translation", Description: "Spanish translation"}},
		},
	)

	// Create the module with injected LLM
	module := NewTranslationModule(signature, mockLLM)

	// Create test context with memory store
	ctx := core.WithExecutionState(context.Background())
	store := agents.NewInMemoryStore()
	ctx = memory.WithMemoryStore(ctx, store)

	// Process input
	result, err := module.Process(ctx, map[string]any{
		"summary": "Test summary to translate",
	})

	// Verify results
	require.NoError(t, err)
	assert.Equal(t, "This is a test translation", result["translation"])

	// Verify LLM was called
	mockLLM.AssertExpectations(t)

	// Verify something was stored in Redis
	keys, err := store.List()
	require.NoError(t, err)
	assert.Len(t, keys, 1)
}

func TestWorkflow(t *testing.T) {
	// Create a mock LLM
	mockLLM := new(MockLLM)

	// Setup the mock LLM to return fixed responses
	summaryResponse := &core.LLMResponse{
		Content: "This is a test summary",
	}
	translationResponse := &core.LLMResponse{
		Content: "Spanish: This is a test translation",
	}

	// We need to setup the mock to return different responses for different calls
	mockLLM.On("Generate", mock.Anything, mock.MatchedBy(func(prompt string) bool {
		return len(prompt) > 0 && strings.Contains(prompt, "summarize")
	})).Return(summaryResponse, nil)

	mockLLM.On("Generate", mock.Anything, mock.MatchedBy(func(prompt string) bool {
		return len(prompt) > 0 && strings.Contains(prompt, "Translate")
	})).Return(translationResponse, nil)

	// Create a context with execution state
	ctx := core.WithExecutionState(context.Background())

	// Create a memory store for testing
	store := agents.NewInMemoryStore()

	// Create the workflow
	workflow := workflows.NewChainWorkflow(store)

	// Add workflow steps
	summaryStep := createSummaryStep(mockLLM)
	err := workflow.AddStep(summaryStep)
	require.NoError(t, err)

	translationStep := createTranslationStep(mockLLM)
	err = workflow.AddStep(translationStep)
	require.NoError(t, err)

	// Test input content
	testContent := "This is test content that will be summarized and then translated."

	// Execute the workflow
	result, err := workflow.Execute(ctx, map[string]any{
		"content": testContent,
	})
	require.NoError(t, err)

	// Verify outputs
	summary, ok := result["summary"]
	require.True(t, ok, "summary not found in result")
	assert.Equal(t, "This is a test summary", summary)

	translation, ok := result["translation"]
	require.True(t, ok, "translation not found in result")
	assert.Equal(t, "This is a test translation", translation)

	// Verify LLM was called
	mockLLM.AssertExpectations(t)

	// Verify keys in store
	keys, err := store.List()
	require.NoError(t, err)
	assert.Len(t, keys, 2, "Expected 2 keys in store")
}

func TestTTLExpiry(t *testing.T) {
	// Skip in short mode as it involves waiting
	if testing.Short() {
		t.Skip("skipping TTL test in short mode")
	}

	// Create a memory store for testing
	store := agents.NewInMemoryStore()

	// Store a value with TTL
	key := "test_ttl"
	err := store.Store(key, "test value", agents.WithTTL(1*time.Second))
	require.NoError(t, err)

	// Verify it exists initially
	_, err = store.Retrieve(key)
	assert.NoError(t, err)

	// Wait for expiry
	t.Log("Waiting for TTL to expire...")
	time.Sleep(2 * time.Second)

	// Verify it's now expired
	_, err = store.Retrieve(key)
	assert.Error(t, err)
}
