package llms

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

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

func TestOpenRouterRateLimitHandling(t *testing.T) {
	// Create a test server that simulates rate limiting
	requestCount := 0
	resetTime := time.Now().Add(2 * time.Second).UnixMilli()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Increment request count
		requestCount++

		if requestCount == 1 {
			// First request gets rate limited
			w.Header().Set("X-RateLimit-Limit", "20")
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetTime, 10))
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error": {"message": "Rate limit exceeded"}}`))
			return
		}

		// Subsequent requests succeed
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"id": "test-response",
			"object": "chat.completion",
			"created": 1677858242,
			"model": "test-model",
			"choices": [
				{
					"index": 0,
					"message": {
						"role": "assistant",
						"content": "This is a test response after rate limit"
					},
					"finish_reason": "stop"
				}
			],
			"usage": {
				"prompt_tokens": 10,
				"completion_tokens": 20,
				"total_tokens": 30
			}
		}`))
	}))
	defer server.Close()

	// Create OpenRouterLLM with the test server URL
	modelName := "test-model"
	apiKey := "test-api-key"

	llm, err := NewOpenRouterLLM(apiKey, modelName)
	require.NoError(t, err)

	// Override the endpoint to use our test server
	endpointCfg := llm.GetEndpointConfig()
	endpointCfg.BaseURL = server.URL
	endpointCfg.Path = ""

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Make a request that should get rate limited
	_, err = llm.Generate(ctx, "Test request with rate limiting")

	// We expect an error because our code detects rate limiting with 429 status
	require.Error(t, err, "Generate should detect rate limiting")
	require.Contains(t, err.Error(), "non-200 status code")

	// Test that we made only the one request (which detected rate limiting)
	assert.Equal(t, 1, requestCount, "Should have made exactly 1 request")
}

// Add a new test for rate limiting with 200 status code
func TestOpenRouterRateLimitHandlingWith200Status(t *testing.T) {
	// Create a test server that simulates rate limiting with 200 status code
	requestCount := 0
	resetTime := time.Now().Add(2 * time.Second).UnixMilli()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Increment request count
		requestCount++

		if requestCount == 1 {
			// First request gets rate limited but returns 200
			w.Header().Set("X-RateLimit-Limit", "20")
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetTime, 10))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			// Return a valid response but with empty choices - this is how OpenRouter indicates rate limiting
			w.Write([]byte(`{
				"id": "rate-limited-response",
				"object": "chat.completion",
				"created": 1677858242,
				"model": "test-model",
				"choices": [],
				"usage": {
					"prompt_tokens": 10,
					"completion_tokens": 0,
					"total_tokens": 10
				}
			}`))
			return
		}

		// Subsequent requests succeed with normal response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"id": "test-response",
			"object": "chat.completion",
			"created": 1677858242,
			"model": "test-model",
			"choices": [
				{
					"index": 0,
					"message": {
						"role": "assistant",
						"content": "This is a test response after 200 rate limit"
					},
					"finish_reason": "stop"
				}
			],
			"usage": {
				"prompt_tokens": 10,
				"completion_tokens": 25,
				"total_tokens": 35
			}
		}`))
	}))
	defer server.Close()

	// Create OpenRouterLLM with the test server URL
	modelName := "test-model"
	apiKey := "test-api-key"

	llm, err := NewOpenRouterLLM(apiKey, modelName)
	require.NoError(t, err)

	// Override the endpoint to use our test server
	endpointCfg := llm.GetEndpointConfig()
	endpointCfg.BaseURL = server.URL
	endpointCfg.Path = ""

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Make a request that should get rate limited
	_, err = llm.Generate(ctx, "Test request with 200 rate limiting")

	// We expect an error because our updated code now detects rate limiting on 200 status
	// This is the correct behavior
	require.Error(t, err, "Generate should detect rate limiting with 200 status")
	require.Contains(t, err.Error(), "rate limited")

	// Test that we made only the one request (which detected rate limiting)
	assert.Equal(t, 1, requestCount, "Should have made exactly 1 request")
}

