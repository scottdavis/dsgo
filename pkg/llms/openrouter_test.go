package llms

import (
	"testing"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOpenRouterLLM(t *testing.T) {
	testCases := []struct {
		name        string
		apiKey      string
		modelName   string
		expectError bool
	}{
		{
			name:        "Valid configuration",
			apiKey:      "test-api-key",
			modelName:   "anthropic/claude-3-opus-20240229",
			expectError: false,
		},
		{
			name:        "Missing API key",
			apiKey:      "",
			modelName:   "anthropic/claude-3-opus-20240229",
			expectError: true,
		},
		{
			name:        "Missing model name",
			apiKey:      "test-api-key",
			modelName:   "",
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			llm, err := NewOpenRouterLLM(tc.apiKey, tc.modelName)

			if tc.expectError {
				assert.Error(t, err)
				assert.Nil(t, llm)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, llm)
				assert.Equal(t, "openrouter", llm.ProviderName())
				assert.Equal(t, tc.modelName, llm.ModelID())
				
				// Check capabilities
				capabilities := llm.Capabilities()
				assert.Contains(t, capabilities, core.CapabilityCompletion)
				assert.Contains(t, capabilities, core.CapabilityChat)
				assert.Contains(t, capabilities, core.CapabilityJSON)
				
				// Check endpoint configuration
				endpoint := llm.GetEndpointConfig()
				assert.Equal(t, openRouterBaseURL, endpoint.BaseURL)
				assert.Equal(t, "/chat/completions", endpoint.Path)
				assert.Contains(t, endpoint.Headers, "Authorization")
				assert.Contains(t, endpoint.Headers["Authorization"], tc.apiKey)
			}
		})
	}
}

func TestOpenRouterConfig(t *testing.T) {
	// Test creating OpenRouterConfig
	modelName := "anthropic/claude-3-opus-20240229"
	config := NewOpenRouterConfig(modelName)
	
	assert.NotNil(t, config)
	assert.Equal(t, modelName, config.ModelName)
	
	// Test using the config with factory
	factory := &DefaultLLMFactory{}
	llm, err := factory.CreateLLM("test-api-key", config)
	
	require.NoError(t, err)
	assert.NotNil(t, llm)
	assert.Equal(t, "openrouter", llm.ProviderName())
	assert.Equal(t, modelName, llm.ModelID())
}

func TestOpenRouterModelIDString(t *testing.T) {
	// Test using the model ID string format
	modelID := core.ModelID("openrouter:anthropic/claude-3-opus-20240229")
	
	factory := &DefaultLLMFactory{}
	llm, err := factory.CreateLLM("test-api-key", modelID)
	
	require.NoError(t, err)
	assert.NotNil(t, llm)
	assert.Equal(t, "openrouter", llm.ProviderName())
	assert.Equal(t, "anthropic/claude-3-opus-20240229", llm.ModelID())
	
	// Test invalid model ID format
	invalidModelID := core.ModelID("openrouter:")
	_, err = factory.CreateLLM("test-api-key", invalidModelID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid OpenRouter model ID format")
} 