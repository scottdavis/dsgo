package llms

import (
	"context"
	"testing"

	"github.com/scottdavis/dsgo/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to get the underlying LLM from a potentially decorated LLM
func getUnderlyingLLM(llm core.LLM) core.LLM {
	if decorator, ok := llm.(*core.ModelContextDecorator); ok {
		return decorator.Unwrap()
	}
	return llm
}

func TestNewLLM(t *testing.T) {
	testCases := []struct {
		name            string
		apiKey          string
		modelID         core.ModelID
		expectedModelID string
		expectErr       bool
		errMsg          string
		checkType       func(t *testing.T, llm core.LLM)
	}{
		{
			name:            "Anthropic Haiku",
			apiKey:          "test-api-key",
			modelID:         core.ModelAnthropicHaiku,
			expectedModelID: string(core.ModelAnthropicHaiku),
			checkType: func(t *testing.T, llm core.LLM) {
				underlying := getUnderlyingLLM(llm)
				_, ok := underlying.(*AnthropicLLM)
				assert.True(t, ok, "Expected AnthropicLLM")
			},
		},
		{
			name:            "Anthropic Sonnet",
			apiKey:          "test-api-key",
			modelID:         core.ModelAnthropicSonnet,
			expectedModelID: string(core.ModelAnthropicSonnet),
			checkType: func(t *testing.T, llm core.LLM) {
				underlying := getUnderlyingLLM(llm)
				_, ok := underlying.(*AnthropicLLM)
				assert.True(t, ok, "Expected AnthropicLLM")
			},
		},
		{
			name:            "Anthropic Opus",
			apiKey:          "test-api-key",
			modelID:         core.ModelAnthropicOpus,
			expectedModelID: string(core.ModelAnthropicOpus),
			checkType: func(t *testing.T, llm core.LLM) {
				underlying := getUnderlyingLLM(llm)
				_, ok := underlying.(*AnthropicLLM)
				assert.True(t, ok, "Expected AnthropicLLM")
			},
		},
		{
			name:            "Valid Ollama model",
			apiKey:          "",
			modelID:         "ollama:llama2",
			expectedModelID: "llama2",
			checkType: func(t *testing.T, llm core.LLM) {
				underlying := getUnderlyingLLM(llm)
				_, ok := underlying.(*OllamaLLM)
				assert.True(t, ok, "Expected OllamaLLM")
			},
		},
		{
			name:      "Invalid Ollama model format",
			apiKey:    "",
			modelID:   "ollama:",
			expectErr: true,
			errMsg:    "invalid Ollama model ID format. Use 'ollama:<model_name>' or 'ollama:<host>:<model_name>'",
		},
		{
			name:      "Unsupported model",
			apiKey:    "test-api-key",
			modelID:   "unsupported-model",
			expectErr: true,
			errMsg:    "unsupported model ID: unsupported-model",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			llm, err := NewLLM(tc.apiKey, tc.modelID)

			if tc.expectErr {
				require.Error(t, err)
				assert.Equal(t, tc.errMsg, err.Error())
				assert.Nil(t, llm)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, llm)

			// Verify the model type using the provided check function
			tc.checkType(t, llm)

			expectedID := tc.expectedModelID
			assert.Equal(t, expectedID, llm.ModelID())

			// Test that model context decoration works when applicable
			if !tc.expectErr {
				ctx := core.WithExecutionState(context.Background())
				// We don't need to actually generate, just ensure model ID is set
				state := core.GetExecutionState(ctx)
				assert.NotNil(t, state)
			}
		})
	}
}

func TestOllamaConfig(t *testing.T) {
	// Test creating an LLM with OllamaConfig
	customHost := "http://custom-ollama:11434"
	modelName := "llama2"

	// Create a config with the custom host and model
	ollamaConfig := NewOllamaConfig(customHost, modelName)

	// Create LLM using the config
	llm, err := NewLLM("", ollamaConfig)
	assert.NoError(t, err)

	// Check that the LLM has the correct model and endpoint
	underlyingLLM := getUnderlyingLLM(llm)
	ollamaLLM, ok := underlyingLLM.(*OllamaLLM)
	assert.True(t, ok, "LLM should be an OllamaLLM")
	assert.Equal(t, customHost, ollamaLLM.GetEndpointConfig().BaseURL,
		"OllamaLLM should use the specified host")
	assert.Equal(t, modelName, ollamaLLM.ModelID(),
		"OllamaLLM should use the specified model name")
}

