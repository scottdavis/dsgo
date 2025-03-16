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
				assert.Contains(t, capabilities, core.CapabilityStreaming)

				// Check endpoint configuration
				endpoint := llm.GetEndpointConfig()
				assert.Equal(t, openRouterBaseURL, endpoint.BaseURL)
				assert.Equal(t, "/chat/completions", endpoint.Path)
				assert.Contains(t, endpoint.Headers, "Authorization")
				assert.Equal(t, fmt.Sprintf("Bearer %s", tc.apiKey), endpoint.Headers["Authorization"])
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

// TestOpenRouterLLM_StreamGenerate tests the streaming capability of OpenRouter
func TestOpenRouterLLM_StreamGenerate(t *testing.T) {
	t.Run("Successful streaming", func(t *testing.T) {
		// Create a test server that simulates streaming
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify the request
			assert.Equal(t, "POST", r.Method)
			assert.Contains(t, r.URL.Path, "/chat/completions")
			assert.Contains(t, r.Header.Get("Content-Type"), "application/json")
			assert.Contains(t, r.Header.Get("Authorization"), "Bearer")

			// Set headers for SSE
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			w.WriteHeader(http.StatusOK)

			// Flush the headers
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}

			// Send multiple chunks
			chunks := []string{
				`{"id":"test-id-1","object":"chat.completion.chunk","created":1677858242,"model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"Hello"}}]}`,
				`{"id":"test-id-2","object":"chat.completion.chunk","created":1677858243,"model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":" world"}}]}`,
				`{"id":"test-id-3","object":"chat.completion.chunk","created":1677858244,"model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"!"}}],"usage":{"prompt_tokens":10,"completion_tokens":3,"total_tokens":13}}`,
			}

			// Write each chunk as an SSE message
			for _, chunk := range chunks {
				fmt.Fprintf(w, "data: %s\n\n", chunk)
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
				time.Sleep(10 * time.Millisecond)
			}

			// Signal the end of the stream
			fmt.Fprintf(w, "data: [DONE]\n\n")
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}))
		defer server.Close()

		// Create OpenRouterLLM with the test server URL
		llm, err := NewOpenRouterLLM("test-api-key", "test-model")
		require.NoError(t, err)

		// Override the endpoint to use our test server
		endpointCfg := llm.GetEndpointConfig()
		endpointCfg.BaseURL = server.URL
		endpointCfg.Path = "/chat/completions"

		// Create context
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		// Call StreamGenerate
		stream, err := llm.StreamGenerate(ctx, "Test prompt")
		require.NoError(t, err)
		require.NotNil(t, stream)

		// Collect all chunks
		var chunks []string
		var lastTokenUsage *core.TokenInfo
		var gotDoneSignal bool

		for chunk := range stream.ChunkChannel {
			if chunk.Error != nil {
				t.Fatalf("Unexpected error in stream: %v", chunk.Error)
			}

			if chunk.Done {
				gotDoneSignal = true
				lastTokenUsage = chunk.Usage
				break
			}

			chunks = append(chunks, chunk.Content)
		}

		// Verify we got all the expected chunks
		assert.Equal(t, []string{"Hello", " world", "!"}, chunks)

		// Verify we got the done signal
		assert.True(t, gotDoneSignal, "Should have received the done signal")

		// Verify token usage
		assert.NotNil(t, lastTokenUsage, "Should have received token usage")
		assert.Equal(t, 10, lastTokenUsage.PromptTokens)
		assert.Equal(t, 3, lastTokenUsage.CompletionTokens)
		assert.Equal(t, 13, lastTokenUsage.TotalTokens)
	})

	t.Run("Error handling", func(t *testing.T) {
		// Create a test server that simulates an error
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Send an error response
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":{"message":"Test error","code":500}}`))
		}))
		defer server.Close()

		// Create OpenRouterLLM with the test server URL
		llm, err := NewOpenRouterLLM("test-api-key", "test-model")
		require.NoError(t, err)

		// Override the endpoint to use our test server
		endpointCfg := llm.GetEndpointConfig()
		endpointCfg.BaseURL = server.URL
		endpointCfg.Path = "/chat/completions"

		// Create context
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		// Call StreamGenerate
		stream, err := llm.StreamGenerate(ctx, "Test prompt")
		require.NoError(t, err)
		require.NotNil(t, stream)

		// We should get an error in the first chunk
		chunk := <-stream.ChunkChannel
		require.NotNil(t, chunk.Error, "Expected an error in the stream")
		assert.Contains(t, chunk.Error.Error(), "OpenRouter API error")
	})

	t.Run("Stream cancellation", func(t *testing.T) {
		// Create a test server that simulates a long-running stream
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Set headers for SSE
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			w.WriteHeader(http.StatusOK)

			// Flush the headers
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}

			// Keep the connection open until the client disconnects
			for i := 0; i < 10; i++ {
				select {
				case <-r.Context().Done():
					return
				default:
					chunk := fmt.Sprintf(`{"id":"test-id-%d","object":"chat.completion.chunk","created":1677858242,"model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"Chunk %d"}}]}`, i, i)
					fmt.Fprintf(w, "data: %s\n\n", chunk)
					if f, ok := w.(http.Flusher); ok {
						f.Flush()
					}
					time.Sleep(100 * time.Millisecond)
				}
			}
		}))
		defer server.Close()

		// Create OpenRouterLLM with the test server URL
		llm, err := NewOpenRouterLLM("test-api-key", "test-model")
		require.NoError(t, err)

		// Override the endpoint to use our test server
		endpointCfg := llm.GetEndpointConfig()
		endpointCfg.BaseURL = server.URL
		endpointCfg.Path = "/chat/completions"

		// Create context
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Call StreamGenerate
		stream, err := llm.StreamGenerate(ctx, "Test prompt")
		require.NoError(t, err)
		require.NotNil(t, stream)

		// Read a few chunks
		var chunks []string
		for i := 0; i < 3; i++ {
			chunk := <-stream.ChunkChannel
			require.Nil(t, chunk.Error)
			require.False(t, chunk.Done)
			chunks = append(chunks, chunk.Content)
		}

		// Cancel the stream
		stream.Cancel()

		// Wait a bit to make sure the goroutine finishes
		time.Sleep(200 * time.Millisecond)

		// Verify we got some chunks before cancelling
		assert.GreaterOrEqual(t, len(chunks), 1)
		assert.Less(t, len(chunks), 10)
	})
}
