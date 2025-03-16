package llms

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

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
			"HTTP-Referer":  "https://github.com/XiaoConstantine/dspy-go", // Optional: identify your app
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
	Model       string              `json:"model"`
	Messages    []openRouterMessage `json:"messages"`
	Temperature float64             `json:"temperature,omitempty"`
	MaxTokens   int                 `json:"max_tokens,omitempty"`
	TopP        float64             `json:"top_p,omitempty"`
	Stream      bool                `json:"stream,omitempty"`
	Stop        []string            `json:"stop,omitempty"`
	Functions   []map[string]any    `json:"functions,omitempty"`
}

type openRouterMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openRouterResponse represents the response structure from OpenRouter API.
type openRouterResponse struct {
	ID      string           `json:"id"`
	Object  string           `json:"object"`
	Created int64            `json:"created"`
	Model   string           `json:"model"`
	Choices []choice         `json:"choices"`
	Usage   usageInfo        `json:"usage"`
	Meta    *openRouterMeta  `json:"meta,omitempty"`
	Error   *openRouterError `json:"error,omitempty"`
}

type choice struct {
	Index        int               `json:"index"`
	Message      openRouterMessage `json:"message"`
	FinishReason string            `json:"finish_reason"`
}

type usageInfo struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type openRouterMeta struct {
	RateLimit *rateLimitMeta `json:"rate_limit,omitempty"`
}

type rateLimitMeta struct {
	Limit     int   `json:"limit,omitempty"`
	Remaining int   `json:"remaining,omitempty"`
	Reset     int64 `json:"reset,omitempty"`
}

type openRouterError struct {
	Message  string               `json:"message"`
	Code     int                  `json:"code"`
	Metadata *openRouterErrorMeta `json:"metadata,omitempty"`
}

type openRouterErrorMeta struct {
	Headers map[string]string `json:"headers,omitempty"`
}