// Test for GenerateWithFunctions with 200 status code rate limiting
func TestOpenRouterGenerateWithFunctionsRateLimitHandlingWith200Status(t *testing.T) {
	// Create a test server that simulates rate limiting with 200 status code
	requestCount := 0
	resetTime := time.Now().Add(2 * time.Second).UnixMilli()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Increment request count
		requestCount++

		if requestCount == 1 {
			// First request gets rate limited but returns 200
			w.Header().Set("X-RateLimit-Limit", "20")
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetTime, 10))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			// Return a valid response but with empty choices - this is how OpenRouter indicates rate limiting
			w.Write([]byte(`{
				"id": "rate-limited-response",
				"object": "chat.completion",
				"created": 1677858242,
				"model": "test-model",
				"choices": [],
				"usage": {
					"prompt_tokens": 10,
					"completion_tokens": 0,
					"total_tokens": 10
				}
			}`))
			return
		}

		// Subsequent requests succeed with normal response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"id": "test-response",
			"object": "chat.completion",
			"created": 1677858242,
			"model": "test-model",
			"choices": [
				{
					"index": 0,
					"message": {
						"role": "assistant",
						"content": "This is a test response after 200 rate limit",
						"function_call": null
					},
					"finish_reason": "stop"
				}
			],
			"usage": {
				"prompt_tokens": 10,
				"completion_tokens": 25,
				"total_tokens": 35
			}
		}`))
	}))
	defer server.Close()

	// Create OpenRouterLLM with the test server URL
	modelName := "test-model"
	apiKey := "test-api-key"

	llm, err := NewOpenRouterLLM(apiKey, modelName)
	require.NoError(t, err)

	// Override the endpoint to use our test server
	endpointCfg := llm.GetEndpointConfig()
	endpointCfg.BaseURL = server.URL
	endpointCfg.Path = ""

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Define a test function
	functions := []map[string]any{
		{
			"name":        "test_function",
			"description": "A test function",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"test_param": map[string]any{
						"type":        "string",
						"description": "A test parameter",
					},
				},
				"required": []string{"test_param"},
			},
		},
	}

	// Make a request that should get rate limited
	_, err = llm.GenerateWithFunctions(ctx, "Test request with 200 rate limiting", functions)

	// We expect an error because our updated code now detects rate limiting on 200 status
	// This is the correct behavior
	require.Error(t, err, "GenerateWithFunctions should detect rate limiting with 200 status")
	require.Contains(t, err.Error(), "rate limited")

	// Test that we made only the one request (which detected rate limiting)
	assert.Equal(t, 1, requestCount, "Should have made exactly 1 request")
}

// Test for rate limiting information in metadata field
func TestOpenRouterRateLimitHandlingWithMetadata(t *testing.T) {
	// Create a test server that simulates rate limiting with metadata
	requestCount := 0
	resetTime := time.Now().Add(2 * time.Second).UnixMilli()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Increment request count
		requestCount++

		// Return a 200 OK but with rate limit info in metadata and empty choices
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		// Note: OpenRouter might not send rate limit headers, only metadata
		// So we're not setting X-RateLimit headers in this test

		// Return response with rate limit info in metadata
		w.Write([]byte(fmt.Sprintf(`{
			"id": "rate-limited-response",
			"object": "chat.completion",
			"created": 1677858242,
			"model": "test-model",
			"choices": [],
			"usage": {
				"prompt_tokens": 10,
				"completion_tokens": 0,
				"total_tokens": 10
			},
			"meta": {
				"rate_limit": {
					"limit": 20,
					"remaining": 0,
					"reset": %d
				}
			}
		}`, resetTime)))
	}))
	defer server.Close()

	// Create OpenRouterLLM with the test server URL
	modelName := "test-model"
	apiKey := "test-api-key"

	llm, err := NewOpenRouterLLM(apiKey, modelName)
	require.NoError(t, err)

	// Override the endpoint to use our test server
	endpointCfg := llm.GetEndpointConfig()
	endpointCfg.BaseURL = server.URL
	endpointCfg.Path = ""

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Make a request that should detect rate limiting from metadata
	_, err = llm.Generate(ctx, "Test request with metadata rate limiting")

	// We expect an error because our code detects rate limiting from metadata
	require.Error(t, err, "Generate should detect rate limiting from metadata")
	require.Contains(t, err.Error(), "rate limited (from metadata)")

	// Test that we made exactly one request
	assert.Equal(t, 1, requestCount, "Should have made exactly 1 request")
}

// Test for rate limiting information in metadata field for GenerateWithFunctions
func TestOpenRouterGenerateWithFunctionsRateLimitHandlingWithMetadata(t *testing.T) {
	// Create a test server that simulates rate limiting with metadata
	requestCount := 0
	resetTime := time.Now().Add(2 * time.Second).UnixMilli()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Increment request count
		requestCount++

		// Return a 200 OK but with rate limit info in metadata and empty choices
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		// Note: OpenRouter might not send rate limit headers, only metadata
		// So we're not setting X-RateLimit headers in this test

		// Return response with rate limit info in metadata
		w.Write([]byte(fmt.Sprintf(`{
			"id": "rate-limited-response",
			"object": "chat.completion",
			"created": 1677858242,
			"model": "test-model",
			"choices": [],
			"usage": {
				"prompt_tokens": 10,
				"completion_tokens": 0,
				"total_tokens": 10
			},
			"meta": {
				"rate_limit": {
					"limit": 20,
					"remaining": 0,
					"reset": %d
				}
			}
		}`, resetTime)))
	}))
	defer server.Close()

	// Create OpenRouterLLM with the test server URL
	modelName := "test-model"
	apiKey := "test-api-key"

	llm, err := NewOpenRouterLLM(apiKey, modelName)
	require.NoError(t, err)

	// Override the endpoint to use our test server
	endpointCfg := llm.GetEndpointConfig()
	endpointCfg.BaseURL = server.URL
	endpointCfg.Path = ""

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Define a test function
	functions := []map[string]any{
		{
			"name":        "test_function",
			"description": "A test function",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"test_param": map[string]any{
						"type":        "string",
						"description": "A test parameter",
					},
				},
				"required": []string{"test_param"},
			},
		},
	}

	// Make a request that should detect rate limiting from metadata
	_, err = llm.GenerateWithFunctions(ctx, "Test request with metadata rate limiting", functions)

	// We expect an error because our code detects rate limiting from metadata
	require.Error(t, err, "GenerateWithFunctions should detect rate limiting from metadata")
	require.Contains(t, err.Error(), "rate limited (from metadata)")

	// Test that we made exactly one request
	assert.Equal(t, 1, requestCount, "Should have made exactly 1 request")
}
