package llms

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/scottdavis/dsgo/pkg/core"
	ollamaApi "github.com/ollama/ollama/api"
	"github.com/stretchr/testify/assert"
)

func TestNewOllamaLLM(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		model    string
	}{
		{"Default endpoint", "", "test-model"},
		{"Custom endpoint", "http://custom:8080", "test-model"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			llm, err := NewOllamaLLM(tt.endpoint, tt.model)
			assert.NoError(t, err)
			assert.NotNil(t, llm)
			if tt.endpoint == "" {
				assert.Equal(t, "http://localhost:11434", llm.GetEndpointConfig().BaseURL)
			} else {
				assert.Equal(t, tt.endpoint, llm.GetEndpointConfig().BaseURL)
			}
			assert.Equal(t, tt.model, llm.ModelID())
		})
	}
}

func TestOllamaLLM_Generate(t *testing.T) {
	testTime, _ := time.Parse(time.RFC3339, "2023-01-01T00:00:00Z")

	tests := []struct {
		name           string
		serverResponse *ollamaApi.GenerateResponse
		serverStatus   int
		expectError    bool
	}{
		{
			name: "Successful generation",
			serverResponse: &ollamaApi.GenerateResponse{
				Model:     "test-model",
				CreatedAt: testTime,
				Response:  "Generated text",
			},
			serverStatus: http.StatusOK,
			expectError:  false,
		},
		{
			name:           "Server error",
			serverResponse: nil,
			serverStatus:   http.StatusInternalServerError,
			expectError:    true,
		},
		{
			name: "Invalid JSON response",
			serverResponse: &ollamaApi.GenerateResponse{
				Model:     "test-model",
				CreatedAt: testTime,
				Response:  "Generated text",
			},
			serverStatus: http.StatusOK,
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/api/generate", r.URL.Path)
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

				var reqBody ollamaApi.GenerateRequest
				err := json.NewDecoder(r.Body).Decode(&reqBody)
				assert.NoError(t, err)

				w.WriteHeader(tt.serverStatus)
				if tt.serverResponse != nil {
					if tt.name == "Invalid JSON response" {
						if _, err := w.Write([]byte(`{"invalid": json`)); err != nil {
							return
						}
					} else {
						if err := json.NewEncoder(w).Encode(tt.serverResponse); err != nil {
							return
						}
					}
				}
			}))
			defer server.Close()

			llm, err := NewOllamaLLM(server.URL, "test-model")
			assert.NoError(t, err)

			response, err := llm.Generate(context.Background(), "Test prompt", core.WithMaxTokens(100), core.WithTemperature(0.7))

			if tt.expectError {
				assert.Error(t, err)
			} else {
				if tt.name == "Invalid JSON response" {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
					assert.Equal(t, tt.serverResponse.Response, response.Content)
				}
			}
		})
	}
}

func TestOllamaLLM_GenerateOptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/generate", r.URL.Path)
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var reqBody ollamaApi.GenerateRequest
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		assert.NoError(t, err)

		// Check that the options were properly set in the request
		assert.NotNil(t, reqBody.Options)
		// JSON numbers are parsed as float64
		assert.Equal(t, 0.7, reqBody.Options["temperature"])
		assert.Equal(t, float64(100), reqBody.Options["max_tokens"])

		// Send a response
		resp := ollamaApi.GenerateResponse{
			Model:    "test-model",
			Response: "Generated text with options",
		}

		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			return
		}
	}))
	defer server.Close()

	llm, err := NewOllamaLLM(server.URL, "test-model")
	assert.NoError(t, err)

	response, err := llm.Generate(context.Background(), "Test prompt",
		core.WithMaxTokens(100),
		core.WithTemperature(0.7))

	assert.NoError(t, err)
	assert.Equal(t, "Generated text with options", response.Content)
}

func TestOllamaLLM_GenerateWithJSON(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse ollamaApi.GenerateResponse
		expectError    bool
		expectedJSON   map[string]any
	}{
		{
			name: "Valid JSON response",
			serverResponse: ollamaApi.GenerateResponse{
				Model:    "test-model",
				Response: `{"key": "value"}`,
			},
			expectError:  false,
			expectedJSON: map[string]any{"key": "value"},
		},
		{
			name: "Invalid JSON response",
			serverResponse: ollamaApi.GenerateResponse{
				Model:    "test-model",
				Response: `invalid json`,
			},
			expectError:  true,
			expectedJSON: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				if err := json.NewEncoder(w).Encode(tt.serverResponse); err != nil {
					return
				}
			}))
			defer server.Close()

			llm, err := NewOllamaLLM(server.URL, "test-model")
			assert.NoError(t, err)

			response, err := llm.GenerateWithJSON(context.Background(), "Test prompt")

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, response)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedJSON, response)
			}
		})
	}
}

