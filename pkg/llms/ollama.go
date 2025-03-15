package llms

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
	"github.com/XiaoConstantine/dspy-go/pkg/errors"
	"github.com/XiaoConstantine/dspy-go/pkg/utils"
	ollamaApi "github.com/ollama/ollama/api"
)

// OllamaLLM implements the core.LLM interface for Ollama-hosted models.
type OllamaLLM struct {
	*core.BaseLLM
}

// NewOllamaLLM creates a new OllamaLLM instance.
func NewOllamaLLM(endpoint, model string) (*OllamaLLM, error) {
	if endpoint == "" {
		endpoint = "http://localhost:11434" // Default Ollama endpoint
	}
	capabilities := []core.Capability{
		core.CapabilityCompletion,
		core.CapabilityChat,
		core.CapabilityJSON,
	}
	endpointCfg := &core.EndpointConfig{
		BaseURL: endpoint,
		Path:    "api/generate", // Ollama's generation endpoint
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		TimeoutSec: 10 * 60, // Default timeout
	}

	return &OllamaLLM{
		BaseLLM: core.NewBaseLLM("ollama", core.ModelID(model), capabilities, endpointCfg),
	}, nil
}

// Generate implements the core.LLM interface.
func (o *OllamaLLM) Generate(ctx context.Context, prompt string, options ...core.GenerateOption) (*core.LLMResponse, error) {
	opts := core.NewGenerateOptions()
	for _, opt := range options {
		opt(opts)
	}

	stream := false
	reqBody := ollamaApi.GenerateRequest{
		Model:   string(o.ModelID()),
		Prompt:  prompt,
		Stream:  &stream,
		Options: map[string]any{
			"max_tokens":   opts.MaxTokens,
			"temperature":  opts.Temperature,
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return &core.LLMResponse{}, errors.WithFields(
			errors.Wrap(err, errors.InvalidInput, "failed to marshal request body"),
			errors.Fields{
				"model": o.ModelID(),
			})
	}

	req, err := http.NewRequestWithContext(ctx, "POST", o.GetEndpointConfig().BaseURL+"/api/generate", bytes.NewBuffer(jsonData))
	if err != nil {
		return &core.LLMResponse{}, errors.WithFields(
			errors.Wrap(err, errors.InvalidInput, "failed to create request"),
			errors.Fields{
				"model": o.ModelID(),
			})
	}

	for key, value := range o.GetEndpointConfig().Headers {
		req.Header.Set(key, value)
	}

	resp, err := o.GetHTTPClient().Do(req)
	if err != nil {
		return &core.LLMResponse{}, errors.WithFields(
			errors.Wrap(err, errors.LLMGenerationFailed, "failed to send request"),
			errors.Fields{
				"model": o.ModelID(),
			})
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &core.LLMResponse{}, errors.WithFields(
			errors.Wrap(err, errors.LLMGenerationFailed, "failed to read response body"),
			errors.Fields{
				"model": o.ModelID(),
			})

	}

	if resp.StatusCode != http.StatusOK {
		return &core.LLMResponse{}, errors.WithFields(
			errors.New(errors.LLMGenerationFailed, fmt.Sprintf("API request failed with status code %d", resp.StatusCode)),
			errors.Fields{
				"model":         o.ModelID(),
				"status_code":   resp.StatusCode,
				"response_body": string(body),
			})
	}

	var ollamaResp ollamaApi.GenerateResponse
	err = json.Unmarshal(body, &ollamaResp)
	if err != nil {
		return &core.LLMResponse{}, errors.WithFields(
			errors.Wrap(err, errors.InvalidResponse, "failed to unmarshal response"),
			errors.Fields{
				"resp":  body[:50],
				"model": o.ModelID(),
			})

	}

	// TODO: add token usage
	return &core.LLMResponse{Content: ollamaResp.Response}, nil
}

// GenerateWithJSON implements the core.LLM interface.
func (o *OllamaLLM) GenerateWithJSON(ctx context.Context, prompt string, options ...core.GenerateOption) (map[string]any, error) {
	response, err := o.Generate(ctx, prompt, options...)
	if err != nil {
		return nil, err
	}

	return utils.ParseJSONResponse(response.Content)
}

func (o *OllamaLLM) GenerateWithFunctions(ctx context.Context, prompt string, functions []map[string]any, options ...core.GenerateOption) (map[string]any, error) {
	panic("Not implemented")
}

// CreateEmbedding generates embeddings for a single input.
func (o *OllamaLLM) CreateEmbedding(ctx context.Context, input string, options ...core.EmbeddingOption) (*core.EmbeddingResult, error) {
	// Apply the provided options
	opts := core.NewEmbeddingOptions()
	for _, opt := range options {
		opt(opts)
	}

	// Create the request body
	reqBody := ollamaApi.EmbedRequest{
		Model:   string(o.ModelID()), // Use the model ID from the LLM instance
		Input:   input,
		Options: opts.Params,
	}

	// Marshal the request body
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal embedding request: %w", err)
	}

	// Create the HTTP request
	req, err := http.NewRequestWithContext(
		ctx,
		"POST",
		fmt.Sprintf("%s/api/embeddings", o.GetEndpointConfig().BaseURL),
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	for key, value := range o.GetEndpointConfig().Headers {
		req.Header.Set(key, value)
	}

	// Execute the request
	resp, err := o.GetHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check for non-200 status codes
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status code %d: %s", resp.StatusCode, string(body))
	}

	// Parse the response directly into a map to extract the embedding
	var responseMap map[string]any
	if err := json.Unmarshal(body, &responseMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Extract the embedding vector and size from the response
	embeddingData, ok := responseMap["embedding"].([]any)
	if !ok {
		return nil, fmt.Errorf("response does not contain embedding data")
	}

	// Convert the embedding data to []float32
	embedding := make([]float32, len(embeddingData))
	for i, value := range embeddingData {
		floatValue, ok := value.(float64) // JSON unmarshals numbers as float64
		if !ok {
			return nil, fmt.Errorf("embedding data contains non-numeric values")
		}
		embedding[i] = float32(floatValue)
	}

	// Extract the size if available
	var size int
	if sizeValue, ok := responseMap["size"].(float64); ok {
		size = int(sizeValue)
	} else {
		size = len(embedding) // Use length as fallback
	}

	// Convert to our standard EmbeddingResult format
	result := &core.EmbeddingResult{
		Vector:     embedding,
		TokenCount: 0, // The Ollama API doesn't provide token count in the response
		Metadata: map[string]any{
			"model":          o.ModelID(),
			"embedding_size": size,
		},
	}

	return result, nil
}

// CreateEmbeddings generates embeddings for multiple inputs in batches.
func (o *OllamaLLM) CreateEmbeddings(ctx context.Context, inputs []string, options ...core.EmbeddingOption) (*core.BatchEmbeddingResult, error) {
	// Apply options
	opts := core.NewEmbeddingOptions()
	for _, opt := range options {
		opt(opts)
	}

	// Use default batch size if not specified
	if opts.BatchSize <= 0 {
		opts.BatchSize = 32
	}

	// Process inputs in batches
	var allResults []core.EmbeddingResult
	var firstError error
	var errorIndex int = -1

	// Process each batch
	for i := 0; i < len(inputs); i += opts.BatchSize {
		end := i + opts.BatchSize
		if end > len(inputs) {
			end = len(inputs)
		}

		// Create batch request
		batch := inputs[i:end]
		reqBody := ollamaApi.EmbeddingRequest{
			Model:   string(o.ModelID()),
			Prompt:  batch[0], // EmbeddingRequest only takes a single string prompt
			Options: opts.Params,
		}

		// Marshal request body
		jsonData, err := json.Marshal(reqBody)
		if err != nil {
			if firstError == nil {
				firstError = fmt.Errorf("failed to marshal batch request: %w", err)
				errorIndex = i
			}
			continue
		}

		// Create HTTP request
		req, err := http.NewRequestWithContext(
			ctx,
			"POST",
			fmt.Sprintf("%s/api/embeddings/batch", o.GetEndpointConfig().BaseURL),
			bytes.NewBuffer(jsonData),
		)
		if err != nil {
			if firstError == nil {
				firstError = fmt.Errorf("failed to create batch request: %w", err)
				errorIndex = i
			}
			continue
		}

		// Set headers
		for key, value := range o.GetEndpointConfig().Headers {
			req.Header.Set(key, value)
		}

		// Execute request
		resp, err := o.GetHTTPClient().Do(req)
		if err != nil {
			if firstError == nil {
				firstError = fmt.Errorf("failed to send batch request: %w", err)
				errorIndex = i
			}
			continue
		}

		// Read and parse response
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			if firstError == nil {
				firstError = fmt.Errorf("failed to read batch response: %w", err)
				errorIndex = i
			}
			continue
		}

		if resp.StatusCode != http.StatusOK {
			if firstError == nil {
				firstError = fmt.Errorf("API request failed with status code %d: %s", resp.StatusCode, string(body))
				errorIndex = i
			}
			continue
		}

		// Parse the batch response directly into a map
		var responseMap map[string]any
		if err := json.Unmarshal(body, &responseMap); err != nil {
			if firstError == nil {
				firstError = fmt.Errorf("failed to unmarshal batch response: %w", err)
				errorIndex = i
			}
			continue
		}

		// Extract the embeddings from the response
		embeddingsData, ok := responseMap["embeddings"].([]any)
		if !ok {
			if firstError == nil {
				firstError = fmt.Errorf("response does not contain embeddings data")
				errorIndex = i
			}
			continue
		}

		// Process each embedding in the batch
		for j, embData := range embeddingsData {
			embMap, ok := embData.(map[string]any)
			if !ok {
				continue // Skip this embedding if it's not a map
			}

			// Extract embedding vector
			embVector, ok := embMap["embedding"].([]any)
			if !ok {
				continue // Skip this embedding if it doesn't have a valid vector
			}

			// Convert the embedding data to []float32
			embedding := make([]float32, len(embVector))
			for k, value := range embVector {
				floatValue, ok := value.(float64) // JSON unmarshals numbers as float64
				if !ok {
					continue // Skip this value if it's not a number
				}
				embedding[k] = float32(floatValue)
			}

			// Extract size if available
			var size int
			if sizeValue, ok := embMap["size"].(float64); ok {
				size = int(sizeValue)
			} else {
				size = len(embedding) // Use length as fallback
			}

			// Create the embedding result
			result := core.EmbeddingResult{
				Vector:     embedding,
				TokenCount: 0, // Ollama doesn't provide token counts in embedding responses
				Metadata: map[string]any{
					"model":          o.ModelID(),
					"embedding_size": size,
					"batch_index":    i + j,
				},
			}
			allResults = append(allResults, result)
		}
	}

	// Return the combined results
	return &core.BatchEmbeddingResult{
		Embeddings: allResults,
		Error:      firstError,
		ErrorIndex: errorIndex,
	}, nil
}