// Helper function to convert millisecond timestamp to hours until reset
func formatResetTime(resetTimeMs string) string {
	// Parse the reset time from string to int64
	resetMs, err := strconv.ParseInt(resetTimeMs, 10, 64)
	if err != nil {
		return fmt.Sprintf("unknown (parse error: %v)", err)
	}

	// Convert milliseconds to time.Time
	resetTime := time.Unix(resetMs/1000, 0)

	// Calculate duration until reset
	duration := time.Until(resetTime)

	// Convert to hours (with decimal precision)
	hoursUntilReset := duration.Hours()

	if hoursUntilReset < 0 {
		return "already passed"
	}

	// Format date/time of reset in a human-readable format
	resetTimeFormatted := resetTime.Format("2006-01-02 15:04:05 MST")

	// Format with 2 decimal places and include the actual reset time
	return fmt.Sprintf("%.2f hours (resets at %s)", hoursUntilReset, resetTimeFormatted)
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

	// Print response headers for debugging
	log.Printf("OpenRouter response headers for model %s:", o.ModelID())
	for name, values := range resp.Header {
		for _, value := range values {
			log.Printf("  %s: %s", name, value)
		}
	}

	// Read the response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.WithFields(
			errors.Wrap(err, errors.Unknown, "failed to read response body"),
			errors.Fields{"model": o.ModelID()})
	}

	// Create a new reader from the bytes for json.Decoder
	bodyReader := bytes.NewReader(bodyBytes)

	// Log the response body (limited to first 1000 chars to avoid overwhelming logs)
	bodyPreview := string(bodyBytes)
	if len(bodyPreview) > 1000 {
		bodyPreview = bodyPreview[:1000] + "... [truncated]"
	}
	log.Printf("OpenRouter response body for model %s:\n%s", o.ModelID(), bodyPreview)

	if resp.StatusCode != http.StatusOK {
		return nil, errors.WithFields(
			errors.New(errors.InvalidResponse, fmt.Sprintf("OpenRouter API returned non-200 status code: %d, body: %s", resp.StatusCode, string(bodyBytes))),
			errors.Fields{"model": o.ModelID(), "statusCode": resp.StatusCode})
	}

	var openRouterResp openRouterResponse
	if err = json.NewDecoder(bodyReader).Decode(&openRouterResp); err != nil {
		return nil, errors.WithFields(
			errors.Wrap(err, errors.Unknown, "failed to decode response from OpenRouter API"),
			errors.Fields{"model": o.ModelID()})
	}

	// Check for error in response
	if openRouterResp.Error != nil {
		// If we have an error with metadata containing rate limit headers
		if openRouterResp.Error.Metadata != nil && openRouterResp.Error.Metadata.Headers != nil {
			limit := openRouterResp.Error.Metadata.Headers["X-RateLimit-Limit"]
			remaining := openRouterResp.Error.Metadata.Headers["X-RateLimit-Remaining"]
			reset := openRouterResp.Error.Metadata.Headers["X-RateLimit-Reset"]

			if remaining == "0" || openRouterResp.Error.Code == 429 {
				// Calculate hours until reset
				resetTimeFormatted := formatResetTime(reset)

				return nil, errors.WithFields(
					errors.New(errors.RateLimitExceeded, fmt.Sprintf("OpenRouter API rate limited (from error metadata): limit: %s, remaining: %s, reset: %s (%s)",
						limit, remaining, reset, resetTimeFormatted)),
					errors.Fields{
						"model":          o.ModelID(),
						"limit":          limit,
						"remaining":      remaining,
						"reset":          reset,
						"reset_in_hours": resetTimeFormatted,
						"error_message":  openRouterResp.Error.Message,
					})
			}
		}

		// Generic error case
		return nil, errors.WithFields(
			errors.New(errors.Unknown, fmt.Sprintf("OpenRouter API returned error: %s", openRouterResp.Error.Message)),
			errors.Fields{"model": o.ModelID(), "error_code": openRouterResp.Error.Code})
	}

	// Print metadata for debugging
	if openRouterResp.Meta != nil && openRouterResp.Meta.RateLimit != nil {
		log.Printf("OpenRouter metadata rate limits for model %s:", o.ModelID())
		log.Printf("  Limit: %d", openRouterResp.Meta.RateLimit.Limit)
		log.Printf("  Remaining: %d", openRouterResp.Meta.RateLimit.Remaining)
		log.Printf("  Reset: %d", openRouterResp.Meta.RateLimit.Reset)
	} else {
		log.Printf("No rate limit metadata found in response for model %s", o.ModelID())
	}

	if len(openRouterResp.Choices) == 0 {
		// Check if we were rate limited - first from headers
		remaining := resp.Header.Get("X-RateLimit-Remaining")
		limit := resp.Header.Get("X-RateLimit-Limit")
		reset := resp.Header.Get("X-RateLimit-Reset")

		// Then check metadata if available
		if openRouterResp.Meta != nil && openRouterResp.Meta.RateLimit != nil {
			// Metadata values take precedence
			if openRouterResp.Meta.RateLimit.Remaining == 0 {
				// Convert metadata values to strings
				metaLimit := strconv.Itoa(openRouterResp.Meta.RateLimit.Limit)
				metaRemaining := strconv.Itoa(openRouterResp.Meta.RateLimit.Remaining)
				metaReset := strconv.FormatInt(openRouterResp.Meta.RateLimit.Reset, 10)

				// Calculate hours until reset
				resetTimeFormatted := formatResetTime(metaReset)

				return nil, errors.WithFields(
					errors.New(errors.RateLimitExceeded, fmt.Sprintf("OpenRouter API rate limited (from metadata): limit: %s, remaining: %s, reset: %s (%s)",
						metaLimit, metaRemaining, metaReset, resetTimeFormatted)),
					errors.Fields{
						"model":          o.ModelID(),
						"limit":          metaLimit,
						"remaining":      metaRemaining,
						"reset":          metaReset,
						"reset_in_hours": resetTimeFormatted,
					})
			}
		} else if remaining == "0" {
			// Use header values if no metadata or if remaining in headers is 0
			// Calculate hours until reset
			resetTimeFormatted := formatResetTime(reset)

			return nil, errors.WithFields(
				errors.New(errors.RateLimitExceeded, fmt.Sprintf("OpenRouter API rate limited (from headers): limit: %s, remaining: %s, reset: %s (%s)",
					limit, remaining, reset, resetTimeFormatted)),
				errors.Fields{
					"model":          o.ModelID(),
					"limit":          limit,
					"remaining":      remaining,
					"reset":          reset,
					"reset_in_hours": resetTimeFormatted,
				})
		}

		// If we get here, we have empty choices but no rate limiting
		return nil, errors.WithFields(
			errors.New(errors.InvalidResponse, "OpenRouter API returned no choices"),
			errors.Fields{"model": o.ModelID(), "limit": limit, "remaining": remaining, "reset": reset})
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

	// Print response headers for debugging
	log.Printf("OpenRouter response headers for model %s (with functions):", o.ModelID())
	for name, values := range resp.Header {
		for _, value := range values {
			log.Printf("  %s: %s", name, value)
		}
	}

	// Read the response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.WithFields(
			errors.Wrap(err, errors.Unknown, "failed to read response body"),
			errors.Fields{"model": o.ModelID()})
	}

	// Create a new reader from the bytes for json.Decoder
	bodyReader := bytes.NewReader(bodyBytes)

	// Log the response body (limited to first 1000 chars to avoid overwhelming logs)
	bodyPreview := string(bodyBytes)
	if len(bodyPreview) > 1000 {
		bodyPreview = bodyPreview[:1000] + "... [truncated]"
	}
	log.Printf("OpenRouter response body for model %s (with functions):\n%s", o.ModelID(), bodyPreview)

	if resp.StatusCode != http.StatusOK {
		return nil, errors.WithFields(
			errors.New(errors.InvalidResponse, fmt.Sprintf("OpenRouter API returned non-200 status code: %d, body: %s", resp.StatusCode, string(bodyBytes))),
			errors.Fields{"model": o.ModelID(), "statusCode": resp.StatusCode})
	}

	var result map[string]any
	if err = json.NewDecoder(bodyReader).Decode(&result); err != nil {
		return nil, errors.WithFields(
			errors.Wrap(err, errors.Unknown, "failed to decode response from OpenRouter API"),
			errors.Fields{"model": o.ModelID()})
	}

	// Check for error in response
	errorObj, hasError := result["error"].(map[string]any)
	if hasError {
		// Check if we have error metadata with rate limit headers
		metadata, hasMetadata := errorObj["metadata"].(map[string]any)
		if hasMetadata {
			headers, hasHeaders := metadata["headers"].(map[string]any)
			if hasHeaders {
				// Extract rate limit info from headers
				limit, _ := headers["X-RateLimit-Limit"].(string)
				remaining, _ := headers["X-RateLimit-Remaining"].(string)
				reset, _ := headers["X-RateLimit-Reset"].(string)

				// Check if we're rate limited
				if remaining == "0" || (errorObj["code"] != nil && errorObj["code"].(float64) == 429) {
					errorMsg, _ := errorObj["message"].(string)

					// Calculate hours until reset
					resetTimeFormatted := formatResetTime(reset)

					return nil, errors.WithFields(
						errors.New(errors.RateLimitExceeded, fmt.Sprintf("OpenRouter API rate limited (from error metadata): limit: %s, remaining: %s, reset: %s (%s)",
							limit, remaining, reset, resetTimeFormatted)),
						errors.Fields{
							"model":          o.ModelID(),
							"error_message":  errorMsg,
							"reset_in_hours": resetTimeFormatted,
						})
				}
			}
		}

		// Generic error case
		errorMsg, _ := errorObj["message"].(string)
		return nil, errors.WithFields(
			errors.New(errors.Unknown, fmt.Sprintf("OpenRouter API returned error: %s", errorMsg)),
			errors.Fields{"model": o.ModelID()})
	}

	// Print metadata for debugging
	if meta, hasMeta := result["meta"].(map[string]any); hasMeta {
		if rateLimit, hasRateLimit := meta["rate_limit"].(map[string]any); hasRateLimit {
			log.Printf("OpenRouter metadata rate limits for model %s (with functions):", o.ModelID())
			if limit, ok := rateLimit["limit"].(float64); ok {
				log.Printf("  Limit: %f", limit)
			}
			if remaining, ok := rateLimit["remaining"].(float64); ok {
				log.Printf("  Remaining: %f", remaining)
			}
			if reset, ok := rateLimit["reset"].(float64); ok {
				log.Printf("  Reset: %f", reset)
			}
		}
	} else {
		log.Printf("No rate limit metadata found in response for model %s (with functions)", o.ModelID())
	}

	if len(result["choices"].([]any)) == 0 {
		// Check if we were rate limited - first from headers
		remaining := resp.Header.Get("X-RateLimit-Remaining")
		limit := resp.Header.Get("X-RateLimit-Limit")
		reset := resp.Header.Get("X-RateLimit-Reset")

		// Also check metadata if available
		meta, hasMeta := result["meta"].(map[string]any)
		if hasMeta {
			rateLimit, hasRateLimit := meta["rate_limit"].(map[string]any)
			if hasRateLimit {
				metaRemaining, hasRemaining := rateLimit["remaining"].(float64)
				if hasRemaining && metaRemaining == 0 {
					// Convert metadata values to strings
					metaLimit := "unknown"
					if limit, hasLimit := rateLimit["limit"].(float64); hasLimit {
						metaLimit = strconv.FormatFloat(limit, 'f', 0, 64)
					}

					metaRemainingStr := strconv.FormatFloat(metaRemaining, 'f', 0, 64)

					metaReset := "unknown"
					if reset, hasReset := rateLimit["reset"].(float64); hasReset {
						metaReset = strconv.FormatFloat(reset, 'f', 0, 64)
					}

					// Calculate hours until reset
					resetTimeFormatted := formatResetTime(metaReset)

					return nil, errors.WithFields(
						errors.New(errors.RateLimitExceeded, fmt.Sprintf("OpenRouter API rate limited (from metadata): limit: %s, remaining: %s, reset: %s (%s)",
							metaLimit, metaRemainingStr, metaReset, resetTimeFormatted)),
						errors.Fields{
							"model":          o.ModelID(),
							"reset_in_hours": resetTimeFormatted,
						})
				}
			}
		} else if remaining == "0" {
			// Use header values if no metadata or if remaining in headers is 0
			// Calculate hours until reset
			resetTimeFormatted := formatResetTime(reset)

			return nil, errors.WithFields(
				errors.New(errors.RateLimitExceeded, fmt.Sprintf("OpenRouter API rate limited (from headers): limit: %s, remaining: %s, reset: %s (%s)",
					limit, remaining, reset, resetTimeFormatted)),
				errors.Fields{
					"model":          o.ModelID(),
					"reset_in_hours": resetTimeFormatted,
				})
		}

		// If we get here, we have empty choices but no rate limiting
		return nil, errors.WithFields(
			errors.New(errors.InvalidResponse, "OpenRouter API returned no choices"),
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