func TestOllamaLLM_CreateEmbedding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/embeddings", r.URL.Path)
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var reqBody ollamaApi.EmbeddingRequest
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		assert.NoError(t, err)

		// Check that the request is properly formed
		assert.Equal(t, "test-model", reqBody.Model)
		assert.Equal(t, "Test input", reqBody.Prompt) // Changed from Input to Prompt to match API
		assert.NotNil(t, reqBody.Options)

		// Send a direct JSON response with embedding data
		mockResponse := ollamaApi.EmbeddingResponse{
			Embedding: []float64{0.1, 0.2, 0.3, 0.4, 0.5},
		}

		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(mockResponse); err != nil {
			return
		}
	}))
	defer server.Close()

	llm, err := NewOllamaLLM(server.URL, "test-model")
	assert.NoError(t, err)

	result, err := llm.CreateEmbedding(context.Background(), "Test input")

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 5, len(result.Vector))
	assert.Equal(t, float32(0.1), result.Vector[0])
	assert.Equal(t, float32(0.5), result.Vector[4])
	assert.Equal(t, 0, result.TokenCount) // Ollama doesn't provide token count
	assert.Equal(t, 5, result.Metadata["embedding_size"])
	assert.Equal(t, "test-model", result.Metadata["model"])
}

func TestOllamaLLM_CreateEmbedding_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "Internal server error"}`))
	}))
	defer server.Close()

	llm, err := NewOllamaLLM(server.URL, "test-model")
	assert.NoError(t, err)

	result, err := llm.CreateEmbedding(context.Background(), "Test input")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "API request failed with status code 500")
}

func TestOllamaLLM_CreateEmbeddings(t *testing.T) {
	// Create a mock server that handles individual embedding requests
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/embeddings", r.URL.Path)
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var reqBody ollamaApi.EmbeddingRequest
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		assert.NoError(t, err)

		// Check that the request is properly formed
		assert.Equal(t, "test-model", reqBody.Model)
		assert.NotNil(t, reqBody.Options)

		// Send a different embedding based on which request this is
		var mockResponse ollamaApi.EmbeddingResponse
		if callCount == 0 {
			assert.Equal(t, "Input 1", reqBody.Prompt)
			mockResponse = ollamaApi.EmbeddingResponse{
				Embedding: []float64{0.1, 0.2, 0.3},
			}
		} else {
			assert.Equal(t, "Input 2", reqBody.Prompt)
			mockResponse = ollamaApi.EmbeddingResponse{
				Embedding: []float64{0.4, 0.5, 0.6},
			}
		}
		callCount++

		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(mockResponse); err != nil {
			return
		}
	}))
	defer server.Close()

	llm, err := NewOllamaLLM(server.URL, "test-model")
	assert.NoError(t, err)

	inputs := []string{"Input 1", "Input 2"}
	result, err := llm.CreateEmbeddings(context.Background(), inputs, core.WithBatchSize(2))

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 2, len(result.Embeddings))
	assert.Equal(t, float32(0.1), result.Embeddings[0].Vector[0])
	assert.Equal(t, float32(0.6), result.Embeddings[1].Vector[2])
	assert.Equal(t, 3, result.Embeddings[0].Metadata["embedding_size"])
	assert.Equal(t, "test-model", result.Embeddings[0].Metadata["model"])
	assert.Equal(t, 0, result.Embeddings[0].TokenCount) // Ollama doesn't provide token count
	assert.Equal(t, 0, result.Embeddings[0].Metadata["batch_index"])
	assert.Equal(t, 1, result.Embeddings[1].Metadata["batch_index"])
}

func TestOllamaLLM_CreateEmbeddings_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "Internal server error"}`))
	}))
	defer server.Close()

	llm, err := NewOllamaLLM(server.URL, "test-model")
	assert.NoError(t, err)

	inputs := []string{"Input 1", "Input 2"}
	result, err := llm.CreateEmbeddings(context.Background(), inputs)

	assert.NoError(t, err) // The method doesn't return an error but stores it in the result
	assert.NotNil(t, result)
	assert.NotNil(t, result.Error)
	assert.Contains(t, result.Error.Error(), "API request failed with status code 500")
	assert.Equal(t, 0, len(result.Embeddings))
	assert.Equal(t, 0, result.ErrorIndex)
}
