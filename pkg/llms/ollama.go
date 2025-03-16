package llms

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/ollama/ollama/api"
	"github.com/scottdavis/dsgo/pkg/core"
	"github.com/scottdavis/dsgo/pkg/errors"
	"github.com/scottdavis/dsgo/pkg/utils"
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

	// Create a pointer to bool for Stream
	streamValue := false

	reqBody := api.GenerateRequest{
		Model:  string(o.ModelID()),
		Prompt: prompt,
		Stream: &streamValue,
		Options: map[string]any{
			"max_tokens":  opts.MaxTokens,
			"temperature": opts.Temperature,
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

	var ollamaResp api.GenerateResponse
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

// StreamGenerate for Ollama.
func (o *OllamaLLM) StreamGenerate(ctx context.Context, prompt string, options ...core.GenerateOption) (*core.StreamResponse, error) {
	opts := core.NewGenerateOptions()
	for _, opt := range options {
		opt(opts)
	}

	// Create a pointer to bool for Stream
	streamValue := true

	reqBody := api.GenerateRequest{
		Model:  string(o.ModelID()),
		Prompt: prompt,
		Stream: &streamValue,
		Options: map[string]any{
			"max_tokens":  opts.MaxTokens,
			"temperature": opts.Temperature,
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, errors.Wrap(err, errors.InvalidInput, "failed to marshal request")
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST",
		o.GetEndpointConfig().BaseURL+"/api/generate", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, errors.Wrap(err, errors.InvalidInput, "failed to create request")
	}

	for key, value := range o.GetEndpointConfig().Headers {
		req.Header.Set(key, value)
	}

	// Create channel and response
	chunkChan := make(chan core.StreamChunk)
	streamCtx, cancelStream := context.WithCancel(ctx)

	response := &core.StreamResponse{
		ChunkChannel: chunkChan,
		Cancel:       cancelStream,
	}

	go func() {
		defer close(chunkChan)

		resp, err := o.GetHTTPClient().Do(req)
		if err != nil {
			chunkChan <- core.StreamChunk{Error: err}
			return
		}
		defer resp.Body.Close()

		// Ollama returns JSONL stream
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			select {
			case <-streamCtx.Done():
				return
			default:
				line := scanner.Text()
				if line == "" {
					continue
				}

				// Use the official Ollama types
				var response api.GenerateResponse
				if err := json.Unmarshal([]byte(line), &response); err != nil {
					continue
				}

				// Send the chunk
				chunkChan <- core.StreamChunk{
					Content: response.Response,
					Done:    response.Done,
				}

				// If done flag is set, we've received the final chunk
				if response.Done {
					return
				}
			}
		}
	}()

	return response, nil
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
	// Ollama doesn't support functions yet
	return nil, fmt.Errorf("GenerateWithFunctions not implemented for Ollama")
}

// CreateEmbedding generates embeddings for a single input.
func (o *OllamaLLM) CreateEmbedding(ctx context.Context, input string, options ...core.EmbeddingOption) (*core.EmbeddingResult, error) {
	// Apply the provided options
	opts := core.NewEmbeddingOptions()
	for _, opt := range options {
		opt(opts)
	}

	// Create the request body
	reqBody := api.EmbeddingRequest{
		Model:   string(o.ModelID()),
		Prompt:  input,
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

	// Parse the response
	var ollamaResp api.EmbeddingResponse
	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Convert to our standard EmbeddingResult format
	// Note: Ollama returns []float64 but our interface expects []float32
	vector := make([]float32, len(ollamaResp.Embedding))
	for i, v := range ollamaResp.Embedding {
		vector[i] = float32(v)
	}

	// Create a result with the embedding vector
	result := &core.EmbeddingResult{
		Vector:     vector,
		TokenCount: 0, // Ollama doesn't provide token count
		Metadata: map[string]any{
			"model":          o.ModelID(),
			"embedding_size": len(ollamaResp.Embedding),
		},
	}

	return result, nil
}

// CreateEmbeddings generates embeddings for multiple inputs in batches.
func (o *OllamaLLM) CreateEmbeddings(ctx context.Context, inputs []string, options ...core.EmbeddingOption) (*core.BatchEmbeddingResult, error) {
	// Ollama doesn't support batch embeddings natively, so we'll implement it as multiple requests
	opts := core.NewEmbeddingOptions()
	for _, opt := range options {
		opt(opts)
	}

	// Process inputs sequentially since Ollama doesn't have a batch endpoint
	allResults := make([]core.EmbeddingResult, 0, len(inputs))
	var firstError error
	var errorIndex int = -1

	for i, input := range inputs {
		result, err := o.CreateEmbedding(ctx, input, options...)
		if err != nil {
			if firstError == nil {
				firstError = fmt.Errorf("failed to create embedding for input %d: %w", i, err)
				errorIndex = i
			}
			continue
		}

		if result != nil {
			// Add batch index to metadata
			if result.Metadata == nil {
				result.Metadata = map[string]any{}
			}
			result.Metadata["batch_index"] = i
			allResults = append(allResults, *result)
		}
	}

	// Return the combined results
	return &core.BatchEmbeddingResult{
		Embeddings: allResults,
		Error:      firstError,
		ErrorIndex: errorIndex,
	}, nil
}