func TestMultipleOllamaConfigs(t *testing.T) {
	// Test that multiple OllamaConfig instances can be used to create different LLMs
	firstHost := "http://first-host:11434"
	secondHost := "http://second-host:11434"
	firstModel := "llama2"
	secondModel := "llama3"

	// Create configs with different hosts and models
	firstConfig := NewOllamaConfig(firstHost, firstModel)
	secondConfig := NewOllamaConfig(secondHost, secondModel)

	// Create LLMs using the configs
	firstLLM, err := NewLLM("", firstConfig)
	assert.NoError(t, err)

	secondLLM, err := NewLLM("", secondConfig)
	assert.NoError(t, err)

	// Verify each LLM has the correct host and model
	firstUnderlyingLLM := getUnderlyingLLM(firstLLM)
	firstOllamaLLM, ok := firstUnderlyingLLM.(*OllamaLLM)
	assert.True(t, ok)
	assert.Equal(t, firstHost, firstOllamaLLM.GetEndpointConfig().BaseURL)
	assert.Equal(t, firstModel, firstOllamaLLM.ModelID())

	secondUnderlyingLLM := getUnderlyingLLM(secondLLM)
	secondOllamaLLM, ok := secondUnderlyingLLM.(*OllamaLLM)
	assert.True(t, ok)
	assert.Equal(t, secondHost, secondOllamaLLM.GetEndpointConfig().BaseURL)
	assert.Equal(t, secondModel, secondOllamaLLM.ModelID())
}

func TestOllamaConfigWithTrailingSlash(t *testing.T) {
	// Test that trailing slashes are properly handled in the host URL
	hostWithSlash := "http://custom-ollama:11434/"
	hostWithoutSlash := "http://custom-ollama:11434"
	modelName := "llama2"

	// Create config with trailing slash
	config := NewOllamaConfig(hostWithSlash, modelName)

	// Verify trailing slash was removed
	assert.Equal(t, hostWithoutSlash, config.Host,
		"NewOllamaConfig should remove trailing slash from host")

	// Create LLM and verify correct host is used
	llm, err := NewLLM("", config)
	assert.NoError(t, err)

	underlyingLLM := getUnderlyingLLM(llm)
	ollamaLLM, ok := underlyingLLM.(*OllamaLLM)
	assert.True(t, ok)
	assert.Equal(t, hostWithoutSlash, ollamaLLM.GetEndpointConfig().BaseURL,
		"OllamaLLM should use host without trailing slash")
}

func TestOllamaModelIDWithCustomHost(t *testing.T) {
	testCases := []struct {
		name          string
		modelID       core.ModelID
		expectedHost  string
		expectedModel string
		expectErr     bool
		errMsg        string
	}{
		{
			name:          "Legacy format",
			modelID:       "ollama:llama2",
			expectedHost:  "http://localhost:11434",
			expectedModel: "llama2",
			expectErr:     false,
		},
		{
			name:          "Custom host format with port",
			modelID:       "ollama:custom-host:11434:llama2",
			expectedHost:  "http://custom-host:11434",
			expectedModel: "llama2",
			expectErr:     false,
		},
		{
			name:          "Simple host format",
			modelID:       "ollama:localhost:llama2",
			expectedHost:  "http://localhost",
			expectedModel: "llama2",
			expectErr:     false,
		},
		{
			name:          "Custom host with HTTP prefix",
			modelID:       "ollama:http://custom-host:11434:llama2",
			expectedHost:  "http://custom-host:11434",
			expectedModel: "llama2",
			expectErr:     false,
		},
		{
			name:          "Custom host with HTTPS prefix",
			modelID:       "ollama:https://custom-host:11434:llama2",
			expectedHost:  "https://custom-host:11434",
			expectedModel: "llama2",
			expectErr:     false,
		},
		{
			name:      "Invalid format - empty model",
			modelID:   "ollama:custom-host:",
			expectErr: true,
			errMsg:    "invalid Ollama model ID format. Use 'ollama:<host>:<model_name>' with non-empty host and model name",
		},
		{
			name:      "Invalid format - empty host",
			modelID:   "ollama::llama2",
			expectErr: true,
			errMsg:    "invalid Ollama model ID format. Use 'ollama:<host>:<model_name>' with non-empty host and model name",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			llm, err := NewLLM("", tc.modelID)

			if tc.expectErr {
				require.Error(t, err)
				assert.Equal(t, tc.errMsg, err.Error())
				assert.Nil(t, llm)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, llm)

			// Check that the LLM has the correct model and endpoint
			underlyingLLM := getUnderlyingLLM(llm)
			ollamaLLM, ok := underlyingLLM.(*OllamaLLM)
			assert.True(t, ok, "LLM should be an OllamaLLM")

			assert.Equal(t, tc.expectedHost, ollamaLLM.GetEndpointConfig().BaseURL,
				"OllamaLLM should use the specified host")
			assert.Equal(t, tc.expectedModel, ollamaLLM.ModelID(),
				"OllamaLLM should use the specified model name")
		})
	}
}
