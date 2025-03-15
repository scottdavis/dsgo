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
)

// OpenRouterConfig represents configuration for an OpenRouter client
type OpenRouterConfig struct {
	// ModelName is the name of the OpenRouter model (e.g., "anthropic/claude-3-opus-20240229")
	ModelName string
}

// NewOpenRouterConfig creates a new OpenRouterConfig with the specified model name
func NewOpenRouterConfig(modelName string) *OpenRouterConfig {
	return &OpenRouterConfig{
		ModelName: modelName,
	}
}

// OpenRouterLLM implements the core.LLM interface for OpenRouter-hosted models.
type OpenRouterLLM struct {
	*core.BaseLLM
	apiKey string
}

// The base URL for the OpenRouter API
const openRouterBaseURL = "https://openrouter.ai/api/v1"

// NewOpenRouterLLM creates a new OpenRouterLLM instance.
func NewOpenRouterLLM(apiKey string, modelName string) (*OpenRouterLLM, error) {
	if apiKey == "" {
		return nil, errors.New(errors.InvalidInput, "OpenRouter API key is required")
	}

	if modelName == "" {
		return nil, errors.New(errors.InvalidInput, "OpenRouter model name is required")
	}

	capabilities := []core.Capability{
		core.CapabilityCompletion,
		core.CapabilityChat,
		core.CapabilityJSON,
	}
	
	endpointCfg := &core.EndpointConfig{
		BaseURL: openRouterBaseURL,
		Path:    "/chat/completions",
		Headers: map[string]string{
			"Content-Type":  "application/json",
			"Authorization": "Bearer " + apiKey,
			"HTTP-Referer": "https://github.com/XiaoConstantine/dspy-go", // Optional: identify your app
		},
		TimeoutSec: 10 * 60, // Default timeout
	}

	return &OpenRouterLLM{
		apiKey:  apiKey,
		BaseLLM: core.NewBaseLLM("openrouter", core.ModelID(modelName), capabilities, endpointCfg),
	}, nil
}

// openRouterRequest represents the request structure for OpenRouter API.
type openRouterRequest struct {
	Model       string                 `json:"model"`
	Messages    []openRouterMessage    `json:"messages"`
	Temperature float64                `json:"temperature,omitempty"`
	MaxTokens   int                    `json:"max_tokens,omitempty"`
	TopP        float64                `json:"top_p,omitempty"`
	Stream      bool                   `json:"stream,omitempty"`
	Stop        []string               `json:"stop,omitempty"`
	Functions   []map[string]any       `json:"functions,omitempty"`
}

type openRouterMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openRouterResponse represents the response structure from OpenRouter API.
type openRouterResponse struct {
	ID      string    `json:"id"`
	Object  string    `json:"object"`
	Created int64     `json:"created"`
	Model   string    `json:"model"`
	Choices []choice  `json:"choices"`
	Usage   usageInfo `json:"usage"`
}

type choice struct {
	Index        int             `json:"index"`
	Message      openRouterMessage `json:"message"`
	FinishReason string          `json:"finish_reason"`
}

type usageInfo struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Generate implements the core.LLM interface.
func (o *OpenRouterLLM) Generate(ctx context.Context, prompt string, options ...core.GenerateOption) (*core.LLMResponse, error) {
	opts := core.NewGenerateOptions()
	for _, opt := range options {
		opt(opts)
	}

	messages := []openRouterMessage{
		{
			Role:    "user",
			Content: prompt,
		},
	}

	reqBody := openRouterRequest{
		Model:       o.ModelID(),
		Messages:    messages,
		Temperature: opts.Temperature,
		MaxTokens:   opts.MaxTokens,
		Stop:        opts.Stop,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, errors.WithFields(
			errors.Wrap(err, errors.Unknown, "failed to marshal request body"),
			errors.Fields{"model": o.ModelID()})
	}

	endpoint := o.GetEndpointConfig()
	url := fmt.Sprintf("%s%s", endpoint.BaseURL, endpoint.Path)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, errors.WithFields(
			errors.Wrap(err, errors.Unknown, "failed to create request"),
			errors.Fields{"model": o.ModelID()})
	}

	for key, value := range endpoint.Headers {
		req.Header.Set(key, value)
	}

	client := o.GetHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.WithFields(
			errors.Wrap(err, errors.Unknown, "failed to send request to OpenRouter API"),
			errors.Fields{"model": o.ModelID()})
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, errors.WithFields(
			errors.New(errors.InvalidResponse, fmt.Sprintf("OpenRouter API returned non-200 status code: %d, body: %s", resp.StatusCode, string(bodyBytes))),
			errors.Fields{"model": o.ModelID(), "statusCode": resp.StatusCode})
	}

	var openRouterResp openRouterResponse
	if err = json.NewDecoder(resp.Body).Decode(&openRouterResp); err != nil {
		return nil, errors.WithFields(
			errors.Wrap(err, errors.Unknown, "failed to decode response from OpenRouter API"),
			errors.Fields{"model": o.ModelID()})
	}

	if len(openRouterResp.Choices) == 0 {
		return nil, errors.WithFields(
			errors.New(errors.InvalidResponse, "OpenRouter API returned empty choices"),
			errors.Fields{"model": o.ModelID()})
	}

	// Extract the completion from the first choice
	content := openRouterResp.Choices[0].Message.Content
	tokenInfo := &core.TokenInfo{
		PromptTokens:     openRouterResp.Usage.PromptTokens,
		CompletionTokens: openRouterResp.Usage.CompletionTokens,
		TotalTokens:      openRouterResp.Usage.TotalTokens,
	}

	return &core.LLMResponse{
		Content: content,
		Usage:   tokenInfo,
	}, nil
}

// GenerateWithJSON implements the core.LLM interface.
func (o *OpenRouterLLM) GenerateWithJSON(ctx context.Context, prompt string, options ...core.GenerateOption) (map[string]any, error) {
	response, err := o.Generate(ctx, prompt, options...)
	if err != nil {
		return nil, err
	}

	return utils.ParseJSONResponse(response.Content)
}

// GenerateWithFunctions implements the core.LLM interface.
func (o *OpenRouterLLM) GenerateWithFunctions(ctx context.Context, prompt string, functions []map[string]any, options ...core.GenerateOption) (map[string]any, error) {
	opts := core.NewGenerateOptions()
	for _, opt := range options {
		opt(opts)
	}
	
	messages := []openRouterMessage{
		{
			Role:    "user",
			Content: prompt,
		},
	}

	reqBody := openRouterRequest{
		Model:       o.ModelID(),
		Messages:    messages,
		Temperature: opts.Temperature,
		MaxTokens:   opts.MaxTokens,
		Stop:        opts.Stop,
		Functions:   functions,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, errors.WithFields(
			errors.Wrap(err, errors.Unknown, "failed to marshal request body"),
			errors.Fields{"model": o.ModelID()})
	}

	endpoint := o.GetEndpointConfig()
	url := fmt.Sprintf("%s%s", endpoint.BaseURL, endpoint.Path)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, errors.WithFields(
			errors.Wrap(err, errors.Unknown, "failed to create request"),
			errors.Fields{"model": o.ModelID()})
	}

	for key, value := range endpoint.Headers {
		req.Header.Set(key, value)
	}

	client := o.GetHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.WithFields(
			errors.Wrap(err, errors.Unknown, "failed to send request to OpenRouter API"),
			errors.Fields{"model": o.ModelID()})
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, errors.WithFields(
			errors.New(errors.InvalidResponse, fmt.Sprintf("OpenRouter API returned non-200 status code: %d, body: %s", resp.StatusCode, string(bodyBytes))),
			errors.Fields{"model": o.ModelID(), "statusCode": resp.StatusCode})
	}

	var result map[string]any
	if err = json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, errors.WithFields(
			errors.Wrap(err, errors.Unknown, "failed to decode response from OpenRouter API"),
			errors.Fields{"model": o.ModelID()})
	}

	return result, nil
}

// CreateEmbedding implements the core.LLM interface.
func (o *OpenRouterLLM) CreateEmbedding(ctx context.Context, input string, options ...core.EmbeddingOption) (*core.EmbeddingResult, error) {
	// OpenRouter does support embeddings, but might require implementing with their API
	return nil, errors.New(errors.Unknown, "CreateEmbedding not implemented for OpenRouter")
}

// CreateEmbeddings implements the core.LLM interface.
func (o *OpenRouterLLM) CreateEmbeddings(ctx context.Context, inputs []string, options ...core.EmbeddingOption) (*core.BatchEmbeddingResult, error) {
	// OpenRouter does support embeddings, but might require implementing with their API
	return nil, errors.New(errors.Unknown, "CreateEmbeddings not implemented for OpenRouter")
} 